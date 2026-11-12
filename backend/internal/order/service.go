package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/picking"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrWarehouseNotFound = errors.New("order: warehouse not found")
	ErrProductNotFound   = errors.New("order: product not found")
	ErrNotEditable       = errors.New("order: order can no longer be edited")
	ErrNotCancellable    = errors.New("order: order is not in a cancellable status")
	ErrNotReleasable     = errors.New("order: order is not in a status that can create a picking task")
)

// productChecker and warehouseChecker are the narrow slices of the
// Phase 3/4 repositories this package needs — the same narrow-interface
// pattern every prior phase established.
type productChecker interface {
	FindByID(ctx context.Context, id int64) (*product.Product, error)
}

type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

// pickingOrchestrator is the narrow slice of internal/picking's own
// Service this package needs for CreatePickingTask — CreateTx (Phase 25's
// addition to internal/picking, see its own doc) so the order's claim
// and the picking task's creation commit or roll back together as one
// atomic unit, and Get so Service.Get can resolve a linked task's live
// status for FulfillmentStatus without internal/order ever storing or
// syncing that state itself.
type pickingOrchestrator interface {
	CreateTx(ctx context.Context, tx *gorm.DB, req picking.CreateRequest, userID int64) (int64, error)
	Get(ctx context.Context, id int64) (*picking.Response, error)
}

// shippingReader is the narrow slice of internal/shipping's own Service
// this package needs — FindByPickingTaskID, added specifically for this
// phase (see that method's own doc) so Service.Get can resolve whether
// (and how) a linked picking task has since been shipped.
type shippingReader interface {
	FindByPickingTaskID(ctx context.Context, pickingTaskID int64) (*shipping.Response, error)
}

// Service holds Order Management's business rules — a lightweight
// orchestration layer that never mutates inventory or writes an
// inventory_transactions row itself. CreatePickingTask is the one point
// this package ever calls into another workflow package's Service, and
// only ever picking.Service.CreateTx — never AddStock/RemoveStock/
// MoveStock, and never a second reservation mechanism.
type Service struct {
	repo       *Repository
	products   productChecker
	warehouses warehouseChecker
	picking    pickingOrchestrator
	shipping   shippingReader
}

func NewService(repo *Repository, products productChecker, warehouses warehouseChecker, pickingSvc pickingOrchestrator, shippingSvc shippingReader) *Service {
	return &Service{repo: repo, products: products, warehouses: warehouses, picking: pickingSvc, shipping: shippingSvc}
}

func (s *Service) List(ctx context.Context, params ListParams) ([]ListResponse, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	params.Page, params.Limit = page, limit

	rows, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		fulfillment := deriveFulfillmentStatus(Status(row.Status), pickingStatusPtr(row.PickingStatus), shipmentStatusPtr(row.ShipmentStatus))
		responses = append(responses, ListResponse{
			ID: row.ID, OrderNo: row.OrderNo, Customer: row.Customer, ExternalReference: row.ExternalReference,
			WarehouseCode: row.WarehouseCode, ItemCount: row.ItemCount,
			Status: Status(row.Status), FulfillmentStatus: fulfillment, CreatedAt: row.CreatedAt,
		})
	}
	return responses, total, nil
}

// Get resolves an order's full detail, including a live-derived
// FulfillmentStatus — via picking.Service.Get and shipping.Service.
// FindByPickingTaskID (only when a picking task is actually linked; both
// are cheap single-row lookups, unlike List's own bulk SQL join — see
// Repository.List's doc for why the two paths differ).
func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	row, err := s.repo.FindDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindItemsDetail(ctx, id)
	if err != nil {
		return nil, err
	}

	var pickingStatus *picking.Status
	var shipmentStatus *shipping.Status
	if row.PickingTaskID != nil {
		pickingResp, err := s.picking.Get(ctx, *row.PickingTaskID)
		if err != nil {
			return nil, err
		}
		pickingStatus = &pickingResp.Status

		shipResp, err := s.shipping.FindByPickingTaskID(ctx, *row.PickingTaskID)
		if err != nil {
			return nil, err
		}
		if shipResp != nil {
			shipmentStatus = &shipResp.Status
		}
	}

	return &Response{
		ID: row.ID, OrderNo: row.OrderNo, Customer: row.Customer, ExternalReference: row.ExternalReference,
		WarehouseID: row.WarehouseID, WarehouseCode: row.WarehouseCode,
		PickingTaskID: row.PickingTaskID, PickNo: row.PickNo,
		Status:            Status(row.Status),
		FulfillmentStatus: deriveFulfillmentStatus(Status(row.Status), pickingStatus, shipmentStatus),
		Remark:            row.Remark,
		CreatedBy:         row.CreatedBy, CreatedByName: row.CreatedByName,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		CancelledAt: row.CancelledAt, CancelledBy: row.CancelledBy, CancelledByName: row.CancelledByName,
		CancelRemark: row.CancelRemark,
		Items:        items,
	}, nil
}

// pickingStatusPtr/shipmentStatusPtr adapt List's own raw joined
// *string columns to the typed pointers deriveFulfillmentStatus expects
// — nil stays nil (no linked task/shipment), matching Get's own
// pickingStatus/shipmentStatus construction exactly.
func pickingStatusPtr(s *string) *picking.Status {
	if s == nil {
		return nil
	}
	v := picking.Status(*s)
	return &v
}

func shipmentStatusPtr(s *string) *shipping.Status {
	if s == nil {
		return nil
	}
	v := shipping.Status(*s)
	return &v
}

// Create validates and persists a header + its lines. Never touches
// inventory: an order is a plain (product, ordered_qty) intent until
// CreatePickingTask orchestrates a real picking.Service task against
// chosen locations/lots.
func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var orderID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		o := &Order{
			Customer: req.Customer, ExternalReference: req.ExternalReference,
			WarehouseID: req.WarehouseID, Status: StatusPending, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateOrder(ctx, o); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateExternalReference
			}
			return err
		}
		orderID = o.ID

		for _, itemReq := range req.Items {
			if err := s.createItem(ctx, txRepo, o.ID, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "order.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, orderID)
}

func (s *Service) createItem(ctx context.Context, txRepo *Repository, orderID int64, req CreateItemRequest) error {
	if req.OrderedQty <= 0 {
		return ErrValidation{Message: "ordered_qty must be greater than zero"}
	}
	p, err := s.products.FindByID(ctx, req.ProductID)
	if err != nil {
		return ErrProductNotFound
	}
	uom := req.UOM
	if uom == "" {
		uom = string(p.BaseUOM)
	}
	return txRepo.CreateItem(ctx, &Item{OrderID: orderID, ProductID: req.ProductID, OrderedQty: req.OrderedQty, UOM: uom})
}

// Update replaces the header fields and every line at once — full-
// replace, not a per-line diff, safe only because Status.CanUpdate
// (PENDING) means CreatePickingTask has never reserved anything against
// these lines yet.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*Response, error) {
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		o, err := txRepo.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !o.Status.CanUpdate() {
			return ErrNotEditable
		}

		o.Customer = req.Customer
		o.ExternalReference = req.ExternalReference
		o.WarehouseID = req.WarehouseID
		o.Remark = req.Remark
		if err := txRepo.UpdateHeader(ctx, o); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateExternalReference
			}
			return err
		}

		if err := txRepo.DeleteItemsByOrder(ctx, id); err != nil {
			return err
		}
		for _, itemReq := range req.Items {
			if err := s.createItem(ctx, txRepo, id, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "order.update", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// CreatePickingTask is the orchestration action: validate the client's
// per-item location/lot assignments against this order's own lines,
// atomically claim the order (Repository.ClaimForPickingTask), then call
// picking.Service.CreateTx — inside the SAME transaction, so a claim
// with no resulting task (picking creation failing, e.g. insufficient
// stock) rolls back automatically and leaves the order exactly as it was
// before this call, never half-linked. Order itself never calls
// ReserveStockTx/AddStockTx/RemoveStockTx/MoveStockTx — every inventory
// effect here is picking.Service's own Phase 12 reservation, run for
// this order's items exactly as if they had been typed directly into
// PickingView.vue's own "+ New Picking Task" form.
func (s *Service) CreatePickingTask(ctx context.Context, id int64, req CreatePickingTaskRequest, userID int64) (*Response, error) {
	order, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !order.Status.CanCreatePickingTask() {
		return nil, ErrNotReleasable
	}

	items, err := s.repo.FindItemsByOrder(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrValidation{Message: "order has no items"}
	}

	pickingItems, err := buildPickingItems(items, req.Items)
	if err != nil {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		rows, err := txRepo.ClaimForPickingTask(ctx, id)
		if err != nil {
			return err
		}
		if rows == 0 {
			// Covers "already PICKING" (a prior call already succeeded, or
			// is racing this one), "already CANCELLED", and a genuine
			// concurrent double-click — model.go's own CanCreatePickingTask
			// doc calls this out explicitly: this status check exists so a
			// second call gets a clean 409 instead of the raw
			// UQ_orders_picking_task_id violation it would otherwise hit.
			return ErrNotReleasable
		}

		pickingTaskID, err := s.picking.CreateTx(ctx, tx, picking.CreateRequest{
			OrderNo: order.OrderNo, Customer: order.Customer, WarehouseID: order.WarehouseID,
			Priority: picking.Priority(req.Priority), DueDate: req.DueDate, Remark: order.Remark, Items: pickingItems,
		}, userID)
		if err != nil {
			// Rolling back this whole transaction also rolls back the claim
			// above — the order is left exactly PENDING/unlinked again, not
			// stuck in a half-released state.
			return err
		}
		return txRepo.LinkPickingTask(ctx, id, pickingTaskID)
	})
	dbctx.LogTransaction(ctx, "order.create_picking_task", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// buildPickingItems is CreatePickingTask's pure validation core,
// extracted so the assignment-matching rules can be unit tested without
// a database — the same "extract the pure part" precedent every prior
// phase has already established (see dto.go's own deriveFulfillmentStatus
// doc). Requires exactly one LocationAssignment per order item: no
// duplicates targeting the same order_item_id, no assignment left over
// for an order_item_id that isn't one of this order's own items, and
// none of this order's items left unassigned.
func buildPickingItems(items []Item, assignmentReqs []LocationAssignment) ([]picking.CreateItemRequest, error) {
	assignments := make(map[int64]LocationAssignment, len(assignmentReqs))
	for _, a := range assignmentReqs {
		if _, dup := assignments[a.OrderItemID]; dup {
			return nil, ErrValidation{Message: fmt.Sprintf("duplicate location assignment for order item %d", a.OrderItemID)}
		}
		assignments[a.OrderItemID] = a
	}
	if len(assignments) != len(items) {
		return nil, ErrValidation{Message: "every order item must have exactly one location assignment, and no more"}
	}

	pickingItems := make([]picking.CreateItemRequest, 0, len(items))
	for _, it := range items {
		a, ok := assignments[it.ID]
		if !ok {
			return nil, ErrValidation{Message: fmt.Sprintf("missing location assignment for order item %d", it.ID)}
		}
		pickingItems = append(pickingItems, picking.CreateItemRequest{
			ProductID: it.ProductID, LotNo: a.LotNo, LocationID: a.LocationID, OrderedQty: it.OrderedQty, UOM: it.UOM,
		})
	}
	return pickingItems, nil
}

// Cancel is only valid while PENDING (Status.CanCancel) — once a picking
// task has been created for this order, CreatePickingTask's own
// UQ_orders_picking_task_id link means the correct cancellation path is
// internal/picking's own Cancel (which already releases its own
// reservation), not a second, competing cancel path here. Order's Cancel
// never calls ReleaseReservationTx or touches inventory at all: nothing
// was ever reserved by Order itself.
func (s *Service) Cancel(ctx context.Context, id int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		o, err := txRepo.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !o.Status.CanCancel() {
			return ErrNotCancellable
		}
		o.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelOrder(ctx, o)
	})
	dbctx.LogTransaction(ctx, "order.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

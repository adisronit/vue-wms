package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/customer"
	"wms-backend/internal/customerdeliverylocation"
	"wms-backend/internal/inventory"
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
	// ErrNotPending (Phase 33A) is Service.Release's own status guard —
	// Release only ever runs from PENDING (Status.CanRelease), the one
	// and only transition into RELEASED. Also covers a genuine concurrent
	// double-click (Repository.ClaimForRelease's own atomic guarded
	// UPDATE affecting 0 rows), the same "status check, then an atomic
	// claim as the real race-free guard" shape CreatePickingTask's own
	// ErrNotReleasable already establishes.
	ErrNotPending = errors.New("order: order is not in a status that can be released")
	// ErrShortageApprovalRequired (Phase 33A) is returned by Service.
	// Release when its own availability check finds at least one line
	// short and the caller did not set ReleaseRequest.ApproveShortage —
	// the "Controlled shortage approval" gate. The caller is expected to
	// have already called GET /orders/{id}/availability (the same
	// computation, exposed read-only) to learn which lines are short
	// before deciding whether to retry with the override.
	ErrShortageApprovalRequired = errors.New("order: availability shortage found; resubmit with approve_shortage to proceed")
	// ErrCustomerNotFound/ErrShipToNotFound cover both "doesn't exist at
	// all" and "belongs to a different tenant/customer" with the same
	// 404 — never distinguishing the two, the same convention
	// internal/customerdeliverylocation's own ErrCustomerNotFound doc
	// establishes (tenant-isolation-security.md §6).
	ErrCustomerNotFound = errors.New("order: customer not found")
	ErrShipToNotFound   = errors.New("order: ship-to location not found")
)

// productChecker is the narrow slice of internal/product's own
// Repository this package needs. Phase 30 Batch 3 upgrades this from the
// old unscoped FindByID to FindByIDForTenant (added in Batch 1, already
// used by internal/picking's own Batch 2 fix) so an order line's
// product_id is verified to belong to the caller's own tenant, not
// merely to exist.
type productChecker interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*product.Product, error)
}

// warehouseChecker is still the old, unscoped Phase 3 lookup — the
// Warehouse module itself has not been tenant-scoped in any Phase 30
// batch yet (no tenant-scoped warehouse repository method exists
// anywhere in the codebase), so warehouse ownership remains unverified
// here, exactly like internal/picking's own warehouseChecker (Batch 2).
// Reported as a remaining gap, not silently treated as secure — see this
// package's final Phase 30 Batch 3 report.
type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

// customerChecker is the narrow slice of internal/customer's own
// Repository this package needs for the Customer Ship-to integration —
// FindByID is already tenant-scoped (returns the same ErrNotFound for
// "doesn't exist" and "belongs to another tenant", per that package's
// own doc), so a customer_id belonging to another tenant is rejected
// here exactly like a nonexistent one.
type customerChecker interface {
	FindByID(ctx context.Context, tenantID, id int64) (*customer.Customer, error)
}

// shipToChecker is the narrow slice of internal/customerdeliverylocation's
// own Repository this package needs. FindByID is tenant-scoped the same
// way customerChecker's is; FindDefaultForCustomer backs the "auto-select
// the customer's own active default ship-to when none is explicitly
// chosen" behavior Service.applyCustomerShipTo implements.
type shipToChecker interface {
	FindByID(ctx context.Context, tenantID, id int64) (*customerdeliverylocation.Location, error)
	FindDefaultForCustomer(ctx context.Context, tenantID, customerID int64) (*customerdeliverylocation.Location, error)
}

// pickingOrchestrator is the narrow slice of internal/picking's own
// Service this package needs for CreatePickingTask — CreateTx (Phase 25's
// addition to internal/picking, see its own doc) so the order's claim
// and the picking task's creation commit or roll back together as one
// atomic unit, and Get so Service.Get can resolve a linked task's live
// status for FulfillmentStatus without internal/order ever storing or
// syncing that state itself.
//
// Phase 30 Batch 3: Order Management is now itself tenant-scoped, so
// both calls below pass the caller's own verified tenantID (extracted by
// the handler from the authenticated request's tenant context) instead
// of Batch 2's inventory.UnverifiedCallerTenantID placeholder — closing
// the Order->Picking boundary that Batch 2's own report flagged as the
// deferred half of this gap.
type pickingOrchestrator interface {
	CreateTx(ctx context.Context, tenantID int64, tx *gorm.DB, req picking.CreateRequest, userID int64) (int64, error)
	Get(ctx context.Context, tenantID, id int64) (*picking.Response, error)
}

// shippingReader is the narrow slice of internal/shipping's own Service
// this package needs — FindByPickingTaskID, added specifically for this
// phase (see that method's own doc) so Service.Get can resolve whether
// (and how) a linked picking task has since been shipped.
//
// Phase 30 Batch 4: internal/shipping is now itself tenant-scoped, so
// this call passes the caller's own verified tenantID instead of being
// omitted entirely — closing the Order->Shipping boundary this
// package's own Batch 3 report flagged as deferred until Shipping
// itself was scoped.
type shippingReader interface {
	FindByPickingTaskID(ctx context.Context, tenantID, pickingTaskID int64) (*shipping.Response, error)
}

// availabilityChecker is the narrow, read-only slice of internal/
// inventory's own Service this package needs for the Review & Release
// step (Phase 33A) — a single SELECT-backed query, never a reservation
// or any other mutation. See inventory.Service.
// AvailabilityByProductWarehouse's own doc.
type availabilityChecker interface {
	AvailabilityByProductWarehouse(ctx context.Context, tenantID, productID, warehouseID int64) (inventory.ProductAvailability, error)
}

// Service holds Order Management's business rules — a lightweight
// orchestration layer that never mutates inventory or writes an
// inventory_transactions row itself. CreatePickingTask is the one point
// this package ever calls into another workflow package's Service, and
// only ever picking.Service.CreateTx — never AddStock/RemoveStock/
// MoveStock, and never a second reservation mechanism. Phase 33A's own
// Release/CheckAvailability add a read-only dependency on inventory
// (availabilityChecker) but introduce no new mutation path.
type Service struct {
	repo       *Repository
	products   productChecker
	warehouses warehouseChecker
	customers  customerChecker
	shipTos    shipToChecker
	picking    pickingOrchestrator
	shipping   shippingReader
	inventory  availabilityChecker
}

func NewService(repo *Repository, products productChecker, warehouses warehouseChecker, customers customerChecker, shipTos shipToChecker, pickingSvc pickingOrchestrator, shippingSvc shippingReader, inventorySvc availabilityChecker) *Service {
	return &Service{repo: repo, products: products, warehouses: warehouses, customers: customers, shipTos: shipTos, picking: pickingSvc, shipping: shippingSvc, inventory: inventorySvc}
}

// applyCustomerShipTo validates and snapshots a Create/Update request's
// customer_id/ship_to_id onto o, in place. customerID == nil means no
// customer link at all — o.Customer (whatever free text the caller set
// on the struct literal beforehand) is left exactly as-is, and every
// ship_to_* field is cleared (full-replace semantics, matching every
// other header field Update already follows). shipToID set without
// customerID is rejected outright: a ship-to is always resolved in the
// context of a specific customer, never from the ship-to id alone.
//
// Ownership/state checks, in order: customerID must resolve via
// customerChecker (tenant-scoped; cross-tenant is indistinguishable from
// nonexistent — ErrCustomerNotFound either way). If shipToID is given,
// it must resolve via shipToChecker (same tenant-scoping) AND its own
// CustomerID must equal customerID — a cross-customer shipToID is
// rejected as ErrShipToNotFound, the same not-found-equivalent treatment
// as cross-tenant, never confirming that the id exists under a
// different customer. Only once ownership is proven does INACTIVE get
// its own distinct, more specific rejection (ErrValidation) — revealing
// "this ship-to is inactive" is not an ownership leak once the caller
// already holds proof the row belongs to their own customer.
//
// If shipToID is omitted, the customer's own ACTIVE default (if any) is
// auto-selected; if no default exists, the order is simply left with a
// customer link and no ship-to snapshot — not an error, since "auto-
// select active default; allow override" never requires one to exist.
func (s *Service) applyCustomerShipTo(ctx context.Context, tenantID int64, o *Order, customerID, shipToID *int64) error {
	o.CustomerID = nil
	o.ShipToID = nil
	o.ShipToName = nil
	o.ShipToAddress = nil
	o.ShipToProvince = nil
	o.ShipToDistrict = nil
	o.ShipToPostalCode = nil
	o.ShipToContactPerson = nil
	o.ShipToContactPhone = nil

	if customerID == nil {
		if shipToID != nil {
			return ErrValidation{Message: "ship_to_id requires customer_id"}
		}
		return nil
	}

	cust, err := s.customers.FindByID(ctx, tenantID, *customerID)
	if err != nil {
		return ErrCustomerNotFound
	}
	o.CustomerID = customerID
	// The resolved customer's own display name is authoritative once a
	// real customer is linked — overwrites whatever free-text Customer
	// the caller may also have supplied in the same request, so the two
	// can never silently disagree.
	name := cust.CompanyName
	o.Customer = &name

	var shipTo *customerdeliverylocation.Location
	if shipToID != nil {
		shipTo, err = s.shipTos.FindByID(ctx, tenantID, *shipToID)
		if err != nil {
			return ErrShipToNotFound
		}
		if shipTo.CustomerID != *customerID {
			return ErrShipToNotFound
		}
		if shipTo.Status != customerdeliverylocation.StatusActive {
			return ErrValidation{Message: "ship-to location is inactive"}
		}
	} else {
		def, err := s.shipTos.FindDefaultForCustomer(ctx, tenantID, *customerID)
		if err != nil && !errors.Is(err, customerdeliverylocation.ErrNotFound) {
			return err
		}
		shipTo = def
	}

	if shipTo != nil {
		o.ShipToID = &shipTo.ID
		o.ShipToName = &shipTo.Name
		o.ShipToAddress = &shipTo.Address
		o.ShipToProvince = &shipTo.Province
		o.ShipToDistrict = &shipTo.District
		o.ShipToPostalCode = &shipTo.PostalCode
		o.ShipToContactPerson = &shipTo.ContactPerson
		o.ShipToContactPhone = &shipTo.ContactPhone
	}
	return nil
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	params.Page, params.Limit = page, limit

	rows, total, err := s.repo.List(ctx, tenantID, params)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		fulfillment := deriveFulfillmentStatus(Status(row.Status), pickingStatusPtr(row.PickingStatus), shipmentStatusPtr(row.ShipmentStatus))
		responses = append(responses, ListResponse{
			ID: row.ID, OrderNo: row.OrderNo, OrderType: row.OrderType, Customer: row.Customer, ExternalReference: row.ExternalReference,
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
// Repository.List's doc for why the two paths differ). Phase 30 Batch 3:
// tenantID is the caller's own verified tenant — a cross-tenant id is
// rejected by Repository.FindDetail as ErrNotFound before any of this
// runs.
func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	row, err := s.repo.FindDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindItemsDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	var pickingStatus *picking.Status
	var shipmentStatus *shipping.Status
	if row.PickingTaskID != nil {
		// Phase 30 Batch 3: real verified tenantID, not
		// inventory.UnverifiedCallerTenantID — see pickingOrchestrator's
		// own doc above.
		pickingResp, err := s.picking.Get(ctx, tenantID, *row.PickingTaskID)
		if err != nil {
			return nil, err
		}
		pickingStatus = &pickingResp.Status

		// Phase 30 Batch 4: real verified tenantID, now that
		// internal/shipping is itself tenant-scoped — see shippingReader's
		// own doc above.
		shipResp, err := s.shipping.FindByPickingTaskID(ctx, tenantID, *row.PickingTaskID)
		if err != nil {
			return nil, err
		}
		if shipResp != nil {
			shipmentStatus = &shipResp.Status
		}
	}

	return &Response{
		ID: row.ID, OrderNo: row.OrderNo, OrderType: row.OrderType, Priority: row.Priority,
		Customer: row.Customer, ExternalReference: row.ExternalReference,
		CustomerID: row.CustomerID, ShipToID: row.ShipToID,
		ShipToName: row.ShipToName, ShipToAddress: row.ShipToAddress,
		ShipToProvince: row.ShipToProvince, ShipToDistrict: row.ShipToDistrict,
		ShipToPostalCode:    row.ShipToPostalCode,
		ShipToContactPerson: row.ShipToContactPerson, ShipToContactPhone: row.ShipToContactPhone,
		RequiredShippingDate: row.RequiredShippingDate, RequiredDeliveryDate: row.RequiredDeliveryDate,
		Carrier: row.Carrier, ShippingMethod: row.ShippingMethod, DeliveryInstructions: row.DeliveryInstructions,
		PreparedBy: row.PreparedBy, ApprovedBy: row.ApprovedBy,
		WarehouseID: row.WarehouseID, WarehouseCode: row.WarehouseCode,
		PickingTaskID: row.PickingTaskID, PickNo: row.PickNo,
		PickingMode: PickingMode(row.PickingMode), PickingBatchID: row.PickingBatchID,
		Status:            Status(row.Status),
		FulfillmentStatus: deriveFulfillmentStatus(Status(row.Status), pickingStatus, shipmentStatus),
		Remark:            row.Remark,
		ReleasedAt:        row.ReleasedAt, ReleasedBy: row.ReleasedBy, ReleasedByName: row.ReleasedByName,
		ShortageApproved: row.ShortageApproved,
		CreatedBy:        row.CreatedBy, CreatedByName: row.CreatedByName,
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
// chosen locations/lots. Phase 30 Batch 3: tenantID is the caller's own
// verified tenant — stamped onto the header and every line, and used to
// verify each line's product_id actually belongs to it (see
// productChecker's own doc).
func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}
	orderType, priority, err := normalizeOrderTypeAndPriority(req.OrderType, req.Priority)
	if err != nil {
		return nil, err
	}
	pickingMode, err := normalizePickingMode(req.PickingMode)
	if err != nil {
		return nil, err
	}

	o := &Order{
		Customer: req.Customer, ExternalReference: req.ExternalReference,
		OrderType: orderType, Priority: priority, PickingMode: pickingMode,
		RequiredShippingDate: req.RequiredShippingDate, RequiredDeliveryDate: req.RequiredDeliveryDate,
		Carrier: req.Carrier, ShippingMethod: req.ShippingMethod, DeliveryInstructions: req.DeliveryInstructions,
		PreparedBy: req.PreparedBy, ApprovedBy: req.ApprovedBy,
		WarehouseID: req.WarehouseID, Status: StatusPending, Remark: req.Remark, CreatedBy: &userID,
	}
	if err := s.applyCustomerShipTo(ctx, tenantID, o, req.CustomerID, req.ShipToID); err != nil {
		return nil, err
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var orderID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		if err := txRepo.CreateOrder(ctx, tenantID, o); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateExternalReference
			}
			return err
		}
		orderID = o.ID

		for _, itemReq := range req.Items {
			if err := s.createItem(ctx, tenantID, txRepo, o.ID, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "order.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, orderID)
}

func (s *Service) createItem(ctx context.Context, tenantID int64, txRepo *Repository, orderID int64, req CreateItemRequest) error {
	if req.OrderedQty <= 0 {
		return ErrValidation{Message: "ordered_qty must be greater than zero"}
	}
	// Phase 30 Batch 3: verified against the caller's own tenant via
	// FindByIDForTenant — a product belonging to another tenant now
	// returns ErrProductNotFound exactly like a nonexistent one.
	p, err := s.products.FindByIDForTenant(ctx, tenantID, req.ProductID)
	if err != nil {
		return ErrProductNotFound
	}
	uom := req.UOM
	if uom == "" {
		uom = string(p.BaseUOM)
	}
	// FEFOPreferred (Phase 33A) defaults to true (the same default
	// migrations/0080's own DF_order_items_fefo_preferred applies at the
	// column level) when the caller omits it entirely.
	fefo := true
	if req.FEFOPreferred != nil {
		fefo = *req.FEFOPreferred
	}
	return txRepo.CreateItem(ctx, tenantID, &Item{
		OrderID: orderID, ProductID: req.ProductID, OrderedQty: req.OrderedQty, UOM: uom,
		PreferredLotNo: req.PreferredLotNo, FEFOPreferred: fefo, LineNotes: req.LineNotes,
	})
}

// normalizeOrderTypeAndPriority defaults and validates Create/Update's
// own order_type/priority — extracted as its own pure function so the
// validation rules are unit-testable without a database, the same
// "extract the pure part" precedent dto.go's deriveFulfillmentStatus doc
// describes. Empty input defaults to SALES_ORDER/NORMAL; any non-empty
// value must still be one of the CK_orders_order_type/CK_orders_priority
// vocabularies — the database CHECK constraints would also refuse an
// invalid value, but this gives a specific 422 instead of a raw
// constraint-violation message, the same reasoning every other
// service-layer IsValid() check in this codebase already follows.
func normalizeOrderTypeAndPriority(orderType, priority string) (string, string, error) {
	if orderType == "" {
		orderType = "SALES_ORDER"
	}
	if orderType != "SALES_ORDER" && orderType != "INTERNAL_TRANSFER" {
		return "", "", ErrValidation{Message: "order_type must be SALES_ORDER or INTERNAL_TRANSFER"}
	}
	if priority == "" {
		priority = "NORMAL"
	}
	if priority != "LOW" && priority != "NORMAL" && priority != "HIGH" {
		return "", "", ErrValidation{Message: "priority must be LOW, NORMAL, or HIGH"}
	}
	return orderType, priority, nil
}

// normalizePickingMode defaults and validates Create/Update's own
// picking_mode (Phase 33C.1) — empty input defaults to PICK_BY_ORDER;
// any non-empty value must still be one of PickingMode.IsValid's
// vocabulary. See Order.PickingMode's own doc — this flag never changes
// CreatePickingTask's own behavior, only internal/pickingbatch's.
func normalizePickingMode(mode string) (PickingMode, error) {
	if mode == "" {
		return PickingModeByOrder, nil
	}
	m := PickingMode(mode)
	if !m.IsValid() {
		return "", ErrValidation{Message: "picking_mode must be PICK_BY_ORDER or PICK_BY_PRODUCT"}
	}
	return m, nil
}

// Update replaces the header fields and every line at once — full-
// replace, not a per-line diff, safe only because Status.CanUpdate
// (PENDING) means CreatePickingTask has never reserved anything against
// these lines yet. Phase 30 Batch 3: tenantID is the caller's own
// verified tenant — a cross-tenant id is rejected by the tenant-scoped
// FindByID as ErrNotFound before any write.
func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (*Response, error) {
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}
	orderType, priority, err := normalizeOrderTypeAndPriority(req.OrderType, req.Priority)
	if err != nil {
		return nil, err
	}
	pickingMode, err := normalizePickingMode(req.PickingMode)
	if err != nil {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		o, err := txRepo.FindByID(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if !o.Status.CanUpdate() {
			return ErrNotEditable
		}

		o.Customer = req.Customer
		o.ExternalReference = req.ExternalReference
		o.OrderType = orderType
		o.Priority = priority
		o.PickingMode = pickingMode
		o.RequiredShippingDate = req.RequiredShippingDate
		o.RequiredDeliveryDate = req.RequiredDeliveryDate
		o.Carrier = req.Carrier
		o.ShippingMethod = req.ShippingMethod
		o.DeliveryInstructions = req.DeliveryInstructions
		o.PreparedBy = req.PreparedBy
		o.ApprovedBy = req.ApprovedBy
		o.WarehouseID = req.WarehouseID
		o.Remark = req.Remark
		if err := s.applyCustomerShipTo(ctx, tenantID, o, req.CustomerID, req.ShipToID); err != nil {
			return err
		}
		if err := txRepo.UpdateHeader(ctx, tenantID, o); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateExternalReference
			}
			return err
		}

		if err := txRepo.DeleteItemsByOrder(ctx, tenantID, id); err != nil {
			return err
		}
		for _, itemReq := range req.Items {
			if err := s.createItem(ctx, tenantID, txRepo, id, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "order.update", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, id)
}

// CheckAvailability is the Review & Release screen's own read-only
// preview (GET /orders/{id}/availability) — exactly the same
// computation Release itself runs before deciding whether a PENDING ->
// RELEASED transition may proceed, exposed separately so the frontend
// can show the shortage table BEFORE the caller commits to a decision.
// Never reserves or mutates inventory: only inventory.Service.
// AvailabilityByProductWarehouse (a single SELECT) is called, once per
// line. Callable at any order status — a stale/already-RELEASED order's
// availability is still a meaningful, harmless thing to look at.
func (s *Service) CheckAvailability(ctx context.Context, tenantID, id int64) (*AvailabilityCheckResponse, error) {
	order, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindItemsDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return s.checkAvailability(ctx, tenantID, order, items)
}

// checkAvailability is CheckAvailability's/Release's shared core. A
// line is short when OrderedQty exceeds the warehouse-wide AvailableQty
// inventory.Service reports for that product — see inventory.Service.
// AvailabilityByProductWarehouse's own doc for why this is a warehouse
// total, not a specific location/lot (an order line is location-
// agnostic until CreatePickingTask).
func (s *Service) checkAvailability(ctx context.Context, tenantID int64, order *Order, items []ItemResponse) (*AvailabilityCheckResponse, error) {
	resp := &AvailabilityCheckResponse{OrderID: order.ID}
	for _, it := range items {
		avail, err := s.inventory.AvailabilityByProductWarehouse(ctx, tenantID, it.ProductID, order.WarehouseID)
		if err != nil {
			return nil, err
		}
		shortfall := it.OrderedQty - avail.AvailableQty
		hasShortage := shortfall > 0
		if !hasShortage {
			shortfall = 0
		} else {
			resp.HasShortage = true
		}
		resp.Lines = append(resp.Lines, AvailabilityLine{
			OrderItemID: it.ID, ProductID: it.ProductID, SKU: it.SKU, ProductName: it.ProductName,
			OrderedQty: it.OrderedQty, AvailableQty: avail.AvailableQty, ReservedQty: avail.ReservedQty, OnHandQty: avail.OnHandQty,
			HasShortage: hasShortage, ShortfallQty: shortfall,
		})
	}
	return resp, nil
}

// Release is the new Review & Release step (Phase 33A, POST /orders/
// {id}/release): PENDING -> RELEASED, gated on the SAME availability
// check CheckAvailability exposes read-only. If any line is short and
// the caller did not set ReleaseRequest.ApproveShortage, this returns
// ErrShortageApprovalRequired and leaves the order untouched — the
// "Controlled shortage approval" override requires both this explicit
// flag AND order.confirm (enforced by the route, not by this method),
// never one alone. Writes shortage_flag/shortfall_qty onto every line
// (even when there is no shortage at all — a line with no shortage is
// written back as false/NULL, overwriting whatever a PRIOR Release
// attempt may have stamped, though CanRelease means this only ever runs
// once per order in practice). Still never calls into internal/picking
// or reserves anything itself — that remains CreatePickingTask's own,
// unchanged responsibility, now simply gated on RELEASED instead of
// PENDING directly (see Status.CanCreatePickingTask's own doc).
func (s *Service) Release(ctx context.Context, tenantID, id int64, req ReleaseRequest, userID int64) (*Response, error) {
	order, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !order.Status.CanRelease() {
		return nil, ErrNotPending
	}

	items, err := s.repo.FindItemsDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrValidation{Message: "order has no items"}
	}

	avail, err := s.checkAvailability(ctx, tenantID, order, items)
	if err != nil {
		return nil, err
	}
	if avail.HasShortage && !req.ApproveShortage {
		return nil, ErrShortageApprovalRequired
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		rows, err := txRepo.ClaimForRelease(ctx, tenantID, id, userID, avail.HasShortage)
		if err != nil {
			return err
		}
		if rows == 0 {
			// Covers "already RELEASED/PICKING/CANCELLED" (a prior call
			// already succeeded, or is racing this one) and a cross-tenant
			// orderID, the same uniform-409 reasoning ClaimForPickingTask's
			// own doc gives.
			return ErrNotPending
		}
		for _, line := range avail.Lines {
			shortfall := line.ShortfallQty
			if err := txRepo.UpdateItemShortage(ctx, tenantID, line.OrderItemID, line.HasShortage, shortfall); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "order.release", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, id)
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
//
// Phase 30 Batch 3: tenantID is the caller's own verified tenant. It is
// threaded into ClaimForPickingTask (so a cross-tenant orderID can never
// be claimed) and into picking.CreateTx (so the resulting picking task,
// and everything it in turn verifies — Product/Location ownership,
// Inventory reservation — is created in the SAME tenant this order
// belongs to, never inventory.UnverifiedCallerTenantID). Location
// ownership for req.Items' location_id values is verified inside
// picking.Service.CreateTx itself (resolveLocation, Batch 2) using this
// same tenantID — internal/order never queries locations directly.
func (s *Service) CreatePickingTask(ctx context.Context, tenantID, id int64, req CreatePickingTaskRequest, userID int64) (*Response, error) {
	order, err := s.repo.FindByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !order.Status.CanCreatePickingTask() {
		return nil, ErrNotReleasable
	}

	items, err := s.repo.FindItemsByOrder(ctx, tenantID, id)
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
		rows, err := txRepo.ClaimForPickingTask(ctx, tenantID, id)
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
			// Also covers a cross-tenant orderID (ClaimForPickingTask's own
			// tenant_id predicate), indistinguishable here from any other
			// non-releasable state by design — never reveals whether the id
			// exists in another tenant.
			return ErrNotReleasable
		}

		// Phase 30 Batch 3: real verified tenantID, not
		// inventory.UnverifiedCallerTenantID — see pickingOrchestrator's
		// own doc above.
		pickingTaskID, err := s.picking.CreateTx(ctx, tenantID, tx, picking.CreateRequest{
			OrderNo: order.OrderNo, Customer: order.Customer, WarehouseID: order.WarehouseID,
			// One-way copy of this order's own already-validated/
			// snapshotted ship-to fields — picking.Service never
			// re-validates customer_id/ship_to_id ownership itself, see
			// picking.CreateRequest's own doc.
			CustomerID: order.CustomerID, ShipToID: order.ShipToID,
			ShipToName: order.ShipToName, ShipToAddress: order.ShipToAddress,
			ShipToProvince: order.ShipToProvince, ShipToDistrict: order.ShipToDistrict,
			ShipToPostalCode:    order.ShipToPostalCode,
			ShipToContactPerson: order.ShipToContactPerson, ShipToContactPhone: order.ShipToContactPhone,
			Priority: picking.Priority(req.Priority), DueDate: req.DueDate, Remark: order.Remark, Items: pickingItems,
		}, userID)
		if err != nil {
			// Rolling back this whole transaction also rolls back the claim
			// above — the order is left exactly PENDING/unlinked again, not
			// stuck in a half-released state.
			return err
		}
		return txRepo.LinkPickingTask(ctx, tenantID, id, pickingTaskID)
	})
	dbctx.LogTransaction(ctx, "order.create_picking_task", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, id)
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
// was ever reserved by Order itself. Phase 30 Batch 3: tenantID is the
// caller's own verified tenant — a cross-tenant id is rejected by the
// tenant-scoped FindByID as ErrNotFound before any write.
func (s *Service) Cancel(ctx context.Context, tenantID, id int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		o, err := txRepo.FindByID(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if !o.Status.CanCancel() {
			return ErrNotCancellable
		}
		o.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelOrder(ctx, tenantID, o)
	})
	dbctx.LogTransaction(ctx, "order.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, id)
}

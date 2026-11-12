package shipping

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/picking"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/platform/dbctx"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrPickingTaskNotFound = errors.New("shipping: picking task not found")
	ErrNotShippable        = errors.New("shipping: picking task is not in a shippable state")
	ErrAlreadyShipped      = errors.New("shipping: picking task already has a shipment")
	ErrNoShippableItems    = errors.New("shipping: picking task has no completed lines to ship")
	ErrInvalidTransition   = errors.New("shipping: shipment is not in a state that allows this action")
	ErrNotEditable         = errors.New("shipping: shipment can no longer be edited")
)

// pickingChecker is the narrow slice of Phase 9's picking.Service this
// package needs — satisfied structurally by *picking.Service, the same
// narrow-interface pattern internal/putaway established for its own
// receiptChecker dependency on internal/receiving.
type pickingChecker interface {
	Get(ctx context.Context, id int64) (*picking.Response, error)
}

// Service holds the Shipping workflow's business rules. It deliberately
// has NO dependency on internal/inventory — Shipping performs no stock
// mutation of its own (Picking already removed the stock and wrote the
// ledger when it picked); Confirm only ever transitions this package's
// own status machine. See package doc.
type Service struct {
	repo    *Repository
	picking pickingChecker
}

func NewService(repo *Repository, pickingSvc pickingChecker) *Service {
	return &Service{repo: repo, picking: pickingSvc}
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
	return s.repo.List(ctx, params)
}

// ListForExport is Phase 22's CSV-export path — see internal/receiving's
// own ListForExport doc for the shared reasoning.
func (s *Service) ListForExport(ctx context.Context, params ListParams) ([]ListResponse, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.List(ctx, params)
	return rows, err
}

func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	header, err := s.repo.FindDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, ShipmentNo: header.ShipmentNo, PickingTaskID: header.PickingTaskID,
		PickNo: header.PickNo, OrderNo: header.OrderNo, Customer: header.Customer,
		Carrier: header.Carrier, TrackingNo: header.TrackingNo, Packages: header.Packages, Weight: header.Weight,
		ShipDate: header.ShipDate, Address: header.Address, Remark: header.Remark, Status: Status(header.Status),
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
		DispatchedAt: header.DispatchedAt, DeliveredAt: header.DeliveredAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items,
	}, nil
}

// FindByPickingTaskID returns the shipment already linked to a picking
// task, or (nil, nil) if none exists yet — Phase 25's internal/order
// uses this to derive an order's read-only fulfillment status ("has
// this order's picking task been shipped, and if so, what's its
// current shipping status") without internal/order ever storing or
// syncing shipment state itself. Unlike Get, "not found" is not an
// error here: a picking task legitimately having no shipment yet is
// the normal, expected case for this caller.
func (s *Service) FindByPickingTaskID(ctx context.Context, pickingTaskID int64) (*Response, error) {
	shipment, err := s.repo.FindByPickingTaskID(ctx, pickingTaskID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, shipment.ID)
}

// Sources returns completed picking tasks not yet shipped — see
// Repository.Sources' doc.
func (s *Service) Sources(ctx context.Context) ([]SourceResponse, error) {
	return s.repo.Sources(ctx)
}

// Create validates the linked picking task and copies its completed
// lines' already-picked quantities verbatim — the client never supplies
// a per-line quantity at all (CreateRequest has no Items field), so
// "shipment quantity does not exceed picked quantity" is enforced by
// construction, not by a range check on client input. Never calls
// AddStock/RemoveStock/MoveStock: the stock was already removed by
// Picking's own Confirm (Phase 9); this only records that it is now
// being shipped.
func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	task, err := s.picking.Get(ctx, req.PickingTaskID)
	if err != nil {
		return nil, ErrPickingTaskNotFound
	}
	if task.Status != picking.StatusCompleted && task.Status != picking.StatusShortPick {
		return nil, ErrNotShippable
	}

	var shippableItems []picking.ItemResponse
	for _, it := range task.Items {
		if it.Status == picking.ItemStatusCompleted && it.PickedQty > 0 {
			shippableItems = append(shippableItems, it)
		}
	}
	if len(shippableItems) == 0 {
		return nil, ErrNoShippableItems
	}

	if req.Carrier != nil && *req.Carrier != "" && !Carrier(*req.Carrier).IsValid() {
		return nil, ErrValidation{Message: "carrier must be one of DHL, KERRY, FLASH, INTERNAL"}
	}

	packages := req.Packages
	if packages <= 0 {
		packages = 1
	}

	// The duplicate-shipment check is the first thing here that touches
	// the database — everything above is pure validation against the
	// already-fetched picking task and the request body.
	if _, err := s.repo.FindByPickingTaskID(ctx, req.PickingTaskID); err == nil {
		return nil, ErrAlreadyShipped
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc and
	// PROJECT_STATUS.md's Phase 8 policy, applied from the start here.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var shipmentID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		shipment := &Shipment{
			PickingTaskID: req.PickingTaskID, Carrier: req.Carrier, TrackingNo: req.TrackingNo,
			Packages: packages, Weight: req.Weight, ShipDate: req.ShipDate, Address: req.Address,
			Remark: req.Remark, Status: StatusReady, CreatedBy: &userID,
		}
		if err := txRepo.CreateShipment(ctx, shipment); err != nil {
			return err
		}
		shipmentID = shipment.ID

		for _, it := range shippableItems {
			if err := txRepo.CreateItem(ctx, &Item{
				ShipmentID: shipment.ID, PickingTaskItemID: it.ID, ProductID: it.ProductID,
				LotNo: it.LotNo, LocationID: it.LocationID, Qty: it.PickedQty, UOM: it.UOM,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "shipping.create", start, err)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyShipped
		}
		return nil, err
	}

	return s.Get(ctx, shipmentID)
}

// Update edits header fields only while the shipment is still READY
// (see Status.CanEdit) — mirrors Receiving's Update.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*Response, error) {
	shipment, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !shipment.Status.CanEdit() {
		return nil, ErrNotEditable
	}
	if req.Carrier != nil && *req.Carrier != "" && !Carrier(*req.Carrier).IsValid() {
		return nil, ErrValidation{Message: "carrier must be one of DHL, KERRY, FLASH, INTERNAL"}
	}

	packages := req.Packages
	if packages <= 0 {
		packages = 1
	}

	shipment.Carrier = req.Carrier
	shipment.TrackingNo = req.TrackingNo
	shipment.Packages = packages
	shipment.Weight = req.Weight
	shipment.ShipDate = req.ShipDate
	shipment.Address = req.Address
	shipment.Remark = req.Remark

	if err := s.repo.UpdateHeader(ctx, shipment); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Confirm is the atomic core of this phase's status machine: validate
// the requested transition -> apply it -> commit. Unlike Receiving/
// Putaway/Picking's Confirm, there is no inventory mutation here at all
// — no AddStock/RemoveStock/MoveStock call anywhere in this method or
// package, since Picking already performed the only stock removal this
// workflow needs (Phase 9). The idempotency guard still applies (see
// migrations/0022's doc) so a duplicate DISPATCH/DELIVER/ISSUE click
// can't apply the same transition twice or double-stamp
// dispatched_at/delivered_at.
func (s *Service) Confirm(ctx context.Context, id int64, req ConfirmRequest, userID int64) (*Response, error) {
	// This is the exact transaction shape Phase 8 hardened against
	// client-disconnect orphaning — detach before opening it, from the
	// start here, per the centralized policy (see PROJECT_STATUS.md).
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the shipment exists BEFORE claiming the idempotency key:
		// shipment_confirmations.shipment_id is a real FK
		// (migrations/0022), so inserting the guard for a nonexistent id
		// would raise a raw FK-violation 500 instead of a clean 404 — a
		// real bug found live during Phase 11's own testing (fixed there
		// in internal/stockadjustment/internal/stockcount first, applied
		// here identically per Phase 12's cross-package audit — see
		// PROJECT_STATUS.md). This SELECT has no side effect, so moving it
		// ahead of the INSERT does not weaken the idempotency guarantee.
		shipment, err := txRepo.FindByID(ctx, id)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the PRIMARY KEY constraint and everything below is
		// skipped and rolled back — see migrations/0022's doc.
		if err := txRepo.InsertConfirmation(ctx, req.IdempotencyKey, id); err != nil {
			return err
		}

		switch req.Action {
		case "DISPATCH":
			if !shipment.Status.CanDispatch() {
				return ErrInvalidTransition
			}
			if shipment.Carrier == nil || *shipment.Carrier == "" {
				return ErrValidation{Message: "carrier is required before dispatch"}
			}
			if shipment.TrackingNo == nil || *shipment.TrackingNo == "" {
				return ErrValidation{Message: "tracking number is required before dispatch"}
			}
			now := time.Now().UTC()
			shipment.Status = StatusDispatched
			shipment.DispatchedAt = &now

		case "DELIVER":
			if !shipment.Status.CanDeliver() {
				return ErrInvalidTransition
			}
			now := time.Now().UTC()
			shipment.Status = StatusDelivered
			shipment.DeliveredAt = &now

		case "ISSUE":
			if !shipment.Status.CanReportIssue() {
				return ErrInvalidTransition
			}
			remark := req.Remark
			if remark == "" && shipment.Remark != nil {
				remark = *shipment.Remark
			}
			if remark == "" {
				return ErrValidation{Message: "remark is required to report an issue"}
			}
			shipment.Remark = &remark
			shipment.Status = StatusIssue

		default:
			return ErrValidation{Message: "action must be one of DISPATCH, DELIVER, ISSUE"}
		}

		return txRepo.UpdateStatus(ctx, shipment)
	})
	dbctx.LogTransaction(ctx, "shipping.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second transition.
		return s.Get(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Cancel is Phase 15's addition — allowed only from READY, i.e. before
// dispatch (Status.CanCancel). Never calls AddStock/RemoveStock/
// MoveStock: this package has no inventory dependency at all (see
// package doc), and per this phase's own instructions, the stock
// Picking already removed when it picked these lines is never restored
// here even after a cancel — a cancelled shipment simply never goes
// out, it does not un-pick anything.
func (s *Service) Cancel(ctx context.Context, id int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		shipment, err := txRepo.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !shipment.Status.CanCancel() {
			// Covers "already dispatched/delivered/issue-flagged" (unsafe:
			// the shipment has already left, or already terminal) and
			// "already CANCELLED" (prevents double cancellation).
			return ErrInvalidTransition
		}

		// Populate the Phase 18 audit columns only on this, the successful
		// path — CanCancel's own guard above already rejected an already-
		// CANCELLED (or already-dispatched) shipment before this point, so a
		// duplicate/invalid cancel attempt never reaches here and can never
		// overwrite the original cancellation's audit data. Remark is
		// stored in the dedicated cancel_remark column, not the shipment's
		// own Remark field — unlike Phase 15's original behavior, Cancel no
		// longer clobbers whatever remark already existed on the shipment.
		shipment.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelShipment(ctx, shipment)
	})
	dbctx.LogTransaction(ctx, "shipping.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

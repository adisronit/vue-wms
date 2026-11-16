package relocation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/product"
	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrWarehouseNotFound = errors.New("relocation: warehouse not found")
	ErrLocationNotFound  = errors.New("relocation: location not found")
	ErrProductNotFound   = errors.New("relocation: product not found")
	ErrInvalidTransition = errors.New("relocation: task is not in a confirmable status")
	ErrNotReadyable      = errors.New("relocation: task is not in a status that can be marked ready")
	ErrEmptyTask         = errors.New("relocation: at least one item is required before marking a task ready")
	ErrCrossWarehouse    = errors.New("relocation: source and destination locations must belong to the task's warehouse")
	ErrNoSourceStock     = errors.New("relocation: no stock found for this product/lot at the source location")
	ErrNotCancellable    = errors.New("relocation: task is not in a cancellable status")
	ErrCapacityExceeded  = errors.New("relocation: destination location's configured capacity would be exceeded")
)

// productChecker and warehouseChecker are the narrow slices of
// internal/product and internal/warehouse this package needs —
// satisfied structurally by *product.Repository and
// *warehouse.Repository, matching internal/putaway's own interfaces.
type productChecker interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*product.Product, error)
}

type warehouseChecker interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*warehouse.Warehouse, error)
}

// Service holds the Relocation workflow's business rules — the status
// machine, validation, and the Confirm transaction that calls
// inventory.Service.MoveAvailableStockTx instead of mutating stock
// itself, and never calls AddStock: relocation only ever moves stock
// that is already somewhere in the warehouse to a different bin in the
// SAME warehouse (see ErrCrossWarehouse).
type Service struct {
	repo       *Repository
	products   productChecker
	warehouses warehouseChecker
	inventory  *inventory.Service
}

func NewService(repo *Repository, products productChecker, warehouses warehouseChecker, inventorySvc *inventory.Service) *Service {
	return &Service{repo: repo, products: products, warehouses: warehouses, inventory: inventorySvc}
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
	return s.repo.List(ctx, tenantID, params)
}

// ListForExport mirrors putaway.Service.ListForExport's CSV-export path.
func (s *Service) ListForExport(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.List(ctx, tenantID, params)
	return rows, err
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	header, err := s.repo.FindTaskDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, RelocationNo: header.RelocationNo,
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode,
		Status: Status(header.Status), Remark: header.Remark,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt, ReadyAt: header.ReadyAt, CompletedAt: header.CompletedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items,
	}, nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
	if _, err := s.warehouses.FindByIDForTenant(ctx, tenantID, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var taskID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		task := &Task{
			WarehouseID: req.WarehouseID, Status: StatusDraft, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateTask(ctx, tenantID, task); err != nil {
			return err
		}
		taskID = task.ID

		for _, itemReq := range req.Items {
			if err := s.createItem(ctx, txRepo, tenantID, task, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "relocation.create", start, err)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, tenantID, taskID)
}

func (s *Service) createItem(ctx context.Context, txRepo *Repository, tenantID int64, task *Task, req CreateItemRequest) error {
	if req.RequestedQty <= 0 {
		return ErrValidation{Message: "requested_qty must be greater than zero"}
	}
	if req.SourceLocationID == req.DestinationLocationID {
		return ErrValidation{Message: "source_location_id and destination_location_id must differ"}
	}

	p, err := s.products.FindByIDForTenant(ctx, tenantID, req.ProductID)
	if err != nil {
		return ErrProductNotFound
	}

	srcInfo, err := txRepo.resolveLocation(ctx, tenantID, req.SourceLocationID)
	if err != nil {
		return err
	}
	if srcInfo.WarehouseID != task.WarehouseID {
		return ErrCrossWarehouse
	}

	dstInfo, err := txRepo.resolveLocation(ctx, tenantID, req.DestinationLocationID)
	if err != nil {
		return err
	}
	if err := s.validateDestination(ctx, txRepo, tenantID, dstInfo, req.DestinationLocationID, task.WarehouseID, req.RequestedQty); err != nil {
		return err
	}

	// Confirms a balance actually exists for (product, source, lot) — an
	// early, clear error instead of deferring every check to Confirm.
	// The authoritative, concurrency-safe check still happens inside
	// Confirm's own MoveAvailableStockTx call (see its doc).
	if _, err := s.inventory.FindBalance(ctx, tenantID, req.ProductID, req.SourceLocationID, req.LotNo); err != nil {
		if errors.Is(err, inventory.ErrNotFound) {
			return ErrNoSourceStock
		}
		return err
	}

	uom := req.UOM
	if uom == "" {
		uom = string(p.BaseUOM)
	}

	item := &TaskItem{
		RelocationTaskID: task.ID, ProductID: req.ProductID, LotNo: req.LotNo, LPN: req.LPN,
		SourceLocationID: req.SourceLocationID, DestinationLocationID: req.DestinationLocationID,
		RequestedQty: req.RequestedQty, UOM: uom, Status: ItemStatusPending, Remark: req.Remark,
	}
	return txRepo.CreateItem(ctx, tenantID, item)
}

// validateDestination enforces cross-warehouse, active-status,
// storage-compatibility, and configured-capacity rules for a
// relocation's destination — the four checks Phase 32 explicitly asked
// for ("Validate destination storage compatibility, location status and
// capacity where configured").
//
// Storage compatibility is deliberately broader than putaway's
// STORAGE-only rule: relocation also legitimately feeds a PICKING face
// (e.g. replenishing a pick location from bulk storage), so both
// STORAGE and PICKING zone types are accepted; RECEIVING/SHIPPING/
// PACKING/QC/HOLD are transient/functional zones that are never a valid
// relocation destination.
func (s *Service) validateDestination(ctx context.Context, txRepo *Repository, tenantID int64, dstInfo *locationInfo, destinationLocationID, taskWarehouseID int64, addQty float64) error {
	if dstInfo.WarehouseID != taskWarehouseID {
		return ErrCrossWarehouse
	}
	if dstInfo.Status != "ACTIVE" {
		return ErrValidation{Message: "destination location is not active"}
	}
	if dstInfo.ZoneType != "STORAGE" && dstInfo.ZoneType != "PICKING" {
		return ErrValidation{Message: "destination location must be in a STORAGE or PICKING zone"}
	}
	return s.checkCapacity(ctx, txRepo, tenantID, dstInfo, destinationLocationID, addQty)
}

// checkCapacity enforces the destination's configured Capacity ("where
// configured" — a nil Capacity means no limit): the location's current
// total on-hand quantity plus addQty must not exceed it. Advisory,
// read-then-compare check, matching putaway.Service.checkCapacity.
func (s *Service) checkCapacity(ctx context.Context, txRepo *Repository, tenantID int64, dstInfo *locationInfo, destinationLocationID int64, addQty float64) error {
	if dstInfo.Capacity == nil {
		return nil
	}
	occupied, err := txRepo.LocationOccupiedQty(ctx, tenantID, destinationLocationID)
	if err != nil {
		return err
	}
	if occupied+addQty > float64(*dstInfo.Capacity) {
		return ErrCapacityExceeded
	}
	return nil
}

// Ready transitions a DRAFT task to READY, locking its lines from
// further edits — the explicit gate Phase 32 asked for before any
// Confirm call is allowed. Requires at least one item.
func (s *Service) Ready(ctx context.Context, tenantID, taskID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		task, err := txRepo.FindTaskByID(ctx, tenantID, taskID)
		if err != nil {
			return err
		}
		if !task.Status.CanReady() {
			return ErrNotReadyable
		}
		items, err := txRepo.FindItemsByTask(ctx, tenantID, taskID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return ErrEmptyTask
		}
		now := time.Now().UTC()
		task.Status = StatusReady
		task.ReadyAt = &now
		return txRepo.UpdateTask(ctx, tenantID, task)
	})
	dbctx.LogTransaction(ctx, "relocation.ready", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, taskID)
}

// Confirm is the atomic core of this phase: validate -> resolve the
// source balance's authoritative lot/expiry -> call
// inventory.MoveAvailableStockTx (same transaction, ledger rows written
// by MoveAvailableStockTx itself) -> update task item moved_qty ->
// recompute task status -> commit. Any failure rolls back everything,
// including any lines already processed earlier in this same call.
// Never calls AddStock — MoveAvailableStockTx only ever relocates an
// existing, currently-AVAILABLE balance (excluding whatever is reserved
// or QC-held), so system-wide quantity for the moved (product, lot) is
// unchanged by construction, and held/reserved stock can never be
// relocated this way — not just by convention, by the same atomic
// guarded UPDATE AdjustHold/AdjustReservation already use.
func (s *Service) Confirm(ctx context.Context, tenantID, taskID int64, req ConfirmRequest, userID int64) (*Response, error) {
	if len(req.Items) == 0 {
		return nil, ErrValidation{Message: "at least one item is required to confirm relocation"}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the task exists BEFORE claiming the idempotency key:
		// relocation_confirmations.relocation_task_id is a real FK
		// (migrations/0078), so inserting the guard for a nonexistent id
		// would raise a raw FK-violation 500 instead of a clean 404.
		task, err := txRepo.FindTaskByID(ctx, tenantID, taskID)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the PRIMARY KEY constraint and everything below is
		// skipped and rolled back — see migrations/0078's doc.
		if err := txRepo.InsertConfirmation(ctx, tenantID, req.IdempotencyKey, taskID); err != nil {
			return err
		}

		if !task.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		invRepo := inventory.NewRepository(tx)
		refType := "RELOCATION"
		refID := task.RelocationNo

		for _, lineReq := range req.Items {
			item, err := txRepo.FindItemByID(ctx, tenantID, lineReq.ItemID)
			if err != nil {
				return err
			}
			if item.RelocationTaskID != taskID {
				return ErrItemNotFound
			}
			if item.Status == ItemStatusCompleted {
				return ErrValidation{Message: fmt.Sprintf("line %d has already been completed", item.ID)}
			}
			if lineReq.Quantity <= 0 {
				return ErrValidation{Message: "quantity must be greater than zero to confirm a line"}
			}

			destinationID := item.DestinationLocationID
			if lineReq.DestinationLocationID != 0 {
				destinationID = lineReq.DestinationLocationID
			}
			if destinationID == item.SourceLocationID {
				return ErrValidation{Message: "source and destination locations must differ"}
			}

			dstInfo, err := txRepo.resolveLocation(ctx, tenantID, destinationID)
			if err != nil {
				return err
			}
			if err := s.validateDestination(ctx, txRepo, tenantID, dstInfo, destinationID, task.WarehouseID, lineReq.Quantity); err != nil {
				return err
			}

			// Product identity is fixed by item.ProductID. Lot and expiry
			// come from the source balance row itself, not user input, so
			// "lot identity must match source stock" and "expiry remains
			// unchanged" both hold by construction rather than by a
			// separate check — matches putaway's own Confirm.
			balance, err := s.inventory.FindBalance(ctx, tenantID, item.ProductID, item.SourceLocationID, item.LotNo)
			if err != nil {
				if errors.Is(err, inventory.ErrNotFound) {
					return ErrNoSourceStock
				}
				return err
			}

			if err := s.inventory.MoveAvailableStockTx(ctx, invRepo, inventory.MoveStockRequest{
				TenantID:  tenantID,
				ProductID: item.ProductID, FromLocationID: item.SourceLocationID, ToLocationID: destinationID,
				LotNo: item.LotNo, ExpiryDate: balance.ExpiryDate, Quantity: lineReq.Quantity,
				Type: inventory.TransactionMove, ReferenceType: &refType, ReferenceID: &refID,
				CreatedBy: &userID,
			}); err != nil {
				switch {
				case errors.Is(err, inventory.ErrInsufficientAvailableStock):
					return ErrValidation{Message: "insufficient available source stock to move this quantity — the remainder is reserved or held for QC and cannot be relocated"}
				case errors.Is(err, inventory.ErrNoStock):
					return ErrValidation{Message: "insufficient source stock to move this quantity"}
				default:
					return err
				}
			}

			// Atomic guarded increment (see AdjustMovedQty's doc) instead of
			// item.MovedQty += lineReq.Quantity followed by a blind
			// write-back, the same lost-update protection putaway's own
			// Confirm uses. Re-read afterward so status derivation uses the
			// row's true, post-increment total.
			if _, err := txRepo.AdjustMovedQty(ctx, tenantID, item.ID, lineReq.Quantity); err != nil {
				return err
			}
			updated, err := txRepo.FindItemByID(ctx, tenantID, item.ID)
			if err != nil {
				return err
			}
			updated.DestinationLocationID = destinationID
			updated.Status = itemStatus(updated.RequestedQty, updated.MovedQty)
			if err := txRepo.UpdateItemAfterConfirm(ctx, tenantID, updated); err != nil {
				return err
			}
		}

		allItems, err := txRepo.FindItemsByTask(ctx, tenantID, taskID)
		if err != nil {
			return err
		}
		task.Status = computeTaskStatus(allItems)
		if task.Status == StatusCompleted {
			now := time.Now().UTC()
			task.CompletedAt = &now
		}
		return txRepo.UpdateTask(ctx, tenantID, task)
	})
	dbctx.LogTransaction(ctx, "relocation.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second stock move.
		return s.Get(ctx, tenantID, taskID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, taskID)
}

// Cancel marks the task CANCELLED. Unlike putaway.Service.Cancel (which
// additionally rejects cancellation if ANY line has already moved
// stock), relocation deliberately allows cancelling an IN_PROGRESS
// task: Phase 32 explicitly asked for both "Partial relocation" and
// "Draft, Confirm, Cancel" as first-class capabilities together, and a
// WMS operator's real use case is "relocate what I've gotten to so far,
// abandon the rest" — not an all-or-nothing commitment once the first
// line moves. Lines that already moved stock (COMPLETED/
// PARTIALLY_COMPLETED) are left exactly as they are; only the header
// moves to CANCELLED, so nothing already-confirmed is rolled back or
// compensated for.
func (s *Service) Cancel(ctx context.Context, tenantID, taskID int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		task, err := txRepo.FindTaskByID(ctx, tenantID, taskID)
		if err != nil {
			return err
		}
		if !task.Status.CanCancel() {
			// Covers "already CANCELLED" (prevents double cancellation)
			// and "already COMPLETED" (every line already terminal).
			return ErrNotCancellable
		}
		task.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelTask(ctx, tenantID, task)
	})
	dbctx.LogTransaction(ctx, "relocation.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, taskID)
}

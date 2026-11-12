package picking

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
	ErrWarehouseNotFound = errors.New("picking: warehouse not found")
	ErrLocationNotFound  = errors.New("picking: location not found")
	ErrProductNotFound   = errors.New("picking: product not found")
	ErrInvalidTransition = errors.New("picking: task is not in a confirmable status")
	ErrNotCancellable    = errors.New("picking: task is not in a cancellable status")
	ErrCrossWarehouse    = errors.New("picking: location must belong to the task's warehouse")
	ErrNoSourceStock     = errors.New("picking: no stock found for this product/lot at the source location")
)

// productChecker and warehouseChecker are the narrow slices of the
// Phase 4/3 repositories this package needs — satisfied structurally by
// *product.Repository and *warehouse.Repository, the same narrow-
// interface pattern internal/receiving and internal/putaway already
// established for their own dependencies.
type productChecker interface {
	FindByID(ctx context.Context, id int64) (*product.Product, error)
}

type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

// Service holds the Picking workflow's business rules — the status
// machine, validation, and the Confirm transaction that calls Phase 5's
// inventory.Service.RemoveStockTx instead of mutating stock itself, and
// never calls AddStock/MoveStock: picking only ever removes stock for
// outbound fulfillment.
type Service struct {
	repo       *Repository
	products   productChecker
	warehouses warehouseChecker
	inventory  *inventory.Service
}

func NewService(repo *Repository, products productChecker, warehouses warehouseChecker, inventorySvc *inventory.Service) *Service {
	return &Service{repo: repo, products: products, warehouses: warehouses, inventory: inventorySvc}
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

// CountOpen exposes Repository.CountOpen through Service — Phase 23's
// Dashboard "Pending Picking" card.
func (s *Service) CountOpen(ctx context.Context) (int64, error) {
	return s.repo.CountOpen(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	header, err := s.repo.FindTaskDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, PickNo: header.PickNo, OrderNo: header.OrderNo, Customer: header.Customer,
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode,
		Status: Status(header.Status), Priority: Priority(header.Priority), DueDate: header.DueDate,
		Remark: header.Remark, CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt, CompletedAt: header.CompletedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items,
	}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	priority, err := s.validateCreate(ctx, req)
	if err != nil {
		return nil, err
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc, and
	// PROJECT_STATUS.md's Phase 8 policy (transaction-boundary ownership
	// lives in the Service layer, not the Handler, from the start here).
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var taskID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		id, err := s.createCore(ctx, txRepo, inventory.NewRepository(tx), req, priority, userID)
		taskID = id
		return err
	})
	dbctx.LogTransaction(ctx, "picking.create", start, err)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, taskID)
}

// CreateTx is Create's core logic (header + line inserts, each with its
// own Phase 12 reservation), bound to a *gorm.DB the caller already has
// open inside its own transaction — added in Phase 25 for
// internal/order's CreatePickingTask, which needs "claim the order +
// create the picking task" to commit or roll back together as one
// atomic unit, the same CreateTx-alongside-Create shape Phase 16
// established for internal/stockadjustment (see that package's own
// CreateTx doc). Returns just the new task's ID, not a full Response —
// the caller must read it back itself once its own transaction commits,
// the identical constraint stockadjustment.Service.CreateTx documents.
// Runs the exact same validation and createItem/ReserveStockTx logic as
// Create — nothing about the reservation mechanism is duplicated or
// reimplemented for this caller.
func (s *Service) CreateTx(ctx context.Context, tx *gorm.DB, req CreateRequest, userID int64) (int64, error) {
	priority, err := s.validateCreate(ctx, req)
	if err != nil {
		return 0, err
	}
	return s.createCore(ctx, NewRepository(tx), inventory.NewRepository(tx), req, priority, userID)
}

// validateCreate is the pre-transaction validation both Create and
// CreateTx run — warehouse existence and priority — before either opens
// (or receives) a transaction.
func (s *Service) validateCreate(ctx context.Context, req CreateRequest) (Priority, error) {
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return "", ErrWarehouseNotFound
	}
	priority := req.Priority
	if priority == "" {
		priority = PriorityNormal
	}
	if !priority.IsValid() {
		return "", ErrValidation{Message: "priority must be one of LOW, NORMAL, HIGH"}
	}
	return priority, nil
}

// createCore is the shared insert logic both Create (via its own
// transaction) and CreateTx (via the caller's) run against a
// transactional Repository/inventory.Repository pair.
func (s *Service) createCore(ctx context.Context, txRepo *Repository, invRepo *inventory.Repository, req CreateRequest, priority Priority, userID int64) (int64, error) {
	task := &Task{
		OrderNo: req.OrderNo, Customer: req.Customer, WarehouseID: req.WarehouseID,
		Status: StatusPending, Priority: priority, DueDate: req.DueDate, Remark: req.Remark, CreatedBy: &userID,
	}
	if err := txRepo.CreateTask(ctx, task); err != nil {
		return 0, err
	}

	for _, itemReq := range req.Items {
		if err := s.createItem(ctx, txRepo, invRepo, task, itemReq); err != nil {
			return 0, err
		}
	}
	return task.ID, nil
}

func (s *Service) createItem(ctx context.Context, txRepo *Repository, invRepo *inventory.Repository, task *Task, req CreateItemRequest) error {
	if req.OrderedQty <= 0 {
		return ErrValidation{Message: "ordered_qty must be greater than zero"}
	}

	p, err := s.products.FindByID(ctx, req.ProductID)
	if err != nil {
		return ErrProductNotFound
	}

	locInfo, err := txRepo.resolveLocation(ctx, req.LocationID)
	if err != nil {
		return err
	}
	if locInfo.WarehouseID != task.WarehouseID {
		return ErrCrossWarehouse
	}
	if locInfo.Status != "ACTIVE" {
		return ErrValidation{Message: "location is not active"}
	}

	// Reserve the line's full ordered_qty against (product, location,
	// lot) right now, at task-creation time — this is Phase 12's
	// reservation point, replacing the earlier phase's advisory-only
	// FindBalance existence check. ReserveStockTx's guarded UPDATE (see
	// its doc and AdjustReservation's) is what actually makes this
	// concurrency-safe: two concurrent Create calls against the same
	// tuple can never together reserve more than is on hand, because the
	// arithmetic and the availability check are one atomic SQL statement,
	// not a Go read-then-write. Never touches quantity and never writes a
	// ledger row — a reservation is a claim, not a movement.
	if _, err := s.inventory.ReserveStockTx(ctx, invRepo, inventory.ReserveRequest{
		ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo, Quantity: req.OrderedQty,
	}); err != nil {
		if errors.Is(err, inventory.ErrNoStock) {
			return ErrNoSourceStock
		}
		if errors.Is(err, inventory.ErrInsufficientStock) {
			return ErrValidation{Message: "insufficient available stock to reserve this quantity"}
		}
		return err
	}

	uom := req.UOM
	if uom == "" {
		uom = string(p.BaseUOM)
	}

	item := &TaskItem{
		PickingTaskID: task.ID, ProductID: req.ProductID, LotNo: req.LotNo, LocationID: req.LocationID,
		OrderedQty: req.OrderedQty, UOM: uom, Status: ItemStatusPending,
	}
	return txRepo.CreateItem(ctx, item)
}

// Confirm is the atomic core of this phase: validate -> call
// inventory.RemoveStockTx (same transaction, ledger row written by
// RemoveStockTx itself) -> atomically increment picked_qty (guarded
// UPDATE, not a Go read-modify-write — see AdjustPickedQty's doc and
// PROJECT_STATUS.md's Phase 9 race-fix section) -> recompute task status
// -> commit. Any failure rolls back everything, including any lines
// already processed earlier in this same call. Never calls AddStock or
// MoveStock — RemoveStockTx only ever draws down an existing balance.
func (s *Service) Confirm(ctx context.Context, taskID int64, req ConfirmRequest, userID int64) (*Response, error) {
	if len(req.Items) == 0 {
		return nil, ErrValidation{Message: "at least one item is required to confirm picking"}
	}

	// This is the exact transaction shape that orphaned live on Putaway
	// (Phase 7) and Receiving (Phase 8) when the client disconnected
	// mid-flight — detach before opening it, from the start here, per
	// Phase 8's centralized policy (see PROJECT_STATUS.md).
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the task exists BEFORE claiming the idempotency key:
		// picking_confirmations.picking_task_id is a real FK
		// (migrations/0019), so inserting the guard for a nonexistent id
		// would raise a raw FK-violation 500 instead of a clean 404 — a
		// real bug found live during Phase 11's own testing (fixed there
		// in internal/stockadjustment/internal/stockcount first, applied
		// here identically per Phase 12's cross-package audit — see
		// PROJECT_STATUS.md). This SELECT has no side effect, so moving it
		// ahead of the INSERT does not weaken the idempotency guarantee.
		task, err := txRepo.FindTaskByID(ctx, taskID)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the PRIMARY KEY constraint and everything below is
		// skipped and rolled back — see migrations/0019's doc.
		if err := txRepo.InsertConfirmation(ctx, req.IdempotencyKey, taskID); err != nil {
			return err
		}

		if !task.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		invRepo := inventory.NewRepository(tx)
		refType := "PICKING"
		refID := task.PickNo

		for _, lineReq := range req.Items {
			item, err := txRepo.FindItemByID(ctx, lineReq.ItemID)
			if err != nil {
				return err
			}
			if item.PickingTaskID != taskID {
				return ErrItemNotFound
			}
			if item.Status == ItemStatusCompleted || item.Status == ItemStatusShort {
				return ErrValidation{Message: fmt.Sprintf("line %d has already been resolved", item.ID)}
			}

			if lineReq.Action == "SHORT" {
				if lineReq.Remark == "" {
					return ErrValidation{Message: "remark is required to report a short pick"}
				}
				remark := lineReq.Remark
				item.Remark = &remark
				item.Status = itemStatus(item.OrderedQty, item.PickedQty, true)

				// A SHORT permanently ends this line: whatever fraction of
				// its ordered_qty was reserved at task-creation time but
				// never actually picked must be released now, or it would
				// sit stuck against this (product, location, lot) forever,
				// silently starving other tasks' availability — see
				// ReleaseReservationTx's doc. Never removes stock for the
				// short amount: nothing was picked, nothing left the
				// building.
				unreserveQty := item.OrderedQty - item.PickedQty
				if unreserveQty > 0 {
					if _, err := s.inventory.ReleaseReservationTx(ctx, invRepo, inventory.ReserveRequest{
						ProductID: item.ProductID, LocationID: item.LocationID, LotNo: item.LotNo, Quantity: unreserveQty,
					}); err != nil {
						return err
					}
				}

				if err := txRepo.UpdateItemAfterConfirm(ctx, item); err != nil {
					return err
				}
				continue
			}

			if lineReq.Quantity <= 0 {
				return ErrValidation{Message: "quantity must be greater than zero to confirm a pick"}
			}

			// ConsumeReservationTx releases this call's share of the
			// reservation placed at task-creation time and removes the
			// same quantity from the real balance (writing the PICK ledger
			// row), atomically — see its doc. Replaces the earlier phase's
			// direct RemoveStockTx call now that stock is reserved before
			// it's ever physically picked.
			if _, err := s.inventory.ConsumeReservationTx(ctx, invRepo, inventory.StockChangeRequest{
				ProductID: item.ProductID, LocationID: item.LocationID, LotNo: item.LotNo,
				Quantity: lineReq.Quantity, Type: inventory.TransactionPick,
				ReferenceType: &refType, ReferenceID: &refID, CreatedBy: &userID,
			}); err != nil {
				if errors.Is(err, inventory.ErrInsufficientStock) || errors.Is(err, inventory.ErrNoStock) {
					return ErrValidation{Message: "insufficient stock to pick this quantity"}
				}
				if errors.Is(err, inventory.ErrReservationUnderflow) {
					return ErrValidation{Message: "quantity exceeds what is currently reserved for this line"}
				}
				return err
			}

			// Atomic guarded increment, not a Go read-modify-write — see
			// AdjustPickedQty's doc. Re-read afterward so status derivation
			// uses the row's true, post-decrement total.
			if _, err := txRepo.AdjustPickedQty(ctx, item.ID, lineReq.Quantity); err != nil {
				return err
			}
			updated, err := txRepo.FindItemByID(ctx, item.ID)
			if err != nil {
				return err
			}
			updated.Status = itemStatus(updated.OrderedQty, updated.PickedQty, false)
			if err := txRepo.UpdateItemAfterConfirm(ctx, updated); err != nil {
				return err
			}
		}

		allItems, err := txRepo.FindItemsByTask(ctx, taskID)
		if err != nil {
			return err
		}
		task.Status = computeTaskStatus(allItems)
		if task.Status == StatusCompleted || task.Status == StatusShortPick {
			now := time.Now().UTC()
			task.CompletedAt = &now
		}
		return txRepo.UpdateTask(ctx, task)
	})
	dbctx.LogTransaction(ctx, "picking.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second stock removal.
		return s.Get(ctx, taskID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

// Cancel is Phase 13's addition — the reservation-lifecycle counterpart
// to Confirm. It never calls AddStockTx/RemoveStockTx/MoveStockTx and
// never writes an inventory_transactions row: cancellation is the
// mirror image of Create's ReserveStockTx call, releasing whatever a
// task's still-open lines claimed without ever having drawn down real
// stock for them. quantity itself is never touched, matching Phase 12's
// own "reservation is a claim, not a movement" principle.
func (s *Service) Cancel(ctx context.Context, taskID int64, req CancelRequest, userID int64) (*Response, error) {
	// This is the exact transaction shape Phase 8 hardened against
	// client-disconnect orphaning — detach before opening it, from the
	// start here, per the centralized policy (see PROJECT_STATUS.md).
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the task exists before deciding anything about it — the
		// same existence-before-mutation ordering Phase 12's cross-package
		// audit established as the correct pattern (see Confirm's own
		// doc above), even though Cancel has no idempotency-guard INSERT
		// to race against.
		task, err := txRepo.FindTaskByID(ctx, taskID)
		if err != nil {
			return err
		}
		if !task.Status.CanCancel() {
			// Covers both "already CANCELLED" (prevents double
			// cancellation: the first successful Cancel already moved the
			// task out of PENDING/IN_PROGRESS, so a second call — retry,
			// double-click, or a genuine race that lost against the first
			// call's row lock below — is rejected here, cleanly, before
			// touching any reservation) and "already COMPLETED/SHORT_PICK"
			// (stock has already been fully picked/resolved; per Phase
			// 13's own instructions, cancellation is never allowed past
			// that point).
			return ErrNotCancellable
		}

		items, err := txRepo.FindItemsByTask(ctx, taskID)
		if err != nil {
			return err
		}

		invRepo := inventory.NewRepository(tx)
		for _, item := range items {
			// Lines already COMPLETED or SHORT have already had their
			// entire reservation resolved to zero by Confirm itself (see
			// ConsumeReservationTx/ReleaseReservationTx's own call sites
			// above) — OrderedQty-PickedQty on a SHORT line is not a
			// reliable "still reserved" signal (SHORT deliberately leaves
			// PickedQty short of OrderedQty forever), so skipping by
			// Status here, not just by remaining quantity, is required to
			// avoid attempting to release a reservation a second time.
			if item.Status == ItemStatusCompleted || item.Status == ItemStatusShort {
				continue
			}
			remaining := item.OrderedQty - item.PickedQty
			if remaining <= 0 {
				continue
			}
			if _, err := s.inventory.ReleaseReservationTx(ctx, invRepo, inventory.ReserveRequest{
				ProductID: item.ProductID, LocationID: item.LocationID, LotNo: item.LotNo, Quantity: remaining,
			}); err != nil {
				return err
			}
		}

		// Populate the Phase 18 audit columns only on this, the successful
		// path — CanCancel's own guard above already rejected an already-
		// CANCELLED (or otherwise terminal) task before this point, so a
		// duplicate/invalid cancel attempt never reaches here and can never
		// overwrite the original cancellation's audit data. Remark is
		// stored in the dedicated cancel_remark column, not the task's own
		// Remark field — unlike Phase 13's original behavior, Cancel no
		// longer clobbers whatever remark already existed on the task.
		task.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelTask(ctx, task)
	})
	dbctx.LogTransaction(ctx, "picking.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

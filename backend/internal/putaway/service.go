package putaway

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
	"wms-backend/internal/receiving"
	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrWarehouseNotFound   = errors.New("putaway: warehouse not found")
	ErrLocationNotFound    = errors.New("putaway: location not found")
	ErrProductNotFound     = errors.New("putaway: product not found")
	ErrReceiptNotFound     = errors.New("putaway: receipt not found")
	ErrInvalidTransition   = errors.New("putaway: task is not in a confirmable status")
	ErrCrossWarehouse      = errors.New("putaway: source and destination locations must belong to the task's warehouse")
	ErrNoSourceStock       = errors.New("putaway: no stock found for this product/lot at the source location")
	ErrNotCancellable      = errors.New("putaway: task is not in a cancellable status")
	ErrCancelHasMovedStock = errors.New("putaway: cannot cancel a task that has already moved stock for one or more lines")
)

// productChecker, warehouseChecker, and receiptChecker are the narrow
// slices of the Phase 4/3/6 services this package needs — satisfied
// structurally by *product.Repository, *warehouse.Repository, and
// *receiving.Service, the same narrow-interface pattern
// internal/receiving already established for its own Phase 3/4
// dependencies.
type productChecker interface {
	FindByID(ctx context.Context, id int64) (*product.Product, error)
}

type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

type receiptChecker interface {
	Get(ctx context.Context, id int64) (*receiving.Response, error)
}

// Service holds the Putaway workflow's business rules — the status
// machine, validation, and the Confirm transaction that calls Phase 5's
// inventory.Service.MoveStockTx instead of mutating stock itself, and
// never calls AddStock: putaway only ever relocates stock a Phase 6
// receipt already brought into the system.
type Service struct {
	repo       *Repository
	products   productChecker
	warehouses warehouseChecker
	receipts   receiptChecker
	inventory  *inventory.Service
}

func NewService(repo *Repository, products productChecker, warehouses warehouseChecker, receipts receiptChecker, inventorySvc *inventory.Service) *Service {
	return &Service{repo: repo, products: products, warehouses: warehouses, receipts: receipts, inventory: inventorySvc}
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
	header, err := s.repo.FindTaskDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, PutawayNo: header.PutawayNo, ReceiptID: header.ReceiptID, ReceiptNo: header.ReceiptNo,
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode,
		Status: Status(header.Status), Priority: Priority(header.Priority), AssignedTo: header.AssignedTo,
		Remark: header.Remark, CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt, CompletedAt: header.CompletedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items,
	}, nil
}

// Sources returns a completed/partially-received receipt's lines still
// eligible for putaway — GET /putaway/sources/{receiptId}, powering
// NewPutawayView.vue's "Source Document" -> loadSourceItems() flow with
// real data instead of its mockSourceItems object.
func (s *Service) Sources(ctx context.Context, receiptID int64) ([]SourceLineResponse, error) {
	if _, err := s.receipts.Get(ctx, receiptID); err != nil {
		return nil, ErrReceiptNotFound
	}
	return s.repo.SourceLines(ctx, receiptID)
}

func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}

	if req.ReceiptID != nil {
		receipt, err := s.receipts.Get(ctx, *req.ReceiptID)
		if err != nil {
			return nil, ErrReceiptNotFound
		}
		if receipt.WarehouseID != req.WarehouseID {
			return nil, ErrValidation{Message: "warehouse_id must match the source receipt's warehouse"}
		}
	}

	priority := req.Priority
	if priority == "" {
		priority = PriorityNormal
	}
	if !priority.IsValid() {
		return nil, ErrValidation{Message: "priority must be one of LOW, NORMAL, HIGH"}
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc. Moved
	// here from handler.go in Phase 8: transaction-boundary ownership
	// belongs in the Service layer, not the HTTP layer (see
	// PROJECT_STATUS.md's Phase 8 policy).
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var taskID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		task := &Task{
			ReceiptID: req.ReceiptID, WarehouseID: req.WarehouseID, Status: StatusPending, Priority: priority,
			AssignedTo: req.AssignedTo, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateTask(ctx, task); err != nil {
			return err
		}
		taskID = task.ID

		for _, itemReq := range req.Items {
			if err := s.createItem(ctx, txRepo, task, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "putaway.create", start, err)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, taskID)
}

func (s *Service) createItem(ctx context.Context, txRepo *Repository, task *Task, req CreateItemRequest) error {
	if req.RequestedQty <= 0 {
		return ErrValidation{Message: "requested_qty must be greater than zero"}
	}
	if req.SourceLocationID == req.DestinationLocationID {
		return ErrValidation{Message: "source_location_id and destination_location_id must differ"}
	}

	p, err := s.products.FindByID(ctx, req.ProductID)
	if err != nil {
		return ErrProductNotFound
	}

	srcInfo, err := txRepo.resolveLocation(ctx, req.SourceLocationID)
	if err != nil {
		return err
	}
	if srcInfo.WarehouseID != task.WarehouseID {
		return ErrCrossWarehouse
	}

	dstInfo, err := txRepo.resolveLocation(ctx, req.DestinationLocationID)
	if err != nil {
		return err
	}
	if dstInfo.WarehouseID != task.WarehouseID {
		return ErrCrossWarehouse
	}
	if dstInfo.Status != "ACTIVE" {
		return ErrValidation{Message: "destination location is not active"}
	}
	// Destination must be a real storage bin, not (e.g.) back into the
	// receiving dock it just came from — the one location-classification
	// rule NewPutawayView.vue's own subtitle actually states ("Move QC
	// passed goods from staging area to storage bin locations"). Reuses
	// Zone.Type (Phase 3) as-is; no schema change needed — see
	// PROJECT_STATUS.md.
	if dstInfo.ZoneType != "STORAGE" {
		return ErrValidation{Message: "destination location must be in a STORAGE zone"}
	}

	// Confirms a balance actually exists for (product, source, lot) —
	// an early, clear error instead of deferring every check to Confirm.
	// The authoritative, concurrency-safe check still happens inside
	// Confirm's own MoveStockTx call (see its doc).
	if _, err := s.inventory.FindBalance(ctx, req.ProductID, req.SourceLocationID, req.LotNo); err != nil {
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
		PutawayTaskID: task.ID, ReceiptItemID: req.ReceiptItemID, ProductID: req.ProductID, LotNo: req.LotNo,
		SourceLocationID: req.SourceLocationID, DestinationLocationID: req.DestinationLocationID,
		RequestedQty: req.RequestedQty, UOM: uom, Status: ItemStatusPending, Remark: req.Remark,
	}
	return txRepo.CreateItem(ctx, item)
}

// Confirm is the atomic core of this phase: validate -> resolve the
// source balance's authoritative product/lot/expiry -> call
// inventory.MoveStockTx (same transaction, ledger rows written by
// MoveStockTx itself) -> update task item moved_qty -> recompute task
// status -> commit. Any failure rolls back everything, including any
// lines already processed earlier in this same call. Never calls
// AddStock — MoveStockTx only ever relocates an existing balance, so
// system-wide quantity for the moved (product, lot) is unchanged by
// construction, not just by convention.
func (s *Service) Confirm(ctx context.Context, taskID int64, req ConfirmRequest, userID int64) (*Response, error) {
	if len(req.Items) == 0 {
		return nil, ErrValidation{Message: "at least one item is required to confirm putaway"}
	}

	// This is the exact transaction shape that orphaned live when the
	// client disconnected mid-flight during this phase's own
	// verification (see PROJECT_STATUS.md). Detach before opening it —
	// moved here from handler.go in Phase 8, replacing the original
	// handler-level mutationContext fix with the same policy applied at
	// the Service layer, consistently with Receiving/Inventory.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the task exists BEFORE claiming the idempotency key:
		// putaway_confirmations.putaway_task_id is a real FK
		// (migrations/0016), so inserting the guard for a nonexistent id
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
		// skipped and rolled back — see migrations/0016's doc.
		if err := txRepo.InsertConfirmation(ctx, req.IdempotencyKey, taskID); err != nil {
			return err
		}

		if !task.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		invRepo := inventory.NewRepository(tx)
		refType := "PUTAWAY"
		refID := task.PutawayNo

		for _, lineReq := range req.Items {
			item, err := txRepo.FindItemByID(ctx, lineReq.ItemID)
			if err != nil {
				return err
			}
			if item.PutawayTaskID != taskID {
				return ErrItemNotFound
			}
			if item.Status == ItemStatusCompleted || item.Status == ItemStatusIssue {
				return ErrValidation{Message: fmt.Sprintf("line %d has already been resolved", item.ID)}
			}

			if lineReq.Action == "ISSUE" {
				if lineReq.Remark == "" {
					return ErrValidation{Message: "remark is required to report an issue"}
				}
				remark := lineReq.Remark
				item.Remark = &remark
				item.Status = itemStatus(item.RequestedQty, item.MovedQty, true)
				if err := txRepo.UpdateItemAfterConfirm(ctx, item); err != nil {
					return err
				}
				continue
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

			dstInfo, err := txRepo.resolveLocation(ctx, destinationID)
			if err != nil {
				return err
			}
			if dstInfo.WarehouseID != task.WarehouseID {
				return ErrCrossWarehouse
			}
			if dstInfo.Status != "ACTIVE" {
				return ErrValidation{Message: "destination location is not active"}
			}
			if dstInfo.ZoneType != "STORAGE" {
				return ErrValidation{Message: "destination location must be in a STORAGE zone"}
			}

			// Product identity is fixed by item.ProductID (ConfirmItemRequest
			// carries no product_id — there is no way to redirect a line to a
			// different product at confirm time). Lot and expiry come from
			// the source balance row itself, not user input, so "lot
			// identity must match source stock" and "expiry must remain
			// unchanged" both hold by construction rather than by a
			// separate check.
			balance, err := s.inventory.FindBalance(ctx, item.ProductID, item.SourceLocationID, item.LotNo)
			if err != nil {
				if errors.Is(err, inventory.ErrNotFound) {
					return ErrNoSourceStock
				}
				return err
			}

			if err := s.inventory.MoveStockTx(ctx, invRepo, inventory.MoveStockRequest{
				ProductID: item.ProductID, FromLocationID: item.SourceLocationID, ToLocationID: destinationID,
				LotNo: item.LotNo, ExpiryDate: balance.ExpiryDate, Quantity: lineReq.Quantity,
				Type: inventory.TransactionPutaway, ReferenceType: &refType, ReferenceID: &refID,
				CreatedBy: &userID,
			}); err != nil {
				if errors.Is(err, inventory.ErrInsufficientStock) || errors.Is(err, inventory.ErrNoStock) {
					return ErrValidation{Message: "insufficient source stock to move this quantity"}
				}
				return err
			}

			// Atomic guarded increment (see AdjustMovedQty's doc) instead of
			// item.MovedQty += lineReq.Quantity followed by a blind
			// write-back — that read-modify-write was exactly the
			// lost-update race Phase 8's concurrency regression test found
			// on receiving's equivalent field. Re-read afterward so status
			// derivation uses the row's true, post-increment total, not
			// this loop's pre-transaction snapshot.
			if _, err := txRepo.AdjustMovedQty(ctx, item.ID, lineReq.Quantity); err != nil {
				return err
			}
			updated, err := txRepo.FindItemByID(ctx, item.ID)
			if err != nil {
				return err
			}
			updated.DestinationLocationID = destinationID
			updated.Status = itemStatus(updated.RequestedQty, updated.MovedQty, false)
			if err := txRepo.UpdateItemAfterConfirm(ctx, updated); err != nil {
				return err
			}
		}

		allItems, err := txRepo.FindItemsByTask(ctx, taskID)
		if err != nil {
			return err
		}
		task.Status = computeTaskStatus(allItems)
		if task.Status == StatusCompleted || task.Status == StatusIssue {
			now := time.Now().UTC()
			task.CompletedAt = &now
		}
		return txRepo.UpdateTask(ctx, task)
	})
	dbctx.LogTransaction(ctx, "putaway.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second stock move.
		return s.Get(ctx, taskID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

// Cancel is Phase 15's addition. Status.CanCancel's header-level check
// (PENDING/IN_PROGRESS) is necessary but not sufficient here — unlike
// Receiving/Picking, a Putaway task can be IN_PROGRESS purely because
// one line was flagged ISSUE, with every other line still untouched
// (MovedQty == 0 everywhere); Cancel must still allow that case, since
// an ISSUE flag alone never moved any stock. What it must NOT allow is
// cancelling a task where any line's MovedQty > 0 — that line's
// MoveStockTx already committed a real, irreversible relocation this
// phase does not invent a compensating MoveStock for. So this method
// re-reads every line and rejects (ErrCancelHasMovedStock, 409) if any
// of them has moved anything, regardless of what the header status
// alone would otherwise allow. Never calls AddStock/RemoveStock/
// MoveStock itself — Cancel is a pure status transition here, since
// Putaway (unlike Picking) never reserves anything at task-creation
// time for Cancel to release.
func (s *Service) Cancel(ctx context.Context, taskID int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		task, err := txRepo.FindTaskByID(ctx, taskID)
		if err != nil {
			return err
		}
		if !task.Status.CanCancel() {
			// Covers "already CANCELLED" (prevents double cancellation)
			// and "already COMPLETED/ISSUE" (every line already terminal).
			return ErrNotCancellable
		}

		items, err := txRepo.FindItemsByTask(ctx, taskID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.MovedQty > 0 {
				return ErrCancelHasMovedStock
			}
		}

		// Populate the Phase 18 audit columns only on this, the successful
		// path — CanCancel's own guard plus the MovedQty check above already
		// rejected an already-CANCELLED (or already-moved) task before this
		// point, so a duplicate/invalid cancel attempt never reaches here
		// and can never overwrite the original cancellation's audit data.
		// Remark is stored in the dedicated cancel_remark column, not the
		// task's own Remark field — unlike Phase 15's original behavior,
		// Cancel no longer clobbers whatever remark already existed.
		task.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelTask(ctx, task)
	})
	dbctx.LogTransaction(ctx, "putaway.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

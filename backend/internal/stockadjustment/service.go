package stockadjustment

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/product"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrProductNotFound   = errors.New("stockadjustment: product not found")
	ErrLocationNotFound  = errors.New("stockadjustment: location not found")
	ErrNotEditable       = errors.New("stockadjustment: adjustment can no longer be edited")
	ErrInvalidTransition = errors.New("stockadjustment: adjustment is not in a confirmable status")
	ErrInsufficientStock = errors.New("stockadjustment: insufficient stock for this decrease")
)

// productChecker and locationChecker are the narrow slices of the
// Phase 4/3 repositories this package needs — the same narrow-interface
// pattern every prior phase established.
type productChecker interface {
	FindByID(ctx context.Context, id int64) (*product.Product, error)
}

type locationChecker interface {
	FindByID(ctx context.Context, id int64) (*location.Location, error)
}

// Service holds the Stock Adjustment workflow's business rules — the
// status machine, validation, and the Confirm/Approve transaction that
// calls Phase 5's inventory.Service.AddStockTx/RemoveStockTx instead of
// mutating inventory.quantity directly. No approval ever overwrites a
// balance — only INCREASE/DECREASE deltas through Inventory Core's own
// atomic guarded UPDATE.
type Service struct {
	repo      *Repository
	products  productChecker
	locations locationChecker
	inventory *inventory.Service
	// pendingWarningHours is Phase 20's configured follow-up threshold
	// (config.StockAdjustmentPendingWarningHours) — read-only here,
	// passed straight through to pendingAging for both List and Get.
	pendingWarningHours int
}

func NewService(repo *Repository, products productChecker, locations locationChecker, inventorySvc *inventory.Service, pendingWarningHours int) *Service {
	return &Service{repo: repo, products: products, locations: locations, inventory: inventorySvc, pendingWarningHours: pendingWarningHours}
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
	return s.repo.List(ctx, params, s.pendingWarningHours)
}

// ListForExport is Phase 22's CSV-export path — see internal/receiving's
// own ListForExport doc for the shared reasoning.
func (s *Service) ListForExport(ctx context.Context, params ListParams) ([]ListResponse, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.List(ctx, params, s.pendingWarningHours)
	return rows, err
}

func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	row, err := s.repo.FindDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	aging := pendingAging(Status(row.Status), row.UpdatedAt, time.Now().UTC(), s.pendingWarningHours)
	return &Response{
		ID: row.ID, AdjustmentNo: row.AdjustmentNo, ProductID: row.ProductID, SKU: row.SKU,
		ProductName: row.ProductName, LocationID: row.LocationID, LocationCode: row.LocationCode,
		LotNo: row.LotNo, AdjustType: AdjustType(row.AdjustType), AdjustQty: row.AdjustQty,
		BeforeQty: row.BeforeQty, AfterQty: row.AfterQty, Reason: Reason(row.Reason), Remark: row.Remark,
		Status: Status(row.Status), SourceStockCountID: row.SourceStockCountID, SourceStockCountNo: row.SourceStockCountNo,
		CreatedBy: row.CreatedBy, CreatedByName: row.CreatedByName,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ApprovedAt: row.ApprovedAt,
		PendingSince: aging.PendingSince, AgeHours: aging.AgeHours, AgeDays: aging.AgeDays,
		IsOverdue: aging.IsOverdue, PendingWarningHours: aging.PendingWarningHours,
	}, nil
}

// FindBySourceCountID exposes Repository.FindBySourceCountID through
// Service — Phase 17's addition so internal/stockcount can list which
// adjustment documents a specific count generated without depending on
// this package's own Repository type directly (the same narrow-
// interface-over-Service pattern every cross-package dependency in this
// codebase already uses).
func (s *Service) FindBySourceCountID(ctx context.Context, countID int64) ([]SourceCountSummary, error) {
	return s.repo.FindBySourceCountID(ctx, countID)
}

// Create validates and snapshots a display-only "before" balance (never
// used to compute the actual mutation — see model.go's doc), then
// persists the header + its one line. No inventory mutation happens
// here regardless of status: DRAFT and PENDING are both inert: only
// Confirm's APPROVE action ever calls AddStockTx/RemoveStockTx.
func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	if err := s.validateItemFields(ctx, req.ProductID, req.LocationID, req.AdjustType, req.AdjustQty, req.Reason); err != nil {
		return nil, err
	}

	status := req.Status
	if status == "" {
		status = StatusDraft
	}
	if status != StatusDraft && status != StatusPending {
		return nil, ErrValidation{Message: "status must be DRAFT or PENDING when creating an adjustment"}
	}

	// A missing balance is not an error here — an INCREASE against a
	// location with no stock yet is valid (first stock ever placed
	// there); before_qty is simply left nil (display-only, see doc).
	var beforeQty *float64
	if bal, err := s.inventory.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo); err == nil {
		q := bal.Quantity
		beforeQty = &q
	} else if !errors.Is(err, inventory.ErrNotFound) {
		return nil, err
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var adjustmentID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		id, err := createTx(ctx, txRepo, req, status, beforeQty, userID)
		adjustmentID = id
		return err
	})
	dbctx.LogTransaction(ctx, "stockadjustment.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, adjustmentID)
}

// CreateTx is Create's core logic (header + one line insert), but
// against a *gorm.DB the caller already has open inside its own
// transaction — added in Phase 16 for internal/stockcount's
// CreateAdjustments, which needs "claim the count + create one or more
// adjustment documents" to commit or roll back together as one atomic
// unit, closing a pre-existing gap where each generated document
// committed independently via its own call to Create. Returns just the
// new adjustment's ID, not a full Response: a Response read-back
// through s.repo's own connection would not see this row until the
// caller's own transaction commits, so the caller must read it back
// itself (via its own already-open transaction, or after commit).
// validateItemFields/status/before_qty are computed the same way Create
// computes them — this does not skip any of Create's own validation.
func (s *Service) CreateTx(ctx context.Context, tx *gorm.DB, req CreateRequest, userID int64) (int64, error) {
	if err := s.validateItemFields(ctx, req.ProductID, req.LocationID, req.AdjustType, req.AdjustQty, req.Reason); err != nil {
		return 0, err
	}

	status := req.Status
	if status == "" {
		status = StatusDraft
	}
	if status != StatusDraft && status != StatusPending {
		return 0, ErrValidation{Message: "status must be DRAFT or PENDING when creating an adjustment"}
	}

	var beforeQty *float64
	if bal, err := s.inventory.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo); err == nil {
		q := bal.Quantity
		beforeQty = &q
	} else if !errors.Is(err, inventory.ErrNotFound) {
		return 0, err
	}

	return createTx(ctx, NewRepository(tx), req, status, beforeQty, userID)
}

// createTx is the shared insert logic both Create (via its own
// transaction) and CreateTx (via the caller's) run against a
// transactional Repository.
func createTx(ctx context.Context, txRepo *Repository, req CreateRequest, status Status, beforeQty *float64, userID int64) (int64, error) {
	adjustment := &Adjustment{Status: status, CreatedBy: &userID, SourceStockCountID: req.SourceStockCountID}
	if err := txRepo.CreateAdjustment(ctx, adjustment); err != nil {
		return 0, err
	}
	if err := txRepo.CreateItem(ctx, &Item{
		AdjustmentID: adjustment.ID, ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
		AdjustType: req.AdjustType, AdjustQty: req.AdjustQty, BeforeQty: beforeQty,
		Reason: req.Reason, Remark: req.Remark,
	}); err != nil {
		return 0, err
	}
	return adjustment.ID, nil
}

// Update edits the one line's fields (and optionally submits DRAFT ->
// PENDING) only while the adjustment is still DRAFT (see Status.CanEdit).
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*Response, error) {
	adjustment, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !adjustment.Status.CanEdit() {
		return nil, ErrNotEditable
	}

	item, err := s.repo.FindItemByAdjustmentID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.validateItemFields(ctx, item.ProductID, item.LocationID, req.AdjustType, req.AdjustQty, req.Reason); err != nil {
		return nil, err
	}

	status := req.Status
	if status == "" {
		status = StatusDraft
	}
	if status != StatusDraft && status != StatusPending {
		return nil, ErrValidation{Message: "status must be DRAFT or PENDING"}
	}

	item.LotNo = req.LotNo
	item.AdjustType = req.AdjustType
	item.AdjustQty = req.AdjustQty
	item.Reason = req.Reason
	item.Remark = req.Remark
	if err := s.repo.UpdateItemFields(ctx, item); err != nil {
		return nil, err
	}

	if status != adjustment.Status {
		adjustment.Status = status
		if err := s.repo.UpdateStatus(ctx, adjustment); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, id)
}

func (s *Service) validateItemFields(ctx context.Context, productID, locationID int64, adjustType AdjustType, adjustQty float64, reason Reason) error {
	if !adjustType.IsValid() {
		return ErrValidation{Message: "adjust_type must be INCREASE or DECREASE"}
	}
	if adjustQty <= 0 {
		return ErrValidation{Message: "adjust_qty must be greater than zero"}
	}
	if !reason.IsValid() {
		return ErrValidation{Message: "reason must be one of Cycle Count Variance, Damaged Goods, Lost Goods, Found Goods, System Correction"}
	}
	if _, err := s.products.FindByID(ctx, productID); err != nil {
		return ErrProductNotFound
	}
	if _, err := s.locations.FindByID(ctx, locationID); err != nil {
		return ErrLocationNotFound
	}
	return nil
}

// Confirm is the approval action: APPROVE calls inventory.AddStockTx
// (INCREASE) or inventory.RemoveStockTx (DECREASE) inside the same
// transaction as the status update and idempotency guard — the one and
// only point in this package's lifecycle where inventory.quantity ever
// changes, and always through Inventory Core's own atomic guarded
// UPDATE, never a direct write. REJECT performs no inventory mutation
// at all. Any failure rolls back the entire transaction.
func (s *Service) Confirm(ctx context.Context, id int64, req ConfirmRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the document exists BEFORE claiming the idempotency key:
		// stock_adjustment_confirmations.stock_adjustment_id is a real FK
		// (migrations/0025), so inserting the guard for a nonexistent id
		// would raise a raw FK-violation 500 instead of a clean 404 — a
		// real bug found live during this phase's own testing, present
		// in the identical ordering Putaway/Picking/Shipping's own
		// Confirm already use (see PROJECT_STATUS.md's Known issues;
		// fixed here, left as a flagged cross-package issue there since
		// touching those packages is out of this phase's scope). This
		// SELECT has no side effect, so moving it ahead of the INSERT
		// does not weaken the idempotency guarantee at all.
		adjustment, err := txRepo.FindByID(ctx, id)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the PRIMARY KEY constraint and everything below is
		// skipped and rolled back — see migrations/0025's doc.
		if err := txRepo.InsertConfirmation(ctx, req.IdempotencyKey, id); err != nil {
			return err
		}

		if !adjustment.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		item, err := txRepo.FindItemByAdjustmentID(ctx, id)
		if err != nil {
			return err
		}

		switch req.Action {
		case "APPROVE":
			invRepo := inventory.NewRepository(tx)
			refType := "STOCK_ADJUSTMENT"
			refID := adjustment.AdjustmentNo
			reason := string(item.Reason)

			var result *inventory.Inventory
			if item.AdjustType == AdjustIncrease {
				result, err = s.inventory.AddStockTx(ctx, invRepo, inventory.StockChangeRequest{
					ProductID: item.ProductID, LocationID: item.LocationID, LotNo: item.LotNo,
					Quantity: item.AdjustQty, Type: inventory.TransactionAdjust,
					ReferenceType: &refType, ReferenceID: &refID, Reason: &reason, CreatedBy: &userID,
				})
			} else {
				result, err = s.inventory.RemoveStockTx(ctx, invRepo, inventory.StockChangeRequest{
					ProductID: item.ProductID, LocationID: item.LocationID, LotNo: item.LotNo,
					Quantity: item.AdjustQty, Type: inventory.TransactionAdjust,
					ReferenceType: &refType, ReferenceID: &refID, Reason: &reason, CreatedBy: &userID,
				})
			}
			if err != nil {
				if errors.Is(err, inventory.ErrInsufficientStock) || errors.Is(err, inventory.ErrNoStock) {
					return ErrInsufficientStock
				}
				return err
			}

			afterQty := result.Quantity
			item.AfterQty = &afterQty
			if err := txRepo.UpdateItemAfterApprove(ctx, item); err != nil {
				return err
			}

			adjustment.Status = StatusApproved
			adjustment.ApprovedBy = &userID
			now := time.Now().UTC()
			adjustment.ApprovedAt = &now

		case "REJECT":
			if req.Remark == "" {
				return ErrValidation{Message: "remark is required to reject an adjustment"}
			}
			remark := req.Remark
			item.Remark = &remark
			if err := txRepo.UpdateItemFields(ctx, item); err != nil {
				return err
			}
			adjustment.Status = StatusRejected

		default:
			return ErrValidation{Message: "action must be one of APPROVE, REJECT"}
		}

		return txRepo.UpdateStatus(ctx, adjustment)
	})
	dbctx.LogTransaction(ctx, "stockadjustment.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second mutation.
		return s.Get(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

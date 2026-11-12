package replenishment

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
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
	ErrWarehouseNotFound = errors.New("replenishment: warehouse not found")
	ErrProductNotFound   = errors.New("replenishment: product not found")
	ErrInvalidTransition = errors.New("replenishment: task is not in a confirmable status")
	ErrNotCancellable    = errors.New("replenishment: task is not in a cancellable status")
	ErrNotStartable      = errors.New("replenishment: task is not in a startable status")
	ErrCannotReportIssue = errors.New("replenishment: task is not in a state that can report an issue")
	ErrCrossWarehouse    = errors.New("replenishment: location must belong to the task's warehouse")
	ErrNoSourceStock     = errors.New("replenishment: no stock found for this product/lot at the source location")
)

// productChecker and warehouseChecker are the narrow slices of the
// Phase 4/3 repositories this package needs — the same narrow-interface
// pattern internal/receiving/internal/picking already established.
type productChecker interface {
	FindByID(ctx context.Context, id int64) (*product.Product, error)
}

type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

// Service holds Replenishment's business rules — the status machine,
// validation, and the Confirm transaction that calls Phase 7's
// inventory.Service.MoveStockTx instead of mutating stock itself. Never
// calls AddStock/RemoveStock directly: a replenishment task only ever
// relocates stock already in the warehouse from one location to
// another.
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

func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	row, err := s.repo.FindTaskDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: row.ID, TaskNo: row.TaskNo, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName,
		FromLocationID: row.FromLocationID, FromLocationCode: row.FromLocationCode,
		ToLocationID: row.ToLocationID, ToLocationCode: row.ToLocationCode,
		LotNo: row.LotNo, Quantity: row.Quantity,
		WarehouseID: row.WarehouseID, WarehouseCode: row.WarehouseCode, Status: Status(row.Status),
		Remark: row.Remark, IssueRemark: row.IssueRemark,
		CreatedBy: row.CreatedBy, CreatedByName: row.CreatedByName,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		StartedAt: row.StartedAt, CompletedAt: row.CompletedAt,
		CancelledAt: row.CancelledAt, CancelledBy: row.CancelledBy, CancelledByName: row.CancelledByName,
		CancelRemark: row.CancelRemark,
	}, nil
}

// Create validates the request and reserves Quantity against
// (ProductID, FromLocationID, LotNo) — Phase 12's own ReserveStockTx
// pattern, applied here so two concurrent replenishment tasks can never
// together claim more than a source location actually has available.
// Never touches ToLocationID's balance: nothing physically moves until
// Confirm.
func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	if req.Quantity <= 0 {
		return nil, ErrValidation{Message: "quantity must be greater than zero"}
	}
	if req.FromLocationID == req.ToLocationID {
		return nil, ErrValidation{Message: "from_location_id and to_location_id must differ"}
	}
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}
	if _, err := s.products.FindByID(ctx, req.ProductID); err != nil {
		return nil, ErrProductNotFound
	}

	fromInfo, err := s.repo.resolveLocation(ctx, req.FromLocationID)
	if err != nil {
		return nil, err
	}
	if fromInfo.WarehouseID != req.WarehouseID {
		return nil, ErrCrossWarehouse
	}
	toInfo, err := s.repo.resolveLocation(ctx, req.ToLocationID)
	if err != nil {
		return nil, err
	}
	if toInfo.WarehouseID != req.WarehouseID {
		return nil, ErrCrossWarehouse
	}
	if fromInfo.Status != "ACTIVE" || toInfo.Status != "ACTIVE" {
		return nil, ErrValidation{Message: "both locations must be active"}
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var taskID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		invRepo := inventory.NewRepository(tx)

		// Reserve first: if the source doesn't have enough available
		// stock, nothing is created — the same ordering Picking's own
		// createItem uses (reserve, then persist the row only once the
		// reservation succeeded).
		if _, err := s.inventory.ReserveStockTx(ctx, invRepo, inventory.ReserveRequest{
			ProductID: req.ProductID, LocationID: req.FromLocationID, LotNo: req.LotNo, Quantity: req.Quantity,
		}); err != nil {
			if errors.Is(err, inventory.ErrNoStock) {
				return ErrNoSourceStock
			}
			if errors.Is(err, inventory.ErrInsufficientStock) {
				return ErrValidation{Message: "insufficient available stock to reserve this quantity"}
			}
			return err
		}

		task := &Task{
			ProductID: req.ProductID, FromLocationID: req.FromLocationID, ToLocationID: req.ToLocationID,
			LotNo: req.LotNo, Quantity: req.Quantity, WarehouseID: req.WarehouseID,
			Status: StatusPending, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateTask(ctx, task); err != nil {
			return err
		}
		taskID = task.ID
		return nil
	})
	dbctx.LogTransaction(ctx, "replenishment.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

// Start moves a task from PENDING to IN_PROGRESS — an optional signal
// ("a person is now actively working this") with no inventory effect
// at all, matching this phase's own "no complex optimizer" simplicity:
// Confirm accepts PENDING or IN_PROGRESS equally, so Start is never a
// gate, only a status the frontend can display.
func (s *Service) Start(ctx context.Context, taskID int64) (*Response, error) {
	task, err := s.repo.FindTaskByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !task.Status.CanStart() {
		return nil, ErrNotStartable
	}
	now := time.Now().UTC()
	task.Status = StatusInProgress
	task.StartedAt = &now
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

// Confirm is the atomic core of this phase: validate -> release the
// reservation placed at Create time -> inventory.MoveStockTx (same
// transaction, both ledger rows written by MoveStockTx itself) ->
// COMPLETED. Idempotent via the same InsertConfirmation-as-first-
// statement pattern every other workflow package uses.
func (s *Service) Confirm(ctx context.Context, taskID int64, req ConfirmRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		task, err := txRepo.FindTaskByID(ctx, taskID)
		if err != nil {
			return err
		}

		if err := txRepo.InsertConfirmation(ctx, req.IdempotencyKey, taskID); err != nil {
			return err
		}

		if !task.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		invRepo := inventory.NewRepository(tx)

		// Release this task's own reservation before moving — the
		// reservation was only ever a claim against the source location,
		// not a partial move; MoveStockTx below performs the real
		// quantity/ledger change. Mirrors Picking's own
		// ConsumeReservationTx composition (release, then act), split
		// into two calls here since MoveStockTx (unlike RemoveStockTx)
		// has no built-in reservation-consuming variant.
		if _, err := s.inventory.ReleaseReservationTx(ctx, invRepo, inventory.ReserveRequest{
			ProductID: task.ProductID, LocationID: task.FromLocationID, LotNo: task.LotNo, Quantity: task.Quantity,
		}); err != nil {
			if errors.Is(err, inventory.ErrReservationUnderflow) || errors.Is(err, inventory.ErrNoStock) {
				return ErrValidation{Message: "reservation no longer matches current stock — cannot confirm"}
			}
			return err
		}

		refType := "REPLENISHMENT"
		refID := task.TaskNo
		if err := s.inventory.MoveStockTx(ctx, invRepo, inventory.MoveStockRequest{
			ProductID: task.ProductID, FromLocationID: task.FromLocationID, ToLocationID: task.ToLocationID,
			LotNo: task.LotNo, Quantity: task.Quantity, Type: inventory.TransactionMove,
			ReferenceType: &refType, ReferenceID: &refID, CreatedBy: &userID,
		}); err != nil {
			if errors.Is(err, inventory.ErrInsufficientStock) || errors.Is(err, inventory.ErrNoStock) {
				return ErrValidation{Message: "insufficient stock at source location to complete this move"}
			}
			return err
		}

		now := time.Now().UTC()
		task.Status = StatusCompleted
		task.CompletedAt = &now
		return txRepo.UpdateTask(ctx, task)
	})
	dbctx.LogTransaction(ctx, "replenishment.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		return s.Get(ctx, taskID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

// Cancel releases whatever reservation Create placed and marks the task
// CANCELLED. Never calls MoveStockTx and never writes an
// inventory_transactions row — cancellation is the mirror image of
// Create's ReserveStockTx call, matching Picking's own Phase 13
// "cancellation is never a stock movement" principle exactly.
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
			return ErrNotCancellable
		}

		invRepo := inventory.NewRepository(tx)
		if _, err := s.inventory.ReleaseReservationTx(ctx, invRepo, inventory.ReserveRequest{
			ProductID: task.ProductID, LocationID: task.FromLocationID, LotNo: task.LotNo, Quantity: task.Quantity,
		}); err != nil {
			return err
		}

		task.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelTask(ctx, task)
	})
	dbctx.LogTransaction(ctx, "replenishment.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

// ReportIssue flags a real-world problem (damaged stock, location
// mismatch, ...) that prevents the move from ever completing. Exactly
// like Cancel, it only releases the reservation — never calls
// MoveStockTx, never mutates a balance or writes a ledger row. Distinct
// from Cancel only in the status it lands on and that a remark is
// mandatory, so staff always know *why* a flagged task never completed.
func (s *Service) ReportIssue(ctx context.Context, taskID int64, req IssueRequest) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		task, err := txRepo.FindTaskByID(ctx, taskID)
		if err != nil {
			return err
		}
		if !task.Status.CanReportIssue() {
			return ErrCannotReportIssue
		}

		invRepo := inventory.NewRepository(tx)
		if _, err := s.inventory.ReleaseReservationTx(ctx, invRepo, inventory.ReserveRequest{
			ProductID: task.ProductID, LocationID: task.FromLocationID, LotNo: task.LotNo, Quantity: task.Quantity,
		}); err != nil {
			return err
		}

		remark := req.Remark
		task.Status = StatusIssue
		task.IssueRemark = &remark
		return txRepo.UpdateTask(ctx, task)
	})
	dbctx.LogTransaction(ctx, "replenishment.report_issue", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, taskID)
}

// Recommend is this phase's "simple generate/recommend endpoint — no
// complex optimizer": for every product/location balance in the given
// warehouse, a location whose available_qty has fallen to or below
// that product's own min_stock (the exact same threshold
// internal/inventory's own Status derivation already uses for
// LOW_STOCK — see its dto.go doc — reused here, not reinvented) is a
// candidate destination. Its suggested source is simply the same
// product's OTHER location in the same warehouse currently holding the
// most available stock (ListForRecommend's own ORDER BY does this
// sort), capped so the suggestion never exceeds either the
// destination's room up to max_stock or the source's own available
// quantity. Deliberately advisory only: two recommendations may name
// the same source location (this pass never reserves anything or
// tracks what an earlier recommendation in the same response "already
// claimed") — the real availability check only happens for real, once
// a recommendation is turned into a Create call, via ReserveStockTx's
// own guarded UPDATE. A location with no inventory row at all (a pick
// face that has never held this product) is invisible to this query
// and can never be recommended — documented as a known limitation
// rather than solved by inventing a product-to-pick-location mapping
// this schema doesn't have.
func (s *Service) Recommend(ctx context.Context, warehouseID int64) ([]RecommendationResponse, error) {
	if _, err := s.warehouses.FindByID(ctx, warehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}

	rows, err := s.repo.ListForRecommend(ctx, warehouseID)
	if err != nil {
		return nil, err
	}
	return computeRecommendations(rows), nil
}

// computeRecommendations is Recommend's pure core, extracted so the
// algorithm can be unit tested without a database — the same "extract
// the pure part" precedent Phase 18/20/21 each already established for
// their own applyCancel/pendingAging/buildReminderNotificationText.
// rows is assumed sorted by (product_id, available_qty DESC) exactly
// as Repository.ListForRecommend's own ORDER BY produces.
func computeRecommendations(rows []recommendRow) []RecommendationResponse {
	groups := make(map[int64][]recommendRow)
	var order []int64
	for _, r := range rows {
		if _, seen := groups[r.ProductID]; !seen {
			order = append(order, r.ProductID)
		}
		groups[r.ProductID] = append(groups[r.ProductID], r)
	}

	var recs []RecommendationResponse
	for _, productID := range order {
		productRows := groups[productID]
		for _, low := range productRows {
			if low.AvailableQty > low.MinStock {
				continue
			}
			var surplus *recommendRow
			for i := range productRows {
				cand := productRows[i]
				if cand.LocationID == low.LocationID {
					continue
				}
				if cand.AvailableQty <= 0 {
					continue
				}
				surplus = &cand
				break // productRows is sorted desc by available_qty, so the first match is the largest.
			}
			if surplus == nil {
				continue
			}

			suggested := low.MaxStock - low.AvailableQty
			if suggested > surplus.AvailableQty {
				suggested = surplus.AvailableQty
			}
			if suggested <= 0 {
				continue
			}

			recs = append(recs, RecommendationResponse{
				ProductID: low.ProductID, SKU: low.SKU, ProductName: low.ProductName,
				FromLocationID: surplus.LocationID, FromLocationCode: surplus.LocationCode,
				ToLocationID: low.LocationID, ToLocationCode: low.LocationCode,
				LotNo: surplus.LotNo, SuggestedQty: suggested,
			})
		}
	}
	return recs
}

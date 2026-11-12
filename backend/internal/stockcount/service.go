package stockcount

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/product"
	"wms-backend/internal/stockadjustment"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrWarehouseNotFound = errors.New("stockcount: warehouse not found")
	ErrZoneNotFound      = errors.New("stockcount: zone not found")
	ErrProductNotFound   = errors.New("stockcount: product not found")
	ErrLocationNotFound  = errors.New("stockcount: location not found")
	ErrNotEditable       = errors.New("stockcount: count can no longer be edited")
	ErrInvalidTransition = errors.New("stockcount: count is not in a confirmable status")
	ErrInsufficientStock = errors.New("stockcount: insufficient stock to apply this negative variance")
	// ErrStaleCount is returned by Confirm when a line's live inventory
	// balance no longer matches the quantity frozen when the line was
	// added — see Confirm's doc for why this is checked instead of
	// silently applying a possibly-wrong variance.
	ErrStaleCount = errors.New("stockcount: inventory changed since this line was counted; recount required")
	// ErrVarianceAlreadyApplied is Phase 16's addition — returned by both
	// Confirm and CreateAdjustments when Repository.ClaimStatus's guarded
	// UPDATE affects 0 rows, meaning this count's variance has already
	// been claimed by whichever of the two paths got there first (or,
	// for a nonexistent id already caught by the earlier FindByID call,
	// never reached at all). See Status's own doc for the full design.
	ErrVarianceAlreadyApplied = errors.New("stockcount: this count's variance has already been applied via confirm or create-adjustments")
)

// productChecker, locationChecker, warehouseChecker, and zoneChecker are
// the narrow slices of the Phase 3/4 repositories this package needs —
// the same narrow-interface pattern every prior phase established.
type productChecker interface {
	FindByID(ctx context.Context, id int64) (*product.Product, error)
}

type locationChecker interface {
	FindByID(ctx context.Context, id int64) (*location.Location, error)
}

type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

type zoneChecker interface {
	FindByID(ctx context.Context, id int64) (*zone.Zone, error)
}

// adjustmentCreator is the narrow slice of internal/stockadjustment's
// own Service this package needs for "Create Adjustment" — the mock's
// alternate path to Confirm Count (see CreateAdjustments' doc). Mirrors
// the cross-package narrow-interface pattern internal/putaway already
// established for internal/receiving. CreateTx was added in Phase 16 so
// CreateAdjustments can create every generated document inside its own
// transaction (see that method's doc) instead of each one committing
// independently via Create. FindBySourceCountID was added in Phase 17
// for GeneratedAdjustments (see Service.Get's doc).
type adjustmentCreator interface {
	Create(ctx context.Context, req stockadjustment.CreateRequest, userID int64) (*stockadjustment.Response, error)
	CreateTx(ctx context.Context, tx *gorm.DB, req stockadjustment.CreateRequest, userID int64) (int64, error)
	FindBySourceCountID(ctx context.Context, countID int64) ([]stockadjustment.SourceCountSummary, error)
}

// Service holds the Stock Count workflow's business rules — snapshot-
// on-add, record-on-scan, and the Confirm transaction that applies each
// line's variance through Phase 5's inventory.Service.AddStockTx/
// RemoveStockTx instead of ever overwriting inventory.quantity with the
// counted value directly.
type Service struct {
	repo        *Repository
	warehouses  warehouseChecker
	zones       zoneChecker
	products    productChecker
	locations   locationChecker
	inventory   *inventory.Service
	adjustments adjustmentCreator
}

func NewService(repo *Repository, warehouses warehouseChecker, zones zoneChecker, products productChecker, locations locationChecker, inventorySvc *inventory.Service, adjustments adjustmentCreator) *Service {
	return &Service{
		repo: repo, warehouses: warehouses, zones: zones, products: products, locations: locations,
		inventory: inventorySvc, adjustments: adjustments,
	}
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

// Get is GET /stock-counts/{id}. GeneratedAdjustments (Phase 17) is
// always populated fresh from internal/stockadjustment's own table via
// FindBySourceCountID — never stored or duplicated on this package's
// own rows — so it always reflects reality exactly: empty for a count
// that was Confirmed directly (Confirm never creates an adjustment) or
// hasn't been acted on yet, and populated only once Create Adjustment
// has actually run for this count. Since that path is now claimed
// exactly once per count (Phase 16's ClaimStatus), this can never show
// a stale or duplicated set of documents.
func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	header, err := s.repo.FindDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	generated, err := s.adjustments.FindBySourceCountID(ctx, id)
	if err != nil {
		return nil, err
	}
	generatedResp := make([]GeneratedAdjustmentResponse, 0, len(generated))
	for _, g := range generated {
		generatedResp = append(generatedResp, GeneratedAdjustmentResponse{
			ID: g.ID, AdjustmentNo: g.AdjustmentNo, Status: string(g.Status), Reason: string(g.Reason), CreatedAt: g.CreatedAt,
		})
	}
	return &Response{
		ID: header.ID, CountNo: header.CountNo, CountType: CountType(header.CountType),
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode,
		ZoneID: header.ZoneID, ZoneCode: header.ZoneCode, CountDate: header.CountDate,
		Status: Status(header.Status), CreatedBy: header.CreatedBy, CreatedByName: header.CreatedByName,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
		ConfirmedAt: header.ConfirmedAt, Items: items, GeneratedAdjustments: generatedResp,
	}, nil
}

// Create makes "+ New Count Task" real: validates the header, then adds
// any items supplied up front (each one snapshotted the same way
// AddItem does). A count with zero items is valid — "+ Add Item"/the
// scan bar can populate it afterward, matching the mock's own
// mounted()-populated-then-editable table.
func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	if !req.CountType.IsValid() {
		return nil, ErrValidation{Message: "count_type must be one of CYCLE_COUNT, FULL_COUNT, LOCATION_COUNT, SKU_COUNT"}
	}
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}
	if req.ZoneID != nil {
		if _, err := s.zones.FindByID(ctx, *req.ZoneID); err != nil {
			return nil, ErrZoneNotFound
		}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var countID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		count := &Count{
			CountType: req.CountType, WarehouseID: req.WarehouseID, ZoneID: req.ZoneID,
			CountDate: req.CountDate, Status: StatusDraft, CreatedBy: &userID,
		}
		if err := txRepo.CreateCount(ctx, count); err != nil {
			return err
		}
		countID = count.ID

		for _, itemReq := range req.Items {
			if err := s.addItem(ctx, txRepo, count.ID, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "stockcount.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, countID)
}

// AddItem is POST /stock-counts/{id}/items — the real backing for
// "+ Add Item" and, once a matching line exists, the scan bar's own
// lookup. Only allowed while the count is still DRAFT.
func (s *Service) AddItem(ctx context.Context, countID int64, req CreateItemRequest) (*Response, error) {
	count, err := s.repo.FindByID(ctx, countID)
	if err != nil {
		return nil, err
	}
	if !count.Status.CanEdit() {
		return nil, ErrNotEditable
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		return s.addItem(ctx, txRepo, countID, req)
	})
	dbctx.LogTransaction(ctx, "stockcount.add_item", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, countID)
}

// addItem snapshots the line's expected quantity from Inventory Core's
// LIVE balance at this exact moment, then freezes it forever in
// system_qty (see model.go's doc — it is never refreshed after this
// point, by construction: nothing in this package ever writes to that
// column again). A missing balance is not an error — it simply means
// zero is the expected quantity (a legitimate count scenario: verifying
// an empty bin actually has nothing in it).
func (s *Service) addItem(ctx context.Context, txRepo *Repository, countID int64, req CreateItemRequest) error {
	p, err := s.products.FindByID(ctx, req.ProductID)
	if err != nil {
		return ErrProductNotFound
	}
	if _, err := s.locations.FindByID(ctx, req.LocationID); err != nil {
		return ErrLocationNotFound
	}

	systemQty := 0.0
	if bal, err := s.inventory.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo); err == nil {
		systemQty = bal.Quantity
	} else if !errors.Is(err, inventory.ErrNotFound) {
		return err
	}

	uom := req.UOM
	if uom == "" {
		uom = string(p.BaseUOM)
	}

	return txRepo.CreateItem(ctx, &Item{
		CountID: countID, ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
		UOM: uom, SystemQty: systemQty, Condition: ConditionGood, Status: ItemStatusPending,
	})
}

// RecordCount is PUT /stock-counts/{id}/items/{itemId} — the scan bar's
// "Confirm Count" action and the table's own editable Count Qty/
// Condition/Remark cells both call this. Computes and stores
// VarianceQty immediately (CountQty - the line's frozen SystemQty) so
// it is traceable without being recomputed later — see model.go's doc.
// Only allowed while the count is still DRAFT.
func (s *Service) RecordCount(ctx context.Context, countID, itemID int64, req RecordCountRequest) (*Response, error) {
	count, err := s.repo.FindByID(ctx, countID)
	if err != nil {
		return nil, err
	}
	if !count.Status.CanEdit() {
		return nil, ErrNotEditable
	}

	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.CountID != countID {
		return nil, ErrItemNotFound
	}

	condition := req.Condition
	if condition == "" {
		condition = ConditionGood
	}
	if !condition.IsValid() {
		return nil, ErrValidation{Message: "condition must be one of GOOD, DAMAGED, HOLD"}
	}

	status := req.Status
	if status == "" {
		status = ItemStatusCounted
	}
	if !status.IsValid() {
		return nil, ErrValidation{Message: "status must be one of PENDING, COUNTED, RECOUNT"}
	}

	countQty := req.CountQty
	variance := countQty - item.SystemQty

	item.CountQty = &countQty
	item.VarianceQty = &variance
	item.Condition = condition
	item.Status = status
	item.Remark = req.Remark

	if err := s.repo.RecordCount(ctx, item); err != nil {
		return nil, err
	}
	return s.Get(ctx, countID)
}

// Confirm is "Confirm Count" — the atomic core of this phase: validate
// every line has actually been counted -> for each line, re-check its
// LIVE inventory balance still matches the SystemQty frozen when it was
// added (a stale-count guard: if someone else moved stock at that
// location after this count started, the variance this count computed
// is no longer trustworthy) -> apply the variance through Inventory
// Core (AddStockTx if positive, RemoveStockTx if negative, skipped
// entirely if zero) -> mark the count CONFIRMED. Any failure —
// including a stale line found partway through — rolls back everything
// in this call, including lines already applied earlier in the same
// call. Never writes inventory.quantity directly.
func (s *Service) Confirm(ctx context.Context, countID int64, req ConfirmRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the count exists BEFORE claiming the idempotency key:
		// stock_count_confirmations.stock_count_id is a real FK
		// (migrations/0028), so inserting the guard for a nonexistent id
		// would raise a raw FK-violation 500 instead of a clean 404 — the
		// same real bug found live in internal/stockadjustment's own
		// Confirm during this phase's testing (see that package's doc and
		// PROJECT_STATUS.md's Known issues for the cross-package scope).
		// This SELECT has no side effect, so moving it ahead of the
		// INSERT does not weaken the idempotency guarantee at all.
		count, err := txRepo.FindByID(ctx, countID)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the PRIMARY KEY constraint and everything below is
		// skipped and rolled back — see migrations/0028's doc.
		if err := txRepo.InsertConfirmation(ctx, req.IdempotencyKey, countID); err != nil {
			return err
		}

		// Claim this count for the Confirm path — Phase 16's guarded
		// UPDATE (see ClaimStatus's own doc), not a plain
		// count.Status.CanConfirm() check followed by a blind write: 0
		// rows affected means the count is already CONFIRMED, already
		// ADJUSTMENT_CREATED (Create Adjustment already ran for it), or a
		// concurrent Confirm call with a different idempotency key won
		// the race to claim it first. Any of those is correctly reported
		// as ErrInvalidTransition — from the caller's perspective, this
		// count simply isn't confirmable right now.
		rows, err := txRepo.ClaimStatus(ctx, countID, StatusConfirmed)
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrInvalidTransition
		}

		items, err := txRepo.FindItemsByCount(ctx, countID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return ErrValidation{Message: "at least one item is required to confirm a count"}
		}
		for _, it := range items {
			if it.Status == ItemStatusPending {
				return ErrValidation{Message: "some items are still pending count"}
			}
		}

		invRepo := inventory.NewRepository(tx)
		refType := "STOCK_COUNT"
		refID := count.CountNo

		for _, it := range items {
			if it.CountQty == nil {
				continue
			}

			// Stale-count guard: re-read the CURRENT balance, inside this
			// same transaction, immediately before deciding whether to
			// mutate it — not a substitute for AddStockTx/RemoveStockTx's
			// own atomic guarded UPDATE (which remains the sole authority
			// against negative stock), but a business-rule check that a
			// drifted balance shouldn't have a now-stale variance silently
			// applied on top of it. See ErrStaleCount's doc.
			currentQty := 0.0
			if bal, err := s.inventory.FindBalance(ctx, it.ProductID, it.LocationID, it.LotNo); err == nil {
				currentQty = bal.Quantity
			} else if !errors.Is(err, inventory.ErrNotFound) {
				return err
			}
			if currentQty != it.SystemQty {
				return fmt.Errorf("%w: item %d (expected %.4f, now %.4f)", ErrStaleCount, it.ID, it.SystemQty, currentQty)
			}

			variance := *it.CountQty - it.SystemQty
			if variance == 0 {
				continue
			}

			reason := "Stock count variance"
			var mutErr error
			if variance > 0 {
				_, mutErr = s.inventory.AddStockTx(ctx, invRepo, inventory.StockChangeRequest{
					ProductID: it.ProductID, LocationID: it.LocationID, LotNo: it.LotNo,
					Quantity: variance, Type: inventory.TransactionCount,
					ReferenceType: &refType, ReferenceID: &refID, Reason: &reason, CreatedBy: &userID,
				})
			} else {
				_, mutErr = s.inventory.RemoveStockTx(ctx, invRepo, inventory.StockChangeRequest{
					ProductID: it.ProductID, LocationID: it.LocationID, LotNo: it.LotNo,
					Quantity: -variance, Type: inventory.TransactionCount,
					ReferenceType: &refType, ReferenceID: &refID, Reason: &reason, CreatedBy: &userID,
				})
			}
			if mutErr != nil {
				if errors.Is(mutErr, inventory.ErrInsufficientStock) || errors.Is(mutErr, inventory.ErrNoStock) {
					return ErrInsufficientStock
				}
				return mutErr
			}
		}

		now := time.Now().UTC()
		count.Status = StatusConfirmed
		count.ConfirmedAt = &now
		return txRepo.UpdateStatus(ctx, count)
	})
	dbctx.LogTransaction(ctx, "stockcount.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second mutation.
		return s.Get(ctx, countID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, countID)
}

// CreateAdjustments is "Create Adjustment" — the mock's alternate path
// to Confirm Count, hardened in Phase 16 to be mutually exclusive with
// it. Generates one real internal/stockadjustment PENDING document per
// non-zero-variance line (reason "Cycle Count Variance") inside the
// SAME transaction as the count's own status claim, via CreateTx rather
// than Create — closing a pre-existing atomicity gap where each
// generated document committed independently, so a failure partway
// through could leave a partial set of documents with no rollback.
// Before generating anything, claims the count for this path via the
// same guarded Repository.ClaimStatus UPDATE Confirm uses (DRAFT ->
// ADJUSTMENT_CREATED): 0 rows affected means the count is already
// CONFIRMED (Confirm already applied this variance directly), already
// ADJUSTMENT_CREATED (this exact call already ran once), or a
// concurrent call won the race — any of those correctly refuses with
// ErrVarianceAlreadyApplied rather than generating a second set of
// documents for the same variance. Neither this method nor the created
// documents mutate inventory by themselves — only
// internal/stockadjustment's own Confirm (APPROVE action) does, at
// whatever later point someone approves them, exactly as if they had
// been created by hand.
func (s *Service) CreateAdjustments(ctx context.Context, countID int64, userID int64) (*CreateAdjustmentsResponse, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var ids []int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		count, err := txRepo.FindByID(ctx, countID)
		if err != nil {
			return err
		}

		items, err := txRepo.FindItemsByCount(ctx, countID)
		if err != nil {
			return err
		}
		hasVariance := false
		for _, it := range items {
			if it.VarianceQty != nil && *it.VarianceQty != 0 {
				hasVariance = true
				break
			}
		}
		if !hasVariance {
			return ErrValidation{Message: "no variance found"}
		}

		rows, err := txRepo.ClaimStatus(ctx, countID, StatusAdjustmentCreated)
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrVarianceAlreadyApplied
		}

		for _, it := range items {
			if it.VarianceQty == nil || *it.VarianceQty == 0 {
				continue
			}

			adjustType := stockadjustment.AdjustIncrease
			qty := *it.VarianceQty
			if qty < 0 {
				adjustType = stockadjustment.AdjustDecrease
				qty = -qty
			}
			remark := fmt.Sprintf("Generated from stock count %s", count.CountNo)

			id, err := s.adjustments.CreateTx(ctx, tx, stockadjustment.CreateRequest{
				ProductID: it.ProductID, LocationID: it.LocationID, LotNo: it.LotNo,
				AdjustType: adjustType, AdjustQty: qty, Reason: stockadjustment.ReasonCycleCountVariance,
				Remark: &remark, Status: stockadjustment.StatusPending,
				SourceStockCountID: &countID,
			}, userID)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "stockcount.create_adjustments", start, err)
	if err != nil {
		return nil, err
	}
	return &CreateAdjustmentsResponse{AdjustmentIDs: ids}, nil
}

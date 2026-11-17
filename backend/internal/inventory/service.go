package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/platform/database/sqlerr"
	"wms-backend/internal/platform/dbctx"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// ErrInsufficientStock is returned by RemoveStock/MoveStock when the
// requested quantity exceeds what's available at that (product,
// location, lot) — the database CHECK constraint would also refuse the
// write, but AdjustBalance's guarded UPDATE catches it first and lets
// the service return a specific, actionable error instead of a raw
// constraint-violation message.
var ErrInsufficientStock = errors.New("inventory: insufficient stock")

// ErrNoStock is returned by RemoveStock/MoveStock when there is no
// balance row at all for the requested (product, location, lot) —
// distinct from ErrInsufficientStock (row exists, just not enough).
var ErrNoStock = errors.New("inventory: no stock at this location")

// ErrReservationUnderflow is returned by ReleaseReservationTx/
// ConsumeReservationTx when Quantity exceeds what's currently reserved
// at that (product, location, lot) — AdjustReservation's guarded UPDATE
// catches it first, the same way AdjustBalance catches
// ErrInsufficientStock.
var ErrReservationUnderflow = errors.New("inventory: cannot release or consume more than is currently reserved")

// ErrInsufficientAvailableStock is returned by MoveStockAvailableTx when
// Quantity exceeds what's currently AVAILABLE (Quantity - Reserved -
// Hold) at the source tuple — distinct from ErrInsufficientStock, which
// only guards against exceeding raw on-hand Quantity and would
// otherwise let a move dip into reserved or QC-held units. See
// AdjustAvailableBalance's own doc.
var ErrInsufficientAvailableStock = errors.New("inventory: cannot move more than is currently available (excludes reserved/held quantity)")

// ErrHoldUnderflow is returned by ReleaseHoldTx/RejectHoldTx (Phase 31B)
// when Quantity exceeds what's currently held at that (product,
// location, lot) — AdjustHold's guarded UPDATE catches it first, the
// same mechanism ErrReservationUnderflow above uses for reservations.
// This is the authoritative guard against QC releasing or rejecting
// more than inventory.HoldQuantity actually has, regardless of what the
// caller's own bookkeeping believes.
var ErrHoldUnderflow = errors.New("inventory: cannot release or reject more than is currently held")

// ErrProductNotFound and ErrLocationNotFound (Phase 30 Batch 1) are
// returned by verifyOwnership when ProductID/LocationID either doesn't
// exist at all or exists under a different tenant — the two cases are
// deliberately indistinguishable (tenant-isolation-security.md §6),
// the same "existence checker, not ownership checker" gap
// phase27-audit-report.md §3(c) flagged project-wide, now closed for
// every mutation entry point in this package.
var ErrProductNotFound = errors.New("inventory: product not found")
var ErrLocationNotFound = errors.New("inventory: location not found")

// UnverifiedCallerTenantID (Phase 30 Batch 1) is the explicit,
// greppable placeholder every caller that has no verified per-request
// tenant context of its own yet must pass until its own Phase 30 batch
// lands (Picking: Batch 2, Order: Batch 3, Shipping: Batch 4; Receiving/
// Putaway/StockAdjustment/StockCount/Replenishment/Dashboard are not
// yet scheduled). It equals the bootstrap tenant (migrations/0046-0047's
// own DEFAULT(1)) so every existing single-tenant installation keeps
// working unchanged — this constant changes NOTHING about today's
// behavior, it only makes every remaining unverified call site
// searchable (`grep -rn UnverifiedCallerTenantID`) instead of a bare
// magic literal. It must never be read from a request body or any
// client-supplied value — every site that sets it today is Go code
// constructing a request struct, never json.Unmarshal.
const UnverifiedCallerTenantID int64 = 1

// productChecker and locationChecker are the narrow ownership-check
// interfaces this package needs. Both are intentionally primitive-typed
// (no internal/product or internal/location import) — internal/location
// already imports internal/inventory (for CountByLocation), so this
// package importing internal/location's types back would be a cycle;
// internal/product avoids the same risk for symmetry and because it
// needs nothing more than an Exists call either.
type productChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

type locationChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

// ReserveRequest is the input to ReserveStockTx/ReleaseReservationTx —
// deliberately narrower than StockChangeRequest: a reservation never
// writes a ledger row (it isn't a stock movement, just a claim against
// an existing balance), so it carries no Type/ReferenceType/Reason/
// CreatedBy. TenantID (Phase 30 Batch 1) travels on the request struct,
// not as a separate Tx-method parameter, so every existing caller's
// call-site change is "add one field to a struct literal," not a
// function-signature/arity change that would ripple into their own
// transaction-opening code.
type ReserveRequest struct {
	TenantID   int64
	ProductID  int64
	LocationID int64
	LotNo      string
	Quantity   float64
}

// StockChangeRequest is the input to AddStock/RemoveStock. Quantity is
// always a positive magnitude — direction is implied by which method is
// called, not by the sign of Quantity. TenantID (Phase 30 Batch 1): see
// ReserveRequest's doc.
type StockChangeRequest struct {
	TenantID   int64
	ProductID  int64
	LocationID int64
	LotNo      string
	ExpiryDate *time.Time
	Quantity   float64
	// HoldQuantity (Phase 31A) is the portion of Quantity to post into
	// inventory.hold_quantity instead of leaving it immediately
	// available — only meaningful on AddStock/AddStockTx; RemoveStock
	// ignores it. Must be 0 <= HoldQuantity <= Quantity (see
	// validateStockChange). Zero (the default) preserves every existing
	// caller's exact pre-Phase-31A behavior.
	HoldQuantity  float64
	Type          TransactionType
	ReferenceType *string
	ReferenceID   *string
	Reason        *string
	CreatedBy     *int64
}

// MoveStockRequest is the input to MoveStock/MoveStockTx. ExpiryDate,
// Type, ReferenceType, and ReferenceID are optional — added in Phase 7
// so a caller (internal/putaway) can supply the moved lot's authoritative
// expiry (read from the source balance row, not user input) and a
// PUTAWAY-specific transaction type/reference instead of the generic
// defaults below. Any existing caller that leaves them zero-valued gets
// MoveStock's original MOVE-typed, auto-generated-reference behavior
// unchanged. TenantID (Phase 30 Batch 1): see ReserveRequest's doc.
type MoveStockRequest struct {
	TenantID       int64
	ProductID      int64
	FromLocationID int64
	ToLocationID   int64
	LotNo          string
	ExpiryDate     *time.Time
	Quantity       float64
	Type           TransactionType
	ReferenceType  *string
	ReferenceID    *string
	Reason         *string
	CreatedBy      *int64
}

// Service is the reusable inventory foundation: read-side queries for
// the Stock Inquiry screen, plus AddStock/RemoveStock/MoveStock — the
// only sanctioned way any part of this codebase changes a stock
// balance.
type Service struct {
	repo      *Repository
	products  productChecker
	locations locationChecker
}

func NewService(repo *Repository, products productChecker, locations locationChecker) *Service {
	return &Service{repo: repo, products: products, locations: locations}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Response, int64, error) {
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

// ListForExport is Phase 22's CSV-export path for the Inventory Stock
// Report — see internal/receiving's own ListForExport doc for the
// shared reasoning.
func (s *Service) ListForExport(ctx context.Context, tenantID int64, params ListParams) ([]Response, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.List(ctx, tenantID, params)
	return rows, err
}

// ListTransactionsForExport backs the async Inventory Transaction
// History export (internal/report's worker calls this) — see
// TransactionExportRow's own doc. tenantID is the export_jobs row's own
// (already-stamped) TenantID, supplied by the caller.
func (s *Service) ListTransactionsForExport(ctx context.Context, tenantID int64, filters TransactionExportFilters) ([]TransactionExportRow, error) {
	return s.repo.ListTransactionsForExport(ctx, tenantID, filters)
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	return s.repo.FindByID(ctx, tenantID, id)
}

func (s *Service) Summary(ctx context.Context, tenantID int64) (SummaryResponse, error) {
	return s.repo.Summary(ctx, tenantID)
}

func (s *Service) Transactions(ctx context.Context, tenantID, inventoryID int64) ([]TransactionResponse, error) {
	return s.repo.Transactions(ctx, tenantID, inventoryID, 50)
}

// verifyOwnership confirms productID and locationID both belong to
// tenantID before any mutation proceeds — the missing half of every
// "existence checker, not ownership checker" call site
// phase27-audit-report.md §3(c) found project-wide. Called at the start
// of every mutation path (addStockTx/removeStockTx/ReserveStockTx/
// ReleaseReservationTx/ConsumeReservationTx) so no caller can reach
// AdjustBalance/AdjustReservation with a cross-tenant product_id or
// location_id, regardless of what the caller itself has or hasn't
// verified.
func (s *Service) verifyOwnership(ctx context.Context, tenantID, productID, locationID int64) error {
	if ok, err := s.products.Exists(ctx, tenantID, productID); err != nil {
		return err
	} else if !ok {
		return ErrProductNotFound
	}
	if ok, err := s.locations.Exists(ctx, tenantID, locationID); err != nil {
		return err
	} else if !ok {
		return ErrLocationNotFound
	}
	return nil
}

// AddStock increases the balance at (ProductID, LocationID, LotNo) by
// Quantity, creating the balance row if this is the first stock ever
// placed there, and appends a matching ledger row — both in one
// database transaction.
func (s *Service) AddStock(ctx context.Context, req StockChangeRequest) (*Inventory, error) {
	if err := validateStockChange(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}

	// Detach here, not at the top of the method: validation above has no
	// durable side effect yet and is fine to abort with the caller: see
	// internal/platform/dbctx's doc for the cancellation-semantics policy.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var result *Inventory
	start := time.Now()
	err := s.repo.Transaction(ctx, func(txRepo *Repository) error {
		inv, err := addStockTx(ctx, txRepo, req)
		if err != nil {
			return err
		}
		result = inv
		return nil
	})
	dbctx.LogTransaction(ctx, "inventory.add_stock", start, err)
	return result, err
}

// AddStockTx is AddStock's exact logic, but against a Repository the
// caller already has open inside its own database transaction (e.g.
// internal/receiving's Confirm, which needs "update receipt line +
// AddStock + insert ledger row" to commit or roll back together as one
// unit) — added in Phase 6 so that caller can compose this instead of
// duplicating the balance/ledger-write logic or opening a second,
// disconnected transaction. AddStock above is unchanged and still opens
// its own transaction for callers with no existing one to join.
func (s *Service) AddStockTx(ctx context.Context, txRepo *Repository, req StockChangeRequest) (*Inventory, error) {
	if err := validateStockChange(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}
	return addStockTx(ctx, txRepo, req)
}

// RemoveStock decreases the balance at (ProductID, LocationID, LotNo)
// by Quantity. Returns ErrNoStock if the tuple has no balance row at
// all, ErrInsufficientStock if it exists but Quantity exceeds what's
// available — the caller's transaction rolls back in either case (no
// ledger row is written).
func (s *Service) RemoveStock(ctx context.Context, req StockChangeRequest) (*Inventory, error) {
	if err := validateStockChange(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var result *Inventory
	start := time.Now()
	err := s.repo.Transaction(ctx, func(txRepo *Repository) error {
		inv, err := removeStockTx(ctx, txRepo, req)
		if err != nil {
			return err
		}
		result = inv
		return nil
	})
	dbctx.LogTransaction(ctx, "inventory.remove_stock", start, err)
	return result, err
}

// RemoveStockTx is RemoveStock's exact logic, but against a Repository
// the caller already has open inside its own transaction (e.g.
// internal/picking's Confirm, which needs "update pick line + RemoveStock
// + insert ledger row" to commit or roll back together as one unit) —
// added in Phase 9 so that caller can compose this instead of
// duplicating the balance/ledger-write logic or opening a second,
// disconnected transaction. RemoveStock above is unchanged and still
// opens its own transaction for callers with no existing one to join.
func (s *Service) RemoveStockTx(ctx context.Context, txRepo *Repository, req StockChangeRequest) (*Inventory, error) {
	if err := validateStockChange(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}
	return removeStockTx(ctx, txRepo, req)
}

// MoveStock removes Quantity from FromLocationID and adds it to
// ToLocationID, as one database transaction with two correlated ledger
// rows (sharing a generated reference_id, transaction_type=MOVE unless
// req.Type overrides it) — either both legs succeed or neither does.
func (s *Service) MoveStock(ctx context.Context, req MoveStockRequest) error {
	if err := validateMoveStock(req); err != nil {
		return err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.FromLocationID); err != nil {
		return err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.ToLocationID); err != nil {
		return err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(txRepo *Repository) error {
		return moveStockTx(ctx, txRepo, req)
	})
	dbctx.LogTransaction(ctx, "inventory.move_stock", start, err)
	return err
}

// MoveStockTx is MoveStock's exact logic, but against a Repository the
// caller already has open inside its own transaction (e.g.
// internal/putaway's Confirm, which needs "update task/item + MoveStock"
// to commit or roll back together as one unit) — added in Phase 7 so
// that caller can compose this instead of duplicating the balance/
// ledger-write logic or opening a second, disconnected transaction.
// MoveStock above is unchanged and still opens its own transaction for
// callers with no existing one to join.
func (s *Service) MoveStockTx(ctx context.Context, txRepo *Repository, req MoveStockRequest) error {
	if err := validateMoveStock(req); err != nil {
		return err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.FromLocationID); err != nil {
		return err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.ToLocationID); err != nil {
		return err
	}
	return moveStockTx(ctx, txRepo, req)
}

// MoveAvailableStockTx is MoveStockTx's exact shape, but the source leg
// uses AdjustAvailableBalance instead of AdjustBalance — the removal can
// never dip into reserved_quantity or hold_quantity, so a caller can
// never relocate reserved or QC-held stock out of its current location.
// Added in Phase 31C for internal/putaway, the explicit safety
// requirement that "held/rejected stock must never be moved into
// available storage through standard putaway" — moving an item changes
// WHERE a balance sits, and Available = Quantity - Reserved - Hold must
// remain exactly what it was before the move at both ends (only the
// LocationID changes), which plain MoveStockTx cannot guarantee since
// its AdjustBalance has no notion of reserved/held.
func (s *Service) MoveAvailableStockTx(ctx context.Context, txRepo *Repository, req MoveStockRequest) error {
	if err := validateMoveStock(req); err != nil {
		return err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.FromLocationID); err != nil {
		return err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.ToLocationID); err != nil {
		return err
	}

	txType := req.Type
	if txType == "" {
		txType = TransactionMove
	}
	refType := req.ReferenceType
	if refType == nil {
		t := "MOVE"
		refType = &t
	}
	moveRef := req.ReferenceID
	if moveRef == nil {
		ref := uuid.NewString()
		moveRef = &ref
	}

	rows, err := txRepo.AdjustAvailableBalance(ctx, req.TenantID, req.ProductID, req.FromLocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return err
	}
	if rows == 0 {
		if _, ferr := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.FromLocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return ErrNoStock
		}
		return ErrInsufficientAvailableStock
	}
	if err := txRepo.InsertTransaction(ctx, req.TenantID, &Transaction{
		TransactionType: txType, ProductID: req.ProductID, LocationID: req.FromLocationID, LotNo: req.LotNo,
		Quantity: -req.Quantity, ReferenceType: refType, ReferenceID: moveRef, Reason: req.Reason, CreatedBy: req.CreatedBy,
	}); err != nil {
		return err
	}

	_, err = addStockTx(ctx, txRepo, StockChangeRequest{
		TenantID:  req.TenantID,
		ProductID: req.ProductID, LocationID: req.ToLocationID, LotNo: req.LotNo,
		ExpiryDate: req.ExpiryDate,
		Quantity:   req.Quantity, Type: txType,
		ReferenceType: refType, ReferenceID: moveRef, Reason: req.Reason, CreatedBy: req.CreatedBy,
	})
	return err
}

// ReserveStockTx increases reserved_quantity at (ProductID, LocationID,
// LotNo) by Quantity — added in Phase 12 so internal/picking can claim
// stock at task-creation time without moving it yet. Never touches
// quantity and never writes a ledger row: a reservation is not a stock
// movement. Guarded by AdjustReservation's single UPDATE statement, so
// concurrent reservations against the same tuple can never together
// exceed on-hand quantity — see PROJECT_STATUS.md's Phase 12 concurrency
// proof. Returns ErrNoStock if the tuple has no balance row at all,
// ErrInsufficientStock if a row exists but Quantity exceeds what's
// currently available (quantity - reserved_quantity).
func (s *Service) ReserveStockTx(ctx context.Context, txRepo *Repository, req ReserveRequest) (*Inventory, error) {
	if err := validateReserveRequest(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}
	rows, err := txRepo.AdjustReservation(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, req.Quantity)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		// AdjustReservation's guarded UPDATE already made the authoritative,
		// race-free decision that this can't proceed; this lookup is only
		// to tell the caller *why*, not to decide anything — same pattern
		// as removeStockTx's own disambiguation lookup.
		if _, ferr := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrInsufficientStock
	}
	return txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo)
}

// ReleaseReservationTx decreases reserved_quantity at (ProductID,
// LocationID, LotNo) by Quantity, without touching quantity or writing a
// ledger row — used when a reservation is no longer needed and no stock
// actually left the building (a SHORT pick's unpicked remainder).
func (s *Service) ReleaseReservationTx(ctx context.Context, txRepo *Repository, req ReserveRequest) (*Inventory, error) {
	if err := validateReserveRequest(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}
	rows, err := txRepo.AdjustReservation(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		if _, ferr := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrReservationUnderflow
	}
	return txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo)
}

// ConsumeReservationTx is the PICKED-confirm composite: release the
// reservation placed at task-creation time, then remove that exact
// quantity from the real balance via removeStockTx (writing the usual
// PICK ledger row) — both guarded UPDATEs inside the caller's own
// already-open transaction, so a failure at either step rolls back both
// together. A picked reservation is always consumed 1:1: req.Quantity is
// both the release amount and the removal amount, never independent
// values. Returns ErrReservationUnderflow if req.Quantity exceeds what's
// currently reserved for this tuple — this also catches an attempt to
// pick more than was ever reserved for the line, which is a correctness
// bonus this reservation design adds for free.
func (s *Service) ConsumeReservationTx(ctx context.Context, txRepo *Repository, req StockChangeRequest) (*Inventory, error) {
	if err := validateStockChange(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}
	rows, err := txRepo.AdjustReservation(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		if _, ferr := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrReservationUnderflow
	}
	return removeStockTx(ctx, txRepo, req)
}

// ReleaseHoldTx decreases hold_quantity at (ProductID, LocationID,
// LotNo) by Quantity, without touching quantity or writing a ledger
// row — QC's PASS disposition (Phase 31B): the units were already
// counted in Quantity when Receiving posted them into Hold (see
// Inventory.Available's own doc), so releasing a hold is not itself a
// stock movement, the same reasoning ReleaseReservationTx already
// applies to reservations. Reuses ReserveRequest's exact shape (same
// fields, no new struct needed). Returns ErrNoStock if the tuple has no
// balance row at all, ErrHoldUnderflow if Quantity exceeds what's
// currently held.
func (s *Service) ReleaseHoldTx(ctx context.Context, txRepo *Repository, req ReserveRequest) (*Inventory, error) {
	if err := validateReserveRequest(req); err != nil {
		return nil, err
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}
	rows, err := txRepo.AdjustHold(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		if _, ferr := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrHoldUnderflow
	}
	return txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo)
}

// RejectHoldTx atomically deducts Quantity from BOTH the on-hand
// Quantity and the HoldQuantity at (ProductID, LocationID, LotNo) — QC's
// REJECT disposition (Phase 31B): rejected stock leaves the building
// (or is otherwise written off) and must never remain counted as either
// on-hand or held. Ignores req.Type — the ledger row this writes is
// always TransactionReject, never spoofable by the caller, the one
// property this dedicated method exists to guarantee. Writes exactly
// one ledger row (a single signed quantity delta, the same shape every
// other *Tx method's ledger write uses). Returns ErrNoStock if the
// tuple has no balance row at all, ErrHoldUnderflow if Quantity exceeds
// what's currently held — the authoritative guard: a caller can never
// reject more than inventory.HoldQuantity actually has, regardless of
// what QC's own bookkeeping believes.
func (s *Service) RejectHoldTx(ctx context.Context, txRepo *Repository, req StockChangeRequest) (*Inventory, error) {
	if req.TenantID == 0 {
		return nil, ErrValidation{Message: "tenant_id is required"}
	}
	if req.ProductID == 0 || req.LocationID == 0 {
		return nil, ErrValidation{Message: "product_id and location_id are required"}
	}
	if req.Quantity <= 0 {
		return nil, ErrValidation{Message: "quantity must be greater than zero"}
	}
	if err := s.verifyOwnership(ctx, req.TenantID, req.ProductID, req.LocationID); err != nil {
		return nil, err
	}

	holdRows, err := txRepo.AdjustHold(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return nil, err
	}
	if holdRows == 0 {
		if _, ferr := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrHoldUnderflow
	}

	// hold_quantity never exceeds quantity (CK_inventory_hold_quantity_
	// not_exceeding_quantity), so a hold release of Quantity having just
	// succeeded above guarantees quantity >= Quantity too — this can only
	// fail if that invariant were somehow already broken, not a real
	// expected path.
	balRows, err := txRepo.AdjustBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return nil, err
	}
	if balRows == 0 {
		return nil, ErrInsufficientStock
	}

	inv, err := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo)
	if err != nil {
		return nil, err
	}
	if err := txRepo.InsertTransaction(ctx, req.TenantID, &Transaction{
		TransactionType: TransactionReject, ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
		Quantity: -req.Quantity, ReferenceType: req.ReferenceType, ReferenceID: req.ReferenceID,
		Reason: req.Reason, CreatedBy: req.CreatedBy,
	}); err != nil {
		return nil, err
	}
	return inv, nil
}

func validateReserveRequest(req ReserveRequest) error {
	if req.TenantID == 0 {
		return ErrValidation{Message: "tenant_id is required"}
	}
	if req.ProductID == 0 || req.LocationID == 0 {
		return ErrValidation{Message: "product_id and location_id are required"}
	}
	if req.Quantity <= 0 {
		return ErrValidation{Message: "quantity must be greater than zero"}
	}
	return nil
}

// FindBalance exposes Repository.FindBalance through Service — added in
// Phase 7 so internal/putaway can read a source (product, location, lot)
// balance's authoritative current ExpiryDate before moving it (see
// MoveStockRequest's doc: putaway must never let the caller invent an
// expiry, only carry forward whatever the source row already has).
func (s *Service) FindBalance(ctx context.Context, tenantID, productID, locationID int64, lotNo string) (*Inventory, error) {
	return s.repo.FindBalance(ctx, tenantID, productID, locationID, lotNo)
}

// ListTransactionsByReference exposes Repository.ListTransactionsByReference
// through Service — see that method's own doc.
func (s *Service) ListTransactionsByReference(ctx context.Context, tenantID int64, referenceType, referenceID string) ([]Transaction, error) {
	return s.repo.ListTransactionsByReference(ctx, tenantID, referenceType, referenceID)
}

// ProductAvailability is AvailabilityByProductWarehouse's result — a
// pure, read-only snapshot, never a reservation or any other mutation.
type ProductAvailability struct {
	AvailableQty float64
	ReservedQty  float64
	HoldQty      float64
	OnHandQty    float64
}

// AvailabilityByProductWarehouse exposes Repository.
// AvailabilityByProductWarehouse through Service — added in Phase 33A
// for internal/order's Review & Release step. Deliberately has no
// side effect whatsoever: it issues one SELECT and returns, never
// touching AdjustReservation/AdjustBalance/AdjustHold — Order 33A must
// never reserve or mutate inventory itself, only read it (see this
// package's own doc and internal/order/service.go's CheckAvailability
// doc).
func (s *Service) AvailabilityByProductWarehouse(ctx context.Context, tenantID, productID, warehouseID int64) (ProductAvailability, error) {
	available, reserved, hold, onHand, err := s.repo.AvailabilityByProductWarehouse(ctx, tenantID, productID, warehouseID)
	if err != nil {
		return ProductAvailability{}, err
	}
	return ProductAvailability{AvailableQty: available, ReservedQty: reserved, HoldQty: hold, OnHandQty: onHand}, nil
}

// LotAvailability is one row of ListAvailableLotsByProductWarehouse's
// FEFO/FIFO-ordered result — see that method's own doc.
type LotAvailability struct {
	LocationID   int64
	LocationCode string
	LotNo        string
	ExpiryDate   *time.Time
	AvailableQty float64
	OnHandQty    float64
	CreatedAt    time.Time
}

// ListAvailableLotsByProductWarehouse exposes Repository.
// ListAvailableLotsByProductWarehouse through Service — Phase 33B's
// read-only FEFO/FIFO lot-suggestion primitive for internal/picking
// (and any future caller). See the repository method's own doc for why
// this is new rather than reused from an existing query, and for why it
// duplicates no reservation/consumption logic.
func (s *Service) ListAvailableLotsByProductWarehouse(ctx context.Context, tenantID, productID, warehouseID int64) ([]LotAvailability, error) {
	return s.repo.ListAvailableLotsByProductWarehouse(ctx, tenantID, productID, warehouseID)
}

func validateMoveStock(req MoveStockRequest) error {
	if req.TenantID == 0 {
		return ErrValidation{Message: "tenant_id is required"}
	}
	if req.ProductID == 0 || req.FromLocationID == 0 || req.ToLocationID == 0 {
		return ErrValidation{Message: "product_id, from_location_id, and to_location_id are required"}
	}
	if req.FromLocationID == req.ToLocationID {
		return ErrValidation{Message: "from_location_id and to_location_id must differ"}
	}
	if req.Quantity <= 0 {
		return ErrValidation{Message: "quantity must be greater than zero"}
	}
	return nil
}

// moveStockTx does the actual work of MoveStock/MoveStockTx against an
// already-open transactional repository.
func moveStockTx(ctx context.Context, txRepo *Repository, req MoveStockRequest) error {
	txType := req.Type
	if txType == "" {
		txType = TransactionMove
	}
	refType := req.ReferenceType
	if refType == nil {
		t := "MOVE"
		refType = &t
	}
	moveRef := req.ReferenceID
	if moveRef == nil {
		ref := uuid.NewString()
		moveRef = &ref
	}

	if _, err := removeStockTx(ctx, txRepo, StockChangeRequest{
		TenantID:  req.TenantID,
		ProductID: req.ProductID, LocationID: req.FromLocationID, LotNo: req.LotNo,
		Quantity: req.Quantity, Type: txType,
		ReferenceType: refType, ReferenceID: moveRef, Reason: req.Reason, CreatedBy: req.CreatedBy,
	}); err != nil {
		return err
	}

	_, err := addStockTx(ctx, txRepo, StockChangeRequest{
		TenantID:  req.TenantID,
		ProductID: req.ProductID, LocationID: req.ToLocationID, LotNo: req.LotNo,
		ExpiryDate: req.ExpiryDate,
		Quantity:   req.Quantity, Type: txType,
		ReferenceType: refType, ReferenceID: moveRef, Reason: req.Reason, CreatedBy: req.CreatedBy,
	})
	return err
}

// addStockTx does the actual work of AddStock against an
// already-open transactional repository, so MoveStock can compose it
// with removeStockTx inside one shared transaction instead of two.
//
// The AdjustBalance-then-CreateBalance pair below is a genuine
// check-then-insert race when two transactions hit the same brand-new
// (tenant_id, product_id, location_id, lot_no) tuple at once: both see
// rows==0 from AdjustBalance (neither row exists yet), both attempt
// CreateBalance, and the loser hits UQ_inventory_product_location_lot
// (migration 0009) instead of the row it should have just updated.
// Measured at ~3-7% under 10-way concurrent hammering of a fresh
// tuple. The loop below bounds the fix to exactly one retry: if
// CreateBalance fails on a genuine unique violation, the other
// transaction's INSERT won by definition already committed, so
// looping back to AdjustBalance will now find that row and UPDATE it
// instead — turning the race into "whichever transaction's INSERT
// wins, both callers' quantities land." Any other error from
// CreateBalance (notably an FK violation — a permanent, non-transient
// problem) returns immediately without retrying.
func addStockTx(ctx context.Context, txRepo *Repository, req StockChangeRequest) (*Inventory, error) {
	for attempt := 0; attempt < 2; attempt++ {
		rows, err := txRepo.AdjustBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, req.Quantity)
		if err != nil {
			return nil, err
		}

		if rows == 0 {
			// A positive delta can only fail AdjustBalance's WHERE clause if
			// the row doesn't exist yet (the >= 0 guard can't reject a
			// positive delta) — first stock ever placed at this tuple.
			// HoldQuantity is set directly on the brand-new row rather than
			// via a second AdjustHold call — there is no concurrent writer
			// to race against a row this transaction just created (bar the
			// unique-violation race handled immediately below, which only
			// concerns the row's existence, not its HoldQuantity).
			if err := txRepo.CreateBalance(ctx, req.TenantID, &Inventory{
				ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
				ExpiryDate: req.ExpiryDate, Quantity: req.Quantity, HoldQuantity: req.HoldQuantity,
			}); err != nil {
				if sqlerr.IsUniqueViolation(err) && attempt == 0 {
					// Another transaction committed this exact row between our
					// AdjustBalance and our CreateBalance. Retry once: the next
					// AdjustBalance will find it and UPDATE it correctly.
					continue
				}
				return nil, translateFKViolation(err)
			}
		} else if req.HoldQuantity > 0 {
			// An existing balance row already had Quantity bumped by
			// AdjustBalance above, in this same transaction; AdjustHold's own
			// guard (hold_quantity+reserved_quantity<=quantity) is checked
			// against that already-updated row, so this can only return 0
			// rows if validateStockChange's own HoldQuantity<=Quantity
			// invariant was somehow violated — defensive, not expected to
			// trigger in practice.
			hrows, err := txRepo.AdjustHold(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, req.HoldQuantity)
			if err != nil {
				return nil, err
			}
			if hrows == 0 {
				return nil, ErrValidation{Message: "hold_quantity would exceed on-hand quantity"}
			}
		}

		break
	}

	inv, err := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo)
	if err != nil {
		return nil, err
	}

	if err := txRepo.InsertTransaction(ctx, req.TenantID, &Transaction{
		TransactionType: req.Type, ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
		Quantity: req.Quantity, ReferenceType: req.ReferenceType, ReferenceID: req.ReferenceID,
		Reason: req.Reason, CreatedBy: req.CreatedBy,
	}); err != nil {
		return nil, err
	}

	return inv, nil
}

// removeStockTx does the actual work of RemoveStock against an
// already-open transactional repository, for the same reason
// addStockTx exists.
func removeStockTx(ctx context.Context, txRepo *Repository, req StockChangeRequest) (*Inventory, error) {
	delta := -req.Quantity

	rows, err := txRepo.AdjustBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo, delta)
	if err != nil {
		return nil, err
	}

	if rows == 0 {
		// AdjustBalance's guarded UPDATE already made the authoritative,
		// race-free decision that this can't proceed; this lookup is only
		// to tell the caller *why* (missing vs. insufficient), not to
		// decide anything.
		if _, ferr := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrInsufficientStock
	}

	inv, err := txRepo.FindBalance(ctx, req.TenantID, req.ProductID, req.LocationID, req.LotNo)
	if err != nil {
		return nil, err
	}

	if err := txRepo.InsertTransaction(ctx, req.TenantID, &Transaction{
		TransactionType: req.Type, ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
		Quantity: delta, ReferenceType: req.ReferenceType, ReferenceID: req.ReferenceID,
		Reason: req.Reason, CreatedBy: req.CreatedBy,
	}); err != nil {
		return nil, err
	}

	return inv, nil
}

func validateStockChange(req StockChangeRequest) error {
	if req.TenantID == 0 {
		return ErrValidation{Message: "tenant_id is required"}
	}
	if req.ProductID == 0 || req.LocationID == 0 {
		return ErrValidation{Message: "product_id and location_id are required"}
	}
	if req.Quantity <= 0 {
		return ErrValidation{Message: "quantity must be greater than zero"}
	}
	if !req.Type.IsValid() {
		return ErrValidation{Message: "invalid transaction type"}
	}
	if req.HoldQuantity < 0 || req.HoldQuantity > req.Quantity {
		return ErrValidation{Message: "hold_quantity must be between 0 and quantity"}
	}
	return nil
}

// translateFKViolation maps a genuine FK constraint violation (error
// 547, via internal/platform/database/sqlerr) to the specific sentinel
// the violated constraint implies. Gating on the error number first,
// rather than searching any error's message for a constraint-name
// substring, means an unrelated error whose text happened to contain
// "fk_inventory_product" could never be misclassified as this FK.
func translateFKViolation(err error) error {
	if !sqlerr.IsForeignKeyViolation(err) {
		return err
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "fk_inventory_product") {
		return ErrValidation{Message: "product does not exist"}
	}
	if strings.Contains(msg, "fk_inventory_location") {
		return ErrValidation{Message: "location does not exist"}
	}
	return err
}

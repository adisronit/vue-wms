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

// ReserveRequest is the input to ReserveStockTx/ReleaseReservationTx —
// deliberately narrower than StockChangeRequest: a reservation never
// writes a ledger row (it isn't a stock movement, just a claim against
// an existing balance), so it carries no Type/ReferenceType/Reason/
// CreatedBy.
type ReserveRequest struct {
	ProductID  int64
	LocationID int64
	LotNo      string
	Quantity   float64
}

// StockChangeRequest is the input to AddStock/RemoveStock. Quantity is
// always a positive magnitude — direction is implied by which method is
// called, not by the sign of Quantity.
type StockChangeRequest struct {
	ProductID     int64
	LocationID    int64
	LotNo         string
	ExpiryDate    *time.Time
	Quantity      float64
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
// unchanged.
type MoveStockRequest struct {
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
// balance. No HTTP handler in this phase calls the mutation methods;
// they exist so Receiving/Putaway/Picking/Shipping/Adjustment/Count
// (later phases) never need to reimplement "update balance + write
// ledger row, atomically" themselves.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
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

// ListForExport is Phase 22's CSV-export path for the Inventory Stock
// Report — see internal/receiving's own ListForExport doc for the
// shared reasoning.
func (s *Service) ListForExport(ctx context.Context, params ListParams) ([]Response, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.List(ctx, params)
	return rows, err
}

// ListTransactionsForExport backs the async Inventory Transaction
// History export (internal/report's worker calls this) — see
// TransactionExportRow's own doc.
func (s *Service) ListTransactionsForExport(ctx context.Context, filters TransactionExportFilters) ([]TransactionExportRow, error) {
	return s.repo.ListTransactionsForExport(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) Summary(ctx context.Context) (SummaryResponse, error) {
	return s.repo.Summary(ctx)
}

func (s *Service) Transactions(ctx context.Context, inventoryID int64) ([]TransactionResponse, error) {
	return s.repo.Transactions(ctx, inventoryID, 50)
}

// AddStock increases the balance at (ProductID, LocationID, LotNo) by
// Quantity, creating the balance row if this is the first stock ever
// placed there, and appends a matching ledger row — both in one
// database transaction.
func (s *Service) AddStock(ctx context.Context, req StockChangeRequest) (*Inventory, error) {
	if err := validateStockChange(req); err != nil {
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
	return moveStockTx(ctx, txRepo, req)
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
	rows, err := txRepo.AdjustReservation(ctx, req.ProductID, req.LocationID, req.LotNo, req.Quantity)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		// AdjustReservation's guarded UPDATE already made the authoritative,
		// race-free decision that this can't proceed; this lookup is only
		// to tell the caller *why*, not to decide anything — same pattern
		// as removeStockTx's own disambiguation lookup.
		if _, ferr := txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrInsufficientStock
	}
	return txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo)
}

// ReleaseReservationTx decreases reserved_quantity at (ProductID,
// LocationID, LotNo) by Quantity, without touching quantity or writing a
// ledger row — used when a reservation is no longer needed and no stock
// actually left the building (a SHORT pick's unpicked remainder).
func (s *Service) ReleaseReservationTx(ctx context.Context, txRepo *Repository, req ReserveRequest) (*Inventory, error) {
	if err := validateReserveRequest(req); err != nil {
		return nil, err
	}
	rows, err := txRepo.AdjustReservation(ctx, req.ProductID, req.LocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		if _, ferr := txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrReservationUnderflow
	}
	return txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo)
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
	rows, err := txRepo.AdjustReservation(ctx, req.ProductID, req.LocationID, req.LotNo, -req.Quantity)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		if _, ferr := txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrReservationUnderflow
	}
	return removeStockTx(ctx, txRepo, req)
}

func validateReserveRequest(req ReserveRequest) error {
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
func (s *Service) FindBalance(ctx context.Context, productID, locationID int64, lotNo string) (*Inventory, error) {
	return s.repo.FindBalance(ctx, productID, locationID, lotNo)
}

func validateMoveStock(req MoveStockRequest) error {
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
		ProductID: req.ProductID, LocationID: req.FromLocationID, LotNo: req.LotNo,
		Quantity: req.Quantity, Type: txType,
		ReferenceType: refType, ReferenceID: moveRef, Reason: req.Reason, CreatedBy: req.CreatedBy,
	}); err != nil {
		return err
	}

	_, err := addStockTx(ctx, txRepo, StockChangeRequest{
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
func addStockTx(ctx context.Context, txRepo *Repository, req StockChangeRequest) (*Inventory, error) {
	rows, err := txRepo.AdjustBalance(ctx, req.ProductID, req.LocationID, req.LotNo, req.Quantity)
	if err != nil {
		return nil, err
	}

	if rows == 0 {
		// A positive delta can only fail AdjustBalance's WHERE clause if
		// the row doesn't exist yet (the >= 0 guard can't reject a
		// positive delta) — first stock ever placed at this tuple.
		if err := txRepo.CreateBalance(ctx, &Inventory{
			ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
			ExpiryDate: req.ExpiryDate, Quantity: req.Quantity,
		}); err != nil {
			return nil, translateFKViolation(err)
		}
	}

	inv, err := txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo)
	if err != nil {
		return nil, err
	}

	if err := txRepo.InsertTransaction(ctx, &Transaction{
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

	rows, err := txRepo.AdjustBalance(ctx, req.ProductID, req.LocationID, req.LotNo, delta)
	if err != nil {
		return nil, err
	}

	if rows == 0 {
		// AdjustBalance's guarded UPDATE already made the authoritative,
		// race-free decision that this can't proceed; this lookup is only
		// to tell the caller *why* (missing vs. insufficient), not to
		// decide anything.
		if _, ferr := txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo); errors.Is(ferr, ErrNotFound) {
			return nil, ErrNoStock
		}
		return nil, ErrInsufficientStock
	}

	inv, err := txRepo.FindBalance(ctx, req.ProductID, req.LocationID, req.LotNo)
	if err != nil {
		return nil, err
	}

	if err := txRepo.InsertTransaction(ctx, &Transaction{
		TransactionType: req.Type, ProductID: req.ProductID, LocationID: req.LocationID, LotNo: req.LotNo,
		Quantity: delta, ReferenceType: req.ReferenceType, ReferenceID: req.ReferenceID,
		Reason: req.Reason, CreatedBy: req.CreatedBy,
	}); err != nil {
		return nil, err
	}

	return inv, nil
}

func validateStockChange(req StockChangeRequest) error {
	if req.ProductID == 0 || req.LocationID == 0 {
		return ErrValidation{Message: "product_id and location_id are required"}
	}
	if req.Quantity <= 0 {
		return ErrValidation{Message: "quantity must be greater than zero"}
	}
	if !req.Type.IsValid() {
		return ErrValidation{Message: "invalid transaction type"}
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

package receiving

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
	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrWarehouseNotFound = errors.New("receiving: warehouse not found")
	ErrLocationNotFound  = errors.New("receiving: location not found")
	ErrProductNotFound   = errors.New("receiving: product not found")
	ErrInvalidTransition = errors.New("receiving: receipt is not in a confirmable status")
	ErrNotEditable       = errors.New("receiving: receipt can no longer be edited")
	ErrNotCancellable    = errors.New("receiving: receipt is not in a cancellable status")
)

// productChecker, warehouseChecker, and locationChecker are the narrow
// slices of the Phase 3/4 repositories this package needs — satisfied
// structurally by *product.Repository, *warehouse.Repository, and
// *location.Repository, the same narrow-interface pattern used
// throughout warehouse/zone/location and user/auth.
type productChecker interface {
	FindByID(ctx context.Context, id int64) (*product.Product, error)
}

type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

type locationChecker interface {
	FindByID(ctx context.Context, id int64) (*location.Location, error)
}

// Service holds the Receiving workflow's business rules — the status
// machine, validation, and the Confirm transaction that calls Phase 5's
// inventory.Service.AddStockTx instead of mutating stock itself.
type Service struct {
	repo       *Repository
	products   productChecker
	warehouses warehouseChecker
	locations  locationChecker
	inventory  *inventory.Service
}

func NewService(repo *Repository, products productChecker, warehouses warehouseChecker, locations locationChecker, inventorySvc *inventory.Service) *Service {
	return &Service{repo: repo, products: products, warehouses: warehouses, locations: locations, inventory: inventorySvc}
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
	return s.repo.ListReceipts(ctx, params)
}

// ListForExport is Phase 22's CSV-export path — the same List query and
// filters, honoring params.Search/Status/WarehouseID/Date exactly as
// the paginated screen does, but capped at csvexport.MaxExportRows
// instead of the screen's own 100-row page size. Discards the total
// count (unused by a file export).
func (s *Service) ListForExport(ctx context.Context, params ListParams) ([]ListResponse, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.ListReceipts(ctx, params)
	return rows, err
}

// CountOpen exposes Repository.CountOpen through Service — Phase 23's
// Dashboard "Pending Receiving" card.
func (s *Service) CountOpen(ctx context.Context) (int64, error) {
	return s.repo.CountOpen(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (*Response, error) {
	header, err := s.repo.FindReceiptDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, ReceiptNo: header.ReceiptNo, ReceivingType: ReceivingType(header.ReceivingType),
		ReferenceNo: header.ReferenceNo, Vendor: header.Vendor,
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode,
		StagingLocationID: header.StagingLocationID, StagingLocation: header.StagingLocation,
		Status: Status(header.Status), ReceivingDate: header.ReceivingDate, Remark: header.Remark,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items,
	}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest, userID int64) (*Response, error) {
	if !req.ReceivingType.IsValid() {
		return nil, ErrValidation{Message: "receiving_type must be one of PO, ASN, RETURN, TRANSFER"}
	}

	status := req.Status
	if status == "" {
		status = StatusDraft
	}
	if status != StatusDraft && status != StatusWaiting {
		return nil, ErrValidation{Message: "status must be DRAFT or WAITING when creating a receipt"}
	}

	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}
	if _, err := s.locations.FindByID(ctx, req.StagingLocationID); err != nil {
		return nil, ErrLocationNotFound
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var receiptID int64
	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		receipt := &Receipt{
			ReceivingType: req.ReceivingType, ReferenceNo: req.ReferenceNo, Vendor: req.Vendor,
			WarehouseID: req.WarehouseID, StagingLocationID: req.StagingLocationID, Status: status,
			ReceivingDate: req.ReceivingDate, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateReceipt(ctx, receipt); err != nil {
			return err
		}
		receiptID = receipt.ID

		for _, itemReq := range req.Items {
			if itemReq.LocationID == 0 {
				itemReq.LocationID = req.StagingLocationID
			}
			if err := s.createItem(ctx, txRepo, receipt.ID, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "receiving.create", start, err)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, receiptID)
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*Response, error) {
	receipt, err := s.repo.FindReceiptByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !receipt.Status.CanEdit() {
		return nil, ErrNotEditable
	}
	if _, err := s.locations.FindByID(ctx, req.StagingLocationID); err != nil {
		return nil, ErrLocationNotFound
	}

	receipt.ReferenceNo = req.ReferenceNo
	receipt.Vendor = req.Vendor
	receipt.StagingLocationID = req.StagingLocationID
	receipt.ReceivingDate = req.ReceivingDate
	receipt.Remark = req.Remark

	if err := s.repo.UpdateReceipt(ctx, receipt); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// AddItem is POST /receipts/{id}/items — used both by the "+ Add Item"
// button and the barcode-scan flow (which adds-or-increments a line;
// the frontend decides which by checking its own already-fetched item
// list, then calls either AddItem or UpdateItem accordingly).
func (s *Service) AddItem(ctx context.Context, receiptID int64, req CreateItemRequest) (*Response, error) {
	receipt, err := s.repo.FindReceiptByID(ctx, receiptID)
	if err != nil {
		return nil, err
	}
	if !receipt.Status.CanEdit() {
		return nil, ErrNotEditable
	}
	if req.LocationID == 0 {
		req.LocationID = receipt.StagingLocationID
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		return s.createItem(ctx, txRepo, receiptID, req)
	})
	dbctx.LogTransaction(ctx, "receiving.add_item", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, receiptID)
}

func (s *Service) createItem(ctx context.Context, txRepo *Repository, receiptID int64, req CreateItemRequest) error {
	p, err := s.products.FindByID(ctx, req.ProductID)
	if err != nil {
		return ErrProductNotFound
	}
	if p.SerialRequired {
		return ErrValidation{Message: fmt.Sprintf("product %s requires serial tracking, which is not yet supported by receiving", p.SKU)}
	}
	if p.LotRequired && req.LotNo == "" {
		return ErrValidation{Message: fmt.Sprintf("product %s requires a lot number", p.SKU)}
	}
	if p.ExpiryRequired && req.ExpiryDate == nil {
		return ErrValidation{Message: fmt.Sprintf("product %s requires an expiry date", p.SKU)}
	}
	if req.ExpectedQty < 0 {
		return ErrValidation{Message: "expected_qty must not be negative"}
	}
	if _, err := s.locations.FindByID(ctx, req.LocationID); err != nil {
		return ErrLocationNotFound
	}

	uom := req.UOM
	if uom == "" {
		uom = string(p.BaseUOM)
	}

	item := &ReceiptItem{
		ReceiptID: receiptID, ProductID: req.ProductID, ExpectedQty: req.ExpectedQty, UOM: uom,
		LotNo: req.LotNo, ExpiryDate: req.ExpiryDate, LocationID: req.LocationID, Status: ItemStatusPending,
	}
	return txRepo.CreateItem(ctx, item)
}

func (s *Service) UpdateItem(ctx context.Context, receiptID, itemID int64, req UpdateItemRequest) (*Response, error) {
	receipt, err := s.repo.FindReceiptByID(ctx, receiptID)
	if err != nil {
		return nil, err
	}
	if !receipt.Status.CanEdit() {
		return nil, ErrNotEditable
	}

	item, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.ReceiptID != receiptID {
		return nil, ErrItemNotFound
	}
	if item.Status != ItemStatusPending {
		return nil, ErrValidation{Message: "line has already started receiving and can no longer be edited"}
	}

	p, err := s.products.FindByID(ctx, item.ProductID)
	if err != nil {
		return nil, ErrProductNotFound
	}
	if p.LotRequired && req.LotNo == "" {
		return nil, ErrValidation{Message: fmt.Sprintf("product %s requires a lot number", p.SKU)}
	}
	if p.ExpiryRequired && req.ExpiryDate == nil {
		return nil, ErrValidation{Message: fmt.Sprintf("product %s requires an expiry date", p.SKU)}
	}
	if req.ExpectedQty < 0 {
		return nil, ErrValidation{Message: "expected_qty must not be negative"}
	}
	if _, err := s.locations.FindByID(ctx, req.LocationID); err != nil {
		return nil, ErrLocationNotFound
	}

	item.ExpectedQty = req.ExpectedQty
	item.UOM = req.UOM
	item.LotNo = req.LotNo
	item.ExpiryDate = req.ExpiryDate
	item.LocationID = req.LocationID

	if err := s.repo.UpdateItem(ctx, item); err != nil {
		return nil, err
	}
	return s.Get(ctx, receiptID)
}

// Confirm is the atomic core of this phase: validate -> update receipt
// item received_qty -> call inventory.AddStockTx (same transaction) ->
// ledger row is written by AddStockTx itself -> update receipt status
// -> commit. Any failure rolls back everything, including any lines
// already processed earlier in this same call.
func (s *Service) Confirm(ctx context.Context, receiptID int64, req ConfirmRequest, userID int64) (*Response, error) {
	if len(req.Items) == 0 {
		return nil, ErrValidation{Message: "at least one item is required to confirm receiving"}
	}

	// This is the exact transaction shape that orphaned live in Phase 7
	// (there, on Putaway's Confirm) when the client disconnected
	// mid-flight — Receiving's Confirm shares the identical
	// Repository.Transaction pattern and was never itself exercised
	// against that scenario until Phase 8. Detach before opening it.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the receipt exists BEFORE claiming the idempotency key:
		// receipt_confirmations.receipt_id is a real FK (migrations/0013),
		// so inserting the guard for a nonexistent id would raise a raw
		// FK-violation 500 instead of a clean 404 — a real bug found live
		// during Phase 11's own testing and fixed there in
		// internal/stockadjustment/internal/stockcount first; this is the
		// identical fix applied to the packages Phase 11 flagged as
		// carrying the same defect (see PROJECT_STATUS.md's Phase 12).
		// This SELECT has no side effect, so moving it ahead of the
		// INSERT does not weaken the idempotency guarantee at all.
		receipt, err := txRepo.FindReceiptByID(ctx, receiptID)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the UNIQUE constraint and everything below is skipped
		// and rolled back — see migrations/0013's doc.
		if err := txRepo.InsertConfirmation(ctx, req.IdempotencyKey, receiptID); err != nil {
			return err
		}

		if !receipt.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		invRepo := inventory.NewRepository(tx)
		refType := "RECEIVE"
		refID := receipt.ReceiptNo

		for _, lineReq := range req.Items {
			if lineReq.Quantity <= 0 {
				return ErrValidation{Message: "quantity must be greater than zero for every confirmed line"}
			}

			item, err := txRepo.FindItemByID(ctx, lineReq.ItemID)
			if err != nil {
				return err
			}
			if item.ReceiptID != receiptID {
				return ErrItemNotFound
			}
			if item.Status == ItemStatusCompleted {
				return ErrValidation{Message: fmt.Sprintf("line %d is already fully received", item.ID)}
			}

			p, err := s.products.FindByID(ctx, item.ProductID)
			if err != nil {
				return ErrProductNotFound
			}
			if p.SerialRequired {
				return ErrValidation{Message: fmt.Sprintf("product %s requires serial tracking, which is not yet supported by receiving", p.SKU)}
			}

			lotNo := item.LotNo
			if lineReq.LotNo != "" {
				lotNo = lineReq.LotNo
			}
			if p.LotRequired && lotNo == "" {
				return ErrValidation{Message: fmt.Sprintf("product %s requires a lot number", p.SKU)}
			}

			expiryDate := item.ExpiryDate
			if lineReq.ExpiryDate != nil {
				expiryDate = lineReq.ExpiryDate
			}
			if p.ExpiryRequired && expiryDate == nil {
				return ErrValidation{Message: fmt.Sprintf("product %s requires an expiry date", p.SKU)}
			}

			locationID := item.LocationID
			if lineReq.LocationID != 0 {
				locationID = lineReq.LocationID
			}
			if _, err := s.locations.FindByID(ctx, locationID); err != nil {
				return ErrLocationNotFound
			}

			if _, err := s.inventory.AddStockTx(ctx, invRepo, inventory.StockChangeRequest{
				ProductID: item.ProductID, LocationID: locationID, LotNo: lotNo, ExpiryDate: expiryDate,
				Quantity: lineReq.Quantity, Type: inventory.TransactionReceive,
				ReferenceType: &refType, ReferenceID: &refID, CreatedBy: &userID,
			}); err != nil {
				return err
			}

			// Atomic guarded increment (see AdjustReceivedQty's doc) instead
			// of item.ReceivedQty += lineReq.Quantity followed by a blind
			// write-back — that read-modify-write was exactly the
			// lost-update race Phase 8's concurrency regression test found.
			// Re-read afterward so status derivation uses the row's true,
			// post-increment total, not this loop's pre-transaction snapshot.
			if _, err := txRepo.AdjustReceivedQty(ctx, item.ID, lineReq.Quantity); err != nil {
				return err
			}
			updated, err := txRepo.FindItemByID(ctx, item.ID)
			if err != nil {
				return err
			}
			updated.LotNo = lotNo
			updated.ExpiryDate = expiryDate
			updated.LocationID = locationID
			updated.Status = itemStatus(updated.ExpectedQty, updated.ReceivedQty)
			if err := txRepo.UpdateItemAfterConfirm(ctx, updated); err != nil {
				return err
			}
		}

		allItems, err := txRepo.FindItemsByReceipt(ctx, receiptID)
		if err != nil {
			return err
		}
		receipt.Status = computeReceiptStatus(allItems)
		return txRepo.UpdateReceipt(ctx, receipt)
	})
	dbctx.LogTransaction(ctx, "receiving.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second stock add.
		return s.Get(ctx, receiptID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, receiptID)
}

// Cancel is Phase 15's addition — allowed only while the receipt is
// still DRAFT/WAITING, i.e. before any line's received_qty has ever
// moved above zero (see Status.CanCancel's doc). Never calls
// inventory.AddStockTx/RemoveStockTx: a cancellable receipt by
// definition has no stock movement to undo, and per this phase's own
// instructions, an already-confirmed quantity is never reversed
// automatically — Cancel simply refuses once that boundary has passed
// (ErrNotCancellable, mapped to 409) rather than inventing a
// compensating transaction.
func (s *Service) Cancel(ctx context.Context, receiptID int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		receipt, err := txRepo.FindReceiptByID(ctx, receiptID)
		if err != nil {
			return err
		}
		if !receipt.Status.CanCancel() {
			// Covers both "already CANCELLED" (prevents double
			// cancellation) and "already PARTIALLY_RECEIVED/COMPLETED"
			// (stock has already moved for this receipt).
			return ErrNotCancellable
		}

		// Populate the Phase 18 audit columns only on this, the successful
		// path — CanCancel's own guard above already rejected an already-
		// CANCELLED (or already-received) receipt before this point, so a
		// duplicate/invalid cancel attempt never reaches here and can never
		// overwrite the original cancellation's audit data. Remark is
		// stored in the dedicated cancel_remark column, not the receipt's
		// own Remark field — unlike Phase 15's original behavior, Cancel no
		// longer clobbers whatever remark already existed on the receipt.
		receipt.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelReceipt(ctx, receipt)
	})
	dbctx.LogTransaction(ctx, "receiving.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, receiptID)
}

// computeReceiptStatus derives the header status from its lines: fully
// received (every line COMPLETED) -> COMPLETED; anything received but
// not everything -> PARTIALLY_RECEIVED; nothing received yet -> WAITING.
func computeReceiptStatus(items []ReceiptItem) Status {
	if len(items) == 0 {
		return StatusWaiting
	}
	anyReceived := false
	allComplete := true
	for _, it := range items {
		if it.ReceivedQty > 0 {
			anyReceived = true
		}
		if it.Status != ItemStatusCompleted {
			allComplete = false
		}
	}
	if allComplete {
		return StatusCompleted
	}
	if anyReceived {
		return StatusPartiallyReceived
	}
	return StatusWaiting
}

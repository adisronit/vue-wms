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
	"wms-backend/internal/supplier"
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
	ErrSupplierNotFound  = errors.New("receiving: supplier not found")
	ErrInvalidTransition = errors.New("receiving: receipt is not in a confirmable status")
	ErrNotEditable       = errors.New("receiving: receipt can no longer be edited")
	ErrNotCancellable    = errors.New("receiving: receipt is not in a cancellable status")
)

// productChecker, warehouseChecker, locationChecker, and supplierChecker
// are the narrow ownership-check interfaces this package needs.
//
// Phase 31A tenant-scopes every one of them — Receiving was the last
// inbound/outbound module whose product/warehouse/location lookups were
// plain existence checks, not ownership checks (phase27-audit-report.md
// §3(c)'s "existence checker, not ownership checker" gap, closed for
// Inventory in Phase 30 Batch 1 and for Picking/Order/Shipping in their
// own Phase 30 batches, but never for Receiving itself). productChecker
// now requires FindByIDForTenant (internal/product's own scoped lookup,
// mirroring internal/picking's identical Batch 2 interface);
// locationChecker and warehouseChecker now require the real tenant-
// scoped FindByID each package exposes (internal/location's since
// Location Master Management; internal/warehouse's own FindByIDForTenant,
// added in this same phase — see that method's own doc for why
// Receiving is the first caller of it anywhere in this codebase).
type productChecker interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*product.Product, error)
}

type warehouseChecker interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*warehouse.Warehouse, error)
}

type locationChecker interface {
	FindByID(ctx context.Context, tenantID, id int64) (*location.Location, error)
}

// supplierChecker is the narrow slice of internal/supplier.Repository
// this package needs to both verify ownership of a chosen SupplierID
// and read its company_name for the one-time snapshot — see
// Service.resolveSupplier's own doc.
type supplierChecker interface {
	FindByID(ctx context.Context, tenantID, id int64) (*supplier.Supplier, error)
}

// Service holds the Receiving workflow's business rules — the status
// machine, validation, and the Confirm transaction that calls Phase 5's
// inventory.Service.AddStockTx instead of mutating stock itself.
type Service struct {
	repo       *Repository
	products   productChecker
	warehouses warehouseChecker
	locations  locationChecker
	suppliers  supplierChecker
	inventory  *inventory.Service
}

func NewService(repo *Repository, products productChecker, warehouses warehouseChecker, locations locationChecker, suppliers supplierChecker, inventorySvc *inventory.Service) *Service {
	return &Service{repo: repo, products: products, warehouses: warehouses, locations: locations, suppliers: suppliers, inventory: inventorySvc}
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
	return s.repo.ListReceipts(ctx, tenantID, params)
}

// ListForExport is Phase 22's CSV-export path — the same List query and
// filters, honoring params.Search/Status/WarehouseID/Date exactly as
// the paginated screen does, but capped at csvexport.MaxExportRows
// instead of the screen's own 100-row page size. Discards the total
// count (unused by a file export).
func (s *Service) ListForExport(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, error) {
	params.Page = 1
	params.Limit = csvexport.MaxExportRows
	rows, _, err := s.repo.ListReceipts(ctx, tenantID, params)
	return rows, err
}

// CountOpen exposes Repository.CountOpen through Service — Phase 23's
// Dashboard "Pending Receiving" card. tenantID (Phase 31A): dashboard
// has no verified per-request tenant context of its own yet, so it
// passes inventory.UnverifiedCallerTenantID — same bridging pattern
// internal/picking's own Batch 2 CountOpen established for the same
// caller (see internal/dashboard/service.go's own doc).
func (s *Service) CountOpen(ctx context.Context, tenantID int64) (int64, error) {
	return s.repo.CountOpen(ctx, tenantID)
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	header, err := s.repo.FindReceiptDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, ReceiptNo: header.ReceiptNo, ReceivingType: ReceivingType(header.ReceivingType),
		ReferenceNo: header.ReferenceNo, Vendor: header.Vendor,
		SupplierID: header.SupplierID, SupplierName: header.SupplierName,
		InvoiceNo: header.InvoiceNo, DeliveryNoteNo: header.DeliveryNoteNo,
		ContainerNo: header.ContainerNo, SealNo: header.SealNo,
		TransportCompany: header.TransportCompany, DriverName: header.DriverName,
		VehicleRegNo: header.VehicleRegNo, DockDoor: header.DockDoor,
		ArrivalTime: header.ArrivalTime, UnloadingStartTime: header.UnloadingStartTime,
		UnloadingEndTime: header.UnloadingEndTime, Currency: header.Currency,
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode,
		StagingLocationID: header.StagingLocationID, StagingLocation: header.StagingLocation,
		Status: Status(header.Status), ReceivingDate: header.ReceivingDate, Remark: header.Remark,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items,
	}, nil
}

// resolveSupplier verifies supplierID belongs to tenantID (cross-tenant
// and "doesn't exist" are indistinguishable — tenant-isolation-
// security.md §6) and returns the one-time name snapshot to store on
// the receipt — see Receipt.SupplierNameSnapshot's own doc. Returns
// (nil, nil) unchanged when supplierID is nil: SupplierID is optional,
// Vendor's free-text field alone is still a fully supported path.
func (s *Service) resolveSupplier(ctx context.Context, tenantID int64, supplierID *int64) (*string, error) {
	if supplierID == nil {
		return nil, nil
	}
	sup, err := s.suppliers.FindByID(ctx, tenantID, *supplierID)
	if err != nil {
		return nil, ErrSupplierNotFound
	}
	name := sup.CompanyName
	return &name, nil
}

func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
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

	if _, err := s.warehouses.FindByIDForTenant(ctx, tenantID, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}
	if _, err := s.locations.FindByID(ctx, tenantID, req.StagingLocationID); err != nil {
		return nil, ErrLocationNotFound
	}
	supplierSnapshot, err := s.resolveSupplier(ctx, tenantID, req.SupplierID)
	if err != nil {
		return nil, err
	}

	// Validation above has no durable side effect yet and is fine to
	// abort with the caller; detach only now that a transaction is
	// genuinely about to open — see internal/platform/dbctx's doc.
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var receiptID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		receipt := &Receipt{
			ReceivingType: req.ReceivingType, ReferenceNo: req.ReferenceNo, Vendor: req.Vendor,
			SupplierID: req.SupplierID, SupplierNameSnapshot: supplierSnapshot,
			InvoiceNo: req.InvoiceNo, DeliveryNoteNo: req.DeliveryNoteNo,
			ContainerNo: req.ContainerNo, SealNo: req.SealNo,
			TransportCompany: req.TransportCompany, DriverName: req.DriverName,
			VehicleRegNo: req.VehicleRegNo, DockDoor: req.DockDoor,
			ArrivalTime: req.ArrivalTime, UnloadingStartTime: req.UnloadingStartTime,
			UnloadingEndTime: req.UnloadingEndTime, Currency: req.Currency,
			WarehouseID: req.WarehouseID, StagingLocationID: req.StagingLocationID, Status: status,
			ReceivingDate: req.ReceivingDate, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateReceipt(ctx, tenantID, receipt); err != nil {
			return err
		}
		receiptID = receipt.ID

		for _, itemReq := range req.Items {
			if itemReq.LocationID == 0 {
				itemReq.LocationID = req.StagingLocationID
			}
			if err := s.createItem(ctx, txRepo, tenantID, receipt.ID, itemReq); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "receiving.create", start, err)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, tenantID, receiptID)
}

func (s *Service) Update(ctx context.Context, tenantID, id int64, req UpdateRequest) (*Response, error) {
	receipt, err := s.repo.FindReceiptByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if !receipt.Status.CanEdit() {
		return nil, ErrNotEditable
	}
	if _, err := s.locations.FindByID(ctx, tenantID, req.StagingLocationID); err != nil {
		return nil, ErrLocationNotFound
	}
	supplierSnapshot, err := s.resolveSupplier(ctx, tenantID, req.SupplierID)
	if err != nil {
		return nil, err
	}

	receipt.ReferenceNo = req.ReferenceNo
	receipt.Vendor = req.Vendor
	receipt.SupplierID = req.SupplierID
	receipt.SupplierNameSnapshot = supplierSnapshot
	receipt.InvoiceNo = req.InvoiceNo
	receipt.DeliveryNoteNo = req.DeliveryNoteNo
	receipt.ContainerNo = req.ContainerNo
	receipt.SealNo = req.SealNo
	receipt.TransportCompany = req.TransportCompany
	receipt.DriverName = req.DriverName
	receipt.VehicleRegNo = req.VehicleRegNo
	receipt.DockDoor = req.DockDoor
	receipt.ArrivalTime = req.ArrivalTime
	receipt.UnloadingStartTime = req.UnloadingStartTime
	receipt.UnloadingEndTime = req.UnloadingEndTime
	receipt.Currency = req.Currency
	receipt.StagingLocationID = req.StagingLocationID
	receipt.ReceivingDate = req.ReceivingDate
	receipt.Remark = req.Remark

	if err := s.repo.UpdateReceipt(ctx, tenantID, receipt); err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, id)
}

// AddItem is POST /receipts/{id}/items — used both by the "+ Add Item"
// button and the barcode-scan flow (which adds-or-increments a line;
// the frontend decides which by checking its own already-fetched item
// list, then calls either AddItem or UpdateItem accordingly).
func (s *Service) AddItem(ctx context.Context, tenantID, receiptID int64, req CreateItemRequest) (*Response, error) {
	receipt, err := s.repo.FindReceiptByID(ctx, tenantID, receiptID)
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
		return s.createItem(ctx, txRepo, tenantID, receiptID, req)
	})
	dbctx.LogTransaction(ctx, "receiving.add_item", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
}

func (s *Service) createItem(ctx context.Context, txRepo *Repository, tenantID, receiptID int64, req CreateItemRequest) error {
	p, err := s.products.FindByIDForTenant(ctx, tenantID, req.ProductID)
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
	if _, err := s.locations.FindByID(ctx, tenantID, req.LocationID); err != nil {
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
	return txRepo.CreateItem(ctx, tenantID, item)
}

func (s *Service) UpdateItem(ctx context.Context, tenantID, receiptID, itemID int64, req UpdateItemRequest) (*Response, error) {
	receipt, err := s.repo.FindReceiptByID(ctx, tenantID, receiptID)
	if err != nil {
		return nil, err
	}
	if !receipt.Status.CanEdit() {
		return nil, ErrNotEditable
	}

	item, err := s.repo.FindItemByID(ctx, tenantID, itemID)
	if err != nil {
		return nil, err
	}
	if item.ReceiptID != receiptID {
		return nil, ErrItemNotFound
	}
	if item.Status != ItemStatusPending {
		return nil, ErrValidation{Message: "line has already started receiving and can no longer be edited"}
	}

	p, err := s.products.FindByIDForTenant(ctx, tenantID, item.ProductID)
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
	if _, err := s.locations.FindByID(ctx, tenantID, req.LocationID); err != nil {
		return nil, ErrLocationNotFound
	}

	item.ExpectedQty = req.ExpectedQty
	item.UOM = req.UOM
	item.LotNo = req.LotNo
	item.ExpiryDate = req.ExpiryDate
	item.LocationID = req.LocationID

	if err := s.repo.UpdateItem(ctx, tenantID, item); err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
}

// resolvedSplit is one split batch after defaults (item's current lot/
// expiry/location) have been applied — used both to validate quantity
// conservation and to drive the single AddStockTx + CreateSplit pair
// Confirm issues per batch.
type resolvedSplit struct {
	Quantity        float64
	LotNo           string
	MfgDate         *time.Time
	ExpiryDate      *time.Time
	LocationID      int64
	PalletID        *string
	CartonCount     *int
	TemperatureC    *float64
	ConditionNote   *string
	CountryOfOrigin *string
}

// resolveSplits turns one ConfirmItemRequest into the list of batches to
// actually post, applying the item's current lot/expiry/location as the
// default for any field a split omits — see ConfirmItemRequest.Splits's
// own doc for the implicit-single-split behavior this preserves for
// every pre-Phase-31A request body unchanged.
func resolveSplits(lineReq ConfirmItemRequest, item *ReceiptItem) ([]resolvedSplit, error) {
	if len(lineReq.Splits) == 0 {
		lotNo := item.LotNo
		if lineReq.LotNo != "" {
			lotNo = lineReq.LotNo
		}
		expiryDate := item.ExpiryDate
		if lineReq.ExpiryDate != nil {
			expiryDate = lineReq.ExpiryDate
		}
		locationID := item.LocationID
		if lineReq.LocationID != 0 {
			locationID = lineReq.LocationID
		}
		return []resolvedSplit{{
			Quantity: lineReq.Quantity, LotNo: lotNo, ExpiryDate: expiryDate, LocationID: locationID,
		}}, nil
	}

	var sum float64
	resolved := make([]resolvedSplit, 0, len(lineReq.Splits))
	for _, sp := range lineReq.Splits {
		if sp.Quantity <= 0 {
			return nil, ErrValidation{Message: "each split quantity must be greater than zero"}
		}
		sum += sp.Quantity

		lotNo := item.LotNo
		if sp.LotNo != "" {
			lotNo = sp.LotNo
		}
		expiryDate := item.ExpiryDate
		if sp.ExpiryDate != nil {
			expiryDate = sp.ExpiryDate
		}
		locationID := item.LocationID
		if sp.LocationID != 0 {
			locationID = sp.LocationID
		}

		rs := resolvedSplit{
			Quantity: sp.Quantity, LotNo: lotNo, MfgDate: sp.MfgDate, ExpiryDate: expiryDate, LocationID: locationID,
		}
		if sp.PalletID != "" {
			rs.PalletID = &sp.PalletID
		}
		if sp.CartonCount != 0 {
			cc := sp.CartonCount
			rs.CartonCount = &cc
		}
		rs.TemperatureC = sp.TemperatureC
		if sp.ConditionNote != "" {
			rs.ConditionNote = &sp.ConditionNote
		}
		if sp.CountryOfOrigin != "" {
			rs.CountryOfOrigin = &sp.CountryOfOrigin
		}
		resolved = append(resolved, rs)
	}

	// Quantity conservation: splits must account for exactly this call's
	// quantity, not more and not less — a partial match would silently
	// under- or over-receive relative to what the caller actually
	// claimed for this line this call.
	if sum != lineReq.Quantity {
		return nil, ErrValidation{Message: "split quantities must sum to the line's confirmed quantity"}
	}
	return resolved, nil
}

// Confirm is the atomic core of this phase: validate -> update receipt
// item received_qty -> call inventory.AddStockTx (same transaction, one
// call per lot/expiry/pallet split) -> ledger row(s) written by
// AddStockTx itself -> record each split in receipt_item_splits ->
// update receipt status -> commit. Any failure rolls back everything,
// including any lines already processed earlier in this same call.
//
// Phase 31A: QC-required products (product.QCRequired) post their
// entire confirmed quantity into inventory's HoldQuantity instead of
// leaving it immediately available — see
// inventory.Inventory.Available's own doc for the full model. This is
// the Inventory Core *mechanism* only: no QC pass/release/reject
// workflow exists yet (Phase 31B), so a QC-required line's quantity
// stays held indefinitely until that later phase adds the release path.
func (s *Service) Confirm(ctx context.Context, tenantID, receiptID int64, req ConfirmRequest, userID int64) (*Response, error) {
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
		// FK-violation 500 instead of a clean 404. This SELECT has no
		// side effect, so moving it ahead of the INSERT does not weaken
		// the idempotency guarantee at all.
		receipt, err := txRepo.FindReceiptByID(ctx, tenantID, receiptID)
		if err != nil {
			return err
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the UNIQUE constraint and everything below is skipped
		// and rolled back — see migrations/0013's doc.
		if err := txRepo.InsertConfirmation(ctx, tenantID, req.IdempotencyKey, receiptID); err != nil {
			return err
		}

		if !receipt.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		invRepo := inventory.NewRepository(tx)
		refType := "RECEIVE"
		refID := receipt.ReceiptNo

		// seenPalletIDs catches a duplicate/conflicting LPN (pallet ID)
		// assignment within this one Confirm call — across every line,
		// not just within one, since the same physical pallet can never
		// legitimately hold two different splits in one receiving event.
		// A pallet ID reused across *different* receipts over time is
		// expected (pallets get emptied and reused), so this is
		// deliberately scoped to the request, not a DB-wide uniqueness
		// constraint.
		seenPalletIDs := map[string]bool{}

		for _, lineReq := range req.Items {
			if lineReq.Quantity <= 0 {
				return ErrValidation{Message: "quantity must be greater than zero for every confirmed line"}
			}

			item, err := txRepo.FindItemByID(ctx, tenantID, lineReq.ItemID)
			if err != nil {
				return err
			}
			if item.ReceiptID != receiptID {
				return ErrItemNotFound
			}
			if item.Status == ItemStatusCompleted {
				return ErrValidation{Message: fmt.Sprintf("line %d is already fully received", item.ID)}
			}

			p, err := s.products.FindByIDForTenant(ctx, tenantID, item.ProductID)
			if err != nil {
				return ErrProductNotFound
			}
			if p.SerialRequired {
				return ErrValidation{Message: fmt.Sprintf("product %s requires serial tracking, which is not yet supported by receiving", p.SKU)}
			}

			splits, err := resolveSplits(lineReq, item)
			if err != nil {
				return err
			}

			// lastSplit's lot/expiry/location becomes the line's own
			// denormalized display fields, matching the pre-existing
			// "most recent confirm call's values" meaning exactly (see
			// ReceiptItem's own doc) even when multiple splits are posted
			// in one call.
			lastSplit := splits[len(splits)-1]

			// Every split is checked, not just the last one — a product
			// requiring lot/expiry tracking must have it on EVERY batch
			// posted for it, not only whichever split happens to be
			// listed last in the request (the line-level display fields
			// above are a separate, cosmetic concern from this
			// per-batch tracking requirement).
			for i, sp := range splits {
				if p.LotRequired && sp.LotNo == "" {
					return ErrValidation{Message: fmt.Sprintf("product %s requires a lot number (line %d, split %d)", p.SKU, item.ID, i+1)}
				}
				if p.ExpiryRequired && sp.ExpiryDate == nil {
					return ErrValidation{Message: fmt.Sprintf("product %s requires an expiry date (line %d, split %d)", p.SKU, item.ID, i+1)}
				}
				if sp.PalletID != nil && *sp.PalletID != "" {
					if seenPalletIDs[*sp.PalletID] {
						return ErrValidation{Message: fmt.Sprintf("pallet/LPN %q is assigned to more than one split in this confirmation", *sp.PalletID)}
					}
					seenPalletIDs[*sp.PalletID] = true
				}
			}

			for _, sp := range splits {
				if _, err := s.locations.FindByID(ctx, tenantID, sp.LocationID); err != nil {
					return ErrLocationNotFound
				}

				// HoldQuantity (Phase 31A): a QC-required product's entire
				// posted quantity goes into inventory.hold_quantity — see
				// this method's own doc.
				var holdQty float64
				if p.QCRequired {
					holdQty = sp.Quantity
				}

				if _, err := s.inventory.AddStockTx(ctx, invRepo, inventory.StockChangeRequest{
					TenantID:  tenantID,
					ProductID: item.ProductID, LocationID: sp.LocationID, LotNo: sp.LotNo, ExpiryDate: sp.ExpiryDate,
					Quantity: sp.Quantity, HoldQuantity: holdQty, Type: inventory.TransactionReceive,
					ReferenceType: &refType, ReferenceID: &refID, CreatedBy: &userID,
				}); err != nil {
					return err
				}

				if err := txRepo.CreateSplit(ctx, tenantID, &ReceiptItemSplit{
					ReceiptID: receiptID, ReceiptItemID: item.ID,
					LotNo: sp.LotNo, MfgDate: sp.MfgDate, ExpiryDate: sp.ExpiryDate,
					Quantity: sp.Quantity, UOM: item.UOM,
					PalletID: sp.PalletID, CartonCount: sp.CartonCount, LocationID: sp.LocationID,
					TemperatureC: sp.TemperatureC, ConditionNote: sp.ConditionNote, CountryOfOrigin: sp.CountryOfOrigin,
					CreatedBy: &userID,
				}); err != nil {
					return err
				}
			}

			// Atomic guarded increment (see AdjustReceivedQty's doc) instead
			// of item.ReceivedQty += lineReq.Quantity followed by a blind
			// write-back — that read-modify-write was exactly the
			// lost-update race Phase 8's concurrency regression test found.
			// Re-read afterward so status derivation uses the row's true,
			// post-increment total, not this loop's pre-transaction snapshot.
			if _, err := txRepo.AdjustReceivedQty(ctx, tenantID, item.ID, lineReq.Quantity); err != nil {
				return err
			}
			updated, err := txRepo.FindItemByID(ctx, tenantID, item.ID)
			if err != nil {
				return err
			}
			updated.LotNo = lastSplit.LotNo
			updated.ExpiryDate = lastSplit.ExpiryDate
			updated.LocationID = lastSplit.LocationID
			updated.Status = itemStatus(updated.ExpectedQty, updated.ReceivedQty)
			if err := txRepo.UpdateItemAfterConfirm(ctx, tenantID, updated); err != nil {
				return err
			}
		}

		allItems, err := txRepo.FindItemsByReceipt(ctx, tenantID, receiptID)
		if err != nil {
			return err
		}
		receipt.Status = computeReceiptStatus(allItems)
		return txRepo.UpdateReceipt(ctx, tenantID, receipt)
	})
	dbctx.LogTransaction(ctx, "receiving.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second stock add.
		return s.Get(ctx, tenantID, receiptID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
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
func (s *Service) Cancel(ctx context.Context, tenantID, receiptID int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		receipt, err := txRepo.FindReceiptByID(ctx, tenantID, receiptID)
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
		return txRepo.CancelReceipt(ctx, tenantID, receipt)
	})
	dbctx.LogTransaction(ctx, "receiving.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
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

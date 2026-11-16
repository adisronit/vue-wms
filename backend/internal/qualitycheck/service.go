package qualitycheck

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrWarehouseNotFound = errors.New("qualitycheck: warehouse not found")
	ErrWarehouseMismatch = errors.New("qualitycheck: warehouse_id does not match the receipt's own warehouse")
	ErrNoEligibleLines   = errors.New("qualitycheck: receipt has no QC-required lines with held stock")
	ErrInvalidTransition = errors.New("qualitycheck: inspection is already completed")
)

// warehouseChecker is the narrow ownership-check interface this package
// needs — mirrors internal/receiving's own, satisfied structurally by
// *warehouse.Repository.
type warehouseChecker interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*warehouse.Warehouse, error)
}

// Service holds the QC workflow's business rules — the status machine,
// validation, and the Confirm transaction that calls internal/inventory's
// ReleaseHoldTx/RejectHoldTx instead of mutating stock itself.
//
// Why "remaining to inspect" is always live inventory.HoldQuantity, not
// a column this package tracks: PASS (ReleaseHoldTx) and REJECT
// (RejectHoldTx) both reduce HoldQuantity immediately when confirmed;
// HOLD deliberately does not touch it at all — a HOLD disposition is
// purely an audit annotation ("looked at these units, decided not to
// act yet"), so the units it covers remain exactly as inspectable as
// before. This means live HoldQuantity for an item's (product,
// location, lot) tuple is simultaneously "how much is still held" and
// "how much is still awaiting a final PASS/REJECT decision" — no
// separate running-total bookkeeping can drift from it, and
// reinspecting a previously-HELD quantity later needs no special state
// transition: it's just another Confirm call against the same (or a
// new) inspection item, same as the first.
type Service struct {
	repo       *Repository
	warehouses warehouseChecker
	inventory  *inventory.Service
}

func NewService(repo *Repository, warehouses warehouseChecker, inventorySvc *inventory.Service) *Service {
	return &Service{repo: repo, warehouses: warehouses, inventory: inventorySvc}
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
	return s.repo.ListInspections(ctx, tenantID, params)
}

// EligibleSplits exposes Repository.ListEligibleSplits through Service
// for GET /qc/eligible-splits?receipt_id=... — see that method's own
// doc. Verifies receipt ownership first so a cross-tenant receipt_id
// can never leak which of its lines need QC.
func (s *Service) EligibleSplits(ctx context.Context, tenantID, receiptID int64) ([]EligibleSplitResponse, error) {
	if _, err := s.repo.FindReceiptForTenant(ctx, tenantID, receiptID); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListEligibleSplits(ctx, tenantID, receiptID)
	if err != nil {
		return nil, err
	}
	resp := make([]EligibleSplitResponse, 0, len(rows))
	for _, r := range rows {
		resp = append(resp, EligibleSplitResponse{
			ReceiptItemSplitID: r.ReceiptItemSplitID, ProductID: r.ProductID, SKU: r.SKU, ProductName: r.ProductName,
			LotNo: r.LotNo, ExpiryDate: r.ExpiryDate, LocationID: r.LocationID, LocationCode: r.LocationCode,
			HeldQty: r.HeldQty,
		})
	}
	return resp, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	header, err := s.repo.FindInspectionDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	items, err := s.listItemResponses(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, InspectionNo: header.InspectionNo, ReceiptID: header.ReceiptID, ReceiptNo: header.ReceiptNo,
		WarehouseID: header.WarehouseID, WarehouseCode: header.WarehouseCode,
		InspectorID: header.InspectorID, InspectorName: header.InspectorName,
		QCType: QCType(header.QCType), Status: Status(header.Status), Remark: header.Remark,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
		Items: items,
	}, nil
}

func (s *Service) listItemResponses(ctx context.Context, tenantID, inspectionID int64) ([]ItemResponse, error) {
	rows, err := s.repo.ListItemsJoined(ctx, tenantID, inspectionID)
	if err != nil {
		return nil, err
	}
	dispositionsByItem, err := s.repo.ListDispositionsByInspection(ctx, tenantID, inspectionID)
	if err != nil {
		return nil, err
	}
	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ReceiptItemSplitID: row.ReceiptItemSplitID, ProductID: row.ProductID,
			SKU: row.SKU, ProductName: row.ProductName, LotNo: row.LotNo, ExpiryDate: row.ExpiryDate,
			LocationID: row.LocationID, LocationCode: row.LocationCode,
			HeldQtySnapshot: row.HeldQtySnapshot, RemainingHeldQty: row.RemainingHeldQty, SampleQty: row.SampleQty,
			TemperatureC: row.TemperatureC, PackagingOK: row.PackagingOK, LabelOK: row.LabelOK,
			Status: ItemStatus(row.Status), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			Dispositions: dispositionsByItem[row.ID],
		})
	}
	return items, nil
}

// Create is POST /qc/inspections — auto-populates one InspectionItem
// per eligible receipt_item_splits line (see EligibleSplits' own doc),
// so the caller only ever chooses a receipt, never hand-picks lines.
func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
	qcType := req.QCType
	if qcType == "" {
		qcType = TypeNormal
	}
	if !qcType.IsValid() {
		return nil, ErrValidation{Message: "qc_type must be one of NORMAL, FULL, RANDOM, DAMAGE"}
	}

	if _, err := s.warehouses.FindByIDForTenant(ctx, tenantID, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}
	receipt, err := s.repo.FindReceiptForTenant(ctx, tenantID, req.ReceiptID)
	if err != nil {
		return nil, err
	}
	if receipt.WarehouseID != req.WarehouseID {
		return nil, ErrWarehouseMismatch
	}

	eligible, err := s.repo.ListEligibleSplits(ctx, tenantID, req.ReceiptID)
	if err != nil {
		return nil, err
	}
	if len(eligible) == 0 {
		return nil, ErrNoEligibleLines
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var inspectionID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		insp := &Inspection{
			ReceiptID: req.ReceiptID, WarehouseID: req.WarehouseID, InspectorID: req.InspectorID,
			QCType: qcType, Status: StatusPending, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateInspection(ctx, tenantID, insp); err != nil {
			return err
		}
		inspectionID = insp.ID

		for _, split := range eligible {
			item := &InspectionItem{
				InspectionID: insp.ID, ReceiptItemSplitID: split.ReceiptItemSplitID, ProductID: split.ProductID,
				LotNo: split.LotNo, ExpiryDate: split.ExpiryDate, LocationID: split.LocationID,
				HeldQtySnapshot: split.HeldQty, Status: ItemStatusPending,
			}
			if err := txRepo.CreateItem(ctx, tenantID, item); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "qualitycheck.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, inspectionID)
}

// UpdateItem is PUT /qc/inspections/{id}/items/{itemId} — records the
// checklist/sample fields before a disposition is confirmed. Never
// touches inventory.
func (s *Service) UpdateItem(ctx context.Context, tenantID, inspectionID, itemID int64, req UpdateItemRequest) (*Response, error) {
	if _, err := s.repo.FindInspectionByID(ctx, tenantID, inspectionID); err != nil {
		return nil, err
	}
	item, err := s.repo.FindItemByID(ctx, tenantID, itemID)
	if err != nil {
		return nil, err
	}
	if item.InspectionID != inspectionID {
		return nil, ErrItemNotFound
	}

	item.SampleQty = req.SampleQty
	item.TemperatureC = req.TemperatureC
	item.PackagingOK = req.PackagingOK
	item.LabelOK = req.LabelOK
	if err := s.repo.UpdateItemFields(ctx, tenantID, item); err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, inspectionID)
}

// Confirm is the atomic core of this phase: validate -> for each
// disposition entry, call the matching Inventory Core *Tx method (PASS
// -> ReleaseHoldTx, REJECT -> RejectHoldTx, HOLD -> no inventory call at
// all) in the SAME transaction -> record the qc_dispositions row ->
// recompute item/inspection status -> commit. Any failure rolls back
// everything, including any entries already processed earlier in this
// same call — identical shape to internal/receiving's own Confirm.
func (s *Service) Confirm(ctx context.Context, tenantID, inspectionID int64, req ConfirmRequest, userID int64) (*Response, error) {
	if len(req.Dispositions) == 0 {
		return nil, ErrValidation{Message: "at least one disposition is required to confirm"}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		insp, err := txRepo.FindInspectionByID(ctx, tenantID, inspectionID)
		if err != nil {
			return err
		}
		if insp.Status == StatusCompleted {
			return ErrInvalidTransition
		}

		// Idempotency guard: if this key was already used, the INSERT
		// fails on the UNIQUE constraint and everything below is skipped
		// and rolled back — see migrations/0073's doc.
		if err := txRepo.InsertConfirmation(ctx, tenantID, req.IdempotencyKey, inspectionID); err != nil {
			return err
		}

		invRepo := inventory.NewRepository(tx)
		refType := "QC_REJECT"
		touchedItems := map[int64]bool{}

		for _, entry := range req.Dispositions {
			if !entry.Disposition.IsValid() {
				return ErrValidation{Message: "disposition must be one of PASS, HOLD, REJECT"}
			}
			if entry.Quantity <= 0 {
				return ErrValidation{Message: "quantity must be greater than zero for every disposition"}
			}

			item, err := txRepo.FindItemByID(ctx, tenantID, entry.ItemID)
			if err != nil {
				return err
			}
			if item.InspectionID != inspectionID {
				return ErrItemNotFound
			}

			switch entry.Disposition {
			case DispositionPass:
				if _, err := s.inventory.ReleaseHoldTx(ctx, invRepo, inventory.ReserveRequest{
					TenantID: tenantID, ProductID: item.ProductID, LocationID: item.LocationID, LotNo: item.LotNo,
					Quantity: entry.Quantity,
				}); err != nil {
					return err
				}
			case DispositionReject:
				refID := insp.InspectionNo
				if _, err := s.inventory.RejectHoldTx(ctx, invRepo, inventory.StockChangeRequest{
					TenantID: tenantID, ProductID: item.ProductID, LocationID: item.LocationID, LotNo: item.LotNo,
					Quantity: entry.Quantity, ReferenceType: &refType, ReferenceID: &refID, CreatedBy: &userID,
				}); err != nil {
					return err
				}
			case DispositionHold:
				// No inventory call — see Service's own doc for why a
				// HOLD disposition is a pure audit annotation that never
				// touches HoldQuantity.
			}

			var defectCode, evidenceReference, remark *string
			if entry.DefectCode != "" {
				defectCode = &entry.DefectCode
			}
			if entry.EvidenceReference != "" {
				evidenceReference = &entry.EvidenceReference
			}
			if entry.Remark != "" {
				remark = &entry.Remark
			}
			if err := txRepo.CreateDisposition(ctx, tenantID, &InspectionDisposition{
				InspectionItemID: item.ID, Disposition: entry.Disposition, Quantity: entry.Quantity,
				DefectCode: defectCode, EvidenceReference: evidenceReference, Remark: remark, DecidedBy: &userID,
			}); err != nil {
				return err
			}
			touchedItems[item.ID] = true
		}

		// Recompute status only for items this call actually touched —
		// an untouched item's status is unaffected by this Confirm call.
		for itemID := range touchedItems {
			item, err := txRepo.FindItemByID(ctx, tenantID, itemID)
			if err != nil {
				return err
			}
			bal, err := invRepo.FindBalance(ctx, tenantID, item.ProductID, item.LocationID, item.LotNo)
			remaining := 0.0
			if err == nil {
				remaining = bal.HoldQuantity
			} else if !errors.Is(err, inventory.ErrNotFound) {
				return err
			}
			if err := txRepo.UpdateItemStatus(ctx, tenantID, itemID, itemStatus(remaining)); err != nil {
				return err
			}
		}

		allItems, err := txRepo.FindItemsByInspection(ctx, tenantID, inspectionID)
		if err != nil {
			return err
		}
		return txRepo.UpdateInspectionStatus(ctx, tenantID, inspectionID, computeInspectionStatus(allItems))
	})
	dbctx.LogTransaction(ctx, "qualitycheck.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a second disposition.
		return s.Get(ctx, tenantID, inspectionID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, inspectionID)
}

// itemStatus derives one inspection item's status from the live
// remaining-held quantity for its tuple: 0 held left -> COMPLETED
// (every unit has a final PASS/REJECT decision); some held but a
// disposition already exists (this function is only ever called after
// recording one) -> IN_PROGRESS. A freshly-created item with no
// disposition yet keeps its DB-default PENDING, never recomputed by
// this function.
func itemStatus(remainingHeldQty float64) ItemStatus {
	if remainingHeldQty <= 0 {
		return ItemStatusCompleted
	}
	return ItemStatusInProgress
}

// computeInspectionStatus derives the header status from its items:
// every item COMPLETED -> COMPLETED; anything touched (not PENDING) ->
// IN_PROGRESS; nothing touched yet -> PENDING. Mirrors
// internal/receiving's own computeReceiptStatus shape exactly.
func computeInspectionStatus(items []InspectionItem) Status {
	if len(items) == 0 {
		return StatusPending
	}
	anyStarted := false
	allComplete := true
	for _, it := range items {
		if it.Status != ItemStatusPending {
			anyStarted = true
		}
		if it.Status != ItemStatusCompleted {
			allComplete = false
		}
	}
	if allComplete {
		return StatusCompleted
	}
	if anyStarted {
		return StatusInProgress
	}
	return StatusPending
}

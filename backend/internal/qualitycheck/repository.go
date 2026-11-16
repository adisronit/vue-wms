package qualitycheck

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("qualitycheck: not found")
var ErrItemNotFound = errors.New("qualitycheck: item not found")
var ErrReceiptNotFound = errors.New("qualitycheck: receipt not found")
var ErrDuplicateIdempotencyKey = errors.New("qualitycheck: idempotency key already used")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the confirm workflow) live in Service. Every method takes
// an explicit tenantID and applies it to every query, matching every
// Phase 30/31A inbound/outbound module's own discipline — this package
// was built tenant-scoped from day one, unlike Receiving/Picking/Order/
// Shipping, which each had their own dedicated tenant-isolation batch
// because they predated Phase 29.
//
// Reads receipts/receipt_items/receipt_item_splits/products/locations
// directly via raw SQL rather than importing internal/receiving's Go
// API — QC needs a fundamentally different query shape (which lines
// still have inventory.HoldQuantity > 0 and belong to a QC-required
// product) that doesn't belong in Receiving's own domain, the same
// "cross-cutting read, not a package dependency" pattern
// internal/dashboard already uses against Receiving/Picking/Inventory.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both
// the raw *gorm.DB (so callers can construct an inventory.Repository
// bound to the exact same transaction — see service.go's Confirm) and a
// Repository bound to it for this package's own queries.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

type receiptRow struct {
	ID          int64
	ReceiptNo   string
	WarehouseID int64
	TenantID    int64
}

// FindReceiptForTenant verifies receiptID belongs to tenantID and
// returns the fields Service.Create needs (warehouse_id to cross-check
// against the request, receipt_no for display) — never "found but
// belongs to another tenant" vs. "doesn't exist at all"
// (tenant-isolation-security.md §6).
func (r *Repository) FindReceiptForTenant(ctx context.Context, tenantID, receiptID int64) (*receiptRow, error) {
	var row receiptRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, receipt_no, warehouse_id, tenant_id FROM receipts WHERE id = ? AND tenant_id = ?
	`, receiptID, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrReceiptNotFound
	}
	return &row, nil
}

type eligibleSplitRow struct {
	ReceiptItemSplitID int64
	ProductID          int64
	SKU                string
	ProductName        string
	LotNo              string
	ExpiryDate         *time.Time
	LocationID         int64
	LocationCode       string
	HeldQty            float64
}

// ListEligibleSplits returns every receipt_item_splits line on
// receiptID whose product requires QC (products.qc_required = 1) and
// still carries inventory.HoldQuantity > 0 — the exact candidate set
// Service.Create turns into InspectionItem rows, and what GET
// /qc/eligible-splits?receipt_id=... shows the "Receiving No." picker.
// A split already fully resolved by an earlier inspection (hold reached
// 0) is naturally excluded without any extra bookkeeping — see
// Service's own doc for why live HoldQuantity is the single source of
// truth for "remaining to inspect."
func (r *Repository) ListEligibleSplits(ctx context.Context, tenantID, receiptID int64) ([]eligibleSplitRow, error) {
	var rows []eligibleSplitRow
	// receipt_item_splits (migrations/0069) has no product_id column of
	// its own — product_id lives on its parent receipt_items row (the
	// same denormalization internal/receiving's own ReceiptItemSplit
	// doc describes for receipt_id) — so this must join through
	// receipt_items to reach it and to products.qc_required.
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS receipt_item_split_id, ri.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			s.lot_no AS lot_no, s.expiry_date AS expiry_date,
			s.location_id AS location_id, l.code AS location_code,
			ISNULL(inv.hold_quantity, 0) AS held_qty
		FROM receipt_item_splits s
		JOIN receipt_items ri ON ri.id = s.receipt_item_id
		JOIN products p ON p.id = ri.product_id
		JOIN locations l ON l.id = s.location_id
		LEFT JOIN inventory inv ON inv.tenant_id = s.tenant_id AND inv.product_id = ri.product_id
			AND inv.location_id = s.location_id AND inv.lot_no = s.lot_no
		WHERE s.receipt_id = ? AND s.tenant_id = ? AND p.qc_required = 1 AND ISNULL(inv.hold_quantity, 0) > 0
		ORDER BY s.id ASC
	`, receiptID, tenantID).Scan(&rows).Error
	return rows, err
}

// CreateInspection inserts the header, then derives inspection_no from
// the new IDENTITY id in the same transaction — the exact same
// race-free pattern receipts.receipt_no (internal/receiving) uses.
func (r *Repository) CreateInspection(ctx context.Context, tenantID int64, insp *Inspection) error {
	insp.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(insp).Error; err != nil {
		return err
	}
	insp.InspectionNo = fmt.Sprintf("QC-%06d", insp.ID)
	return r.db.WithContext(ctx).Model(&Inspection{}).Where("id = ? AND tenant_id = ?", insp.ID, tenantID).
		Update("inspection_no", insp.InspectionNo).Error
}

func (r *Repository) FindInspectionByID(ctx context.Context, tenantID, id int64) (*Inspection, error) {
	var insp Inspection
	err := r.db.WithContext(ctx).First(&insp, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &insp, nil
}

type headerRow struct {
	ID            int64
	InspectionNo  string
	ReceiptID     int64
	ReceiptNo     string
	WarehouseID   int64
	WarehouseCode string
	InspectorID   *int64
	InspectorName *string
	QCType        string
	Status        string
	Remark        *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// FindInspectionDetail returns the header with receipt/warehouse/
// inspector display fields resolved, for GET /qc/inspections/{id}'s
// response — mirrors internal/receiving.Repository.FindReceiptDetail's
// own shape exactly.
func (r *Repository) FindInspectionDetail(ctx context.Context, tenantID, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT qi.id AS id, qi.inspection_no AS inspection_no, qi.receipt_id AS receipt_id, rc.receipt_no AS receipt_no,
			qi.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			qi.inspector_id AS inspector_id, iu.full_name AS inspector_name,
			qi.qc_type AS qc_type, qi.status AS status, qi.remark AS remark,
			qi.created_at AS created_at, qi.updated_at AS updated_at
		FROM qc_inspections qi
		JOIN receipts rc ON rc.id = qi.receipt_id
		JOIN warehouses w ON w.id = qi.warehouse_id
		LEFT JOIN users iu ON iu.id = qi.inspector_id
		WHERE qi.id = ? AND qi.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

type listRow struct {
	ID            int64
	InspectionNo  string
	ReceiptNo     string
	WarehouseCode string
	QCType        string
	Status        string
	ItemCount     int
	CreatedAt     time.Time
}

func (r *Repository) ListInspections(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM qc_inspections qi
		JOIN receipts rc ON rc.id = qi.receipt_id
		JOIN warehouses w ON w.id = qi.warehouse_id
		LEFT JOIN qc_inspection_items qii ON qii.inspection_id = qi.id
	`
	where := `WHERE qi.tenant_id = ?
		AND (? = '' OR qi.inspection_no LIKE ? OR rc.receipt_no LIKE ?)
		AND (? = '' OR qi.status = ?)
		AND (? = 0 OR qi.warehouse_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like,
		string(params.Status), string(params.Status),
		params.WarehouseID, params.WarehouseID,
	}

	var total int64
	countSQL := "SELECT COUNT(DISTINCT qi.id) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT qi.id AS id, qi.inspection_no AS inspection_no, rc.receipt_no AS receipt_no,
			w.code AS warehouse_code, qi.qc_type AS qc_type, qi.status AS status,
			COUNT(qii.id) AS item_count, qi.created_at AS created_at
	` + base + where + `
		GROUP BY qi.id, qi.inspection_no, rc.receipt_no, w.code, qi.qc_type, qi.status, qi.created_at
		ORDER BY qi.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			ID: row.ID, InspectionNo: row.InspectionNo, ReceiptNo: row.ReceiptNo,
			WarehouseCode: row.WarehouseCode, QCType: QCType(row.QCType), Status: Status(row.Status),
			ItemCount: row.ItemCount, CreatedAt: row.CreatedAt,
		})
	}
	return responses, total, nil
}

func (r *Repository) CreateItem(ctx context.Context, tenantID int64, item *InspectionItem) error {
	item.TenantID = tenantID
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, tenantID, id int64) (*InspectionItem, error) {
	var item InspectionItem
	err := r.db.WithContext(ctx).First(&item, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FindItemsByInspection returns the raw InspectionItem rows (not the
// joined response shape) — used internally by Confirm/Service to
// recompute the header's status after applying dispositions.
func (r *Repository) FindItemsByInspection(ctx context.Context, tenantID, inspectionID int64) ([]InspectionItem, error) {
	var items []InspectionItem
	err := r.db.WithContext(ctx).Where("inspection_id = ? AND tenant_id = ?", inspectionID, tenantID).Find(&items).Error
	return items, err
}

type itemJoinRow struct {
	ID                 int64
	ReceiptItemSplitID int64
	ProductID          int64
	SKU                string
	ProductName        string
	LotNo              string
	ExpiryDate         *time.Time
	LocationID         int64
	LocationCode       string
	HeldQtySnapshot    float64
	RemainingHeldQty   float64
	SampleQty          float64
	TemperatureC       *float64
	PackagingOK        *bool
	LabelOK            *bool
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ListItemsJoined returns every item on inspectionID with product/
// location display fields and the live remaining-held quantity
// resolved in one query (LEFT JOIN inventory on the item's own tuple)
// — see Service's own doc for why that live value, not
// held_qty_snapshot, is authoritative.
func (r *Repository) ListItemsJoined(ctx context.Context, tenantID, inspectionID int64) ([]itemJoinRow, error) {
	var rows []itemJoinRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT qii.id AS id, qii.receipt_item_split_id AS receipt_item_split_id, qii.product_id AS product_id,
			p.sku AS sku, p.name AS product_name,
			qii.lot_no AS lot_no, qii.expiry_date AS expiry_date,
			qii.location_id AS location_id, l.code AS location_code,
			qii.held_qty_snapshot AS held_qty_snapshot,
			ISNULL(inv.hold_quantity, 0) AS remaining_held_qty,
			qii.sample_qty AS sample_qty, qii.temperature_c AS temperature_c,
			qii.packaging_ok AS packaging_ok, qii.label_ok AS label_ok,
			qii.status AS status, qii.created_at AS created_at, qii.updated_at AS updated_at
		FROM qc_inspection_items qii
		JOIN products p ON p.id = qii.product_id
		JOIN locations l ON l.id = qii.location_id
		LEFT JOIN inventory inv ON inv.tenant_id = qii.tenant_id AND inv.product_id = qii.product_id
			AND inv.location_id = qii.location_id AND inv.lot_no = qii.lot_no
		WHERE qii.inspection_id = ? AND qii.tenant_id = ?
		ORDER BY qii.id ASC
	`, inspectionID, tenantID).Scan(&rows).Error
	return rows, err
}

func (r *Repository) UpdateItemFields(ctx context.Context, tenantID int64, item *InspectionItem) error {
	result := r.db.WithContext(ctx).Model(&InspectionItem{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"sample_qty":    item.SampleQty,
		"temperature_c": item.TemperatureC,
		"packaging_ok":  item.PackagingOK,
		"label_ok":      item.LabelOK,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

func (r *Repository) UpdateItemStatus(ctx context.Context, tenantID, itemID int64, status ItemStatus) error {
	result := r.db.WithContext(ctx).Model(&InspectionItem{}).Where("id = ? AND tenant_id = ?", itemID, tenantID).
		Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

func (r *Repository) UpdateInspectionStatus(ctx context.Context, tenantID, inspectionID int64, status Status) error {
	result := r.db.WithContext(ctx).Model(&Inspection{}).Where("id = ? AND tenant_id = ?", inspectionID, tenantID).
		Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) CreateDisposition(ctx context.Context, tenantID int64, d *InspectionDisposition) error {
	d.TenantID = tenantID
	return r.db.WithContext(ctx).Create(d).Error
}

type dispositionRow struct {
	ID                int64
	InspectionItemID  int64
	Disposition       string
	Quantity          float64
	DefectCode        *string
	EvidenceReference *string
	Remark            *string
	DecidedBy         *int64
	DecidedByName     *string
	CreatedAt         time.Time
}

// ListDispositionsByInspection returns every disposition row for every
// item on inspectionID, grouped by inspection_item_id — one query for
// the whole inspection rather than one per item.
func (r *Repository) ListDispositionsByInspection(ctx context.Context, tenantID, inspectionID int64) (map[int64][]DispositionResponse, error) {
	var rows []dispositionRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT d.id AS id, d.inspection_item_id AS inspection_item_id, d.disposition AS disposition,
			d.quantity AS quantity, d.defect_code AS defect_code, d.evidence_reference AS evidence_reference,
			d.remark AS remark, d.decided_by AS decided_by, u.full_name AS decided_by_name, d.created_at AS created_at
		FROM qc_dispositions d
		JOIN qc_inspection_items qii ON qii.id = d.inspection_item_id
		LEFT JOIN users u ON u.id = d.decided_by
		WHERE qii.inspection_id = ? AND d.tenant_id = ?
		ORDER BY d.id ASC
	`, inspectionID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	byItem := make(map[int64][]DispositionResponse, len(rows))
	for _, row := range rows {
		byItem[row.InspectionItemID] = append(byItem[row.InspectionItemID], DispositionResponse{
			ID: row.ID, Disposition: Disposition(row.Disposition), Quantity: row.Quantity,
			DefectCode: row.DefectCode, EvidenceReference: row.EvidenceReference, Remark: row.Remark,
			DecidedBy: row.DecidedBy, DecidedByName: row.DecidedByName, CreatedAt: row.CreatedAt,
		})
	}
	return byItem, nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0073's doc. Must be called inside the same transaction as
// the stock mutations it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, inspectionID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, TenantID: tenantID, InspectionID: inspectionID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

package receiving

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("receiving: not found")
var ErrItemNotFound = errors.New("receiving: item not found")
var ErrDuplicateIdempotencyKey = errors.New("receiving: idempotency key already used")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the confirm workflow) live in Service.
//
// Phase 31A: every method below now takes an explicit tenantID and
// applies it to every query — Receiving was the last inbound/outbound
// workflow module with zero tenant parameters anywhere (every other
// module had at least started Phase 30; see phase27-audit-report.md's
// own Receiving row). Mirrors internal/order/internal/picking/
// internal/shipping's own Phase 30 Batch 2-4 discipline exactly.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both the
// raw *gorm.DB (so callers can construct an inventory.Repository bound
// to the exact same transaction — see service.go's Confirm) and a
// Repository bound to it for receiving's own queries.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateReceipt inserts the header, then derives receipt_no from the
// new IDENTITY id in the same transaction — see migrations/0011's doc
// for why this makes uniqueness race-free without a separate sequence.
func (r *Repository) CreateReceipt(ctx context.Context, tenantID int64, receipt *Receipt) error {
	receipt.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(receipt).Error; err != nil {
		return err
	}
	receipt.ReceiptNo = fmt.Sprintf("RCV-%06d", receipt.ID)
	return r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ? AND tenant_id = ?", receipt.ID, tenantID).
		Update("receipt_no", receipt.ReceiptNo).Error
}

// FindReceiptByID returns id's row scoped to tenantID, or ErrNotFound —
// never "found but belongs to another tenant" vs. "doesn't exist at
// all" (tenant-isolation-security.md §6).
func (r *Repository) FindReceiptByID(ctx context.Context, tenantID, id int64) (*Receipt, error) {
	var rec Receipt
	err := r.db.WithContext(ctx).First(&rec, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

type headerRow struct {
	ID                 int64
	ReceiptNo          string
	ReceivingType      string
	ReferenceNo        *string
	Vendor             *string
	SupplierID         *int64
	SupplierName       *string
	InvoiceNo          *string
	DeliveryNoteNo     *string
	ContainerNo        *string
	SealNo             *string
	TransportCompany   *string
	DriverName         *string
	VehicleRegNo       *string
	DockDoor           *string
	ArrivalTime        *time.Time
	UnloadingStartTime *time.Time
	UnloadingEndTime   *time.Time
	Currency           *string
	WarehouseID        int64
	WarehouseCode      string
	StagingLocationID  int64
	StagingLocation    string
	Status             string
	ReceivingDate      *time.Time
	Remark             *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CancelledAt        *time.Time
	CancelledBy        *int64
	CancelledByName    *string
	CancelRemark       *string
}

// FindReceiptDetail returns the header with warehouse/location/supplier
// codes resolved, for GET /receipts/{id}'s response — a separate query
// from FindReceiptByID (which returns the plain GORM model, used
// internally by Update/Confirm where the joined display fields aren't
// needed). The LEFT JOIN to users (Phase 18) resolves cancelled_by to a
// display name, the same pattern Phase 14 established for
// created_by_name on stock counts.
//
// SupplierName (Phase 31A) resolves to the LIVE supplier's company_name
// when supplier_id is still set and that row still exists, falling back
// to the stored supplier_name_snapshot otherwise (e.g. the supplier was
// deleted after this receipt was created) — COALESCE expresses that
// fallback in SQL rather than two round trips. This intentionally
// differs from Customer Ship-to's pure one-time-snapshot display
// ([[wms_customer_shipto_order_integration]]): Suppliers have no
// "INACTIVE but still linkable" status gate the way delivery locations
// do, so showing the live name when available is more useful than a
// frozen one, while the snapshot still guarantees a sensible display if
// the supplier itself is later deleted.
func (r *Repository) FindReceiptDetail(ctx context.Context, tenantID, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT rc.id AS id, rc.receipt_no AS receipt_no, rc.receiving_type AS receiving_type,
			rc.reference_no AS reference_no, rc.vendor AS vendor,
			rc.supplier_id AS supplier_id, COALESCE(s.company_name, rc.supplier_name_snapshot) AS supplier_name,
			rc.invoice_no AS invoice_no, rc.delivery_note_no AS delivery_note_no,
			rc.container_no AS container_no, rc.seal_no AS seal_no,
			rc.transport_company AS transport_company, rc.driver_name AS driver_name,
			rc.vehicle_reg_no AS vehicle_reg_no, rc.dock_door AS dock_door,
			rc.arrival_time AS arrival_time, rc.unloading_start_time AS unloading_start_time,
			rc.unloading_end_time AS unloading_end_time, rc.currency AS currency,
			rc.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			rc.staging_location_id AS staging_location_id, l.code AS staging_location,
			rc.status AS status, rc.receiving_date AS receiving_date, rc.remark AS remark,
			rc.created_at AS created_at, rc.updated_at AS updated_at,
			rc.cancelled_at AS cancelled_at, rc.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			rc.cancel_remark AS cancel_remark
		FROM receipts rc
		JOIN warehouses w ON w.id = rc.warehouse_id
		JOIN locations l ON l.id = rc.staging_location_id
		LEFT JOIN suppliers s ON s.id = rc.supplier_id
		LEFT JOIN users cu ON cu.id = rc.cancelled_by
		WHERE rc.id = ? AND rc.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *Repository) UpdateReceipt(ctx context.Context, tenantID int64, receipt *Receipt) error {
	result := r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ? AND tenant_id = ?", receipt.ID, tenantID).Updates(map[string]any{
		"reference_no":           receipt.ReferenceNo,
		"vendor":                 receipt.Vendor,
		"supplier_id":            receipt.SupplierID,
		"supplier_name_snapshot": receipt.SupplierNameSnapshot,
		"invoice_no":             receipt.InvoiceNo,
		"delivery_note_no":       receipt.DeliveryNoteNo,
		"container_no":           receipt.ContainerNo,
		"seal_no":                receipt.SealNo,
		"transport_company":      receipt.TransportCompany,
		"driver_name":            receipt.DriverName,
		"vehicle_reg_no":         receipt.VehicleRegNo,
		"dock_door":              receipt.DockDoor,
		"arrival_time":           receipt.ArrivalTime,
		"unloading_start_time":   receipt.UnloadingStartTime,
		"unloading_end_time":     receipt.UnloadingEndTime,
		"currency":               receipt.Currency,
		"staging_location_id":    receipt.StagingLocationID,
		"receiving_date":         receipt.ReceivingDate,
		"remark":                 receipt.Remark,
		"status":                 receipt.Status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CancelReceipt writes the exact columns Cancel derives — status plus
// the three Phase 18 audit columns — in one explicit UPDATE, deliberately
// separate from UpdateReceipt (which has no business ever touching
// cancellation columns). Never called except from a Cancel that has
// already passed Status.CanCancel, so cancelled_at/cancelled_by/
// cancel_remark are only ever written once per receipt.
func (r *Repository) CancelReceipt(ctx context.Context, tenantID int64, receipt *Receipt) error {
	result := r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ? AND tenant_id = ?", receipt.ID, tenantID).Updates(map[string]any{
		"status":        receipt.Status,
		"cancelled_at":  receipt.CancelledAt,
		"cancelled_by":  receipt.CancelledBy,
		"cancel_remark": receipt.CancelRemark,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

type listRow struct {
	ID            int64
	ReceiptNo     string
	ReferenceNo   *string
	Vendor        *string
	WarehouseCode string
	ReceivingDate *time.Time
	Status        string
	ItemCount     int
	ExpectedTotal float64
	ReceivedTotal float64
}

func (r *Repository) ListReceipts(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM receipts rc
		JOIN warehouses w ON w.id = rc.warehouse_id
		LEFT JOIN receipt_items ri ON ri.receipt_id = rc.id
	`
	where := `WHERE rc.tenant_id = ?
		AND (? = '' OR rc.receipt_no LIKE ? OR rc.reference_no LIKE ? OR rc.vendor LIKE ?)
		AND (? = '' OR rc.status = ?)
		AND (? = 0 OR rc.warehouse_id = ?)
		AND (? IS NULL OR rc.receiving_date = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like,
		string(params.Status), string(params.Status),
		params.WarehouseID, params.WarehouseID,
		params.Date, params.Date,
	}

	var total int64
	countSQL := "SELECT COUNT(DISTINCT rc.id) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT rc.id AS id, rc.receipt_no AS receipt_no, rc.reference_no AS reference_no, rc.vendor AS vendor,
			w.code AS warehouse_code, rc.receiving_date AS receiving_date, rc.status AS status,
			COUNT(ri.id) AS item_count,
			COALESCE(SUM(ri.expected_qty), 0) AS expected_total,
			COALESCE(SUM(ri.received_qty), 0) AS received_total
	` + base + where + `
		GROUP BY rc.id, rc.receipt_no, rc.reference_no, rc.vendor, w.code, rc.receiving_date, rc.status
		ORDER BY rc.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		progress := 0.0
		if row.ExpectedTotal > 0 {
			progress = (row.ReceivedTotal / row.ExpectedTotal) * 100
		} else if row.ReceivedTotal > 0 {
			progress = 100
		}
		responses = append(responses, ListResponse{
			ID: row.ID, ReceiptNo: row.ReceiptNo, ReferenceNo: row.ReferenceNo, Vendor: row.Vendor,
			WarehouseCode: row.WarehouseCode, ReceivingDate: row.ReceivingDate, ItemCount: row.ItemCount,
			ProgressPct: progress, Status: Status(row.Status),
		})
	}
	return responses, total, nil
}

// CountOpen returns how many receipts, within tenantID, are still in a
// status Status.CanConfirm() would accept (DRAFT/WAITING/
// PARTIALLY_RECEIVED) — Phase 23's Dashboard "Pending Receiving" card.
func (r *Repository) CountOpen(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Receipt{}).
		Where("tenant_id = ? AND status IN ?", tenantID, []string{string(StatusDraft), string(StatusWaiting), string(StatusPartiallyReceived)}).
		Count(&count).Error
	return count, err
}

type itemRow struct {
	ID          int64
	ProductID   int64
	SKU         string
	ProductName string
	Barcode     *string
	ExpectedQty float64
	ReceivedQty float64
	UOM         string
	LotNo       string
	ExpiryDate  *time.Time
	LocationID  int64
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Repository) ListItems(ctx context.Context, tenantID, receiptID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT ri.id AS id, ri.product_id AS product_id, p.sku AS sku, p.name AS product_name, p.barcode AS barcode,
			ri.expected_qty AS expected_qty, ri.received_qty AS received_qty, ri.uom AS uom,
			ri.lot_no AS lot_no, ri.expiry_date AS expiry_date, ri.location_id AS location_id,
			ri.status AS status, ri.created_at AS created_at, ri.updated_at AS updated_at
		FROM receipt_items ri
		JOIN products p ON p.id = ri.product_id
		WHERE ri.receipt_id = ? AND ri.tenant_id = ?
		ORDER BY ri.id ASC
	`, receiptID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	splitsByItem, err := r.listSplitsByReceipt(ctx, tenantID, receiptID)
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName, Barcode: row.Barcode,
			ExpectedQty: row.ExpectedQty, ReceivedQty: row.ReceivedQty, UOM: row.UOM, LotNo: row.LotNo,
			ExpiryDate: row.ExpiryDate, LocationID: row.LocationID, Status: ItemStatus(row.Status),
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			Splits: splitsByItem[row.ID],
		})
	}
	return items, nil
}

func (r *Repository) CreateItem(ctx context.Context, tenantID int64, item *ReceiptItem) error {
	item.TenantID = tenantID
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, tenantID, id int64) (*ReceiptItem, error) {
	var item ReceiptItem
	err := r.db.WithContext(ctx).First(&item, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FindItemsByReceipt returns the raw ReceiptItem rows (not the joined
// response shape) — used internally by Confirm to compute the header's
// new status after applying deltas.
func (r *Repository) FindItemsByReceipt(ctx context.Context, tenantID, receiptID int64) ([]ReceiptItem, error) {
	var items []ReceiptItem
	err := r.db.WithContext(ctx).Where("receipt_id = ? AND tenant_id = ?", receiptID, tenantID).Find(&items).Error
	return items, err
}

func (r *Repository) UpdateItem(ctx context.Context, tenantID int64, item *ReceiptItem) error {
	result := r.db.WithContext(ctx).Model(&ReceiptItem{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"expected_qty": item.ExpectedQty,
		"received_qty": item.ReceivedQty,
		"uom":          item.UOM,
		"lot_no":       item.LotNo,
		"expiry_date":  item.ExpiryDate,
		"location_id":  item.LocationID,
		"status":       item.Status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// AdjustReceivedQty atomically increments a line's received_qty by delta
// in one UPDATE statement — the arithmetic happens inside the database,
// not as a Go read-then-write, the same concurrency-safety pattern as
// internal/inventory.Repository.AdjustBalance. See PROJECT_STATUS.md's
// Phase 9 note for the lost-update bug this fixed.
func (r *Repository) AdjustReceivedQty(ctx context.Context, tenantID, itemID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE receipt_items
		SET received_qty = received_qty + ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ?
	`, delta, itemID, tenantID)
	return result.RowsAffected, result.Error
}

// UpdateItemAfterConfirm writes the fields Confirm derives after calling
// AdjustReceivedQty — deliberately excludes received_qty itself (unlike
// UpdateItem above) so this write can never clobber a concurrent
// transaction's own atomic increment with a value computed from a
// stale read.
func (r *Repository) UpdateItemAfterConfirm(ctx context.Context, tenantID int64, item *ReceiptItem) error {
	result := r.db.WithContext(ctx).Model(&ReceiptItem{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"lot_no":      item.LotNo,
		"expiry_date": item.ExpiryDate,
		"location_id": item.LocationID,
		"status":      item.Status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0013's doc. Must be called inside the same transaction as
// the stock mutations it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, receiptID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, TenantID: tenantID, ReceiptID: receiptID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// CreateSplit inserts one receipt_item_splits row — see
// ReceiptItemSplit's own doc. Called once per lot/expiry/pallet batch a
// Confirm call posts, inside that same transaction.
func (r *Repository) CreateSplit(ctx context.Context, tenantID int64, split *ReceiptItemSplit) error {
	split.TenantID = tenantID
	return r.db.WithContext(ctx).Create(split).Error
}

type splitRow struct {
	ID              int64
	ReceiptItemID   int64
	LotNo           string
	MfgDate         *time.Time
	ExpiryDate      *time.Time
	Quantity        float64
	UOM             string
	PalletID        *string
	CartonCount     *int
	LocationID      int64
	LocationCode    string
	TemperatureC    *float64
	ConditionNote   *string
	CountryOfOrigin *string
	CreatedAt       time.Time
}

// listSplitsByReceipt returns every split row for every line on
// receiptID, grouped by receipt_item_id — one query for the whole
// receipt (ListItems' own caller) rather than one query per line.
func (r *Repository) listSplitsByReceipt(ctx context.Context, tenantID, receiptID int64) (map[int64][]SplitResponse, error) {
	var rows []splitRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS id, s.receipt_item_id AS receipt_item_id, s.lot_no AS lot_no,
			s.mfg_date AS mfg_date, s.expiry_date AS expiry_date, s.quantity AS quantity, s.uom AS uom,
			s.pallet_id AS pallet_id, s.carton_count AS carton_count,
			s.location_id AS location_id, l.code AS location_code,
			s.temperature_c AS temperature_c, s.condition_note AS condition_note, s.country_of_origin AS country_of_origin,
			s.created_at AS created_at
		FROM receipt_item_splits s
		JOIN locations l ON l.id = s.location_id
		WHERE s.receipt_id = ? AND s.tenant_id = ?
		ORDER BY s.id ASC
	`, receiptID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	byItem := make(map[int64][]SplitResponse, len(rows))
	for _, row := range rows {
		byItem[row.ReceiptItemID] = append(byItem[row.ReceiptItemID], SplitResponse{
			ID: row.ID, LotNo: row.LotNo, MfgDate: row.MfgDate, ExpiryDate: row.ExpiryDate,
			Quantity: row.Quantity, UOM: row.UOM, PalletID: row.PalletID, CartonCount: row.CartonCount,
			LocationID: row.LocationID, LocationCode: row.LocationCode,
			TemperatureC: row.TemperatureC, ConditionNote: row.ConditionNote, CountryOfOrigin: row.CountryOfOrigin,
			CreatedAt: row.CreatedAt,
		})
	}
	return byItem, nil
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr). This package's
// idempotency guard is what originally exposed the need for it:
// receipt_confirmations.idempotency_key is a PRIMARY KEY (not a plain
// UNIQUE constraint), and SQL Server's message text for that case is
// "Violation of PRIMARY KEY constraint ... Cannot insert duplicate key
// ..." — it contains neither "2627" nor "unique", so the substring
// checks this function used to have (copied from other internal/*
// packages, whose constrained columns are plain UNIQUE) never matched
// here, and InsertConfirmation's duplicate-key case fell through as a
// raw 500 instead of ErrDuplicateIdempotencyKey until caught live. See
// sqlerr's doc for the number-based fix.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

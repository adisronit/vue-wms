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
func (r *Repository) CreateReceipt(ctx context.Context, receipt *Receipt) error {
	if err := r.db.WithContext(ctx).Create(receipt).Error; err != nil {
		return err
	}
	receipt.ReceiptNo = fmt.Sprintf("RCV-%06d", receipt.ID)
	return r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ?", receipt.ID).
		Update("receipt_no", receipt.ReceiptNo).Error
}

func (r *Repository) FindReceiptByID(ctx context.Context, id int64) (*Receipt, error) {
	var rec Receipt
	err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

type headerRow struct {
	ID                int64
	ReceiptNo         string
	ReceivingType     string
	ReferenceNo       *string
	Vendor            *string
	WarehouseID       int64
	WarehouseCode     string
	StagingLocationID int64
	StagingLocation   string
	Status            string
	ReceivingDate     *time.Time
	Remark            *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CancelledAt       *time.Time
	CancelledBy       *int64
	CancelledByName   *string
	CancelRemark      *string
}

// FindReceiptDetail returns the header with warehouse/location codes
// resolved, for GET /receipts/{id}'s response — a separate query from
// FindReceiptByID (which returns the plain GORM model, used internally
// by Update/Confirm where the joined display fields aren't needed). The
// LEFT JOIN to users (Phase 18) resolves cancelled_by to a display name,
// the same pattern Phase 14 established for created_by_name on stock
// counts.
func (r *Repository) FindReceiptDetail(ctx context.Context, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT rc.id AS id, rc.receipt_no AS receipt_no, rc.receiving_type AS receiving_type,
			rc.reference_no AS reference_no, rc.vendor AS vendor,
			rc.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			rc.staging_location_id AS staging_location_id, l.code AS staging_location,
			rc.status AS status, rc.receiving_date AS receiving_date, rc.remark AS remark,
			rc.created_at AS created_at, rc.updated_at AS updated_at,
			rc.cancelled_at AS cancelled_at, rc.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			rc.cancel_remark AS cancel_remark
		FROM receipts rc
		JOIN warehouses w ON w.id = rc.warehouse_id
		JOIN locations l ON l.id = rc.staging_location_id
		LEFT JOIN users cu ON cu.id = rc.cancelled_by
		WHERE rc.id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *Repository) UpdateReceipt(ctx context.Context, receipt *Receipt) error {
	return r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ?", receipt.ID).Updates(map[string]any{
		"reference_no":        receipt.ReferenceNo,
		"vendor":              receipt.Vendor,
		"staging_location_id": receipt.StagingLocationID,
		"receiving_date":      receipt.ReceivingDate,
		"remark":              receipt.Remark,
		"status":              receipt.Status,
	}).Error
}

// CancelReceipt writes the exact columns Cancel derives — status plus
// the three Phase 18 audit columns — in one explicit UPDATE, deliberately
// separate from UpdateReceipt (which has no business ever touching
// cancellation columns). Never called except from a Cancel that has
// already passed Status.CanCancel, so cancelled_at/cancelled_by/
// cancel_remark are only ever written once per receipt.
func (r *Repository) CancelReceipt(ctx context.Context, receipt *Receipt) error {
	return r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ?", receipt.ID).Updates(map[string]any{
		"status":        receipt.Status,
		"cancelled_at":  receipt.CancelledAt,
		"cancelled_by":  receipt.CancelledBy,
		"cancel_remark": receipt.CancelRemark,
	}).Error
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

func (r *Repository) ListReceipts(ctx context.Context, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM receipts rc
		JOIN warehouses w ON w.id = rc.warehouse_id
		LEFT JOIN receipt_items ri ON ri.receipt_id = rc.id
	`
	where := `WHERE (? = '' OR rc.receipt_no LIKE ? OR rc.reference_no LIKE ? OR rc.vendor LIKE ?)
		AND (? = '' OR rc.status = ?)
		AND (? = 0 OR rc.warehouse_id = ?)
		AND (? IS NULL OR rc.receiving_date = ?)`

	like := "%" + params.Search + "%"
	args := []any{
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

// CountOpen returns how many receipts are still in a status
// Status.CanConfirm() would accept (DRAFT/WAITING/PARTIALLY_RECEIVED) —
// Phase 23's Dashboard "Pending Receiving" card. A single indexed
// COUNT(*) (IX_receipts_status), not a List() call, since the dashboard
// only ever needs the number, never the rows.
func (r *Repository) CountOpen(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Receipt{}).
		Where("status IN ?", []string{string(StatusDraft), string(StatusWaiting), string(StatusPartiallyReceived)}).
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

func (r *Repository) ListItems(ctx context.Context, receiptID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT ri.id AS id, ri.product_id AS product_id, p.sku AS sku, p.name AS product_name, p.barcode AS barcode,
			ri.expected_qty AS expected_qty, ri.received_qty AS received_qty, ri.uom AS uom,
			ri.lot_no AS lot_no, ri.expiry_date AS expiry_date, ri.location_id AS location_id,
			ri.status AS status, ri.created_at AS created_at, ri.updated_at AS updated_at
		FROM receipt_items ri
		JOIN products p ON p.id = ri.product_id
		WHERE ri.receipt_id = ?
		ORDER BY ri.id ASC
	`, receiptID).Scan(&rows).Error
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
		})
	}
	return items, nil
}

func (r *Repository) CreateItem(ctx context.Context, item *ReceiptItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, id int64) (*ReceiptItem, error) {
	var item ReceiptItem
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
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
func (r *Repository) FindItemsByReceipt(ctx context.Context, receiptID int64) ([]ReceiptItem, error) {
	var items []ReceiptItem
	err := r.db.WithContext(ctx).Where("receipt_id = ?", receiptID).Find(&items).Error
	return items, err
}

func (r *Repository) UpdateItem(ctx context.Context, item *ReceiptItem) error {
	return r.db.WithContext(ctx).Model(&ReceiptItem{}).Where("id = ?", item.ID).Updates(map[string]any{
		"expected_qty": item.ExpectedQty,
		"received_qty": item.ReceivedQty,
		"uom":          item.UOM,
		"lot_no":       item.LotNo,
		"expiry_date":  item.ExpiryDate,
		"location_id":  item.LocationID,
		"status":       item.Status,
	}).Error
}

// AdjustReceivedQty atomically increments a line's received_qty by delta
// in one UPDATE statement — the arithmetic happens inside the database,
// not as a Go read-then-write, the same concurrency-safety pattern as
// internal/inventory.Repository.AdjustBalance. Added in Phase 9 to fix a
// real lost-update bug Phase 8's own concurrency regression test found:
// Confirm previously read received_qty into a Go struct
// (FindItemByID), added the batch's quantity in Go, and wrote the whole
// row back (UpdateItem above) — under two concurrent Confirm calls
// against the same line, both could read the same starting value and
// the second UPDATE's stale computed total would silently overwrite the
// first's, undercounting real progress (the inventory ledger/balance
// itself, written by AddStockTx, was never affected — only this
// display/progress column). See PROJECT_STATUS.md.
func (r *Repository) AdjustReceivedQty(ctx context.Context, itemID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE receipt_items
		SET received_qty = received_qty + ?, updated_at = SYSUTCDATETIME()
		WHERE id = ?
	`, delta, itemID)
	return result.RowsAffected, result.Error
}

// UpdateItemAfterConfirm writes the fields Confirm derives after calling
// AdjustReceivedQty — deliberately excludes received_qty itself (unlike
// UpdateItem above) so this write can never clobber a concurrent
// transaction's own atomic increment with a value computed from a
// stale read.
func (r *Repository) UpdateItemAfterConfirm(ctx context.Context, item *ReceiptItem) error {
	return r.db.WithContext(ctx).Model(&ReceiptItem{}).Where("id = ?", item.ID).Updates(map[string]any{
		"lot_no":      item.LotNo,
		"expiry_date": item.ExpiryDate,
		"location_id": item.LocationID,
		"status":      item.Status,
	}).Error
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0013's doc. Must be called inside the same transaction as
// the stock mutations it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, idempotencyKey string, receiptID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, ReceiptID: receiptID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
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

package stockcount

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("stockcount: not found")
var ErrItemNotFound = errors.New("stockcount: item not found")
var ErrDuplicateIdempotencyKey = errors.New("stockcount: idempotency key already used")

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
// to the exact same transaction) and a Repository bound to it, the same
// pattern every prior phase established.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateCount inserts the header, then derives count_no from the new
// IDENTITY id in the same transaction — the same race-free-by-
// construction approach as every prior document number in this codebase.
func (r *Repository) CreateCount(ctx context.Context, c *Count) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return err
	}
	c.CountNo = fmt.Sprintf("CNT-%06d", c.ID)
	return r.db.WithContext(ctx).Model(&Count{}).Where("id = ?", c.ID).
		Update("count_no", c.CountNo).Error
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Count, error) {
	var c Count
	err := r.db.WithContext(ctx).First(&c, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, c *Count) error {
	return r.db.WithContext(ctx).Model(&Count{}).Where("id = ?", c.ID).Updates(map[string]any{
		"status":       c.Status,
		"confirmed_at": c.ConfirmedAt,
	}).Error
}

// ClaimStatus atomically transitions a count from DRAFT to newStatus in
// one guarded UPDATE statement — Phase 16's mechanism for making
// Confirm and Create Adjustment mutually exclusive, the same single-
// atomic-statement pattern as inventory.AdjustBalance/AdjustReservation,
// applied here to a status transition instead of a numeric quantity.
// Both Service.Confirm and Service.CreateAdjustments call this before
// doing anything else that matters; whichever commits first wins the
// `WHERE status = 'DRAFT'` race, and the loser gets 0 rows affected —
// never a Go-side "read DRAFT, then blindly overwrite" window. This
// closes both the sequential case (one path already ran) and the
// genuine-concurrency case (two requests racing each other) with the
// same guard.
func (r *Repository) ClaimStatus(ctx context.Context, countID int64, newStatus Status) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE stock_counts
		SET status = ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND status = 'DRAFT'
	`, string(newStatus), countID)
	return result.RowsAffected, result.Error
}

func (r *Repository) CreateItem(ctx context.Context, item *Item) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, id int64) (*Item, error) {
	var item Item
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FindItemsByCount returns the raw Item rows (not the joined response
// shape) — used internally by Confirm to walk every line and apply its
// variance.
func (r *Repository) FindItemsByCount(ctx context.Context, countID int64) ([]Item, error) {
	var items []Item
	err := r.db.WithContext(ctx).Where("stock_count_id = ?", countID).Find(&items).Error
	return items, err
}

// RecordCount writes count_qty/variance_qty/condition/status/remark —
// deliberately excludes system_qty entirely (see model.go's doc: it is
// frozen at creation and must never be touched again).
func (r *Repository) RecordCount(ctx context.Context, item *Item) error {
	return r.db.WithContext(ctx).Model(&Item{}).Where("id = ?", item.ID).Updates(map[string]any{
		"count_qty":    item.CountQty,
		"variance_qty": item.VarianceQty,
		"condition":    item.Condition,
		"status":       item.Status,
		"remark":       item.Remark,
	}).Error
}

type listRow struct {
	ID            int64
	CountNo       string
	CountType     string
	WarehouseCode string
	ZoneCode      *string
	CountDate     *time.Time
	ItemCount     int
	VarianceLines int
	Status        string
	CreatedBy     *int64
	CreatedByName *string
	CreatedAt     time.Time
}

// List backs the Stock Count History screen (Phase 14) — search matches
// count_no (the only natural free-text handle a count has; unlike
// Receiving/Picking/etc. there is no order/customer/SKU text on the
// header itself), plus the existing warehouse/status filters and a new
// single-day count_date filter, the same `date = ?` pattern
// internal/picking's own List already established. The LEFT JOIN to
// users and zones are both Phase 14 additions purely for extra display
// columns (created_by_name, zone_code) — neither changes which rows are
// returned or how many, since both are 0/1 rows per stock_counts id.
func (r *Repository) List(ctx context.Context, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM stock_counts sc
		JOIN warehouses w ON w.id = sc.warehouse_id
		LEFT JOIN zones z ON z.id = sc.zone_id
		LEFT JOIN users u ON u.id = sc.created_by
		LEFT JOIN stock_count_items sci ON sci.stock_count_id = sc.id
	`
	where := `WHERE (? = '' OR sc.count_no LIKE ?)
		AND (? = 0 OR sc.warehouse_id = ?)
		AND (? = '' OR sc.status = ?)
		AND (? IS NULL OR sc.count_date = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		params.Search, like,
		params.WarehouseID, params.WarehouseID,
		string(params.Status), string(params.Status),
		params.Date, params.Date,
	}

	countSQL := "SELECT COUNT(DISTINCT sc.id) " + base + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT sc.id AS id, sc.count_no AS count_no, sc.count_type AS count_type, w.code AS warehouse_code,
			z.code AS zone_code, sc.count_date AS count_date, sc.status AS status,
			sc.created_by AS created_by, u.full_name AS created_by_name, sc.created_at AS created_at,
			COUNT(sci.id) AS item_count,
			SUM(CASE WHEN sci.variance_qty IS NOT NULL AND sci.variance_qty <> 0 THEN 1 ELSE 0 END) AS variance_lines
	` + base + where + `
		GROUP BY sc.id, sc.count_no, sc.count_type, w.code, z.code, sc.count_date, sc.status,
			sc.created_by, u.full_name, sc.created_at
		ORDER BY sc.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			ID: row.ID, CountNo: row.CountNo, CountType: CountType(row.CountType), WarehouseCode: row.WarehouseCode,
			ZoneCode: row.ZoneCode, CountDate: row.CountDate, ItemCount: row.ItemCount, VarianceLines: row.VarianceLines,
			Status: Status(row.Status), CreatedBy: row.CreatedBy, CreatedByName: row.CreatedByName, CreatedAt: row.CreatedAt,
		})
	}
	return responses, total, nil
}

type itemRow struct {
	ID           int64
	ProductID    int64
	SKU          string
	ProductName  string
	LocationID   int64
	LocationCode string
	LotNo        string
	UOM          string
	SystemQty    float64
	CountQty     *float64
	VarianceQty  *float64
	Condition    string
	Status       string
	Remark       *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (r *Repository) ListItems(ctx context.Context, countID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT sci.id AS id, sci.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			sci.location_id AS location_id, l.code AS location_code, sci.lot_no AS lot_no, sci.uom AS uom,
			sci.system_qty AS system_qty, sci.count_qty AS count_qty, sci.variance_qty AS variance_qty,
			sci.condition AS condition, sci.status AS status, sci.remark AS remark,
			sci.created_at AS created_at, sci.updated_at AS updated_at
		FROM stock_count_items sci
		JOIN products p ON p.id = sci.product_id
		JOIN locations l ON l.id = sci.location_id
		WHERE sci.stock_count_id = ?
		ORDER BY sci.id ASC
	`, countID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName,
			LocationID: row.LocationID, LocationCode: row.LocationCode, LotNo: row.LotNo, UOM: row.UOM,
			SystemQty: row.SystemQty, CountQty: row.CountQty, VarianceQty: row.VarianceQty,
			Condition: Condition(row.Condition), Status: ItemStatus(row.Status), Remark: row.Remark,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return items, nil
}

type headerRow struct {
	ID            int64
	CountNo       string
	CountType     string
	WarehouseID   int64
	WarehouseCode string
	ZoneID        *int64
	ZoneCode      *string
	CountDate     *time.Time
	Status        string
	CreatedBy     *int64
	CreatedByName *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ConfirmedAt   *time.Time
}

// FindDetail returns the header with warehouse/zone codes resolved, for
// GET /stock-counts/{id}'s response. The LEFT JOIN to users (Phase 14)
// resolves created_by to a display name for the History detail view —
// added the same way zones already was in Phase 11, and for the same
// reason: a display-only column no business logic depends on.
func (r *Repository) FindDetail(ctx context.Context, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT sc.id AS id, sc.count_no AS count_no, sc.count_type AS count_type,
			sc.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			sc.zone_id AS zone_id, z.code AS zone_code,
			sc.count_date AS count_date, sc.status AS status,
			sc.created_by AS created_by, u.full_name AS created_by_name,
			sc.created_at AS created_at, sc.updated_at AS updated_at, sc.confirmed_at AS confirmed_at
		FROM stock_counts sc
		JOIN warehouses w ON w.id = sc.warehouse_id
		LEFT JOIN zones z ON z.id = sc.zone_id
		LEFT JOIN users u ON u.id = sc.created_by
		WHERE sc.id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0028's doc. Must be called inside the same transaction as
// the stock mutations it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, idempotencyKey string, countID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, CountID: countID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) — built correct from
// the start, the same as every phase since Phase 8.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

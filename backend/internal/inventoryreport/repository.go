package inventoryreport

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// scopeWhere builds the WHERE clause every main-report query shares.
// now is an explicit parameter (not SQL's own current-time function)
// so BelowQty-independent, date-relative filters (ExpiringWithinDays,
// ExpiredOnly, InactiveDays) are deterministic and testable.
func scopeWhere(tenantID int64, f Filters, now time.Time) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("inv.tenant_id = ?")

	if f.WarehouseID != nil {
		sb.WriteString(" AND w.id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.ZoneID != nil {
		sb.WriteString(" AND z.id = ?")
		args = append(args, *f.ZoneID)
	}
	if f.LocationID != nil {
		sb.WriteString(" AND loc.id = ?")
		args = append(args, *f.LocationID)
	}
	if f.StorageType != "" {
		sb.WriteString(" AND loc.type = ?")
		args = append(args, f.StorageType)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(" AND (p.sku LIKE ? OR p.name LIKE ? OR inv.lot_no LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}
	if f.LotNo != "" {
		sb.WriteString(" AND inv.lot_no = ?")
		args = append(args, f.LotNo)
	}
	if f.BelowQty != nil {
		sb.WriteString(" AND (inv.quantity - inv.reserved_quantity - inv.hold_quantity) < ?")
		args = append(args, *f.BelowQty)
	}
	if f.ExpiredOnly {
		sb.WriteString(" AND inv.expiry_date IS NOT NULL AND inv.expiry_date < ?")
		args = append(args, now)
	} else if f.ExpiringWithinDays != nil {
		sb.WriteString(" AND inv.expiry_date IS NOT NULL AND inv.expiry_date <= ?")
		args = append(args, now.AddDate(0, 0, *f.ExpiringWithinDays))
	}
	if f.InactiveDays != nil {
		cutoff := now.AddDate(0, 0, -*f.InactiveDays)
		sb.WriteString(" AND (ma.last_movement IS NULL OR ma.last_movement < ?)")
		args = append(args, cutoff)
	}

	return sb.String(), args
}

const movementAggCTE = `
movement_agg AS (
	SELECT product_id, location_id, lot_no, MAX(created_at) AS last_movement
	FROM inventory_transactions
	WHERE tenant_id = ?
	GROUP BY product_id, location_id, lot_no
)
`

const inventoryStatusCase = `CASE
	WHEN inv.hold_quantity > 0 THEN 'ON_HOLD'
	WHEN inv.reserved_quantity > 0 THEN 'RESERVED'
	WHEN inv.quantity <= 0 THEN 'ZERO'
	ELSE 'AVAILABLE'
END`

const listSelectAndFrom = `
SELECT
	inv.id AS inventory_id,
	w.code AS warehouse_code, w.name AS warehouse_name,
	z.code AS zone_code, z.name AS zone_name,
	loc.code AS location_code, loc.name AS location_name, loc.type AS storage_type,
	p.sku, p.name AS product_name, b.name AS brand_name, pc.name AS category_name,
	inv.lot_no, inv.expiry_date, p.base_uom AS uom,
	inv.quantity AS on_hand, inv.reserved_quantity AS reserved, inv.hold_quantity AS hold,
	(inv.quantity - inv.reserved_quantity - inv.hold_quantity) AS available,
	ma.last_movement AS last_movement_date,
	` + inventoryStatusCase + ` AS inventory_status
FROM inventory inv
JOIN products p ON p.id = inv.product_id
LEFT JOIN brands b ON b.id = p.brand_id
LEFT JOIN product_categories pc ON pc.id = p.category_id
JOIN locations loc ON loc.id = inv.location_id
JOIN zones z ON z.id = loc.zone_id
JOIN warehouses w ON w.id = z.warehouse_id
LEFT JOIN movement_agg ma ON ma.product_id = inv.product_id AND ma.location_id = inv.location_id AND ma.lot_no = inv.lot_no
`

var sortColumns = map[string]string{
	"on_hand":            "inv.quantity",
	"available":          "(inv.quantity - inv.reserved_quantity - inv.hold_quantity)",
	"expiry_date":        "inv.expiry_date",
	"last_movement_date": "ma.last_movement",
	"warehouse_name":     "w.name",
	"sku":                "p.sku",
}

func sortClause(sortBy, sortDir string) string {
	col, ok := sortColumns[sortBy]
	if !ok {
		col = sortColumns["on_hand"]
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + ", inv.id " + dir
}

type rawRow struct {
	InventoryID      int64
	WarehouseCode    string
	WarehouseName    string
	ZoneCode         string
	ZoneName         string
	LocationCode     string
	LocationName     string
	StorageType      string
	SKU              string
	ProductName      string
	BrandName        *string
	CategoryName     *string
	LotNo            string
	ExpiryDate       *time.Time
	UOM              string
	OnHand           float64
	Reserved         float64
	Hold             float64
	Available        float64
	LastMovementDate *time.Time
	InventoryStatus  string
}

func (r rawRow) toRow() Row {
	return Row{
		InventoryID: r.InventoryID, WarehouseCode: r.WarehouseCode, WarehouseName: r.WarehouseName,
		ZoneCode: r.ZoneCode, ZoneName: r.ZoneName, LocationCode: r.LocationCode, LocationName: r.LocationName,
		StorageType: r.StorageType, SKU: r.SKU, ProductName: r.ProductName, BrandName: r.BrandName, CategoryName: r.CategoryName,
		LotNo: r.LotNo, ExpiryDate: r.ExpiryDate, LPN: nil, UOM: r.UOM,
		OnHand: r.OnHand, Reserved: r.Reserved, Hold: r.Hold, Available: r.Available,
		LastMovementDate: r.LastMovementDate, InventoryStatus: r.InventoryStatus,
	}
}

func (repo *Repository) List(ctx context.Context, tenantID int64, params ListParams, now time.Time) ([]Row, int64, error) {
	where, whereArgs := scopeWhere(tenantID, params.Filters, now)
	cteArgs := []interface{}{tenantID}
	args := append(append([]interface{}{}, cteArgs...), whereArgs...)

	var total int64
	countSQL := "WITH " + movementAggCTE + " SELECT COUNT(*) FROM (" + listSelectAndFrom + " WHERE " + where + ") t"
	if err := repo.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	dataSQL := "WITH " + movementAggCTE + " " + listSelectAndFrom + " WHERE " + where +
		" ORDER BY " + sortClause(params.SortBy, params.SortDir) +
		" OFFSET ? ROWS FETCH NEXT ? ROWS ONLY"
	dataArgs := append(append([]interface{}{}, args...), offset, limit)

	var raw []rawRow
	if err := repo.db.WithContext(ctx).Raw(dataSQL, dataArgs...).Scan(&raw).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]Row, 0, len(raw))
	for _, rr := range raw {
		rows = append(rows, rr.toRow())
	}
	return rows, total, nil
}

func (repo *Repository) ListForExport(ctx context.Context, tenantID int64, filters Filters, now time.Time, maxRows int) ([]Row, error) {
	where, whereArgs := scopeWhere(tenantID, filters, now)
	args := append([]interface{}{tenantID}, whereArgs...)

	dataSQL := "WITH " + movementAggCTE + " " + listSelectAndFrom + " WHERE " + where +
		" ORDER BY " + sortClause("on_hand", "desc") +
		" OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY"
	dataArgs := append(append([]interface{}{}, args...), maxRows)

	var raw []rawRow
	if err := repo.db.WithContext(ctx).Raw(dataSQL, dataArgs...).Scan(&raw).Error; err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(raw))
	for _, rr := range raw {
		rows = append(rows, rr.toRow())
	}
	return rows, nil
}

func (repo *Repository) Summary(ctx context.Context, tenantID int64, filters Filters, now time.Time) (Summary, error) {
	where, whereArgs := scopeWhere(tenantID, filters, now)
	args := append([]interface{}{tenantID}, whereArgs...)

	var summary Summary

	var onHandRows []UOMQty
	onHandSQL := "WITH " + movementAggCTE + ` SELECT p.base_uom AS uom, SUM(inv.quantity) AS qty FROM inventory inv
		JOIN products p ON p.id = inv.product_id
		LEFT JOIN brands b ON b.id = p.brand_id
		LEFT JOIN product_categories pc ON pc.id = p.category_id
		JOIN locations loc ON loc.id = inv.location_id
		JOIN zones z ON z.id = loc.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		LEFT JOIN movement_agg ma ON ma.product_id = inv.product_id AND ma.location_id = inv.location_id AND ma.lot_no = inv.lot_no
		WHERE ` + where + ` GROUP BY p.base_uom`
	if err := repo.db.WithContext(ctx).Raw(onHandSQL, args...).Scan(&onHandRows).Error; err != nil {
		return Summary{}, err
	}
	summary.TotalOnHandByUOM = onHandRows

	var availRows []UOMQty
	availSQL := "WITH " + movementAggCTE + ` SELECT p.base_uom AS uom, SUM(inv.quantity - inv.reserved_quantity - inv.hold_quantity) AS qty FROM inventory inv
		JOIN products p ON p.id = inv.product_id
		LEFT JOIN brands b ON b.id = p.brand_id
		LEFT JOIN product_categories pc ON pc.id = p.category_id
		JOIN locations loc ON loc.id = inv.location_id
		JOIN zones z ON z.id = loc.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		LEFT JOIN movement_agg ma ON ma.product_id = inv.product_id AND ma.location_id = inv.location_id AND ma.lot_no = inv.lot_no
		WHERE ` + where + ` GROUP BY p.base_uom`
	if err := repo.db.WithContext(ctx).Raw(availSQL, args...).Scan(&availRows).Error; err != nil {
		return Summary{}, err
	}
	summary.TotalAvailableByUOM = availRows

	if filters.BelowQty != nil {
		var count int64
		sql := "WITH " + movementAggCTE + " SELECT COUNT(*) FROM (" + listSelectAndFrom + " WHERE " + where + ") t"
		if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&count).Error; err != nil {
			return Summary{}, err
		}
		summary.LowStockCount = &count
	}
	if filters.ExpiringWithinDays != nil && !filters.ExpiredOnly {
		count, err := repo.countWithExtra(ctx, tenantID, filters, now, "AND inv.expiry_date IS NOT NULL AND inv.expiry_date >= ? AND inv.expiry_date <= ?", []interface{}{now, now.AddDate(0, 0, *filters.ExpiringWithinDays)})
		if err != nil {
			return Summary{}, err
		}
		summary.NearExpiryCount = &count
	}
	{
		count, err := repo.countWithExtra(ctx, tenantID, filters, now, "AND inv.expiry_date IS NOT NULL AND inv.expiry_date < ?", []interface{}{now})
		if err != nil {
			return Summary{}, err
		}
		summary.ExpiredCount = &count
	}
	if filters.InactiveDays != nil {
		var count int64
		sql := "WITH " + movementAggCTE + " SELECT COUNT(*) FROM (" + listSelectAndFrom + " WHERE " + where + ") t"
		if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&count).Error; err != nil {
			return Summary{}, err
		}
		summary.NonMovingCount = &count
	}

	charts, err := repo.charts(ctx, where, args)
	if err != nil {
		return Summary{}, err
	}
	summary.Charts = charts

	return summary, nil
}

// countWithExtra counts rows matching the base scope (ignoring the
// caller's own Expired/ExpiringWithinDays filters, which this helper
// overrides with its own extraWhere) — used so Summary's
// NearExpiryCount/ExpiredCount reflect the spec's own fixed
// definitions regardless of which lens filter the caller happened to
// have active on the list view.
func (repo *Repository) countWithExtra(ctx context.Context, tenantID int64, filters Filters, now time.Time, extraWhere string, extraArgs []interface{}) (int64, error) {
	neutral := filters
	neutral.ExpiredOnly = false
	neutral.ExpiringWithinDays = nil
	where, whereArgs := scopeWhere(tenantID, neutral, now)
	args := append([]interface{}{tenantID}, whereArgs...)
	args = append(args, extraArgs...)

	var count int64
	sql := "WITH " + movementAggCTE + " SELECT COUNT(*) FROM (" + listSelectAndFrom + " WHERE " + where + " " + extraWhere + ") t"
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (repo *Repository) charts(ctx context.Context, where string, args []interface{}) (Charts, error) {
	var charts Charts

	byWarehouseSQL := "WITH " + movementAggCTE + ` SELECT w.name AS name, SUM(inv.quantity) AS qty FROM inventory inv
		JOIN products p ON p.id = inv.product_id
		LEFT JOIN brands b ON b.id = p.brand_id
		LEFT JOIN product_categories pc ON pc.id = p.category_id
		JOIN locations loc ON loc.id = inv.location_id
		JOIN zones z ON z.id = loc.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		LEFT JOIN movement_agg ma ON ma.product_id = inv.product_id AND ma.location_id = inv.location_id AND ma.lot_no = inv.lot_no
		WHERE ` + where + ` GROUP BY w.name ORDER BY qty DESC`
	if err := repo.db.WithContext(ctx).Raw(byWarehouseSQL, args...).Scan(&charts.ByWarehouse).Error; err != nil {
		return Charts{}, err
	}

	byZoneSQL := "WITH " + movementAggCTE + ` SELECT z.name AS name, SUM(inv.quantity) AS qty FROM inventory inv
		JOIN products p ON p.id = inv.product_id
		LEFT JOIN brands b ON b.id = p.brand_id
		LEFT JOIN product_categories pc ON pc.id = p.category_id
		JOIN locations loc ON loc.id = inv.location_id
		JOIN zones z ON z.id = loc.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		LEFT JOIN movement_agg ma ON ma.product_id = inv.product_id AND ma.location_id = inv.location_id AND ma.lot_no = inv.lot_no
		WHERE ` + where + ` GROUP BY z.name ORDER BY qty DESC`
	if err := repo.db.WithContext(ctx).Raw(byZoneSQL, args...).Scan(&charts.ByZone).Error; err != nil {
		return Charts{}, err
	}

	return charts, nil
}

// --- Stock Movement (separate grain — inventory_transactions) ---

func movementScopeWhere(tenantID int64, f MovementFilters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("it.tenant_id = ?")

	if f.From != nil {
		sb.WriteString(" AND it.created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND it.created_at < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND w.id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.LocationID != nil {
		sb.WriteString(" AND loc.id = ?")
		args = append(args, *f.LocationID)
	}
	if f.TransactionType != "" {
		sb.WriteString(" AND it.transaction_type = ?")
		args = append(args, f.TransactionType)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(" AND (p.sku LIKE ? OR p.name LIKE ? OR it.lot_no LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}

	return sb.String(), args
}

const movementSelectAndFrom = `
SELECT
	it.id AS transaction_id, it.created_at, it.transaction_type,
	w.code AS warehouse_code, w.name AS warehouse_name,
	loc.code AS location_code, p.sku, p.name AS product_name, it.lot_no, it.quantity,
	it.reference_type, it.reference_id, u.full_name AS operator
FROM inventory_transactions it
JOIN products p ON p.id = it.product_id
JOIN locations loc ON loc.id = it.location_id
JOIN zones z ON z.id = loc.zone_id
JOIN warehouses w ON w.id = z.warehouse_id
LEFT JOIN users u ON u.id = it.created_by
`

type rawMovementRow struct {
	TransactionID   int64
	CreatedAt       time.Time
	TransactionType string
	WarehouseCode   string
	WarehouseName   string
	LocationCode    string
	SKU             string
	ProductName     string
	LotNo           string
	Quantity        float64
	ReferenceType   *string
	ReferenceID     *string
	Operator        *string
}

func (r rawMovementRow) toRow() MovementRow {
	return MovementRow{
		TransactionID: r.TransactionID, CreatedAt: r.CreatedAt, TransactionType: r.TransactionType,
		WarehouseCode: r.WarehouseCode, WarehouseName: r.WarehouseName, LocationCode: r.LocationCode,
		SKU: r.SKU, ProductName: r.ProductName, LotNo: r.LotNo, Quantity: r.Quantity,
		ReferenceType: r.ReferenceType, ReferenceID: r.ReferenceID, Operator: r.Operator,
	}
}

func (repo *Repository) ListMovements(ctx context.Context, tenantID int64, params MovementListParams) ([]MovementRow, int64, error) {
	where, args := movementScopeWhere(tenantID, params.MovementFilters)

	var total int64
	countSQL := "SELECT COUNT(*) FROM (" + movementSelectAndFrom + " WHERE " + where + ") t"
	if err := repo.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	dataSQL := movementSelectAndFrom + " WHERE " + where + " ORDER BY it.created_at DESC, it.id DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY"
	dataArgs := append(append([]interface{}{}, args...), offset, limit)

	var raw []rawMovementRow
	if err := repo.db.WithContext(ctx).Raw(dataSQL, dataArgs...).Scan(&raw).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]MovementRow, 0, len(raw))
	for _, rr := range raw {
		rows = append(rows, rr.toRow())
	}
	return rows, total, nil
}

func (repo *Repository) ListMovementsForExport(ctx context.Context, tenantID int64, filters MovementFilters, maxRows int) ([]MovementRow, error) {
	where, args := movementScopeWhere(tenantID, filters)
	dataSQL := movementSelectAndFrom + " WHERE " + where + " ORDER BY it.created_at DESC, it.id DESC OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY"
	dataArgs := append(append([]interface{}{}, args...), maxRows)

	var raw []rawMovementRow
	if err := repo.db.WithContext(ctx).Raw(dataSQL, dataArgs...).Scan(&raw).Error; err != nil {
		return nil, err
	}
	rows := make([]MovementRow, 0, len(raw))
	for _, rr := range raw {
		rows = append(rows, rr.toRow())
	}
	return rows, nil
}

// --- Historical As-of Balance (see package doc's caveat) ---

func (repo *Repository) ListAsOf(ctx context.Context, tenantID int64, params AsOfListParams) ([]AsOfRow, int64, error) {
	var sb strings.Builder
	args := []interface{}{tenantID, params.AsOf}
	sb.WriteString("it.tenant_id = ? AND it.created_at <= ?")
	if params.WarehouseID != nil {
		sb.WriteString(" AND w.id = ?")
		args = append(args, *params.WarehouseID)
	}
	if params.LocationID != nil {
		sb.WriteString(" AND loc.id = ?")
		args = append(args, *params.LocationID)
	}
	if params.Search != "" {
		pattern := "%" + params.Search + "%"
		sb.WriteString(" AND (p.sku LIKE ? OR p.name LIKE ? OR it.lot_no LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}
	where := sb.String()

	baseSQL := `FROM inventory_transactions it
		JOIN products p ON p.id = it.product_id
		JOIN locations loc ON loc.id = it.location_id
		JOIN zones z ON z.id = loc.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		WHERE ` + where + `
		GROUP BY w.code, w.name, loc.code, p.sku, p.name, it.lot_no, p.base_uom
		HAVING SUM(it.quantity) <> 0`

	var total int64
	countSQL := "SELECT COUNT(*) FROM (SELECT 1 AS cnt " + baseSQL + ") t"
	if err := repo.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	dataSQL := `SELECT w.code AS warehouse_code, w.name AS warehouse_name, loc.code AS location_code,
		p.sku, p.name AS product_name, it.lot_no, p.base_uom AS uom, SUM(it.quantity) AS as_of_quantity ` +
		baseSQL + ` ORDER BY p.sku, it.lot_no OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	dataArgs := append(append([]interface{}{}, args...), offset, limit)

	var rows []AsOfRow
	if err := repo.db.WithContext(ctx).Raw(dataSQL, dataArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

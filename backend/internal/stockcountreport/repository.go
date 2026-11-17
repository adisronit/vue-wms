package stockcountreport

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

// scopeHeader builds the header-level WHERE fragment every query
// shares, referencing sc (stock_counts, always joined), w (warehouses),
// z (zones, LEFT JOIN), and u (users, LEFT JOIN) — date/warehouse/
// zone/counter/status, plus Search (count_no, or an EXISTS match
// against this session's own items/products). Safe to use unmodified
// in both a session-grain query (TotalCountSessions) and an item-grain
// query (List, the other KPIs) — it only ever constrains which
// sessions are in scope, never which item within a session.
func scopeHeader(tenantID int64, f Filters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("sc.tenant_id = ?")

	if f.From != nil {
		sb.WriteString(" AND COALESCE(sc.count_date, sc.created_at) >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND COALESCE(sc.count_date, sc.created_at) < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND sc.warehouse_id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.ZoneID != nil {
		sb.WriteString(" AND sc.zone_id = ?")
		args = append(args, *f.ZoneID)
	}
	if f.CounterID != nil {
		sb.WriteString(" AND sc.created_by = ?")
		args = append(args, *f.CounterID)
	}
	if f.Status != "" {
		sb.WriteString(" AND sc.status = ?")
		args = append(args, f.Status)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(` AND (sc.count_no LIKE ? OR EXISTS (
			SELECT 1 FROM stock_count_items sci_s JOIN products p_s ON p_s.id = sci_s.product_id
			WHERE sci_s.stock_count_id = sc.id AND (p_s.sku LIKE ? OR p_s.name LIKE ?)
		))`)
		args = append(args, pattern, pattern, pattern)
	}

	return sb.String(), args
}

// itemConditions returns the extra item-level AND-clauses (ItemStatus,
// VarianceOnly, LocationID, HasAdjustment) for a query that has already
// joined stock_count_items AS sci directly (and, for HasAdjustment,
// stock_adjustments AS sa scoped to sa.source_stock_count_id = sc.id —
// see repository callers) — used as-is by every item-grain query
// (List and the item-grain Summary KPIs), and wrapped in an EXISTS by
// itemConditionsExists below for the one session-grain query.
func itemConditions(f Filters) (string, []interface{}) {
	var sb strings.Builder
	var args []interface{}

	if f.ItemStatus != "" {
		sb.WriteString(" AND sci.status = ?")
		args = append(args, f.ItemStatus)
	}
	if f.VarianceOnly {
		sb.WriteString(" AND sci.variance_qty IS NOT NULL AND sci.variance_qty <> 0")
	}
	if f.LocationID != nil {
		sb.WriteString(" AND sci.location_id = ?")
		args = append(args, *f.LocationID)
	}
	if f.HasAdjustment != nil {
		if *f.HasAdjustment {
			sb.WriteString(" AND sa.id IS NOT NULL")
		} else {
			sb.WriteString(" AND sa.id IS NULL")
		}
	}

	return sb.String(), args
}

// itemConditionsExists wraps itemConditions in an EXISTS against
// stock_count_items (joined to stock_adjustments/stock_adjustment_items
// the same way the item-grain queries do) scoped to one session —
// used only by the session-grain TotalCountSessions query, so a
// VarianceOnly/ItemStatus/etc. filter narrows which SESSIONS count,
// without needing the session-grain query to also join every item-grain
// table itself.
func itemConditionsExists(f Filters) (string, []interface{}) {
	cond, args := itemConditions(f)
	if cond == "" {
		return "", nil
	}
	sql := ` AND EXISTS (
		SELECT 1 FROM stock_count_items sci
		LEFT JOIN stock_adjustments sa ON sa.source_stock_count_id = sc.id
		LEFT JOIN stock_adjustment_items sai ON sai.stock_adjustment_id = sa.id
			AND sai.product_id = sci.product_id AND sai.location_id = sci.location_id AND sai.lot_no = sci.lot_no
		WHERE sci.stock_count_id = sc.id` + strings.Replace(cond, "sa.id", "sai.id", -1) + `
	)`
	return sql, args
}

var sortColumns = map[string]string{
	"count_date":     "COALESCE(sc.count_date, sc.created_at)",
	"count_no":       "sc.count_no",
	"warehouse_name": "w.name",
	"status":         "sc.status",
}

func sortClause(sortBy, sortDir string) string {
	col, ok := sortColumns[sortBy]
	if !ok {
		col = sortColumns["count_date"]
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + ", sc.id " + dir + ", sci.id " + dir
}

const listSelectAndFrom = `
SELECT
	sc.id AS count_id, sci.id AS count_item_id,
	sc.count_no, sc.count_type, COALESCE(sc.count_date, sc.created_at) AS count_date_time,
	w.code AS warehouse_code, w.name AS warehouse_name,
	z.code AS zone_code, z.name AS zone_name,
	loc.code AS location_code, loc.name AS location_name,
	p.sku, p.name AS product_name, b.name AS brand_name, pc.name AS category_name,
	sci.lot_no, sci.uom, sci.system_qty, sci.count_qty, sci.variance_qty,
	CASE WHEN sci.system_qty <> 0 AND sci.count_qty IS NOT NULL THEN (sci.variance_qty / sci.system_qty) * 100 ELSE NULL END AS variance_pct,
	sci.status AS item_status, sc.status AS count_status,
	cu.full_name AS counted_by,
	au.full_name AS approved_by,
	sa.adjustment_no, sa.status AS adjustment_status
FROM stock_counts sc
JOIN stock_count_items sci ON sci.stock_count_id = sc.id
JOIN products p ON p.id = sci.product_id
LEFT JOIN brands b ON b.id = p.brand_id
LEFT JOIN product_categories pc ON pc.id = p.category_id
JOIN locations loc ON loc.id = sci.location_id
JOIN warehouses w ON w.id = sc.warehouse_id
LEFT JOIN zones z ON z.id = sc.zone_id
LEFT JOIN users cu ON cu.id = sc.created_by
LEFT JOIN stock_adjustments sa ON sa.source_stock_count_id = sc.id
LEFT JOIN stock_adjustment_items sai ON sai.stock_adjustment_id = sa.id
	AND sai.product_id = sci.product_id AND sai.location_id = sci.location_id AND sai.lot_no = sci.lot_no
LEFT JOIN users au ON au.id = sa.approved_by
`

type rawRow struct {
	CountID          int64
	CountItemID      int64
	CountNo          string
	CountType        string
	CountDateTime    *time.Time
	WarehouseCode    string
	WarehouseName    string
	ZoneCode         *string
	ZoneName         *string
	LocationCode     string
	LocationName     string
	SKU              string
	ProductName      string
	BrandName        *string
	CategoryName     *string
	LotNo            string
	UOM              string
	SystemQty        float64
	CountQty         *float64
	VarianceQty      *float64
	VariancePct      *float64
	ItemStatus       string
	CountStatus      string
	CountedBy        *string
	ApprovedBy       *string
	AdjustmentNo     *string
	AdjustmentStatus *string
}

func (r rawRow) toRow() Row {
	return Row{
		CountID: r.CountID, CountItemID: r.CountItemID, CountNo: r.CountNo, CountType: r.CountType,
		CountDateTime: r.CountDateTime, WarehouseCode: r.WarehouseCode, WarehouseName: r.WarehouseName,
		ZoneCode: r.ZoneCode, ZoneName: r.ZoneName, LocationCode: r.LocationCode, LocationName: r.LocationName,
		SKU: r.SKU, ProductName: r.ProductName, BrandName: r.BrandName, CategoryName: r.CategoryName,
		LotNo: r.LotNo, LPN: nil, UOM: r.UOM, SystemQty: r.SystemQty, CountedQty: r.CountQty,
		VarianceQty: r.VarianceQty, VariancePct: r.VariancePct,
		ItemStatus: r.ItemStatus, CountStatus: r.CountStatus,
		CountedBy: r.CountedBy, ReviewedBy: nil, ApprovedBy: r.ApprovedBy,
		AdjustmentNo: r.AdjustmentNo, AdjustmentStatus: r.AdjustmentStatus,
	}
}

func (repo *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]Row, int64, error) {
	headerWhere, headerArgs := scopeHeader(tenantID, params.Filters)
	itemWhere, itemArgs := itemConditions(params.Filters)
	where := headerWhere + itemWhere
	args := append(append([]interface{}{}, headerArgs...), itemArgs...)

	var total int64
	countSQL := "SELECT COUNT(*) FROM (" + listSelectAndFrom + " WHERE " + where + ") t"
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

	dataSQL := listSelectAndFrom + " WHERE " + where +
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

func (repo *Repository) ListForExport(ctx context.Context, tenantID int64, filters Filters, maxRows int) ([]Row, error) {
	headerWhere, headerArgs := scopeHeader(tenantID, filters)
	itemWhere, itemArgs := itemConditions(filters)
	where := headerWhere + itemWhere
	args := append(append([]interface{}{}, headerArgs...), itemArgs...)

	dataSQL := listSelectAndFrom + " WHERE " + where +
		" ORDER BY " + sortClause("count_date", "desc") +
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

func (repo *Repository) Summary(ctx context.Context, tenantID int64, filters Filters) (Summary, error) {
	headerWhere, headerArgs := scopeHeader(tenantID, filters)
	itemWhere, itemArgs := itemConditions(filters)
	itemExistsWhere, itemExistsArgs := itemConditionsExists(filters)

	var summary Summary

	// Total count sessions — session grain.
	sessionsSQL := `SELECT COUNT(DISTINCT sc.id) FROM stock_counts sc
		JOIN warehouses w ON w.id = sc.warehouse_id
		LEFT JOIN zones z ON z.id = sc.zone_id
		LEFT JOIN users u ON u.id = sc.created_by
		WHERE ` + headerWhere + itemExistsWhere
	sessionsArgs := append(append([]interface{}{}, headerArgs...), itemExistsArgs...)
	if err := repo.db.WithContext(ctx).Raw(sessionsSQL, sessionsArgs...).Scan(&summary.TotalCountSessions).Error; err != nil {
		return Summary{}, err
	}

	// Item-grain KPIs: counted locations/SKUs, accuracy, net variance,
	// recount rate — all from the same item-grain scope the List query
	// uses (header filters + item filters applied directly, no fan-out
	// concern since this IS the item grain already).
	type itemAgg struct {
		TotalItems        int64
		CountedItems      int64
		ZeroVarianceItems int64
		RecountItems      int64
		NetVariance       *float64
		CountedLocations  int64
		CountedSKUs       int64
	}
	var agg itemAgg
	itemSQL := `SELECT
		COUNT(*) AS total_items,
		SUM(CASE WHEN sci.status IN ('COUNTED', 'RECOUNT') THEN 1 ELSE 0 END) AS counted_items,
		SUM(CASE WHEN sci.status IN ('COUNTED', 'RECOUNT') AND sci.variance_qty = 0 THEN 1 ELSE 0 END) AS zero_variance_items,
		SUM(CASE WHEN sci.status = 'RECOUNT' THEN 1 ELSE 0 END) AS recount_items,
		SUM(CASE WHEN sci.status IN ('COUNTED', 'RECOUNT') THEN sci.variance_qty ELSE 0 END) AS net_variance,
		COUNT(DISTINCT CASE WHEN sci.status IN ('COUNTED', 'RECOUNT') THEN sci.location_id END) AS counted_locations,
		COUNT(DISTINCT CASE WHEN sci.status IN ('COUNTED', 'RECOUNT') THEN sci.product_id END) AS counted_skus
		FROM stock_counts sc
		JOIN stock_count_items sci ON sci.stock_count_id = sc.id
		JOIN warehouses w ON w.id = sc.warehouse_id
		LEFT JOIN zones z ON z.id = sc.zone_id
		LEFT JOIN users u ON u.id = sc.created_by
		LEFT JOIN stock_adjustments sa ON sa.source_stock_count_id = sc.id
		LEFT JOIN stock_adjustment_items sai ON sai.stock_adjustment_id = sa.id
			AND sai.product_id = sci.product_id AND sai.location_id = sci.location_id AND sai.lot_no = sci.lot_no
		WHERE ` + headerWhere + itemWhere
	itemArgsAll := append(append([]interface{}{}, headerArgs...), itemArgs...)
	if err := repo.db.WithContext(ctx).Raw(itemSQL, itemArgsAll...).Scan(&agg).Error; err != nil {
		return Summary{}, err
	}
	summary.CountedLocations = agg.CountedLocations
	summary.CountedSKUs = agg.CountedSKUs
	if agg.NetVariance != nil {
		summary.QuantityVariance = *agg.NetVariance
	}
	if agg.CountedItems > 0 {
		accuracy := float64(agg.ZeroVarianceItems) / float64(agg.CountedItems) * 100
		summary.StockCountAccuracy = &accuracy
	}
	if agg.TotalItems > 0 {
		recountRate := float64(agg.RecountItems) / float64(agg.TotalItems) * 100
		summary.RecountRate = &recountRate
	}

	// Pending approval: adjustments generated from a session in scope,
	// themselves still PENDING. Joined back to the exact originating
	// item so item-level filters (e.g. a warehouse/zone/counter scope)
	// still apply correctly.
	pendingSQL := `SELECT COUNT(DISTINCT sa.id) FROM stock_counts sc
		JOIN stock_count_items sci ON sci.stock_count_id = sc.id
		JOIN warehouses w ON w.id = sc.warehouse_id
		LEFT JOIN zones z ON z.id = sc.zone_id
		LEFT JOIN users u ON u.id = sc.created_by
		JOIN stock_adjustments sa ON sa.source_stock_count_id = sc.id AND sa.status = 'PENDING'
		JOIN stock_adjustment_items sai ON sai.stock_adjustment_id = sa.id
			AND sai.product_id = sci.product_id AND sai.location_id = sci.location_id AND sai.lot_no = sci.lot_no
		WHERE ` + headerWhere + itemWhere
	if err := repo.db.WithContext(ctx).Raw(pendingSQL, itemArgsAll...).Scan(&summary.PendingApproval).Error; err != nil {
		return Summary{}, err
	}

	return summary, nil
}

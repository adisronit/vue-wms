package deliveryreport

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Repository runs the Delivery & Shipping Report's read-only queries
// directly against shipments/shipment_items/packing_jobs/orders/
// warehouses/products/locations/loading_trips/loading_trip_shipments/
// loading_trip_cartons — see the package doc for why this reads those
// tables via raw SQL rather than depending on internal/shipping/
// internal/loading/internal/packing/internal/order's own Go APIs.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// shipmentJoins is the shared join chain every shipment-grain query
// (Row's own base, Summary's counts, every chart) uses — LEFT JOINs
// throughout so a legacy shipment with no linked packing job (none
// expected post Phase 33D.1, but the column is nullable) still appears
// rather than vanishing from the report. The orders join's OR branches
// are mutually exclusive (packing_jobs.order_id is either set — a
// batch-sourced job — or NULL, in which case orders.picking_task_id is
// the only path back) and orders.picking_task_id is unique, so this
// never fans a shipment out into more than one row.
const shipmentJoins = `
FROM shipments sh
LEFT JOIN packing_jobs pj ON pj.id = sh.packing_job_id
LEFT JOIN orders o ON o.id = pj.order_id OR (pj.order_id IS NULL AND o.picking_task_id = pj.picking_task_id)
LEFT JOIN warehouses w ON w.id = pj.warehouse_id
`

// scopeWhere builds the WHERE clause every shipment-grain query shares
// — list, count, and Summary's own aggregates all apply this exact
// same scope, per CLAUDE.md's "exports must respect exactly the same
// filters as the displayed report" rule. Search is an EXISTS subquery
// scoped to sh.id so it never fans a shipment-grain query out across
// its own item lines.
func scopeWhere(tenantID int64, f Filters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("sh.tenant_id = ?")

	if f.From != nil {
		sb.WriteString(" AND sh.created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND sh.created_at < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND pj.warehouse_id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.Carrier != "" {
		sb.WriteString(" AND sh.carrier = ?")
		args = append(args, f.Carrier)
	}
	if f.Status != "" {
		sb.WriteString(" AND sh.status = ?")
		args = append(args, f.Status)
	}
	if f.OrderType != "" {
		sb.WriteString(" AND COALESCE(o.order_type, 'SALES_ORDER') = ?")
		args = append(args, f.OrderType)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(` AND (sh.shipment_no LIKE ? OR pj.order_no LIKE ? OR sh.tracking_no LIKE ? OR EXISTS (
			SELECT 1 FROM shipment_items si_s JOIN products p_s ON p_s.id = si_s.product_id
			WHERE si_s.shipment_id = sh.id AND (p_s.sku LIKE ? OR p_s.name LIKE ?)
		))`)
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}

	return sb.String(), args
}

var sortColumns = map[string]string{
	"ship_date":   "sh.ship_date",
	"shipment_no": "sh.shipment_no",
	"status":      "sh.status",
}

func sortClause(sortBy, sortDir string) string {
	col, ok := sortColumns[sortBy]
	if !ok {
		col = sortColumns["ship_date"]
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + ", sh.id " + dir
}

type rawRow struct {
	ShipmentID            int64
	ShipmentNo            string
	OrderNo               string
	OrderType             string
	CustomerName          *string
	ShipToName            *string
	ShipToAddress         *string
	WarehouseCode         *string
	WarehouseName         *string
	Carrier               *string
	TrackingNo            *string
	Packages              int
	Weight                *float64
	Status                string
	ShipDate              *time.Time
	DispatchedAt          *time.Time
	DeliveredAt           *time.Time
	ShipmentItemID        int64
	ProductID             int64
	SKU                   string
	ProductName           string
	LotNo                 string
	LocationCode          *string
	Qty                   float64
	UOM                   string
	DeliveryLeadTimeHours *float64
	IsException           bool
	ExceptionRemark       *string
}

func (r rawRow) toRow() Row {
	warehouseCode, warehouseName := "", ""
	if r.WarehouseCode != nil {
		warehouseCode = *r.WarehouseCode
	}
	if r.WarehouseName != nil {
		warehouseName = *r.WarehouseName
	}
	return Row{
		ShipmentID: r.ShipmentID, ShipmentNo: r.ShipmentNo, OrderNo: r.OrderNo, OrderType: r.OrderType,
		CustomerName: r.CustomerName, ShipToName: r.ShipToName, ShipToAddress: r.ShipToAddress,
		WarehouseCode: warehouseCode, WarehouseName: warehouseName,
		Carrier: r.Carrier, TrackingNo: r.TrackingNo, Packages: r.Packages, Weight: r.Weight,
		Status: r.Status, ShipDate: r.ShipDate, DispatchedAt: r.DispatchedAt, DeliveredAt: r.DeliveredAt,
		ShipmentItemID: r.ShipmentItemID, ProductID: r.ProductID, SKU: r.SKU, ProductName: r.ProductName,
		LotNo: r.LotNo, LocationCode: r.LocationCode, Qty: r.Qty, UOM: r.UOM,
		DeliveryLeadTimeHours: r.DeliveryLeadTimeHours, IsException: r.IsException, ExceptionRemark: r.ExceptionRemark,
	}
}

const listSelectAndFrom = `
SELECT
	sh.id AS shipment_id, sh.shipment_no, COALESCE(pj.order_no, '') AS order_no,
	COALESCE(o.order_type, 'SALES_ORDER') AS order_type,
	pj.customer AS customer_name, pj.ship_to_name, pj.ship_to_address,
	w.code AS warehouse_code, w.name AS warehouse_name,
	sh.carrier, sh.tracking_no, sh.packages, sh.weight, sh.status,
	sh.ship_date, sh.dispatched_at, sh.delivered_at,
	si.id AS shipment_item_id, si.product_id, p.sku, p.name AS product_name,
	si.lot_no, loc.code AS location_code, si.qty, si.uom,
	CASE WHEN sh.dispatched_at IS NOT NULL AND sh.delivered_at IS NOT NULL
		THEN DATEDIFF(MINUTE, sh.dispatched_at, sh.delivered_at) / 60.0 END AS delivery_lead_time_hours,
	CASE WHEN sh.status = 'ISSUE' THEN CAST(1 AS BIT) ELSE CAST(0 AS BIT) END AS is_exception,
	CASE WHEN sh.status = 'ISSUE' THEN sh.remark END AS exception_remark
` + shipmentJoins + `
JOIN shipment_items si ON si.shipment_id = sh.id
JOIN products p ON p.id = si.product_id
LEFT JOIN locations loc ON loc.id = si.location_id
`

// List returns one page of Shipment Item Detail rows plus the total
// row count across every page.
func (repo *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]Row, int64, error) {
	where, args := scopeWhere(tenantID, params.Filters)

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

// ListForExport returns every matching row up to maxRows, with no
// pagination.
func (repo *Repository) ListForExport(ctx context.Context, tenantID int64, filters Filters, maxRows int) ([]Row, error) {
	where, args := scopeWhere(tenantID, filters)
	dataSQL := listSelectAndFrom + " WHERE " + where +
		" ORDER BY " + sortClause("ship_date", "desc") +
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

// Summary computes the Delivery & Shipping Report's KPI panel and
// chart data, at shipment grain throughout (never Row's own exploded
// item grain) so a shipment with several lines is never counted
// several times.
func (repo *Repository) Summary(ctx context.Context, tenantID int64, filters Filters) (Summary, error) {
	where, args := scopeWhere(tenantID, filters)

	var summary Summary

	type headerAgg struct {
		Total        int64
		NonCancelled int64
		Dispatched   int64
		Delivered    int64
		Issue        int64
		AvgLeadHours *float64
	}
	var h headerAgg
	headerSQL := `SELECT
		COUNT(*) AS total,
		SUM(CASE WHEN sh.status <> 'CANCELLED' THEN 1 ELSE 0 END) AS non_cancelled,
		SUM(CASE WHEN sh.dispatched_at IS NOT NULL THEN 1 ELSE 0 END) AS dispatched,
		SUM(CASE WHEN sh.status = 'DELIVERED' THEN 1 ELSE 0 END) AS delivered,
		SUM(CASE WHEN sh.status = 'ISSUE' THEN 1 ELSE 0 END) AS issue,
		AVG(CASE WHEN sh.dispatched_at IS NOT NULL AND sh.delivered_at IS NOT NULL
			THEN DATEDIFF(MINUTE, sh.dispatched_at, sh.delivered_at) / 60.0 END) AS avg_lead_hours
		` + shipmentJoins + ` WHERE ` + where
	if err := repo.db.WithContext(ctx).Raw(headerSQL, args...).Scan(&h).Error; err != nil {
		return Summary{}, err
	}
	summary.TotalShipments = h.Total
	summary.DispatchRate = rateOf(h.Dispatched, h.NonCancelled)
	summary.DeliveryConfirmationRate = rateOf(h.Delivered, h.Dispatched)
	summary.ExceptionRate = rateOf(h.Issue, h.NonCancelled)
	summary.AvgDeliveryLeadTimeHours = h.AvgLeadHours

	charts, err := repo.charts(ctx, where, args)
	if err != nil {
		return Summary{}, err
	}
	summary.Charts = charts

	totalTrips, completedTrips, avgLoadingHours, err := repo.loadingSummary(ctx, tenantID, filters)
	if err != nil {
		return Summary{}, err
	}
	summary.TotalTrips = totalTrips
	summary.CompletedTrips = completedTrips
	summary.AvgLoadingDurationHours = avgLoadingHours

	return summary, nil
}

func rateOf(numerator, denominator int64) RateWithDenominator {
	r := RateWithDenominator{Numerator: numerator, Denominator: denominator}
	if denominator > 0 {
		pct := float64(numerator) / float64(denominator) * 100
		r.RatePct = &pct
	}
	return r
}

func (repo *Repository) charts(ctx context.Context, where string, args []interface{}) (Charts, error) {
	var charts Charts

	trendSQL := `SELECT CONVERT(varchar(10), sh.created_at, 23) AS date, COUNT(*) AS shipment_count
		` + shipmentJoins + ` WHERE ` + where + `
		GROUP BY CONVERT(varchar(10), sh.created_at, 23) ORDER BY date`
	if err := repo.db.WithContext(ctx).Raw(trendSQL, args...).Scan(&charts.Trend).Error; err != nil {
		return Charts{}, err
	}

	byStatusSQL := `SELECT sh.status AS name, COUNT(*) AS count ` + shipmentJoins + ` WHERE ` + where + `
		GROUP BY sh.status ORDER BY count DESC`
	if err := repo.db.WithContext(ctx).Raw(byStatusSQL, args...).Scan(&charts.ByStatus).Error; err != nil {
		return Charts{}, err
	}

	byCarrierSQL := `SELECT COALESCE(sh.carrier, 'Unspecified') AS name, COUNT(*) AS count
		` + shipmentJoins + ` WHERE ` + where + `
		GROUP BY COALESCE(sh.carrier, 'Unspecified') ORDER BY count DESC`
	if err := repo.db.WithContext(ctx).Raw(byCarrierSQL, args...).Scan(&charts.ByCarrier).Error; err != nil {
		return Charts{}, err
	}

	byWarehouseSQL := `SELECT COALESCE(w.name, 'Unspecified') AS name, COUNT(*) AS count
		` + shipmentJoins + ` WHERE ` + where + `
		GROUP BY COALESCE(w.name, 'Unspecified') ORDER BY count DESC`
	if err := repo.db.WithContext(ctx).Raw(byWarehouseSQL, args...).Scan(&charts.ByWarehouse).Error; err != nil {
		return Charts{}, err
	}

	return charts, nil
}

// loadingSummary scopes loading_trips by tenant + the Filters fields
// that genuinely apply to a trip (From/To on the trip's own created_at,
// Carrier) — WarehouseID/Status/OrderType/Search are shipment-side
// concepts with no equivalent column on loading_trips (the trip header
// has no warehouse_id at all — see package doc), so they are
// deliberately not applied here.
func (repo *Repository) loadingSummary(ctx context.Context, tenantID int64, f Filters) (total, completed int64, avgHours *float64, err error) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("tenant_id = ?")
	if f.From != nil {
		sb.WriteString(" AND created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND created_at < ?")
		args = append(args, *f.To)
	}
	if f.Carrier != "" {
		sb.WriteString(" AND carrier = ?")
		args = append(args, f.Carrier)
	}

	var result struct {
		Total     int64
		Completed int64
		AvgHours  *float64
	}
	sql := `SELECT COUNT(*) AS total, SUM(CASE WHEN status = 'COMPLETED' THEN 1 ELSE 0 END) AS completed,
		AVG(CASE WHEN actual_loading_start_at IS NOT NULL AND actual_loading_end_at IS NOT NULL
			THEN DATEDIFF(MINUTE, actual_loading_start_at, actual_loading_end_at) / 60.0 END) AS avg_hours
		FROM loading_trips WHERE ` + sb.String()
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&result).Error; err != nil {
		return 0, 0, nil, err
	}
	return result.Total, result.Completed, result.AvgHours, nil
}

// tripScopeWhere scopes the Loading & Dispatch Report's own trip-grain
// list — From/To/Carrier on the trip itself, plus Search against
// trip_no/vehicle_no/driver_name.
func tripScopeWhere(tenantID int64, f Filters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("t.tenant_id = ?")
	if f.From != nil {
		sb.WriteString(" AND t.created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND t.created_at < ?")
		args = append(args, *f.To)
	}
	if f.Carrier != "" {
		sb.WriteString(" AND t.carrier = ?")
		args = append(args, f.Carrier)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(" AND (t.trip_no LIKE ? OR t.vehicle_no LIKE ? OR t.driver_name LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}
	return sb.String(), args
}

const tripSelectAndFrom = `
SELECT
	t.id AS trip_id, t.trip_no, t.vehicle_no, t.driver_name, t.carrier, t.loading_dock, t.route, t.status,
	t.scheduled_loading_time, t.actual_loading_start_at, t.actual_loading_end_at, t.completed_at,
	COALESCE(ts.shipment_count, 0) AS shipment_count,
	COALESCE(tc.carton_count, 0) AS carton_count,
	COALESCE(tc.short_count, 0) AS short_carton_count,
	CASE WHEN t.actual_loading_start_at IS NOT NULL AND t.actual_loading_end_at IS NOT NULL
		THEN DATEDIFF(MINUTE, t.actual_loading_start_at, t.actual_loading_end_at) / 60.0 END AS loading_duration_hours
FROM loading_trips t
LEFT JOIN (SELECT trip_id, COUNT(*) AS shipment_count FROM loading_trip_shipments GROUP BY trip_id) ts ON ts.trip_id = t.id
LEFT JOIN (
	SELECT trip_id, COUNT(*) AS carton_count, SUM(CASE WHEN status = 'SHORT' THEN 1 ELSE 0 END) AS short_count
	FROM loading_trip_cartons GROUP BY trip_id
) tc ON tc.trip_id = t.id
`

// ListTrips returns the Loading & Dispatch Report's own trip-grain
// rows, capped at maxRows — this secondary view is CSV-export-only (no
// XLSX/PDF), matching every other Phase 34 secondary view's own
// documented scope decision.
func (repo *Repository) ListTrips(ctx context.Context, tenantID int64, filters Filters, maxRows int) ([]TripRow, error) {
	where, args := tripScopeWhere(tenantID, filters)
	sql := tripSelectAndFrom + " WHERE " + where + " ORDER BY t.created_at DESC, t.id DESC OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY"
	dataArgs := append(append([]interface{}{}, args...), maxRows)

	var rows []TripRow
	if err := repo.db.WithContext(ctx).Raw(sql, dataArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

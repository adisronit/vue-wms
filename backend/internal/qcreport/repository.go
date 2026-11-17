package qcreport

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Repository runs the QC Report's read-only queries directly against
// qc_inspections/qc_inspection_items/qc_dispositions/receipts/
// receipt_item_splits/products/inventory — see the package doc for why
// this reads those tables via raw SQL rather than depending on
// internal/qualitycheck's own Go API.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// scopeWhere builds the WHERE clause every item-grain query shares —
// list, count, and Summary's own aggregates all apply this exact same
// scope for Filters' shared fields, per CLAUDE.md's "exports must
// respect exactly the same filters as the displayed report" rule.
// Disposition/DefectCode are deliberately NOT part of this clause —
// see Filters' own doc comment for why they are list-only refinements.
func scopeWhere(tenantID int64, f Filters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("qi.tenant_id = ?")

	if f.From != nil {
		sb.WriteString(" AND qi.created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND qi.created_at < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND qi.warehouse_id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.SupplierID != nil {
		sb.WriteString(" AND r.supplier_id = ?")
		args = append(args, *f.SupplierID)
	}
	if f.QCType != "" {
		sb.WriteString(" AND qi.qc_type = ?")
		args = append(args, f.QCType)
	}
	if f.InspectorID != nil {
		sb.WriteString(" AND qi.inspector_id = ?")
		args = append(args, *f.InspectorID)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(" AND (qi.inspection_no LIKE ? OR r.receipt_no LIKE ? OR p.sku LIKE ? OR p.name LIKE ?)")
		args = append(args, pattern, pattern, pattern, pattern)
	}

	return sb.String(), args
}

// resolutionStatusCase mirrors internal/receivingreport's own
// qcStatusCase arithmetic exactly (PENDING/PARTIAL/MIXED/PASSED/
// REJECTED), applied at item grain instead of split grain — see
// package doc.
const resolutionStatusCase = `CASE
	WHEN COALESCE(ds.pass_qty, 0) + COALESCE(ds.reject_qty, 0) <= 0 THEN 'PENDING'
	WHEN COALESCE(ds.pass_qty, 0) + COALESCE(ds.reject_qty, 0) < qii.held_qty_snapshot THEN 'PARTIAL'
	WHEN ds.pass_qty > 0 AND ds.reject_qty > 0 THEN 'MIXED'
	WHEN ds.reject_qty > 0 THEN 'REJECTED'
	ELSE 'PASSED'
END`

// dispSumsAndLastJoin computes, per qc_inspection_item, the
// pass/reject quantity totals plus the two timing anchors (last
// decision, first HOLD), and separately the single most recent
// disposition's own defect_code/evidence_reference/remark — both as
// inline derived tables (not named CTEs), so this fragment can be
// nested inside another aggregate query (Summary) without hitting
// SQL Server's "a CTE cannot be nested inside a derived table"
// restriction.
const dispSumsAndLastJoin = `LEFT JOIN (
	SELECT inspection_item_id,
		SUM(CASE WHEN disposition = 'PASS' THEN quantity ELSE 0 END) AS pass_qty,
		SUM(CASE WHEN disposition = 'REJECT' THEN quantity ELSE 0 END) AS reject_qty,
		MAX(created_at) AS last_decision_at,
		MIN(CASE WHEN disposition = 'HOLD' THEN created_at END) AS first_hold_at
	FROM qc_dispositions
	GROUP BY inspection_item_id
) ds ON ds.inspection_item_id = qii.id
LEFT JOIN (
	SELECT inspection_item_id, defect_code, evidence_reference, remark,
		ROW_NUMBER() OVER (PARTITION BY inspection_item_id ORDER BY created_at DESC, id DESC) AS rn
	FROM qc_dispositions
) dl ON dl.inspection_item_id = qii.id AND dl.rn = 1`

const innerSelectAndFrom = `
SELECT
	qi.id AS inspection_id, qi.inspection_no, qi.receipt_id, r.receipt_no,
	w.code AS warehouse_code, w.name AS warehouse_name,
	COALESCE(s.company_name, r.supplier_name_snapshot, r.vendor) AS supplier_name,
	ins.full_name AS inspector_name,
	qi.qc_type, qi.created_at AS inspection_created_at,
	qii.id AS inspection_item_id, p.sku, p.name AS product_name,
	qii.lot_no, qii.expiry_date, loc.code AS location_code,
	qii.held_qty_snapshot, qii.sample_qty, qii.temperature_c, qii.packaging_ok, qii.label_ok,
	COALESCE(ds.pass_qty, 0) AS pass_qty, COALESCE(ds.reject_qty, 0) AS reject_qty,
	CASE WHEN qii.held_qty_snapshot - COALESCE(ds.pass_qty, 0) - COALESCE(ds.reject_qty, 0) > 0
		THEN qii.held_qty_snapshot - COALESCE(ds.pass_qty, 0) - COALESCE(ds.reject_qty, 0) ELSE 0 END AS remaining_hold_qty,
	` + resolutionStatusCase + ` AS resolution_status,
	dl.defect_code, dl.evidence_reference, COALESCE(dl.remark, qi.remark) AS remark,
	ds.last_decision_at, ds.first_hold_at
FROM qc_inspection_items qii
JOIN qc_inspections qi ON qi.id = qii.inspection_id
JOIN receipts r ON r.id = qi.receipt_id
JOIN warehouses w ON w.id = qi.warehouse_id
LEFT JOIN suppliers s ON s.id = r.supplier_id
LEFT JOIN users ins ON ins.id = qi.inspector_id
JOIN products p ON p.id = qii.product_id
LEFT JOIN locations loc ON loc.id = qii.location_id
` + dispSumsAndLastJoin

// itemQuery returns one complete, standalone item-grain SELECT (never
// starting with WITH, so it can be freely nested inside a further
// wrapping query for COUNT/pagination/Summary aggregation) scoped by
// Filters and, when includeRefinements is true, further narrowed by
// Disposition/DefectCode. now is passed in explicitly (not GETUTCDATE())
// so Hold Aging is deterministically testable, mirroring
// internal/inventoryreport's own now-as-a-parameter precedent.
func itemQuery(tenantID int64, f Filters, now time.Time, includeRefinements bool) (string, []interface{}) {
	where, whereArgs := scopeWhere(tenantID, f)

	var sb strings.Builder
	sb.WriteString(`SELECT t.*,
		CASE WHEN t.resolution_status IN ('PASSED', 'REJECTED', 'MIXED')
			THEN DATEDIFF(MINUTE, t.inspection_created_at, t.last_decision_at) / 60.0 END AS qc_turnaround_hours,
		CASE WHEN t.resolution_status IN ('PENDING', 'PARTIAL')
			THEN DATEDIFF(MINUTE, COALESCE(t.first_hold_at, t.inspection_created_at), ?) / 60.0 END AS hold_aging_hours
	FROM (` + innerSelectAndFrom + ` WHERE ` + where + `) t
	WHERE 1 = 1`)

	args := []interface{}{now}
	args = append(args, whereArgs...)

	if includeRefinements {
		if f.Disposition != "" {
			sb.WriteString(" AND t.resolution_status = ?")
			args = append(args, f.Disposition)
		}
		if f.DefectCode != "" {
			sb.WriteString(" AND t.defect_code LIKE ?")
			args = append(args, "%"+f.DefectCode+"%")
		}
	}

	return sb.String(), args
}

var sortColumns = map[string]string{
	"inspection_date":   "t.inspection_created_at",
	"inspection_no":     "t.inspection_no",
	"warehouse_name":    "t.warehouse_name",
	"resolution_status": "t.resolution_status",
}

func sortClause(sortBy, sortDir string) string {
	col, ok := sortColumns[sortBy]
	if !ok {
		col = sortColumns["inspection_date"]
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + ", t.inspection_item_id " + dir
}

// rawRow mirrors itemQuery's full column list — GORM Scan matches by
// column name, not position or completeness, so unused passthrough
// columns (last_decision_at, first_hold_at) are simply never read.
type rawRow struct {
	InspectionID        int64
	InspectionNo        string
	ReceiptID           int64
	ReceiptNo           string
	WarehouseCode       string
	WarehouseName       string
	SupplierName        *string
	InspectorName       *string
	QCType              string
	InspectionCreatedAt time.Time
	InspectionItemID    int64
	SKU                 string
	ProductName         string
	LotNo               string
	ExpiryDate          *time.Time
	LocationCode        *string
	HeldQtySnapshot     float64
	SampleQty           float64
	TemperatureC        *float64
	PackagingOK         *bool
	LabelOK             *bool
	PassQty             float64
	RejectQty           float64
	RemainingHoldQty    float64
	ResolutionStatus    string
	DefectCode          *string
	EvidenceReference   *string
	Remark              *string
	QCTurnaroundHours   *float64
	HoldAgingHours      *float64
}

func (r rawRow) toRow() Row {
	return Row{
		InspectionID: r.InspectionID, InspectionNo: r.InspectionNo, ReceiptID: r.ReceiptID, ReceiptNo: r.ReceiptNo,
		WarehouseCode: r.WarehouseCode, WarehouseName: r.WarehouseName,
		SupplierName: r.SupplierName, InspectorName: r.InspectorName,
		QCType: r.QCType, InspectionCreatedAt: r.InspectionCreatedAt,
		InspectionItemID: r.InspectionItemID, SKU: r.SKU, ProductName: r.ProductName,
		LotNo: r.LotNo, ExpiryDate: r.ExpiryDate, LocationCode: r.LocationCode,
		HeldQtySnapshot: r.HeldQtySnapshot, SampleQty: r.SampleQty, TemperatureC: r.TemperatureC,
		PackagingOK: r.PackagingOK, LabelOK: r.LabelOK,
		PassQty: r.PassQty, RejectQty: r.RejectQty, RemainingHoldQty: r.RemainingHoldQty,
		ResolutionStatus: r.ResolutionStatus,
		DefectCode:       r.DefectCode, EvidenceReference: r.EvidenceReference, Remark: r.Remark,
		QCTurnaroundHours: r.QCTurnaroundHours, HoldAgingHours: r.HoldAgingHours,
	}
}

// List returns one page of report rows plus the total row count
// across every page.
func (repo *Repository) List(ctx context.Context, tenantID int64, params ListParams, now time.Time) ([]Row, int64, error) {
	query, args := itemQuery(tenantID, params.Filters, now, true)

	var total int64
	countSQL := "SELECT COUNT(*) FROM (" + query + ") cnt"
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

	dataSQL := query + " ORDER BY " + sortClause(params.SortBy, params.SortDir) + " OFFSET ? ROWS FETCH NEXT ? ROWS ONLY"
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
// pagination — the shared cap every synchronous export in this
// codebase uses.
func (repo *Repository) ListForExport(ctx context.Context, tenantID int64, filters Filters, now time.Time, maxRows int) ([]Row, error) {
	query, args := itemQuery(tenantID, filters, now, true)
	dataSQL := query + " ORDER BY " + sortClause("inspection_date", "desc") + " OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY"
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

// Summary computes the QC Report's KPI panel and chart data. Every
// aggregate here is computed from the SAME item-grain query List
// uses, scoped by Filters only (never the list-only Disposition/
// DefectCode refinements — see Filters' own doc) so a KPI's
// denominator never silently shifts underneath a drill-down filter.
func (repo *Repository) Summary(ctx context.Context, tenantID int64, filters Filters, now time.Time) (Summary, error) {
	query, args := itemQuery(tenantID, filters, now, false)

	var summary Summary

	type agg struct {
		Total         int64
		SumPass       float64
		SumReject     float64
		SumRemaining  float64
		SumHeld       float64
		OpenCount     int64
		AvgTurnaround *float64
		AvgHoldAging  *float64
		MaxHoldAging  *float64
	}
	var a agg
	aggSQL := `SELECT
		COUNT(*) AS total,
		COALESCE(SUM(pass_qty), 0) AS sum_pass,
		COALESCE(SUM(reject_qty), 0) AS sum_reject,
		COALESCE(SUM(remaining_hold_qty), 0) AS sum_remaining,
		COALESCE(SUM(held_qty_snapshot), 0) AS sum_held,
		SUM(CASE WHEN resolution_status IN ('PENDING', 'PARTIAL') THEN 1 ELSE 0 END) AS open_count,
		AVG(qc_turnaround_hours) AS avg_turnaround,
		AVG(hold_aging_hours) AS avg_hold_aging,
		MAX(hold_aging_hours) AS max_hold_aging
	FROM (` + query + `) q`
	if err := repo.db.WithContext(ctx).Raw(aggSQL, args...).Scan(&a).Error; err != nil {
		return Summary{}, err
	}

	summary.TotalInspectionItems = a.Total
	decided := a.SumPass + a.SumReject
	summary.PassRate = RateWithDenominator{Numerator: a.SumPass, Denominator: decided}
	summary.RejectionRate = RateWithDenominator{Numerator: a.SumReject, Denominator: decided}
	if decided > 0 {
		passRate := a.SumPass / decided * 100
		rejectRate := a.SumReject / decided * 100
		summary.PassRate.RatePct = &passRate
		summary.RejectionRate.RatePct = &rejectRate
	}
	summary.HoldRate = RateWithDenominator{Numerator: a.SumRemaining, Denominator: a.SumHeld}
	if a.SumHeld > 0 {
		holdRate := a.SumRemaining / a.SumHeld * 100
		summary.HoldRate.RatePct = &holdRate
	}
	summary.AvgTurnaroundHours = a.AvgTurnaround
	summary.AvgHoldAgingHours = a.AvgHoldAging
	summary.MaxHoldAgingHours = a.MaxHoldAging
	summary.PendingQCAwaitingDecisionCount = a.OpenCount
	summary.PendingQCAwaitingDecisionQty = a.SumRemaining

	notStartedCount, notStartedQty, err := repo.pendingNotStarted(ctx, tenantID, filters)
	if err != nil {
		return Summary{}, err
	}
	summary.PendingQCNotStartedCount = notStartedCount
	summary.PendingQCNotStartedQty = notStartedQty

	charts, err := repo.charts(ctx, query, args)
	if err != nil {
		return Summary{}, err
	}
	summary.Charts = charts

	return summary, nil
}

// pendingNotStarted counts/sums receipt_item_splits whose product
// requires QC and still carries a live inventory.hold_quantity > 0,
// but has no qc_inspection_items row at all yet — the sub-case of
// "Pending QC" that this report's own item-grain query (and
// internal/receivingreport's own Pending QC Qty) cannot see, because
// no qc_inspection_items row exists to join against. Scoped only by
// WarehouseID/SupplierID/From/To (on the receipt's own receiving
// date) — QCType/InspectorID/Search/Disposition/DefectCode are
// meaningless for an inspection that was never started, so they are
// not applied here (see Filters' own doc).
func (repo *Repository) pendingNotStarted(ctx context.Context, tenantID int64, f Filters) (int64, float64, error) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString(`SELECT COUNT(*) AS cnt, COALESCE(SUM(inv.hold_quantity), 0) AS qty
		FROM receipt_item_splits sp
		JOIN receipt_items ri ON ri.id = sp.receipt_item_id
		JOIN receipts r ON r.id = ri.receipt_id
		JOIN products p ON p.id = ri.product_id
		JOIN inventory inv ON inv.tenant_id = r.tenant_id AND inv.product_id = ri.product_id
			AND inv.location_id = sp.location_id AND inv.lot_no = sp.lot_no
		WHERE r.tenant_id = ? AND p.qc_required = 1 AND inv.hold_quantity > 0
			AND NOT EXISTS (SELECT 1 FROM qc_inspection_items qii WHERE qii.receipt_item_split_id = sp.id)`)

	if f.From != nil {
		sb.WriteString(" AND COALESCE(r.receiving_date, r.created_at) >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND COALESCE(r.receiving_date, r.created_at) < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND r.warehouse_id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.SupplierID != nil {
		sb.WriteString(" AND r.supplier_id = ?")
		args = append(args, *f.SupplierID)
	}

	var result struct {
		Cnt int64
		Qty float64
	}
	if err := repo.db.WithContext(ctx).Raw(sb.String(), args...).Scan(&result).Error; err != nil {
		return 0, 0, err
	}
	return result.Cnt, result.Qty, nil
}

func (repo *Repository) charts(ctx context.Context, query string, args []interface{}) (Charts, error) {
	var charts Charts

	defectSQL := `SELECT defect_code, COUNT(*) AS count, SUM(reject_qty) AS qty
		FROM (` + query + `) q
		WHERE resolution_status IN ('REJECTED', 'MIXED') AND defect_code IS NOT NULL AND defect_code <> ''
		GROUP BY defect_code
		ORDER BY qty DESC`
	if err := repo.db.WithContext(ctx).Raw(defectSQL, args...).Scan(&charts.TopDefects).Error; err != nil {
		return Charts{}, err
	}

	supplierTrendSQL := `SELECT
		CONVERT(varchar(10), inspection_created_at, 23) AS date,
		COALESCE(supplier_name, 'Unspecified') AS supplier,
		SUM(reject_qty) AS reject_qty
		FROM (` + query + `) q
		WHERE reject_qty > 0
		GROUP BY CONVERT(varchar(10), inspection_created_at, 23), COALESCE(supplier_name, 'Unspecified')
		ORDER BY date`
	if err := repo.db.WithContext(ctx).Raw(supplierTrendSQL, args...).Scan(&charts.SupplierRejectionTrend).Error; err != nil {
		return Charts{}, err
	}

	byQCTypeSQL := `SELECT qc_type AS name, COUNT(*) AS count
		FROM (` + query + `) q
		GROUP BY qc_type ORDER BY count DESC`
	if err := repo.db.WithContext(ctx).Raw(byQCTypeSQL, args...).Scan(&charts.ByQCType).Error; err != nil {
		return Charts{}, err
	}

	return charts, nil
}

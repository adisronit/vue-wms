package receivingreport

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Repository runs the Receiving Report's read-only queries directly
// against receipts/receipt_items/receipt_item_splits/
// qc_inspection_items/qc_dispositions/putaway_task_items — see the
// package doc for why this reads those tables via raw SQL rather than
// depending on internal/receiving/qualitycheck/putaway's own Go APIs.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// scopeWhere builds the WHERE clause every Receiving Report query
// shares — list, count, and every Summary aggregate — so the exact
// same filters always produce the exact same scope, per CLAUDE.md's
// "exports must respect exactly the same filters as the displayed
// report" rule. Assumes the query it's used in joins receipts AS r
// (always), warehouses AS w ON w.id = r.warehouse_id, and
// users AS u ON u.id = r.created_by (both LEFT JOINs are harmless even
// when the query itself does not reference w/u).
//
// Search/LotNo are expressed as EXISTS subqueries scoped to r.id
// rather than direct joins, so applying them against a receipt-grain
// or receipt_item-grain aggregate query never fans a receipt out into
// duplicate rows the way a direct JOIN to receipt_items/
// receipt_item_splits would — the one case that would silently double-
// count a KPI.
func scopeWhere(tenantID int64, f Filters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("r.tenant_id = ?")

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
	if f.Status != "" {
		sb.WriteString(" AND r.status = ?")
		args = append(args, f.Status)
	}
	if f.OperatorID != nil {
		sb.WriteString(" AND r.created_by = ?")
		args = append(args, *f.OperatorID)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(` AND (r.receipt_no LIKE ? OR r.reference_no LIKE ? OR EXISTS (
			SELECT 1 FROM receipt_items ri_s JOIN products p_s ON p_s.id = ri_s.product_id
			WHERE ri_s.receipt_id = r.id AND (p_s.sku LIKE ? OR p_s.name LIKE ?)
		))`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if f.LotNo != "" {
		sb.WriteString(` AND EXISTS (
			SELECT 1 FROM receipt_items ri_l WHERE ri_l.receipt_id = r.id AND (
				ri_l.lot_no = ? OR EXISTS (SELECT 1 FROM receipt_item_splits sp_l WHERE sp_l.receipt_item_id = ri_l.id AND sp_l.lot_no = ?)
			)
		)`)
		args = append(args, f.LotNo, f.LotNo)
	}

	return sb.String(), args
}

var sortColumns = map[string]string{
	"receiving_date": "COALESCE(r.receiving_date, r.created_at)",
	"receipt_no":     "r.receipt_no",
	"warehouse_name": "w.name",
	"status":         "r.status",
}

func sortClause(sortBy, sortDir string) string {
	col, ok := sortColumns[sortBy]
	if !ok {
		col = sortColumns["receiving_date"]
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	// receipt_id as a tiebreaker keeps pagination stable across pages
	// when many rows share the same sort value (e.g. same receiving_date).
	return col + " " + dir + ", r.id " + dir
}

// qcPutawayJoins is the shared aggregation CTEs the list query and the
// pending-QC/pending-putaway summary queries both need — kept as one
// constant so the derivation rule is written exactly once.
//
// qc_status derivation: NOT_REQUIRED when the product never required
// QC; PENDING when no inspection item exists yet, or one exists but no
// disposition has been recorded; PARTIAL when some but not all of the
// held quantity has been resolved (PASS+REJECT); PASSED/REJECTED when
// fully resolved one way; MIXED when fully resolved with both outcomes
// present (split across two dispositions) — mirrors Phase 31B's own
// PASS/HOLD/REJECT workflow (HOLD dispositions do not resolve
// anything, they are not counted in pass_qty/reject_qty).
//
// putaway_status derivation: NOT_STARTED when no putaway_task_items
// row references this receipt_item yet; otherwise mirrors
// internal/putaway's own itemStatus rule (ISSUE overrides regardless
// of quantity, then PENDING/IN_PROGRESS/COMPLETED by moved vs
// requested) — aggregated across every task item referencing this
// line, since putaway_task_items.receipt_item_id is line-level, not
// split-level (a pre-existing, documented limitation — see
// internal/putaway's "Sources still line-level" note — not introduced
// here).
const qcPutawayCTEs = `
qc_agg AS (
	SELECT
		qii.receipt_item_split_id AS split_id,
		SUM(qii.held_qty_snapshot) AS held_qty_snapshot,
		COUNT(*) AS inspection_item_count,
		SUM(COALESCE(d.pass_qty, 0)) AS pass_qty,
		SUM(COALESCE(d.reject_qty, 0)) AS reject_qty
	FROM qc_inspection_items qii
	LEFT JOIN (
		SELECT inspection_item_id,
			SUM(CASE WHEN disposition = 'PASS' THEN quantity ELSE 0 END) AS pass_qty,
			SUM(CASE WHEN disposition = 'REJECT' THEN quantity ELSE 0 END) AS reject_qty
		FROM qc_dispositions
		GROUP BY inspection_item_id
	) d ON d.inspection_item_id = qii.id
	GROUP BY qii.receipt_item_split_id
),
putaway_agg AS (
	SELECT
		pti.receipt_item_id AS item_id,
		SUM(pti.requested_qty) AS requested_qty,
		SUM(pti.moved_qty) AS moved_qty,
		MAX(CASE WHEN pti.status = 'ISSUE' THEN 1 ELSE 0 END) AS any_issue,
		COUNT(*) AS task_item_count
	FROM putaway_task_items pti
	GROUP BY pti.receipt_item_id
)
`

const qcStatusCase = `CASE
	WHEN p.qc_required = 0 THEN 'NOT_REQUIRED'
	WHEN qc.held_qty_snapshot IS NULL THEN 'PENDING'
	WHEN (qc.pass_qty + qc.reject_qty) <= 0 THEN 'PENDING'
	WHEN (qc.pass_qty + qc.reject_qty) < qc.held_qty_snapshot THEN 'PARTIAL'
	WHEN qc.pass_qty > 0 AND qc.reject_qty > 0 THEN 'MIXED'
	WHEN qc.reject_qty > 0 THEN 'REJECTED'
	ELSE 'PASSED'
END`

const putawayStatusCase = `CASE
	WHEN pa.task_item_count IS NULL OR pa.task_item_count = 0 THEN 'NOT_STARTED'
	WHEN pa.any_issue = 1 THEN 'ISSUE'
	WHEN pa.moved_qty <= 0 THEN 'PENDING'
	WHEN pa.requested_qty > 0 AND pa.moved_qty < pa.requested_qty THEN 'IN_PROGRESS'
	ELSE 'COMPLETED'
END`

// rawRow mirrors the SELECT list of the list/export query exactly —
// GORM Scan matches by column name, not position, so field order here
// doesn't need to match the SQL text's order.
type rawRow struct {
	ReceiptID       int64
	ReceiptItemID   int64
	SplitID         *int64
	ReceiptNo       string
	ReceivingDate   *time.Time
	ReceivingType   string
	ReferenceNo     *string
	SupplierName    *string
	InvoiceNo       *string
	DeliveryNoteNo  *string
	WarehouseCode   string
	WarehouseName   string
	SKU             string
	ProductName     string
	BrandName       *string
	CategoryName    *string
	ExpectedQty     float64
	ReceivedQty     float64
	UOM             string
	LotNo           *string
	MfgDate         *time.Time
	ExpiryDate      *time.Time
	PalletID        *string
	TemperatureC    *float64
	ReceivingStatus string
	QCStatus        string
	PutawayStatus   string
	OperatorName    *string
}

func (r rawRow) toRow() Row {
	return Row{
		ReceiptID: r.ReceiptID, ReceiptItemID: r.ReceiptItemID, SplitID: r.SplitID,
		ReceiptNo: r.ReceiptNo, ReceivingDate: r.ReceivingDate, ReceivingType: r.ReceivingType,
		ReferenceNo: r.ReferenceNo, SupplierName: r.SupplierName,
		InvoiceNo: r.InvoiceNo, DeliveryNoteNo: r.DeliveryNoteNo,
		WarehouseCode: r.WarehouseCode, WarehouseName: r.WarehouseName,
		SKU: r.SKU, ProductName: r.ProductName, BrandName: r.BrandName, CategoryName: r.CategoryName,
		ExpectedQty: r.ExpectedQty, ReceivedQty: r.ReceivedQty, ShortOverQty: r.ReceivedQty - r.ExpectedQty,
		UOM: r.UOM, LotNo: r.LotNo, MfgDate: r.MfgDate, ExpiryDate: r.ExpiryDate,
		PalletID: r.PalletID, TemperatureC: r.TemperatureC,
		ReceivingStatus: r.ReceivingStatus, QCStatus: r.QCStatus, PutawayStatus: r.PutawayStatus,
		OperatorName: r.OperatorName,
	}
}

const listSelectAndFrom = `
SELECT
	r.id AS receipt_id, ri.id AS receipt_item_id, sp.id AS split_id,
	r.receipt_no, r.receiving_date, r.receiving_type, r.reference_no,
	COALESCE(s.company_name, r.supplier_name_snapshot, r.vendor) AS supplier_name,
	r.invoice_no, r.delivery_note_no,
	w.code AS warehouse_code, w.name AS warehouse_name,
	p.sku, p.name AS product_name, b.name AS brand_name, pc.name AS category_name,
	ri.expected_qty, ri.received_qty, ri.uom,
	COALESCE(sp.lot_no, ri.lot_no) AS lot_no,
	sp.mfg_date, COALESCE(sp.expiry_date, ri.expiry_date) AS expiry_date,
	sp.pallet_id, sp.temperature_c,
	r.status AS receiving_status,
	` + qcStatusCase + ` AS qc_status,
	` + putawayStatusCase + ` AS putaway_status,
	u.full_name AS operator_name
FROM receipts r
JOIN receipt_items ri ON ri.receipt_id = r.id
LEFT JOIN receipt_item_splits sp ON sp.receipt_item_id = ri.id
JOIN products p ON p.id = ri.product_id
LEFT JOIN brands b ON b.id = p.brand_id
LEFT JOIN product_categories pc ON pc.id = p.category_id
JOIN warehouses w ON w.id = r.warehouse_id
LEFT JOIN suppliers s ON s.id = r.supplier_id
LEFT JOIN users u ON u.id = r.created_by
LEFT JOIN qc_agg qc ON qc.split_id = sp.id
LEFT JOIN putaway_agg pa ON pa.item_id = ri.id
`

// List returns one page of report rows plus the total row count across
// every page (not just the current one) — the total the frontend's
// pagination footer shows, matching every other report's "never use
// UI pagination totals as complete report totals" requirement by
// giving it the real total up front instead of letting the caller
// infer it from page size.
func (repo *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]Row, int64, error) {
	where, args := scopeWhere(tenantID, params.Filters)

	var total int64
	countSQL := "WITH " + qcPutawayCTEs + " SELECT COUNT(*) FROM (" + listSelectAndFrom + " WHERE " + where + ") t"
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

	dataSQL := "WITH " + qcPutawayCTEs + " " + listSelectAndFrom + " WHERE " + where +
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
// pagination — the shared cap every synchronous export in this codebase
// uses (csvexport.MaxExportRows), applied identically for CSV, XLSX,
// and PDF exports of this report.
func (repo *Repository) ListForExport(ctx context.Context, tenantID int64, filters Filters, maxRows int) ([]Row, error) {
	where, args := scopeWhere(tenantID, filters)
	dataSQL := "WITH " + qcPutawayCTEs + " " + listSelectAndFrom + " WHERE " + where +
		" ORDER BY " + sortClause("receiving_date", "desc") +
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

// Summary computes the Receiving Report's KPI panel and chart data,
// each at the grain that avoids double-counting: receipt-level counts/
// rates/lead-time from a receipts-only query (no item/split join, so
// one receipt is always exactly one row), received-quantity-by-UOM
// from a receipt_items-only query (one item is always exactly one
// row), and pending QC quantity from a receipt_item_splits query (one
// split is always exactly one row) — never from the same exploded
// (item x split) grain the List query uses, which would multiply
// item-level quantities by however many splits a line happens to have.
func (repo *Repository) Summary(ctx context.Context, tenantID int64, filters Filters) (Summary, error) {
	where, args := scopeWhere(tenantID, filters)

	var summary Summary

	// Total receipts + completion rate + average lead time, all from the
	// same receipt-grain scope. lc: last confirmation timestamp per
	// receipt, only needed for COMPLETED receipts' lead-time average.
	type headerAgg struct {
		Total        int64
		Completed    int64
		AvgLeadHours *float64
	}
	var h headerAgg
	headerSQL := `WITH lc AS (
		SELECT receipt_id, MAX(created_at) AS last_confirmed_at FROM receipt_confirmations GROUP BY receipt_id
	)
	SELECT
		COUNT(*) AS total,
		SUM(CASE WHEN r.status = 'COMPLETED' THEN 1 ELSE 0 END) AS completed,
		AVG(CASE WHEN r.status = 'COMPLETED' THEN DATEDIFF(MINUTE, r.created_at, lc.last_confirmed_at) / 60.0 END) AS avg_lead_hours
	FROM receipts r
	LEFT JOIN lc ON lc.receipt_id = r.id
	JOIN warehouses w ON w.id = r.warehouse_id
	LEFT JOIN suppliers s ON s.id = r.supplier_id
	LEFT JOIN users u ON u.id = r.created_by
	WHERE ` + where
	if err := repo.db.WithContext(ctx).Raw(headerSQL, args...).Scan(&h).Error; err != nil {
		return Summary{}, err
	}
	summary.TotalReceipts = h.Total
	if h.Total > 0 {
		rate := float64(h.Completed) / float64(h.Total) * 100
		summary.CompletionRate = &rate
	}
	summary.AvgReceivingLeadTimeHours = h.AvgLeadHours

	// Received quantity by UOM — item-grain, one row per receipt_item.
	var uomRows []UOMQty
	uomSQL := `SELECT ri.uom AS uom, SUM(ri.received_qty) AS qty
		FROM receipts r JOIN receipt_items ri ON ri.receipt_id = r.id
		JOIN warehouses w ON w.id = r.warehouse_id
		LEFT JOIN suppliers s ON s.id = r.supplier_id
		LEFT JOIN users u ON u.id = r.created_by
		WHERE ` + where + ` GROUP BY ri.uom`
	if err := repo.db.WithContext(ctx).Raw(uomSQL, args...).Scan(&uomRows).Error; err != nil {
		return Summary{}, err
	}
	summary.TotalReceivedQtyByUOM = uomRows

	// Pending putaway quantity — item-grain (putaway_task_items is
	// line-level, matching this report's own PutawayStatus grain).
	var pendingPutaway float64
	putawaySQL := `WITH putaway_agg AS (
			SELECT receipt_item_id AS item_id, SUM(requested_qty) AS requested_qty, SUM(moved_qty) AS moved_qty
			FROM putaway_task_items GROUP BY receipt_item_id
		)
		SELECT COALESCE(SUM(GREATEST_ZERO.diff), 0) FROM (
			SELECT CASE WHEN pa.requested_qty - pa.moved_qty > 0 THEN pa.requested_qty - pa.moved_qty ELSE 0 END AS diff
			FROM receipts r JOIN receipt_items ri ON ri.receipt_id = r.id
			JOIN warehouses w ON w.id = r.warehouse_id
			LEFT JOIN suppliers s ON s.id = r.supplier_id
			LEFT JOIN users u ON u.id = r.created_by
			JOIN putaway_agg pa ON pa.item_id = ri.id
			WHERE ` + where + `
		) AS GREATEST_ZERO`
	if err := repo.db.WithContext(ctx).Raw(putawaySQL, args...).Scan(&pendingPutaway).Error; err != nil {
		return Summary{}, err
	}
	summary.PendingPutawayQty = pendingPutaway

	// Pending QC quantity — split-grain (QC is inspected per split).
	var pendingQC float64
	qcSQL := `WITH qc_agg AS (
			SELECT qii.receipt_item_split_id AS split_id, SUM(qii.held_qty_snapshot) AS held_qty_snapshot,
				SUM(COALESCE(d.pass_qty, 0)) AS pass_qty, SUM(COALESCE(d.reject_qty, 0)) AS reject_qty
			FROM qc_inspection_items qii
			LEFT JOIN (
				SELECT inspection_item_id,
					SUM(CASE WHEN disposition = 'PASS' THEN quantity ELSE 0 END) AS pass_qty,
					SUM(CASE WHEN disposition = 'REJECT' THEN quantity ELSE 0 END) AS reject_qty
				FROM qc_dispositions GROUP BY inspection_item_id
			) d ON d.inspection_item_id = qii.id
			GROUP BY qii.receipt_item_split_id
		)
		SELECT COALESCE(SUM(GREATEST_ZERO.diff), 0) FROM (
			SELECT CASE WHEN qc.held_qty_snapshot - qc.pass_qty - qc.reject_qty > 0 THEN qc.held_qty_snapshot - qc.pass_qty - qc.reject_qty ELSE 0 END AS diff
			FROM receipts r
			JOIN receipt_items ri ON ri.receipt_id = r.id
			JOIN receipt_item_splits sp ON sp.receipt_item_id = ri.id
			JOIN warehouses w ON w.id = r.warehouse_id
			LEFT JOIN suppliers s ON s.id = r.supplier_id
			LEFT JOIN users u ON u.id = r.created_by
			JOIN qc_agg qc ON qc.split_id = sp.id
			WHERE ` + where + `
		) AS GREATEST_ZERO`
	if err := repo.db.WithContext(ctx).Raw(qcSQL, args...).Scan(&pendingQC).Error; err != nil {
		return Summary{}, err
	}
	summary.PendingQCQty = pendingQC

	charts, err := repo.charts(ctx, where, args)
	if err != nil {
		return Summary{}, err
	}
	summary.Charts = charts

	return summary, nil
}

func (repo *Repository) charts(ctx context.Context, where string, args []interface{}) (Charts, error) {
	var charts Charts

	trendSQL := `SELECT
		CONVERT(varchar(10), COALESCE(r.receiving_date, r.created_at), 23) AS date,
		COUNT(DISTINCT r.id) AS receipt_count,
		SUM(ri.received_qty) AS received_qty
		FROM receipts r JOIN receipt_items ri ON ri.receipt_id = r.id
		JOIN warehouses w ON w.id = r.warehouse_id
		LEFT JOIN suppliers s ON s.id = r.supplier_id
		LEFT JOIN users u ON u.id = r.created_by
		WHERE ` + where + `
		GROUP BY CONVERT(varchar(10), COALESCE(r.receiving_date, r.created_at), 23)
		ORDER BY date`
	if err := repo.db.WithContext(ctx).Raw(trendSQL, args...).Scan(&charts.Trend).Error; err != nil {
		return Charts{}, err
	}

	bySupplierSQL := `SELECT
		COALESCE(s.company_name, r.supplier_name_snapshot, r.vendor, 'Unspecified') AS name,
		COUNT(DISTINCT r.id) AS receipt_count,
		SUM(ri.received_qty) AS received_qty
		FROM receipts r JOIN receipt_items ri ON ri.receipt_id = r.id
		JOIN warehouses w ON w.id = r.warehouse_id
		LEFT JOIN suppliers s ON s.id = r.supplier_id
		LEFT JOIN users u ON u.id = r.created_by
		WHERE ` + where + `
		GROUP BY COALESCE(s.company_name, r.supplier_name_snapshot, r.vendor, 'Unspecified')
		ORDER BY received_qty DESC`
	if err := repo.db.WithContext(ctx).Raw(bySupplierSQL, args...).Scan(&charts.BySupplier).Error; err != nil {
		return Charts{}, err
	}

	byWarehouseSQL := `SELECT w.name AS name, COUNT(DISTINCT r.id) AS receipt_count, SUM(ri.received_qty) AS received_qty
		FROM receipts r JOIN receipt_items ri ON ri.receipt_id = r.id
		JOIN warehouses w ON w.id = r.warehouse_id
		LEFT JOIN suppliers s ON s.id = r.supplier_id
		LEFT JOIN users u ON u.id = r.created_by
		WHERE ` + where + `
		GROUP BY w.name ORDER BY received_qty DESC`
	if err := repo.db.WithContext(ctx).Raw(byWarehouseSQL, args...).Scan(&charts.ByWarehouse).Error; err != nil {
		return Charts{}, err
	}

	byOperatorSQL := `SELECT COALESCE(u.full_name, 'Unassigned') AS name, COUNT(DISTINCT r.id) AS receipt_count, SUM(ri.received_qty) AS received_qty
		FROM receipts r JOIN receipt_items ri ON ri.receipt_id = r.id
		JOIN warehouses w ON w.id = r.warehouse_id
		LEFT JOIN suppliers s ON s.id = r.supplier_id
		LEFT JOIN users u ON u.id = r.created_by
		WHERE ` + where + `
		GROUP BY COALESCE(u.full_name, 'Unassigned') ORDER BY received_qty DESC`
	if err := repo.db.WithContext(ctx).Raw(byOperatorSQL, args...).Scan(&charts.ByOperator).Error; err != nil {
		return Charts{}, err
	}

	return charts, nil
}

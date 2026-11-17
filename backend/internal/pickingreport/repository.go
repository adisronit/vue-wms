package pickingreport

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

// scopeWhere builds the WHERE clause every main-report query shares,
// referencing pt (picking_tasks, always joined), w (warehouses), and
// u (users, LEFT JOIN) — date/warehouse/status/picker, plus Search/
// LotNo as EXISTS matches against this task's own items/products so a
// header-grain query (none needed today, but kept consistent with
// receivingreport/stockcountreport's own shape) and the item-grain
// List query both stay correct.
func scopeWhere(tenantID int64, f Filters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("pt.tenant_id = ?")

	if f.From != nil {
		sb.WriteString(" AND pt.created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND pt.created_at < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND pt.warehouse_id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.Status != "" {
		sb.WriteString(" AND pt.status = ?")
		args = append(args, f.Status)
	}
	if f.PickerID != nil {
		sb.WriteString(" AND pt.assigned_picker_id = ?")
		args = append(args, *f.PickerID)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(` AND (pt.pick_no LIKE ? OR pt.order_no LIKE ? OR EXISTS (
			SELECT 1 FROM picking_task_items pti_s JOIN products p_s ON p_s.id = pti_s.product_id
			WHERE pti_s.picking_task_id = pt.id AND (p_s.sku LIKE ? OR p_s.name LIKE ?)
		))`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if f.LotNo != "" {
		sb.WriteString(` AND EXISTS (
			SELECT 1 FROM picking_task_items pti_l WHERE pti_l.picking_task_id = pt.id AND pti_l.lot_no = ?
		)`)
		args = append(args, f.LotNo)
	}

	return sb.String(), args
}

// itemConditions returns the extra item-level AND-clauses (ItemStatus/
// ShortOnly) for a query that has already joined picking_task_items
// AS pti directly.
func itemConditions(f Filters) (string, []interface{}) {
	var sb strings.Builder
	var args []interface{}

	status := f.ItemStatus
	if f.ShortOnly {
		status = "SHORT"
	}
	if status != "" {
		sb.WriteString(" AND pti.status = ?")
		args = append(args, status)
	}

	return sb.String(), args
}

var sortColumns = map[string]string{
	"created_at":     "pt.created_at",
	"pick_no":        "pt.pick_no",
	"warehouse_name": "w.name",
	"status":         "pt.status",
}

func sortClause(sortBy, sortDir string) string {
	col, ok := sortColumns[sortBy]
	if !ok {
		col = sortColumns["created_at"]
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + ", pt.id " + dir + ", pti.id " + dir
}

const dispatchedQtyCTE = `
dispatched_agg AS (
	SELECT si.picking_task_item_id AS task_item_id, SUM(si.qty) AS dispatched_qty
	FROM shipment_items si
	JOIN shipments sh ON sh.id = si.shipment_id AND sh.tenant_id = si.tenant_id
	WHERE sh.status IN ('DISPATCHED', 'IN_TRANSIT', 'DELIVERED')
	GROUP BY si.picking_task_item_id
)
`

const listSelectAndFrom = `
SELECT
	pt.id AS task_id, pti.id AS task_item_id, pt.pick_no, pt.order_no,
	CASE WHEN pb.id IS NOT NULL THEN 'BATCH' ELSE 'PICK_BY_ORDER' END AS picking_mode,
	pb.batch_no,
	COALESCE(pt.customer, pt.ship_to_name) AS customer_name, pt.ship_to_name,
	w.code AS warehouse_code, w.name AS warehouse_name,
	p.sku, p.name AS product_name, b.name AS brand_name, pc.name AS category_name,
	loc.code AS location_code, loc.name AS location_name,
	pti.lot_no, pti.uom,
	pti.ordered_qty AS requested_qty, pti.ordered_qty AS allocated_qty, pti.picked_qty,
	pti.short_qty, pti.short_reason,
	da.dispatched_qty,
	pti.status AS item_status, pt.status AS task_status,
	u.full_name AS assigned_picker,
	pt.assigned_at AS start_date_time, pt.completed_at AS complete_date_time
FROM picking_tasks pt
JOIN picking_task_items pti ON pti.picking_task_id = pt.id
JOIN products p ON p.id = pti.product_id
LEFT JOIN brands b ON b.id = p.brand_id
LEFT JOIN product_categories pc ON pc.id = p.category_id
JOIN locations loc ON loc.id = pti.location_id
JOIN warehouses w ON w.id = pt.warehouse_id
LEFT JOIN users u ON u.id = pt.assigned_picker_id
LEFT JOIN picking_batches pb ON pb.picking_task_id = pt.id AND pb.tenant_id = pt.tenant_id
LEFT JOIN dispatched_agg da ON da.task_item_id = pti.id
`

type rawRow struct {
	TaskID           int64
	TaskItemID       int64
	PickNo           string
	OrderNo          string
	PickingMode      string
	BatchNo          *string
	CustomerName     *string
	ShipToName       *string
	WarehouseCode    string
	WarehouseName    string
	SKU              string
	ProductName      string
	BrandName        *string
	CategoryName     *string
	LocationCode     string
	LocationName     string
	LotNo            string
	UOM              string
	RequestedQty     float64
	AllocatedQty     float64
	PickedQty        float64
	ShortQty         *float64
	ShortReason      *string
	DispatchedQty    *float64
	ItemStatus       string
	TaskStatus       string
	AssignedPicker   *string
	StartDateTime    *time.Time
	CompleteDateTime *time.Time
}

func (r rawRow) toRow() Row {
	return Row{
		TaskID: r.TaskID, TaskItemID: r.TaskItemID, PickNo: r.PickNo, OrderNo: r.OrderNo,
		PickingMode: r.PickingMode, BatchNo: r.BatchNo, CustomerName: r.CustomerName, ShipToName: r.ShipToName,
		WarehouseCode: r.WarehouseCode, WarehouseName: r.WarehouseName,
		SKU: r.SKU, ProductName: r.ProductName, BrandName: r.BrandName, CategoryName: r.CategoryName,
		LocationCode: r.LocationCode, LocationName: r.LocationName,
		LotNo: r.LotNo, ExpiryDate: nil, LPN: nil, UOM: r.UOM,
		RequestedQty: r.RequestedQty, AllocatedQty: r.AllocatedQty, PickedQty: r.PickedQty,
		ShortQty: r.ShortQty, ShortReason: r.ShortReason, DispatchedQty: r.DispatchedQty,
		ItemStatus: r.ItemStatus, TaskStatus: r.TaskStatus, AssignedPicker: r.AssignedPicker,
		StartDateTime: r.StartDateTime, CompleteDateTime: r.CompleteDateTime,
	}
}

func (repo *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]Row, int64, error) {
	headerWhere, headerArgs := scopeWhere(tenantID, params.Filters)
	itemWhere, itemArgs := itemConditions(params.Filters)
	where := headerWhere + itemWhere
	args := append(append([]interface{}{}, headerArgs...), itemArgs...)

	var total int64
	countSQL := "WITH " + dispatchedQtyCTE + " SELECT COUNT(*) FROM (" + listSelectAndFrom + " WHERE " + where + ") t"
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

	dataSQL := "WITH " + dispatchedQtyCTE + " " + listSelectAndFrom + " WHERE " + where +
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
	headerWhere, headerArgs := scopeWhere(tenantID, filters)
	itemWhere, itemArgs := itemConditions(filters)
	where := headerWhere + itemWhere
	args := append(append([]interface{}{}, headerArgs...), itemArgs...)

	dataSQL := "WITH " + dispatchedQtyCTE + " " + listSelectAndFrom + " WHERE " + where +
		" ORDER BY " + sortClause("created_at", "desc") +
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
	headerWhere, headerArgs := scopeWhere(tenantID, filters)
	itemWhere, itemArgs := itemConditions(filters)
	where := headerWhere + itemWhere
	args := append(append([]interface{}{}, headerArgs...), itemArgs...)

	var summary Summary
	type agg struct {
		TotalTasks     int64
		CompletedTasks int64
		TotalRequested float64
		TotalPicked    float64
		TotalShort     *float64
		TotalLines     int64
		ShortLines     int64
	}
	var a agg
	sql := `SELECT
		COUNT(DISTINCT pt.id) AS total_tasks,
		COUNT(DISTINCT CASE WHEN pt.status = 'COMPLETED' THEN pt.id END) AS completed_tasks,
		SUM(pti.ordered_qty) AS total_requested,
		SUM(pti.picked_qty) AS total_picked,
		SUM(pti.short_qty) AS total_short,
		COUNT(*) AS total_lines,
		SUM(CASE WHEN pti.status = 'SHORT' THEN 1 ELSE 0 END) AS short_lines
	FROM picking_tasks pt
	JOIN picking_task_items pti ON pti.picking_task_id = pt.id
	JOIN warehouses w ON w.id = pt.warehouse_id
	LEFT JOIN users u ON u.id = pt.assigned_picker_id
	WHERE ` + where
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&a).Error; err != nil {
		return Summary{}, err
	}
	summary.TotalTasks = a.TotalTasks
	summary.CompletedTasks = a.CompletedTasks
	summary.TotalRequestedQty = a.TotalRequested
	summary.TotalPickedQty = a.TotalPicked
	if a.TotalShort != nil {
		summary.TotalShortQty = *a.TotalShort
	}
	if a.TotalLines > 0 {
		rate := float64(a.ShortLines) / float64(a.TotalLines) * 100
		summary.ShortPickRate = &rate
	}

	return summary, nil
}

// --- Batch/Wave Picking Report (separate grain — see package doc) ---

func batchScopeWhere(tenantID int64, f BatchAllocationFilters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("pb.tenant_id = ?")

	if f.From != nil {
		sb.WriteString(" AND pb.created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND pb.created_at < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND pb.warehouse_id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.Search != "" {
		pattern := "%" + f.Search + "%"
		sb.WriteString(" AND (pb.batch_no LIKE ? OR o.order_no LIKE ?)")
		args = append(args, pattern, pattern)
	}

	return sb.String(), args
}

const batchAllocationSelectAndFrom = `
SELECT
	pb.id AS batch_id, pb.batch_no, pt.pick_no AS task_pick_no,
	pba.order_id, o.order_no, COALESCE(o.customer, o.ship_to_name) AS customer_name,
	w.code AS warehouse_code, w.name AS warehouse_name,
	p.sku, p.name AS product_name, loc.code AS location_code,
	pba.lot_no, pba.uom, pba.allocated_qty, pba.sorted_qty, pba.shortage_qty, pba.exception_reason,
	pti.picked_qty AS shared_task_picked_qty
FROM picking_batches pb
JOIN picking_batch_allocations pba ON pba.batch_id = pb.id
JOIN orders o ON o.id = pba.order_id
JOIN products p ON p.id = pba.product_id
JOIN locations loc ON loc.id = pba.location_id
JOIN warehouses w ON w.id = pb.warehouse_id
LEFT JOIN picking_tasks pt ON pt.id = pb.picking_task_id
LEFT JOIN picking_task_items pti ON pti.id = pba.picking_task_item_id
`

type rawBatchAllocationRow struct {
	BatchID             int64
	BatchNo             string
	TaskPickNo          *string
	OrderID             int64
	OrderNo             string
	CustomerName        *string
	WarehouseCode       string
	WarehouseName       string
	SKU                 string
	ProductName         string
	LocationCode        string
	LotNo               string
	UOM                 string
	AllocatedQty        float64
	SortedQty           float64
	ShortageQty         *float64
	ExceptionReason     *string
	SharedTaskPickedQty *float64
}

func (r rawBatchAllocationRow) toRow() BatchAllocationRow {
	return BatchAllocationRow{
		BatchID: r.BatchID, BatchNo: r.BatchNo, TaskPickNo: r.TaskPickNo,
		OrderID: r.OrderID, OrderNo: r.OrderNo, CustomerName: r.CustomerName,
		WarehouseCode: r.WarehouseCode, WarehouseName: r.WarehouseName,
		SKU: r.SKU, ProductName: r.ProductName, LocationCode: r.LocationCode,
		LotNo: r.LotNo, UOM: r.UOM, AllocatedQty: r.AllocatedQty, SortedQty: r.SortedQty,
		ShortageQty: r.ShortageQty, ExceptionReason: r.ExceptionReason, SharedTaskPickedQty: r.SharedTaskPickedQty,
	}
}

func (repo *Repository) ListBatchAllocations(ctx context.Context, tenantID int64, params BatchAllocationListParams) ([]BatchAllocationRow, int64, error) {
	where, args := batchScopeWhere(tenantID, params.BatchAllocationFilters)

	var total int64
	countSQL := "SELECT COUNT(*) FROM (" + batchAllocationSelectAndFrom + " WHERE " + where + ") t"
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

	dataSQL := batchAllocationSelectAndFrom + " WHERE " + where +
		" ORDER BY pb.created_at DESC, pb.id DESC, pba.id DESC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY"
	dataArgs := append(append([]interface{}{}, args...), offset, limit)

	var raw []rawBatchAllocationRow
	if err := repo.db.WithContext(ctx).Raw(dataSQL, dataArgs...).Scan(&raw).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]BatchAllocationRow, 0, len(raw))
	for _, rr := range raw {
		rows = append(rows, rr.toRow())
	}
	return rows, total, nil
}

func (repo *Repository) ListBatchAllocationsForExport(ctx context.Context, tenantID int64, filters BatchAllocationFilters, maxRows int) ([]BatchAllocationRow, error) {
	where, args := batchScopeWhere(tenantID, filters)
	dataSQL := batchAllocationSelectAndFrom + " WHERE " + where +
		" ORDER BY pb.created_at DESC, pb.id DESC, pba.id DESC OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY"
	dataArgs := append(append([]interface{}{}, args...), maxRows)

	var raw []rawBatchAllocationRow
	if err := repo.db.WithContext(ctx).Raw(dataSQL, dataArgs...).Scan(&raw).Error; err != nil {
		return nil, err
	}
	rows := make([]BatchAllocationRow, 0, len(raw))
	for _, rr := range raw {
		rows = append(rows, rr.toRow())
	}
	return rows, nil
}

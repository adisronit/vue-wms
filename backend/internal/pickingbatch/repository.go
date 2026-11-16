package pickingbatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("pickingbatch: not found")

// Repository is a thin GORM wrapper — business rules (consolidation,
// the atomic multi-order claim, status machine) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, the same shape
// every other workflow package's own Transaction establishes — handing
// fn both the raw *gorm.DB (so Service.Confirm can pass it straight into
// picking.Service.CreateTx, running the atomic order claim and
// picking.Service's own task creation as one commit-or-rollback unit)
// and a Repository bound to it for this package's own queries.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateBatch inserts the header, then derives batch_no from the new
// IDENTITY id in the same transaction — the same race-free-by-
// construction approach as order_no/pick_no/task_no.
func (r *Repository) CreateBatch(ctx context.Context, tenantID int64, b *Batch) error {
	b.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(b).Error; err != nil {
		return err
	}
	b.BatchNo = fmt.Sprintf("BATCH-%06d", b.ID)
	return r.db.WithContext(ctx).Model(&Batch{}).Where("id = ? AND tenant_id = ?", b.ID, tenantID).
		Update("batch_no", b.BatchNo).Error
}

// CreateBatchOrders bulk-inserts the picking_batch_orders membership
// rows at DRAFT creation time — see migrations/0095's own doc for why
// this (not orders.picking_batch_id, only ever written at Confirm) is
// what reserves an order against a second concurrent DRAFT batch.
func (r *Repository) CreateBatchOrders(ctx context.Context, tenantID, batchID int64, orderIDs []int64) error {
	rows := make([]BatchOrder, 0, len(orderIDs))
	for _, id := range orderIDs {
		rows = append(rows, BatchOrder{TenantID: tenantID, BatchID: batchID, OrderID: id})
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}

// CreateAllocations bulk-inserts the picking_batch_allocations rows the
// DRAFT consolidation computed — picking_task_item_id is left NULL on
// every row (backfilled only at Confirm, see BackfillAllocationTaskItems).
func (r *Repository) CreateAllocations(ctx context.Context, tenantID int64, allocations []Allocation) error {
	if len(allocations) == 0 {
		return nil
	}
	for i := range allocations {
		allocations[i].TenantID = tenantID
	}
	return r.db.WithContext(ctx).Create(&allocations).Error
}

// FindBatchByID is scoped to tenantID — a cross-tenant id returns
// ErrNotFound exactly like a nonexistent one, the same IDOR-closed
// convention every other workflow package's own FindByID follows.
func (r *Repository) FindBatchByID(ctx context.Context, tenantID, id int64) (*Batch, error) {
	var b Batch
	err := r.db.WithContext(ctx).First(&b, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

type batchDetailRow struct {
	ID                   int64
	BatchNo              string
	WarehouseID          int64
	WarehouseCode        string
	Status               string
	PickingTaskID        *int64
	PickNo               *string
	CreatedBy            *int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ConfirmedAt          *time.Time
	CancelledAt          *time.Time
	CancelledBy          *int64
	CancelRemark         *string
	SortationStatus      string
	SortationStation     *string
	SortationAssignedTo  *int64
	SortationStartedAt   *time.Time
	SortationCompletedAt *time.Time
}

// FindBatchDetail resolves the header plus warehouse_code and the linked
// task's own pick_no (if any) — scoped to tenantID.
func (r *Repository) FindBatchDetail(ctx context.Context, tenantID, id int64) (*batchDetailRow, error) {
	var row batchDetailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT b.id AS id, b.batch_no AS batch_no, b.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			b.status AS status, b.picking_task_id AS picking_task_id, pt.pick_no AS pick_no,
			b.created_by AS created_by, b.created_at AS created_at, b.updated_at AS updated_at,
			b.confirmed_at AS confirmed_at, b.cancelled_at AS cancelled_at, b.cancelled_by AS cancelled_by,
			b.cancel_remark AS cancel_remark, b.sortation_status AS sortation_status,
			b.sortation_station AS sortation_station, b.sortation_assigned_to AS sortation_assigned_to,
			b.sortation_started_at AS sortation_started_at, b.sortation_completed_at AS sortation_completed_at
		FROM picking_batches b
		JOIN warehouses w ON w.id = b.warehouse_id
		LEFT JOIN picking_tasks pt ON pt.id = b.picking_task_id
		WHERE b.id = ? AND b.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

type batchOrderRow struct {
	OrderID    int64
	OrderNo    string
	Customer   *string
	ShipToName *string
	Status     string
}

// ListBatchOrders resolves the display rows for Response.Orders — scoped
// to tenantID (defense in depth; batchID itself was already proven to
// belong to tenantID by the caller's own FindBatchByID/FindBatchDetail).
func (r *Repository) ListBatchOrders(ctx context.Context, tenantID, batchID int64) ([]batchOrderRow, error) {
	var rows []batchOrderRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT o.id AS order_id, o.order_no AS order_no, o.customer AS customer, o.ship_to_name AS ship_to_name, o.status AS status
		FROM picking_batch_orders pbo
		JOIN orders o ON o.id = pbo.order_id
		WHERE pbo.batch_id = ? AND pbo.tenant_id = ?
		ORDER BY o.id ASC
	`, batchID, tenantID).Scan(&rows).Error
	return rows, err
}

type allocationDetailRow struct {
	ID                int64
	OrderID           int64
	OrderNo           string
	OrderItemID       int64
	ProductID         int64
	SKU               string
	ProductName       string
	LotNo             string
	LocationID        int64
	LocationCode      string
	PickingTaskItemID *int64
	AllocatedQty      float64
	UOM               string
}

// ListAllocationsDetail resolves every allocation row for a batch with
// its full display shape (product/location display fields, the
// originating order's own order_no) — the single source Service.Get
// groups by (product_id, lot_no, location_id) into
// ConsolidatedLineResponse.Lines, for both DRAFT (no task yet) and
// CONFIRMED/COMPLETED batches alike.
func (r *Repository) ListAllocationsDetail(ctx context.Context, tenantID, batchID int64) ([]allocationDetailRow, error) {
	var rows []allocationDetailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.id AS id, a.order_id AS order_id, o.order_no AS order_no, a.order_item_id AS order_item_id,
			a.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			a.lot_no AS lot_no, a.location_id AS location_id, l.code AS location_code,
			a.picking_task_item_id AS picking_task_item_id, a.allocated_qty AS allocated_qty, a.uom AS uom
		FROM picking_batch_allocations a
		JOIN orders o ON o.id = a.order_id
		JOIN products p ON p.id = a.product_id
		JOIN locations l ON l.id = a.location_id
		WHERE a.batch_id = ? AND a.tenant_id = ?
		ORDER BY a.product_id ASC, a.lot_no ASC, a.location_id ASC, a.order_id ASC
	`, batchID, tenantID).Scan(&rows).Error
	return rows, err
}

// ClaimConfirm is Service.Confirm's own atomic idempotency/concurrency
// guard — a single conditional UPDATE: 0 rows affected means the batch
// is not DRAFT (already confirmed, cancelled, or a concurrent Confirm
// call won the race) or does not belong to tenantID. Must run on the
// same tx Service.Confirm's own order claim and picking.Service.CreateTx
// call share, so a failure anywhere downstream rolls this back too.
func (r *Repository) ClaimConfirm(ctx context.Context, tenantID, batchID int64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE picking_batches
		SET status = ?, confirmed_at = SYSUTCDATETIME(), updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND status = ?
	`, string(StatusConfirmed), batchID, tenantID, string(StatusDraft))
	return result.RowsAffected, result.Error
}

// ClaimCancel is Service.Cancel's own atomic guard — DRAFT -> CANCELLED
// only, the same shape as ClaimConfirm.
func (r *Repository) ClaimCancel(ctx context.Context, tenantID, batchID, userID int64, remark *string) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE picking_batches
		SET status = ?, cancelled_at = SYSUTCDATETIME(), cancelled_by = ?, cancel_remark = ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND status = ?
	`, string(StatusCancelled), userID, remark, batchID, tenantID, string(StatusDraft))
	return result.RowsAffected, result.Error
}

// SyncTerminalStatus writes a lazily-derived CONFIRMED -> COMPLETED/
// CANCELLED mirror of the linked picking task's own terminal status —
// called by Service.Get/List when it observes the mismatch. Never runs
// on a DRAFT/already-terminal batch (callers only invoke this when
// status is still CONFIRMED and the task has since reached a terminal
// state) and never undoes a terminal status once written.
func (r *Repository) SyncTerminalStatus(ctx context.Context, tenantID, batchID int64, newStatus Status) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE picking_batches SET status = ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND status = ?
	`, string(newStatus), batchID, tenantID, string(StatusConfirmed)).Error
}

// SetPickingTaskID records the real picking_tasks row Service.Confirm
// just created via picking.Service.CreateTx, inside the same
// transaction as ClaimConfirm.
func (r *Repository) SetPickingTaskID(ctx context.Context, tenantID, batchID, taskID int64) error {
	return r.db.WithContext(ctx).Model(&Batch{}).Where("id = ? AND tenant_id = ?", batchID, tenantID).
		Update("picking_task_id", taskID).Error
}

// BackfillAllocationTaskItems matches every allocation row for batchID
// to the real picking_task_items row sharing its own (product_id,
// lot_no, location_id) tuple within the just-created task, now that
// those rows exist — the tuple is unique within one task by
// construction (Service.buildConsolidatedItems never emits two lines
// for the same tuple), so this join can never ambiguously match more
// than one task item per allocation row.
func (r *Repository) BackfillAllocationTaskItems(ctx context.Context, tenantID, batchID, taskID int64) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE pba
		SET pba.picking_task_item_id = pti.id
		FROM picking_batch_allocations pba
		JOIN picking_task_items pti
			ON pti.tenant_id = pba.tenant_id
			AND pti.product_id = pba.product_id
			AND pti.lot_no = pba.lot_no
			AND pti.location_id = pba.location_id
		WHERE pba.batch_id = ? AND pba.tenant_id = ? AND pti.picking_task_id = ?
	`, batchID, tenantID, taskID).Error
}

// ClaimOrders is Service.Confirm's own atomic MULTI-order claim — one
// guarded UPDATE touching every order_id in orderIDs at once, extending
// internal/order's own single-order ClaimForPickingTask shape to N rows:
// RowsAffected == len(orderIDs) is the only success condition. Any
// order that is no longer RELEASED, no longer PICK_BY_PRODUCT, already
// linked to a batch, or belongs to another tenant simply does not match
// the WHERE clause and is not counted — Service.Confirm treats a partial
// match as a total failure and rolls back the whole transaction (no
// order is left half-claimed), never distinguishing "cross-tenant" from
// any other non-claimable reason in the response.
//
// Deliberately written as one single UPDATE statement with an IN
// clause (not len(orderIDs) separate UPDATEs) so SQL Server evaluates
// and locks every matching row as one atomic unit — the same reasoning
// every other guarded-UPDATE claim in this codebase already relies on,
// simply extended from one row to N.
func (r *Repository) ClaimOrders(ctx context.Context, tenantID int64, orderIDs []int64, batchID int64) (int64, error) {
	if len(orderIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(orderIDs))
	args := make([]any, 0, len(orderIDs)+5)
	args = append(args, "PICKING", batchID)
	for i, id := range orderIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, tenantID, "RELEASED", "PICK_BY_PRODUCT")
	query := fmt.Sprintf(`
		UPDATE orders
		SET status = ?, picking_batch_id = ?, updated_at = SYSUTCDATETIME()
		WHERE id IN (%s) AND tenant_id = ? AND status = ? AND picking_mode = ? AND picking_batch_id IS NULL
	`, strings.Join(placeholders, ","))
	result := r.db.WithContext(ctx).Exec(query, args...)
	return result.RowsAffected, result.Error
}

type orderConsolidationRow struct {
	ID               int64
	OrderNo          string
	Status           string
	PickingMode      string
	WarehouseID      int64
	TenantID         int64
	ActiveBatchCount int
}

// LoadOrdersForConsolidation resolves every eligibility fact Service.
// Create's own validation needs for each candidate order_id, including
// whether it is already a member of another active (DRAFT/CONFIRMED)
// batch — in one query rather than N+1. Scoped to tenantID: an order_id
// belonging to another tenant simply does not appear in the result,
// which Service.Create's own caller-facing error treats identically to
// "order does not exist" (closing the IDOR every other FindByID in this
// codebase already closes).
func (r *Repository) LoadOrdersForConsolidation(ctx context.Context, tenantID int64, orderIDs []int64) ([]orderConsolidationRow, error) {
	if len(orderIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(orderIDs))
	args := make([]any, 0, len(orderIDs)+1)
	for i, id := range orderIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, tenantID)
	var rows []orderConsolidationRow
	query := fmt.Sprintf(`
		SELECT o.id AS id, o.order_no AS order_no, o.status AS status, o.picking_mode AS picking_mode,
			o.warehouse_id AS warehouse_id, o.tenant_id AS tenant_id,
			(SELECT COUNT(*) FROM picking_batch_orders pbo
				JOIN picking_batches pb ON pb.id = pbo.batch_id
				WHERE pbo.order_id = o.id AND pb.status IN ('DRAFT', 'CONFIRMED')) AS active_batch_count
		FROM orders o
		WHERE o.id IN (%s) AND o.tenant_id = ?
	`, strings.Join(placeholders, ","))
	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	return rows, err
}

type orderItemRow struct {
	OrderItemID int64
	OrderID     int64
	ProductID   int64
	SKU         string
	ProductName string
	OrderedQty  float64
	UOM         string
}

// LoadOrderItems resolves every line of every order in orderIDs — the
// raw demand Service.Create groups by product_id to build consolidated
// lines. Scoped to tenantID as defense in depth (every orderID has
// already been proven to belong to tenantID by LoadOrdersForConsolidation).
func (r *Repository) LoadOrderItems(ctx context.Context, tenantID int64, orderIDs []int64) ([]orderItemRow, error) {
	if len(orderIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(orderIDs))
	args := make([]any, 0, len(orderIDs)+1)
	for i, id := range orderIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, tenantID)
	var rows []orderItemRow
	query := fmt.Sprintf(`
		SELECT oi.id AS order_item_id, oi.order_id AS order_id, oi.product_id AS product_id,
			p.sku AS sku, p.name AS product_name, oi.ordered_qty AS ordered_qty, oi.uom AS uom
		FROM order_items oi
		JOIN products p ON p.id = oi.product_id
		WHERE oi.order_id IN (%s) AND oi.tenant_id = ?
		ORDER BY oi.order_id ASC, oi.id ASC
	`, strings.Join(placeholders, ","))
	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	return rows, err
}

type availableOrderRow struct {
	OrderID    int64
	OrderNo    string
	Customer   *string
	ShipToName *string
	ItemCount  int
}

// ListAvailableOrders is the multi-select source (GET /picking-batches/
// available-orders) — every RELEASED, PICK_BY_PRODUCT order in
// warehouseID not already a member of another active (DRAFT/CONFIRMED)
// batch. Scoped to tenantID.
func (r *Repository) ListAvailableOrders(ctx context.Context, tenantID, warehouseID int64) ([]availableOrderRow, error) {
	var rows []availableOrderRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT o.id AS order_id, o.order_no AS order_no, o.customer AS customer, o.ship_to_name AS ship_to_name,
			COUNT(oi.id) AS item_count
		FROM orders o
		LEFT JOIN order_items oi ON oi.order_id = o.id
		WHERE o.tenant_id = ? AND o.warehouse_id = ? AND o.status = 'RELEASED' AND o.picking_mode = 'PICK_BY_PRODUCT'
			AND NOT EXISTS (
				SELECT 1 FROM picking_batch_orders pbo
				JOIN picking_batches pb ON pb.id = pbo.batch_id
				WHERE pbo.order_id = o.id AND pb.status IN ('DRAFT', 'CONFIRMED')
			)
		GROUP BY o.id, o.order_no, o.customer, o.ship_to_name
		ORDER BY o.id DESC
	`, tenantID, warehouseID).Scan(&rows).Error
	return rows, err
}

type batchListRow struct {
	ID            int64
	BatchNo       string
	WarehouseCode string
	OrderCount    int
	LineCount     int
	Status        string
	PickNo        *string
	CreatedAt     time.Time
}

// List returns batch-level rows for GET /picking-batches, with
// aggregated order/line counts — scoped to tenantID.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]batchListRow, int64, error) {
	base := `
		FROM picking_batches b
		JOIN warehouses w ON w.id = b.warehouse_id
		LEFT JOIN picking_tasks pt ON pt.id = b.picking_task_id
		LEFT JOIN picking_batch_orders pbo ON pbo.batch_id = b.id
		LEFT JOIN picking_batch_allocations pba ON pba.batch_id = b.id
	`
	where := `WHERE b.tenant_id = ?
		AND (? = '' OR b.status = ?)
		AND (? = 0 OR b.warehouse_id = ?)`
	args := []any{tenantID, string(params.Status), string(params.Status), params.WarehouseID, params.WarehouseID}

	var total int64
	countSQL := "SELECT COUNT(DISTINCT b.id) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT b.id AS id, b.batch_no AS batch_no, w.code AS warehouse_code,
			COUNT(DISTINCT pbo.order_id) AS order_count,
			COUNT(DISTINCT CONCAT(pba.product_id, '|', pba.lot_no, '|', pba.location_id)) AS line_count,
			b.status AS status, pt.pick_no AS pick_no, b.created_at AS created_at
	` + base + where + `
		GROUP BY b.id, b.batch_no, w.code, b.status, pt.pick_no, b.created_at
		ORDER BY b.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []batchListRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ---------------------------------------------------------------------
// Phase 33C.2 -- Sortation
// ---------------------------------------------------------------------

type sortationLineRow struct {
	ID                int64
	OrderID           int64
	OrderNo           string
	ShipToName        *string
	OrderItemID       int64
	ProductID         int64
	SKU               string
	ProductName       string
	LotNo             string
	LocationID        int64
	LocationCode      string
	PickingTaskItemID *int64
	AllocatedQty      float64
	SortedQty         float64
	ShortageQty       *float64
	ExceptionReason   *string
	UOM               string
}

// ListAllocationsForSortation resolves every picking_batch_allocations
// row for batchID with the full display shape the Sortation worklist
// needs — one row per order-share slice (the same granularity the pick
// floor physically handles: "put 20 of these onto ORD-000001's pile"),
// never pre-aggregated, since two allocation rows can share a product/
// lot/location but belong to different orders and must be sorted
// separately. Scoped to tenantID.
func (r *Repository) ListAllocationsForSortation(ctx context.Context, tenantID, batchID int64) ([]sortationLineRow, error) {
	var rows []sortationLineRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.id AS id, a.order_id AS order_id, o.order_no AS order_no, o.ship_to_name AS ship_to_name,
			a.order_item_id AS order_item_id, a.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			a.lot_no AS lot_no, a.location_id AS location_id, l.code AS location_code,
			a.picking_task_item_id AS picking_task_item_id, a.allocated_qty AS allocated_qty,
			a.sorted_qty AS sorted_qty, a.shortage_qty AS shortage_qty, a.exception_reason AS exception_reason,
			a.uom AS uom
		FROM picking_batch_allocations a
		JOIN orders o ON o.id = a.order_id
		JOIN products p ON p.id = a.product_id
		JOIN locations l ON l.id = a.location_id
		WHERE a.batch_id = ? AND a.tenant_id = ?
		ORDER BY o.id ASC, a.product_id ASC, a.lot_no ASC, a.id ASC
	`, batchID, tenantID).Scan(&rows).Error
	return rows, err
}

// FindAllocationByID resolves a single allocation row, scoped to both
// tenantID AND batchID — a correctly-tenant-scoped allocation id that
// belongs to a DIFFERENT batch than the one in the URL is treated
// identically to "not found", the same IDOR-closed convention every
// other FindByID in this codebase follows.
func (r *Repository) FindAllocationByID(ctx context.Context, tenantID, batchID, allocationID int64) (*Allocation, error) {
	var a Allocation
	err := r.db.WithContext(ctx).First(&a, "id = ? AND batch_id = ? AND tenant_id = ?", allocationID, batchID, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// FindMatchingAllocationsForScan resolves every allocation row that a
// sortation scan's own (order_id, SKU, lot_no) could plausibly target —
// almost always exactly one row, but a single order's own demand line
// can legitimately have been split across more than one FEFO lot at
// Create time (see Service.allocateProduct's own doc), which would
// produce more than one allocation row sharing the same order_id/
// product_id but a DIFFERENT lot_no; this only returns more than one row
// when both SKU *and* lot_no match more than one, e.g. the same (order,
// product, lot) pair was allocated across two different locations.
// Service.ScanSortation consumes these in id order. SKU/lot_no are
// compared case-insensitively and trimmed, matching picking.ScanConfirm/
// packing.PackItem's own scan-comparison convention.
func (r *Repository) FindMatchingAllocationsForScan(ctx context.Context, tenantID, batchID, orderID int64, sku, lotNo string) ([]Allocation, error) {
	var rows []Allocation
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.* FROM picking_batch_allocations a
		JOIN products p ON p.id = a.product_id
		WHERE a.batch_id = ? AND a.tenant_id = ? AND a.order_id = ?
			AND UPPER(LTRIM(RTRIM(p.sku))) = UPPER(LTRIM(RTRIM(?)))
			AND UPPER(LTRIM(RTRIM(a.lot_no))) = UPPER(LTRIM(RTRIM(?)))
		ORDER BY a.id ASC
	`, batchID, tenantID, orderID, sku, lotNo).Scan(&rows).Error
	return rows, err
}

// ResolveOrderIDByBarcode matches a scanned order barcode (order_no,
// case-insensitive/trimmed) against the SPECIFIC orders included in
// batchID — scanning an order_no that is real but not part of this
// batch is rejected exactly like a nonexistent one (ErrNotFound), the
// same "wrong order" mismatch Service.ScanSortation reports uniformly.
func (r *Repository) ResolveOrderIDByBarcode(ctx context.Context, tenantID, batchID int64, barcode string) (int64, error) {
	var orderID int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT o.id FROM picking_batch_orders pbo
		JOIN orders o ON o.id = pbo.order_id
		WHERE pbo.batch_id = ? AND pbo.tenant_id = ? AND UPPER(LTRIM(RTRIM(o.order_no))) = UPPER(LTRIM(RTRIM(?)))
	`, batchID, tenantID, barcode).Scan(&orderID).Error
	if err != nil {
		return 0, err
	}
	if orderID == 0 {
		return 0, ErrNotFound
	}
	return orderID, nil
}

// AdjustSortedQty atomically increments one allocation row's sorted_qty
// by delta in a single UPDATE, bounded by allocated_qty in the very same
// WHERE clause — the concurrency-safe over-sortation guard itself,
// exactly mirroring picking.Repository.AdjustPickedQty/packing.
// Repository.AdjustPackedQty's own "arithmetic and bound-check as one
// atomic SQL statement" shape. RowsAffected == 0 means the bound would
// be exceeded (the caller interprets that as a rejected scan, never a
// silent clamp) or the row does not belong to tenantID.
func (r *Repository) AdjustSortedQty(ctx context.Context, tenantID, allocationID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE picking_batch_allocations
		SET sorted_qty = sorted_qty + ?
		WHERE id = ? AND tenant_id = ? AND sorted_qty + ? <= allocated_qty
	`, delta, allocationID, tenantID, delta)
	return result.RowsAffected, result.Error
}

// FlagShortageException records a sortation-stage shortage/damage
// exception on one allocation row — bounded to rows that have not
// already been flagged (exception_reason IS NULL in the WHERE clause
// itself), so a concurrent duplicate flag attempt affects 0 rows rather
// than silently overwriting the first one's reason/qty.
func (r *Repository) FlagShortageException(ctx context.Context, tenantID, allocationID int64, shortageQty float64, reason string) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE picking_batch_allocations
		SET shortage_qty = ?, exception_reason = ?
		WHERE id = ? AND tenant_id = ? AND exception_reason IS NULL
	`, shortageQty, reason, allocationID, tenantID)
	return result.RowsAffected, result.Error
}

// AllAllocationsResolved reports whether every allocation row in batchID
// has reached "no further sortation attention needed" — see the pure
// allocationResolved predicate's own doc. Implemented as a single COUNT
// query (rows NOT YET resolved) rather than fetching every row into Go,
// since Service only needs the boolean.
func (r *Repository) AllAllocationsResolved(ctx context.Context, tenantID, batchID int64) (bool, error) {
	var unresolved int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM picking_batch_allocations
		WHERE batch_id = ? AND tenant_id = ? AND (sorted_qty + ISNULL(shortage_qty, 0)) < allocated_qty
	`, batchID, tenantID).Scan(&unresolved).Error
	if err != nil {
		return false, err
	}
	return unresolved == 0, nil
}

// StartSortation transitions sortation_status NOT_STARTED -> IN_PROGRESS
// and stamps sortation_started_at, exactly once — a no-op (0 rows
// affected, not an error) if sortation has already started, so
// Service.ScanSortation can call this unconditionally on every
// successful scan without a separate read-then-branch.
func (r *Repository) StartSortation(ctx context.Context, tenantID, batchID int64) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE picking_batches
		SET sortation_status = ?, sortation_started_at = SYSUTCDATETIME(), updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND sortation_status = ?
	`, string(SortationInProgress), batchID, tenantID, string(SortationNotStarted)).Error
}

// CompleteSortation transitions sortation_status -> COMPLETED and stamps
// sortation_completed_at — called only after AllAllocationsResolved
// confirms every line is done. Guarded to only fire from IN_PROGRESS, so
// a redundant call (e.g. two scans finishing the last two lines in a
// race) is naturally idempotent-safe.
func (r *Repository) CompleteSortation(ctx context.Context, tenantID, batchID int64) error {
	return r.db.WithContext(ctx).Exec(`
		UPDATE picking_batches
		SET sortation_status = ?, sortation_completed_at = SYSUTCDATETIME(), updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND sortation_status = ?
	`, string(SortationCompleted), batchID, tenantID, string(SortationInProgress)).Error
}

// AssignSortation sets the station code and/or assigned user — a
// planning/ownership annotation, the same authority level as
// picking.AssignPicker/packing.AssignPacker, never an authorization
// gate. Either argument may be nil to leave that field unchanged.
func (r *Repository) AssignSortation(ctx context.Context, tenantID, batchID int64, station *string, userID *int64) error {
	updates := map[string]any{"updated_at": gorm.Expr("SYSUTCDATETIME()")}
	if station != nil {
		updates["sortation_station"] = *station
	}
	if userID != nil {
		updates["sortation_assigned_to"] = *userID
	}
	result := r.db.WithContext(ctx).Model(&Batch{}).Where("id = ? AND tenant_id = ?", batchID, tenantID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// orderPackingSnapshotRow is the full per-order customer/ship-to
// snapshot internal/packing's own Service.Create needs when building a
// batch-sourced job — see that package's own doc for why it must read
// this from the ORDER directly rather than trusting the shared picking
// task's own (single-order-shaped, now-meaningless-for-a-batch-task)
// copied fields.
type orderPackingSnapshotRow struct {
	OrderID             int64
	OrderNo             string
	Customer            *string
	CustomerID          *int64
	ShipToID            *int64
	ShipToName          *string
	ShipToAddress       *string
	ShipToProvince      *string
	ShipToDistrict      *string
	ShipToPostalCode    *string
	ShipToContactPerson *string
	ShipToContactPhone  *string
	WarehouseID         int64
}

// FindOrderPackingSnapshot resolves orderID's own full customer/ship-to
// snapshot directly from the orders table — exposed (via Service) for
// internal/packing's one-way read-only dependency into this package, so
// that package never needs its own import of internal/order or a
// duplicate query against the orders table.
func (r *Repository) FindOrderPackingSnapshot(ctx context.Context, tenantID, orderID int64) (*orderPackingSnapshotRow, error) {
	var row orderPackingSnapshotRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT o.id AS order_id, o.order_no AS order_no, o.customer AS customer, o.customer_id AS customer_id,
			o.ship_to_id AS ship_to_id, o.ship_to_name AS ship_to_name, o.ship_to_address AS ship_to_address,
			o.ship_to_province AS ship_to_province, o.ship_to_district AS ship_to_district,
			o.ship_to_postal_code AS ship_to_postal_code, o.ship_to_contact_person AS ship_to_contact_person,
			o.ship_to_contact_phone AS ship_to_contact_phone, o.warehouse_id AS warehouse_id
		FROM orders o WHERE o.id = ? AND o.tenant_id = ?
	`, orderID, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.OrderID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// FindBatchByPickingTaskID resolves the picking_batches row (if any)
// that claims pickingTaskID — exposed for internal/packing's own
// Service.Create to detect whether a source picking task is batch-
// sourced. Returns ErrNotFound when the task is NOT batch-sourced (the
// ordinary, today's-only-real-usage PICK_BY_ORDER case), which the
// caller must treat as "proceed with the existing, unmodified
// behavior" rather than a real error.
func (r *Repository) FindBatchByPickingTaskID(ctx context.Context, tenantID, pickingTaskID int64) (*Batch, error) {
	var b Batch
	err := r.db.WithContext(ctx).First(&b, "picking_task_id = ? AND tenant_id = ?", pickingTaskID, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// IsOrderInBatch reports whether orderID is one of batchID's own member
// orders (picking_batch_orders) — the membership check internal/
// packing's Service.Create applies before trusting a client-supplied
// order_id against a batch-sourced task.
func (r *Repository) IsOrderInBatch(ctx context.Context, tenantID, batchID, orderID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM picking_batch_orders WHERE batch_id = ? AND tenant_id = ? AND order_id = ?
	`, batchID, tenantID, orderID).Scan(&count).Error
	return count > 0, err
}

type orderAllocationLineRow struct {
	ID                int64
	PickingTaskItemID *int64
	ProductID         int64
	LotNo             string
	AllocatedQty      float64
	SortedQty         float64
	UOM               string
}

// ListOrderAllocationLines resolves orderID's own allocation rows within
// batchID — the source internal/packing's Service.Create reads to build
// a batch-sourced packing job's lines, using each row's own SortedQty
// (not the shared task's task-wide PickedQty) as the frozen packed_qty
// ceiling. ORDER BY id so the resulting packing_job_items are created in
// a stable, deterministic order.
func (r *Repository) ListOrderAllocationLines(ctx context.Context, tenantID, batchID, orderID int64) ([]orderAllocationLineRow, error) {
	var rows []orderAllocationLineRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, picking_task_item_id, product_id, lot_no, allocated_qty, sorted_qty, uom
		FROM picking_batch_allocations
		WHERE batch_id = ? AND tenant_id = ? AND order_id = ?
		ORDER BY id ASC
	`, batchID, tenantID, orderID).Scan(&rows).Error
	return rows, err
}

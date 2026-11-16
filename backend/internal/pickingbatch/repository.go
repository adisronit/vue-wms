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
	ID            int64
	BatchNo       string
	WarehouseID   int64
	WarehouseCode string
	Status        string
	PickingTaskID *int64
	PickNo        *string
	CreatedBy     *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ConfirmedAt   *time.Time
	CancelledAt   *time.Time
	CancelledBy   *int64
	CancelRemark  *string
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
			b.cancel_remark AS cancel_remark
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

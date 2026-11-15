package picking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("picking: not found")
var ErrItemNotFound = errors.New("picking: item not found")
var ErrDuplicateIdempotencyKey = errors.New("picking: idempotency key already used")

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
// to the exact same transaction — see service.go's Confirm, the same
// pattern internal/receiving and internal/putaway established) and a
// Repository bound to it for picking's own queries.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateTask stamps tenantID onto the header, inserts it, then derives
// pick_no from the new IDENTITY id in the same transaction — the same
// race-free-by-construction approach as receipts.receipt_no /
// putaway_tasks.putaway_no. Phase 30 Batch 2: tenantID must be the
// caller's own verified tenant, never read from the request body.
func (r *Repository) CreateTask(ctx context.Context, tenantID int64, task *Task) error {
	task.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return err
	}
	task.PickNo = fmt.Sprintf("PICK-%06d", task.ID)
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).
		Update("pick_no", task.PickNo).Error
}

func (r *Repository) FindTaskByID(ctx context.Context, tenantID, id int64) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).First(&task, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

type headerRow struct {
	ID              int64
	PickNo          string
	OrderNo         string
	Customer        *string
	WarehouseID     int64
	WarehouseCode   string
	Status          string
	Priority        string
	DueDate         *time.Time
	Remark          *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
	CancelledAt     *time.Time
	CancelledBy     *int64
	CancelledByName *string
	CancelRemark    *string
}

// FindTaskDetail returns the header with warehouse display fields
// resolved, for GET /picking/{id}'s response — a separate query from
// FindTaskByID (which returns the plain GORM model, used internally by
// Confirm where the joined display fields aren't needed). The LEFT JOIN
// to users (Phase 18) resolves cancelled_by to a display name, the same
// pattern Phase 14 established for created_by_name on stock counts.
// Phase 30 Batch 2: scoped to tenantID — a cross-tenant id now returns
// ErrNotFound exactly like a nonexistent one, closing the IDOR this
// query previously had.
func (r *Repository) FindTaskDetail(ctx context.Context, tenantID, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pt.id AS id, pt.pick_no AS pick_no, pt.order_no AS order_no, pt.customer AS customer,
			pt.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			pt.status AS status, pt.priority AS priority, pt.due_date AS due_date, pt.remark AS remark,
			pt.created_at AS created_at, pt.updated_at AS updated_at, pt.completed_at AS completed_at,
			pt.cancelled_at AS cancelled_at, pt.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			pt.cancel_remark AS cancel_remark
		FROM picking_tasks pt
		JOIN warehouses w ON w.id = pt.warehouse_id
		LEFT JOIN users cu ON cu.id = pt.cancelled_by
		WHERE pt.id = ? AND pt.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// UpdateTask is only ever called within a transaction that already
// loaded task via a tenant-scoped FindTaskByID, so the extra predicate
// here is defense in depth, not the primary guard — matching
// internal/inventory's own Batch 1 precedent of scoping every write even
// when an earlier read in the same transaction already proved ownership.
func (r *Repository) UpdateTask(ctx context.Context, tenantID int64, task *Task) error {
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).Updates(map[string]any{
		"status":       task.Status,
		"remark":       task.Remark,
		"completed_at": task.CompletedAt,
	}).Error
}

// CancelTask writes the exact columns Cancel derives — status plus the
// three Phase 18 audit columns — in one explicit UPDATE, deliberately
// separate from UpdateTask (which Confirm also calls, and which has no
// business ever touching cancellation columns). Never called except from
// a Cancel that has already passed Status.CanCancel, so cancelled_at/
// cancelled_by/cancel_remark are only ever written once per task.
func (r *Repository) CancelTask(ctx context.Context, tenantID int64, task *Task) error {
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).Updates(map[string]any{
		"status":        task.Status,
		"cancelled_at":  task.CancelledAt,
		"cancelled_by":  task.CancelledBy,
		"cancel_remark": task.CancelRemark,
	}).Error
}

type listRow struct {
	ID            int64
	PickNo        string
	OrderNo       string
	Customer      *string
	WarehouseCode string
	Priority      string
	DueDate       *time.Time
	Status        string
	ItemCount     int
	OrderedTotal  float64
	PickedTotal   float64
}

// List returns task-level rows matching PickingView.vue's table
// directly (one row per task, with an aggregated item count and
// progress percentage) — unlike Putaway's item-level list, since the
// mock's own table is task-level here. Phase 30 Batch 2: scoped to
// tenantID so one tenant's picking history never leaks into another's
// list/export.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM picking_tasks pt
		JOIN warehouses w ON w.id = pt.warehouse_id
		LEFT JOIN picking_task_items pti ON pti.picking_task_id = pt.id
		LEFT JOIN products p ON p.id = pti.product_id
	`
	where := `WHERE pt.tenant_id = ?
		AND (? = '' OR pt.pick_no LIKE ? OR pt.order_no LIKE ? OR pt.customer LIKE ? OR p.sku LIKE ? OR p.name LIKE ?)
		AND (? = '' OR pt.status = ?)
		AND (? = '' OR pt.priority = ?)
		AND (? = 0 OR pt.warehouse_id = ?)
		AND (? IS NULL OR pt.due_date = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like, like, like,
		string(params.Status), string(params.Status),
		string(params.Priority), string(params.Priority),
		params.WarehouseID, params.WarehouseID,
		params.Date, params.Date,
	}

	countSQL := "SELECT COUNT(DISTINCT pt.id) " + base + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT pt.id AS id, pt.pick_no AS pick_no, pt.order_no AS order_no, pt.customer AS customer,
			w.code AS warehouse_code, pt.priority AS priority, pt.due_date AS due_date, pt.status AS status,
			COUNT(DISTINCT pti.id) AS item_count,
			COALESCE(SUM(pti.ordered_qty), 0) AS ordered_total,
			COALESCE(SUM(pti.picked_qty), 0) AS picked_total
	` + base + where + `
		GROUP BY pt.id, pt.pick_no, pt.order_no, pt.customer, w.code, pt.priority, pt.due_date, pt.status
		ORDER BY pt.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		progress := 0.0
		if row.OrderedTotal > 0 {
			progress = (row.PickedTotal / row.OrderedTotal) * 100
		} else if row.PickedTotal > 0 {
			progress = 100
		}
		responses = append(responses, ListResponse{
			ID: row.ID, PickNo: row.PickNo, OrderNo: row.OrderNo, Customer: row.Customer,
			WarehouseCode: row.WarehouseCode, Priority: Priority(row.Priority), ItemCount: row.ItemCount,
			ProgressPct: progress, DueDate: row.DueDate, Status: Status(row.Status),
		})
	}
	return responses, total, nil
}

type itemRow struct {
	ID           int64
	ProductID    int64
	SKU          string
	ProductName  string
	LotNo        string
	LocationID   int64
	LocationCode string
	OrderedQty   float64
	PickedQty    float64
	UOM          string
	Status       string
	Remark       *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CountOpen returns how many picking tasks are still in a status
// Status.CanConfirm() would accept (PENDING/IN_PROGRESS) — Phase 23's
// Dashboard "Pending Picking" card. A single indexed COUNT(*)
// (IX_picking_tasks_status), not a List() call, since the dashboard
// only ever needs the number, never the rows. Phase 30 Batch 2: scoped
// to tenantID; Dashboard itself is not yet tenant-scoped, so it still
// passes inventory.UnverifiedCallerTenantID here (see dashboard's own
// service.go comment) — the gap is now explicit at every layer rather
// than silently absent from this query.
func (r *Repository) CountOpen(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Task{}).
		Where("tenant_id = ? AND status IN ?", tenantID, []string{string(StatusPending), string(StatusInProgress)}).
		Count(&count).Error
	return count, err
}

// ListItems is only ever called by Service.Get, right after a
// tenant-scoped FindTaskDetail already proved taskID belongs to
// tenantID — the tenant_id predicate here is defense in depth, matching
// UpdateTask's own reasoning above.
func (r *Repository) ListItems(ctx context.Context, tenantID, taskID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pti.id AS id, pti.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			pti.lot_no AS lot_no, pti.location_id AS location_id, l.code AS location_code,
			pti.ordered_qty AS ordered_qty, pti.picked_qty AS picked_qty, pti.uom AS uom,
			pti.status AS status, pti.remark AS remark, pti.created_at AS created_at, pti.updated_at AS updated_at
		FROM picking_task_items pti
		JOIN products p ON p.id = pti.product_id
		JOIN locations l ON l.id = pti.location_id
		WHERE pti.picking_task_id = ? AND pti.tenant_id = ?
		ORDER BY pti.id ASC
	`, taskID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName, LotNo: row.LotNo,
			LocationID: row.LocationID, LocationCode: row.LocationCode,
			OrderedQty: row.OrderedQty, PickedQty: row.PickedQty, UOM: row.UOM,
			Status: ItemStatus(row.Status), Remark: row.Remark, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return items, nil
}

func (r *Repository) CreateItem(ctx context.Context, tenantID int64, item *TaskItem) error {
	item.TenantID = tenantID
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, tenantID, id int64) (*TaskItem, error) {
	var item TaskItem
	err := r.db.WithContext(ctx).First(&item, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FindItemsByTask returns the raw TaskItem rows (not the joined response
// shape) — used internally by Confirm/Cancel to compute the header's new
// status after applying deltas. Scoped to tenantID even though the
// caller already proved task ownership — the lines themselves must also
// be verified, not just the header, since nothing stops a row-level
// insertion bug elsewhere from mixing tenants on this table.
func (r *Repository) FindItemsByTask(ctx context.Context, tenantID, taskID int64) ([]TaskItem, error) {
	var items []TaskItem
	err := r.db.WithContext(ctx).Where("picking_task_id = ? AND tenant_id = ?", taskID, tenantID).Find(&items).Error
	return items, err
}

// AdjustPickedQty atomically increments a line's picked_qty by delta in
// one UPDATE statement — the arithmetic happens inside the database, not
// as a Go read-then-write, the same concurrency-safety pattern as
// internal/inventory.Repository.AdjustBalance. Built this way from the
// start (see internal/receiving and internal/putaway's own Phase 9 fix
// for the lost-update race this exact shape of code previously had on
// their equivalent columns) rather than retrofitted — see
// PROJECT_STATUS.md. Phase 30 Batch 2: tenant_id now lives directly in
// this atomic UPDATE's WHERE clause (not a separate pre-check), the same
// highest-risk fix Batch 1 applied to AdjustBalance/AdjustReservation —
// a mismatched tenant_id here would otherwise corrupt quantity, not just
// leak data.
func (r *Repository) AdjustPickedQty(ctx context.Context, tenantID, itemID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE picking_task_items
		SET picked_qty = picked_qty + ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ?
	`, delta, itemID, tenantID)
	return result.RowsAffected, result.Error
}

// UpdateItemAfterConfirm writes the fields Confirm derives after calling
// AdjustPickedQty (or, for a SHORT line, after no quantity change at
// all) — deliberately excludes picked_qty itself so this write can never
// clobber a concurrent transaction's own atomic increment with a value
// computed from a stale read.
func (r *Repository) UpdateItemAfterConfirm(ctx context.Context, tenantID int64, item *TaskItem) error {
	return r.db.WithContext(ctx).Model(&TaskItem{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"status": item.Status,
		"remark": item.Remark,
	}).Error
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0019's doc. Must be called inside the same transaction as
// the stock mutations it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, taskID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{TenantID: tenantID, IdempotencyKey: idempotencyKey, PickingTaskID: taskID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// locationInfo is what Create/Confirm need to validate a source location
// before drawing stock from it: which warehouse it's really in, and
// whether it's active. Unlike Putaway's destination-location check,
// picking's source location carries no zone-type restriction — nothing
// in PickingView.vue states a rule equivalent to NewPutawayView.vue's
// "storage bin locations only" subtitle, so no such rule is invented
// here (see PROJECT_STATUS.md).
type locationInfo struct {
	WarehouseID int64
	Status      string
}

// resolveLocation is Picking's own location-ownership check — Phase 30
// Batch 2 adds the tenant_id predicate directly to this existing joined
// query rather than introducing a separate checker interface: a location
// belonging to another tenant now simply doesn't match any row, so
// WarehouseID stays 0 and ErrLocationNotFound is returned exactly as for
// a nonexistent id, closing the Location-ownership gap item 4 calls for.
func (r *Repository) resolveLocation(ctx context.Context, tenantID, locationID int64) (*locationInfo, error) {
	var info locationInfo
	err := r.db.WithContext(ctx).Raw(`
		SELECT w.id AS warehouse_id, l.status AS status
		FROM locations l
		JOIN zones z ON z.id = l.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		WHERE l.id = ? AND l.tenant_id = ?
	`, locationID, tenantID).Scan(&info).Error
	if err != nil {
		return nil, err
	}
	if info.WarehouseID == 0 {
		return nil, ErrLocationNotFound
	}
	return &info, nil
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) instead of matching
// substrings in the error message — see internal/receiving and
// internal/putaway's own doc for why this matters for a PRIMARY KEY
// (not plain UNIQUE) violation's real SQL Server message text.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

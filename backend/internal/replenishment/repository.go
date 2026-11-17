package replenishment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("replenishment: not found")
var ErrLocationNotFound = errors.New("replenishment: location not found")
var ErrDuplicateIdempotencyKey = errors.New("replenishment: idempotency key already used")

// Confirmation is the idempotency-guard row — migrations/0037.
type Confirmation struct {
	IdempotencyKey string `gorm:"column:idempotency_key;primaryKey"`
	// TenantID (migrations/0045, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID            int64 `gorm:"column:tenant_id;not null;default:1"`
	ReplenishmentTaskID int64 `gorm:"column:replenishment_task_id"`
}

func (Confirmation) TableName() string { return "replenishment_confirmations" }

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the Confirm transaction) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both the
// raw *gorm.DB (so callers can construct an inventory.Repository bound
// to the exact same transaction) and a Repository bound to it — the
// same shape every other workflow package's own Transaction uses.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateTask stamps tenantID onto the header, inserts it, then derives
// task_no from the new IDENTITY id in the same transaction — the same
// race-free-by-construction approach as receipts.receipt_no /
// picking_tasks.pick_no. Batch 1c (tenant isolation): tenantID must be
// the caller's own verified tenant, never read from the request body.
func (r *Repository) CreateTask(ctx context.Context, tenantID int64, task *Task) error {
	task.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return err
	}
	task.TaskNo = fmt.Sprintf("RPL-%06d", task.ID)
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).
		Update("task_no", task.TaskNo).Error
}

// FindTaskByID returns id's row scoped to tenantID, or ErrNotFound —
// never "found but belongs to another tenant" vs. "doesn't exist at
// all" (tenant-isolation-security.md §6). A cross-tenant id now returns
// the identical error a nonexistent one does.
func (r *Repository) FindTaskByID(ctx context.Context, tenantID, id int64) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// UpdateTask is scoped to tenantID: a mismatched tenantID against a
// real task id affects zero rows rather than silently updating a
// wrong-tenant row.
func (r *Repository) UpdateTask(ctx context.Context, tenantID int64, task *Task) error {
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).Updates(map[string]any{
		"status":        task.Status,
		"started_at":    task.StartedAt,
		"completed_at":  task.CompletedAt,
		"issue_remark":  task.IssueRemark,
		"cancelled_at":  task.CancelledAt,
		"cancelled_by":  task.CancelledBy,
		"cancel_remark": task.CancelRemark,
	}).Error
}

// detailRow is FindTaskDetail's scan target — a single query joining
// product/location/warehouse/user names, matching every other
// workflow package's own header-detail query shape.
type detailRow struct {
	ID               int64
	TaskNo           string
	ProductID        int64
	SKU              string
	ProductName      string
	FromLocationID   int64
	FromLocationCode string
	ToLocationID     int64
	ToLocationCode   string
	LotNo            string
	Quantity         float64
	WarehouseID      int64
	WarehouseCode    string
	Status           string
	Remark           *string
	IssueRemark      *string
	CreatedBy        *int64
	CreatedByName    *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	StartedAt        *time.Time
	CompletedAt      *time.Time
	CancelledAt      *time.Time
	CancelledBy      *int64
	CancelledByName  *string
	CancelRemark     *string
}

// FindTaskDetail returns the joined header-detail row scoped to
// tenantID — a cross-tenant id now returns ErrNotFound exactly like a
// nonexistent one.
func (r *Repository) FindTaskDetail(ctx context.Context, tenantID, id int64) (*detailRow, error) {
	var row detailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT rt.id AS id, rt.task_no AS task_no, rt.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			rt.from_location_id AS from_location_id, fl.code AS from_location_code,
			rt.to_location_id AS to_location_id, tl.code AS to_location_code,
			rt.lot_no AS lot_no, rt.quantity AS quantity,
			rt.warehouse_id AS warehouse_id, w.code AS warehouse_code, rt.status AS status,
			rt.remark AS remark, rt.issue_remark AS issue_remark,
			rt.created_by AS created_by, cu.full_name AS created_by_name,
			rt.created_at AS created_at, rt.updated_at AS updated_at,
			rt.started_at AS started_at, rt.completed_at AS completed_at,
			rt.cancelled_at AS cancelled_at, rt.cancelled_by AS cancelled_by, xu.full_name AS cancelled_by_name,
			rt.cancel_remark AS cancel_remark
		FROM replenishment_tasks rt
		JOIN products p ON p.id = rt.product_id
		JOIN locations fl ON fl.id = rt.from_location_id
		JOIN locations tl ON tl.id = rt.to_location_id
		JOIN warehouses w ON w.id = rt.warehouse_id
		LEFT JOIN users cu ON cu.id = rt.created_by
		LEFT JOIN users xu ON xu.id = rt.cancelled_by
		WHERE rt.id = ? AND rt.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// List is scoped to tenantID: a cross-tenant row can never appear in
// another tenant's list.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM replenishment_tasks rt
		JOIN products p ON p.id = rt.product_id
		JOIN locations fl ON fl.id = rt.from_location_id
		JOIN locations tl ON tl.id = rt.to_location_id
		JOIN warehouses w ON w.id = rt.warehouse_id
	`
	where := `WHERE rt.tenant_id = ?
		AND (? = '' OR rt.task_no LIKE ? OR p.sku LIKE ? OR p.name LIKE ?)
		AND (? = '' OR rt.status = ?)
		AND (? = 0 OR rt.warehouse_id = ?)
		AND (? = 0 OR rt.product_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like,
		string(params.Status), string(params.Status),
		params.WarehouseID, params.WarehouseID,
		params.ProductID, params.ProductID,
	}

	var total int64
	countSQL := "SELECT COUNT(*) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT rt.id AS id, rt.task_no AS task_no, p.sku AS sku, p.name AS product_name,
			fl.code AS from_location_code, tl.code AS to_location_code,
			rt.quantity AS quantity, w.code AS warehouse_code, rt.status AS status, rt.created_at AS created_at
	` + base + where + `
		ORDER BY rt.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []ListResponse
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// locationInfo is what Create needs to validate a location before
// using it — which warehouse it's really in, and whether it's active.
// Identical shape to internal/picking's own resolveLocation.
type locationInfo struct {
	WarehouseID int64
	Status      string
}

// resolveLocation is scoped to tenantID: a location belonging to
// another tenant now returns ErrLocationNotFound exactly like a
// nonexistent one, closing the IDOR this lookup previously had.
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

// InsertConfirmation is the idempotency guard itself — see
// migrations/0037's doc. Must be called inside the same transaction as
// MoveStockTx. Stamps tenantID onto the guard row too, matching every
// other row this package writes.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, taskID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, TenantID: tenantID, ReplenishmentTaskID: taskID}).Error
	if err != nil && sqlerr.IsUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// recommendRow is Recommend's per-(product,location) scan target — see
// Service.Recommend's own doc for the algorithm built on top of it.
type recommendRow struct {
	ProductID    int64
	SKU          string
	ProductName  string
	MinStock     float64
	MaxStock     float64
	LocationID   int64
	LocationCode string
	LotNo        string
	AvailableQty float64
}

// ListForRecommend returns every (product, location, lot) balance in
// the given warehouse for ACTIVE products, ordered so each product's
// own rows are sorted by available_qty descending — Service.Recommend
// relies on that ordering to find "the location with the most surplus"
// without a second query per product.
//
// Batch 1c (tenant isolation): unlike stockadjustment's own
// FindOverduePending, this IS scoped by tenantID — GET
// /replenishment/recommendations is a normal authenticated HTTP
// endpoint (see handler.go's Register) reached with a real per-request
// tenant context, not a background worker scan with no single caller
// to scope to. Filtered on i.tenant_id: inventory rows always carry
// their own true tenant, and warehouseID has already been verified to
// belong to this same tenantID by Service.Recommend's own
// warehouses.FindByIDForTenant call before this query ever runs.
func (r *Repository) ListForRecommend(ctx context.Context, tenantID, warehouseID int64) ([]recommendRow, error) {
	var rows []recommendRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT p.id AS product_id, p.sku AS sku, p.name AS product_name,
			p.min_stock AS min_stock, p.max_stock AS max_stock,
			l.id AS location_id, l.code AS location_code, i.lot_no AS lot_no,
			(i.quantity - i.reserved_quantity) AS available_qty
		FROM inventory i
		JOIN products p ON p.id = i.product_id
		JOIN locations l ON l.id = i.location_id
		JOIN zones z ON z.id = l.zone_id
		WHERE z.warehouse_id = ? AND p.status = 'ACTIVE' AND i.tenant_id = ?
		ORDER BY p.id, available_qty DESC
	`, warehouseID, tenantID).Scan(&rows).Error
	return rows, err
}

// CancelTask persists a Cancel — same shape as UpdateTask, kept as its
// own named method for readability at Service.Cancel's own call site,
// matching internal/picking's own CancelTask precedent.
func (r *Repository) CancelTask(ctx context.Context, tenantID int64, task *Task) error {
	return r.UpdateTask(ctx, tenantID, task)
}

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

// CreateTask inserts the header, then derives task_no from the new
// IDENTITY id in the same transaction — the same race-free-by-
// construction approach as receipts.receipt_no / picking_tasks.pick_no.
func (r *Repository) CreateTask(ctx context.Context, task *Task) error {
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return err
	}
	task.TaskNo = fmt.Sprintf("RPL-%06d", task.ID)
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ?", task.ID).
		Update("task_no", task.TaskNo).Error
}

func (r *Repository) FindTaskByID(ctx context.Context, id int64) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) UpdateTask(ctx context.Context, task *Task) error {
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
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

func (r *Repository) FindTaskDetail(ctx context.Context, id int64) (*detailRow, error) {
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
		WHERE rt.id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM replenishment_tasks rt
		JOIN products p ON p.id = rt.product_id
		JOIN locations fl ON fl.id = rt.from_location_id
		JOIN locations tl ON tl.id = rt.to_location_id
		JOIN warehouses w ON w.id = rt.warehouse_id
	`
	where := `WHERE (? = '' OR rt.task_no LIKE ? OR p.sku LIKE ? OR p.name LIKE ?)
		AND (? = '' OR rt.status = ?)
		AND (? = 0 OR rt.warehouse_id = ?)
		AND (? = 0 OR rt.product_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
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

func (r *Repository) resolveLocation(ctx context.Context, locationID int64) (*locationInfo, error) {
	var info locationInfo
	err := r.db.WithContext(ctx).Raw(`
		SELECT w.id AS warehouse_id, l.status AS status
		FROM locations l
		JOIN zones z ON z.id = l.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		WHERE l.id = ?
	`, locationID).Scan(&info).Error
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
// MoveStockTx.
func (r *Repository) InsertConfirmation(ctx context.Context, idempotencyKey string, taskID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, ReplenishmentTaskID: taskID}).Error
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
func (r *Repository) ListForRecommend(ctx context.Context, warehouseID int64) ([]recommendRow, error) {
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
		WHERE z.warehouse_id = ? AND p.status = 'ACTIVE'
		ORDER BY p.id, available_qty DESC
	`, warehouseID).Scan(&rows).Error
	return rows, err
}

// CancelTask persists a Cancel — same shape as UpdateTask, kept as its
// own named method for readability at Service.Cancel's own call site,
// matching internal/picking's own CancelTask precedent.
func (r *Repository) CancelTask(ctx context.Context, task *Task) error {
	return r.UpdateTask(ctx, task)
}

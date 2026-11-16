package relocation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("relocation: not found")
var ErrItemNotFound = errors.New("relocation: item not found")
var ErrDuplicateIdempotencyKey = errors.New("relocation: idempotency key already used")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the confirm workflow) live in Service. Every method takes
// an explicit tenantID and applies it to every query, mirroring
// internal/putaway's own Phase 31C discipline from the start (this
// package has no pre-tenant-scoping history to retrofit).
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both
// the raw *gorm.DB (so callers can construct an inventory.Repository
// bound to the exact same transaction — see service.go's Confirm) and a
// Repository bound to it for relocation's own queries. Matches
// putaway.Repository.Transaction exactly.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateTask inserts the header, then derives relocation_no from the
// new IDENTITY id in the same transaction — the same race-free-by-
// construction approach as putaway_no/receipt_no.
func (r *Repository) CreateTask(ctx context.Context, tenantID int64, task *Task) error {
	task.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return err
	}
	task.RelocationNo = fmt.Sprintf("RL-%06d", task.ID)
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).
		Update("relocation_no", task.RelocationNo).Error
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
	RelocationNo    string
	WarehouseID     int64
	WarehouseCode   string
	Status          string
	Remark          *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ReadyAt         *time.Time
	CompletedAt     *time.Time
	CancelledAt     *time.Time
	CancelledBy     *int64
	CancelledByName *string
	CancelRemark    *string
}

// FindTaskDetail returns the header with warehouse display fields
// resolved, for GET /relocations/{id}'s response — matches
// putaway.Repository.FindTaskDetail's shape.
func (r *Repository) FindTaskDetail(ctx context.Context, tenantID, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT rt.id AS id, rt.relocation_no AS relocation_no,
			rt.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			rt.status AS status, rt.remark AS remark,
			rt.created_at AS created_at, rt.updated_at AS updated_at, rt.ready_at AS ready_at, rt.completed_at AS completed_at,
			rt.cancelled_at AS cancelled_at, rt.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			rt.cancel_remark AS cancel_remark
		FROM relocation_tasks rt
		JOIN warehouses w ON w.id = rt.warehouse_id
		LEFT JOIN users cu ON cu.id = rt.cancelled_by
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

func (r *Repository) UpdateTask(ctx context.Context, tenantID int64, task *Task) error {
	result := r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).Updates(map[string]any{
		"status":       task.Status,
		"remark":       task.Remark,
		"ready_at":     task.ReadyAt,
		"completed_at": task.CompletedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CancelTask writes the exact columns Cancel derives — status plus the
// three audit columns — in one explicit UPDATE, deliberately separate
// from UpdateTask. Matches putaway.Repository.CancelTask.
func (r *Repository) CancelTask(ctx context.Context, tenantID int64, task *Task) error {
	result := r.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND tenant_id = ?", task.ID, tenantID).Updates(map[string]any{
		"status":        task.Status,
		"cancelled_at":  task.CancelledAt,
		"cancelled_by":  task.CancelledBy,
		"cancel_remark": task.CancelRemark,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

type listRow struct {
	TaskID                  int64
	ItemID                  int64
	RelocationNo            string
	SKU                     string
	ProductName             string
	LotNo                   string
	RequestedQty            float64
	MovedQty                float64
	SourceLocationCode      string
	DestinationLocationCode string
	Status                  string
	TaskStatus              string
}

// List returns item-level rows joined with their task header — matches
// PutawayView.vue's own item-level table convention, adapted for the
// Relocation List screen.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM relocation_task_items rti
		JOIN relocation_tasks rt ON rt.id = rti.relocation_task_id
		JOIN products p ON p.id = rti.product_id
		JOIN locations sl ON sl.id = rti.source_location_id
		JOIN locations dl ON dl.id = rti.destination_location_id
	`
	where := `WHERE rti.tenant_id = ?
		AND (? = '' OR rt.relocation_no LIKE ? OR p.sku LIKE ? OR p.name LIKE ? OR dl.code LIKE ?)
		AND (? = '' OR rti.status = ?)
		AND (? = '' OR rt.status = ?)
		AND (? = 0 OR rt.warehouse_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like, like,
		string(params.Status), string(params.Status),
		string(params.TaskStatus), string(params.TaskStatus),
		params.WarehouseID, params.WarehouseID,
	}

	var total int64
	countSQL := "SELECT COUNT(*) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT rti.id AS item_id, rt.id AS task_id, rt.relocation_no AS relocation_no,
			p.sku AS sku, p.name AS product_name, rti.lot_no AS lot_no,
			rti.requested_qty AS requested_qty, rti.moved_qty AS moved_qty,
			sl.code AS source_location_code, dl.code AS destination_location_code,
			rti.status AS status, rt.status AS task_status
	` + base + where + `
		ORDER BY rt.id DESC, rti.id ASC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			TaskID: row.TaskID, ItemID: row.ItemID, RelocationNo: row.RelocationNo,
			SKU: row.SKU, ProductName: row.ProductName, LotNo: row.LotNo,
			RequestedQty: row.RequestedQty, MovedQty: row.MovedQty,
			SourceLocationCode: row.SourceLocationCode, DestinationLocationCode: row.DestinationLocationCode,
			Status: ItemStatus(row.Status), TaskStatus: Status(row.TaskStatus),
		})
	}
	return responses, total, nil
}

type itemRow struct {
	ID                      int64
	ProductID               int64
	SKU                     string
	ProductName             string
	LotNo                   string
	LPN                     *string
	ExpiryDate              *time.Time
	SourceLocationID        int64
	SourceLocationCode      string
	DestinationLocationID   int64
	DestinationLocationCode string
	RequestedQty            float64
	MovedQty                float64
	UOM                     string
	Status                  string
	Remark                  *string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// ListItems resolves display fields for one task's lines, including
// ExpiryDate — read live from the current inventory balance at the
// line's source (never stored on the line itself, see model.go's TaskItem
// doc), so it always reflects the lot's real expiry, not a stale copy.
func (r *Repository) ListItems(ctx context.Context, tenantID, taskID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT rti.id AS id, rti.product_id AS product_id,
			p.sku AS sku, p.name AS product_name, rti.lot_no AS lot_no, rti.lpn AS lpn,
			inv.expiry_date AS expiry_date,
			rti.source_location_id AS source_location_id, sl.code AS source_location_code,
			rti.destination_location_id AS destination_location_id, dl.code AS destination_location_code,
			rti.requested_qty AS requested_qty, rti.moved_qty AS moved_qty, rti.uom AS uom,
			rti.status AS status, rti.remark AS remark, rti.created_at AS created_at, rti.updated_at AS updated_at
		FROM relocation_task_items rti
		JOIN products p ON p.id = rti.product_id
		JOIN locations sl ON sl.id = rti.source_location_id
		JOIN locations dl ON dl.id = rti.destination_location_id
		LEFT JOIN inventory inv ON inv.product_id = rti.product_id AND inv.location_id = rti.source_location_id AND inv.lot_no = rti.lot_no
		WHERE rti.relocation_task_id = ? AND rti.tenant_id = ?
		ORDER BY rti.id ASC
	`, taskID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName,
			LotNo: row.LotNo, LPN: row.LPN, ExpiryDate: row.ExpiryDate,
			SourceLocationID: row.SourceLocationID, SourceLocationCode: row.SourceLocationCode,
			DestinationLocationID: row.DestinationLocationID, DestinationLocationCode: row.DestinationLocationCode,
			RequestedQty: row.RequestedQty, MovedQty: row.MovedQty, UOM: row.UOM,
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
// shape) — used internally by Confirm to compute the header's new
// status after applying deltas.
func (r *Repository) FindItemsByTask(ctx context.Context, tenantID, taskID int64) ([]TaskItem, error) {
	var items []TaskItem
	err := r.db.WithContext(ctx).Where("relocation_task_id = ? AND tenant_id = ?", taskID, tenantID).Find(&items).Error
	return items, err
}

// AdjustMovedQty atomically increments a line's moved_qty by delta in
// one UPDATE statement — the arithmetic happens inside the database, not
// as a Go read-then-write. Matches putaway.Repository.AdjustMovedQty.
func (r *Repository) AdjustMovedQty(ctx context.Context, tenantID, itemID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE relocation_task_items
		SET moved_qty = moved_qty + ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ?
	`, delta, itemID, tenantID)
	return result.RowsAffected, result.Error
}

// UpdateItemAfterConfirm writes the fields Confirm derives after
// calling AdjustMovedQty — deliberately excludes moved_qty itself so
// this write can never clobber a concurrent transaction's own atomic
// increment with a value computed from a stale read.
func (r *Repository) UpdateItemAfterConfirm(ctx context.Context, tenantID int64, item *TaskItem) error {
	result := r.db.WithContext(ctx).Model(&TaskItem{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"destination_location_id": item.DestinationLocationID,
		"status":                  item.Status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0078's doc. Must be called inside the same transaction as
// the stock mutations it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, taskID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, TenantID: tenantID, RelocationTaskID: taskID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// locationInfo is what Create/Confirm need to validate a location
// before using it — matches putaway.locationInfo exactly.
type locationInfo struct {
	WarehouseID int64
	ZoneType    string
	Status      string
	Capacity    *int
}

// resolveLocation verifies locationID belongs to tenantID and returns
// the fields Create/Confirm need to validate it — never distinguishes
// "found but belongs to another tenant" from "doesn't exist at all"
// (tenant-isolation-security.md §6), matching
// putaway.Repository.resolveLocation's own IDOR-closing shape.
func (r *Repository) resolveLocation(ctx context.Context, tenantID, locationID int64) (*locationInfo, error) {
	var info locationInfo
	err := r.db.WithContext(ctx).Raw(`
		SELECT w.id AS warehouse_id, z.type AS zone_type, l.status AS status, l.capacity AS capacity
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

// LocationOccupiedQty sums every balance's on-hand Quantity currently
// sitting at locationID, within tenantID — the "how full is this bin
// already" half of the Capacity check. Matches
// putaway.Repository.LocationOccupiedQty.
func (r *Repository) LocationOccupiedQty(ctx context.Context, tenantID, locationID int64) (float64, error) {
	var total float64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(quantity), 0) FROM inventory WHERE tenant_id = ? AND location_id = ?
	`, tenantID, locationID).Scan(&total).Error
	return total, err
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier instead of matching substrings in the error message.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

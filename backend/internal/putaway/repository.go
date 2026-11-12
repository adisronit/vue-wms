package putaway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("putaway: not found")
var ErrItemNotFound = errors.New("putaway: item not found")
var ErrDuplicateIdempotencyKey = errors.New("putaway: idempotency key already used")

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
// pattern internal/receiving established) and a Repository bound to it
// for putaway's own queries.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateTask inserts the header, then derives putaway_no from the new
// IDENTITY id in the same transaction — the same race-free-by-
// construction approach as receipts.receipt_no (migrations/0011).
func (r *Repository) CreateTask(ctx context.Context, task *Task) error {
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return err
	}
	task.PutawayNo = fmt.Sprintf("PA-%06d", task.ID)
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ?", task.ID).
		Update("putaway_no", task.PutawayNo).Error
}

func (r *Repository) FindTaskByID(ctx context.Context, id int64) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).First(&task, "id = ?", id).Error
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
	PutawayNo       string
	ReceiptID       *int64
	ReceiptNo       *string
	WarehouseID     int64
	WarehouseCode   string
	Status          string
	Priority        string
	AssignedTo      *string
	Remark          *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
	CancelledAt     *time.Time
	CancelledBy     *int64
	CancelledByName *string
	CancelRemark    *string
}

// FindTaskDetail returns the header with receipt/warehouse display
// fields resolved, for GET /putaway/{id}'s response — a separate query
// from FindTaskByID (which returns the plain GORM model, used internally
// by Update/Confirm where the joined display fields aren't needed). The
// LEFT JOIN to users (Phase 18) resolves cancelled_by to a display name,
// the same pattern Phase 14 established for created_by_name on stock
// counts.
func (r *Repository) FindTaskDetail(ctx context.Context, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pt.id AS id, pt.putaway_no AS putaway_no, pt.receipt_id AS receipt_id, rc.receipt_no AS receipt_no,
			pt.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			pt.status AS status, pt.priority AS priority, pt.assigned_to AS assigned_to, pt.remark AS remark,
			pt.created_at AS created_at, pt.updated_at AS updated_at, pt.completed_at AS completed_at,
			pt.cancelled_at AS cancelled_at, pt.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			pt.cancel_remark AS cancel_remark
		FROM putaway_tasks pt
		JOIN warehouses w ON w.id = pt.warehouse_id
		LEFT JOIN receipts rc ON rc.id = pt.receipt_id
		LEFT JOIN users cu ON cu.id = pt.cancelled_by
		WHERE pt.id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

func (r *Repository) UpdateTask(ctx context.Context, task *Task) error {
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
		"status":       task.Status,
		"assigned_to":  task.AssignedTo,
		"remark":       task.Remark,
		"completed_at": task.CompletedAt,
	}).Error
}

// CancelTask writes the exact columns Cancel derives — status plus the
// three Phase 18 audit columns — in one explicit UPDATE, deliberately
// separate from UpdateTask (which Confirm also calls, and which has no
// business ever touching cancellation columns). Never called except from
// a Cancel that has already passed Status.CanCancel and the MovedQty
// check, so cancelled_at/cancelled_by/cancel_remark are only ever
// written once per task.
func (r *Repository) CancelTask(ctx context.Context, task *Task) error {
	return r.db.WithContext(ctx).Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
		"status":        task.Status,
		"cancelled_at":  task.CancelledAt,
		"cancelled_by":  task.CancelledBy,
		"cancel_remark": task.CancelRemark,
	}).Error
}

type listRow struct {
	TaskID                  int64
	ItemID                  int64
	PutawayNo               string
	ReceiptNo               *string
	SKU                     string
	ProductName             string
	RequestedQty            float64
	MovedQty                float64
	SourceLocationCode      string
	DestinationLocationCode string
	Status                  string
}

// List returns item-level rows joined with their task header — matches
// PutawayView.vue's table directly (one row per item; there is no
// separate item-list drawer the way Receiving needed one).
func (r *Repository) List(ctx context.Context, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM putaway_task_items pti
		JOIN putaway_tasks pt ON pt.id = pti.putaway_task_id
		LEFT JOIN receipts rc ON rc.id = pt.receipt_id
		JOIN products p ON p.id = pti.product_id
		JOIN locations sl ON sl.id = pti.source_location_id
		JOIN locations dl ON dl.id = pti.destination_location_id
	`
	where := `WHERE (? = '' OR pt.putaway_no LIKE ? OR rc.receipt_no LIKE ? OR p.sku LIKE ? OR p.name LIKE ? OR dl.code LIKE ?)
		AND (? = '' OR pti.status = ?)
		AND (? = 0 OR pt.warehouse_id = ?)
		AND (? = 0 OR dl.zone_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		params.Search, like, like, like, like, like,
		string(params.Status), string(params.Status),
		params.WarehouseID, params.WarehouseID,
		params.ZoneID, params.ZoneID,
	}

	var total int64
	countSQL := "SELECT COUNT(*) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT pti.id AS item_id, pt.id AS task_id, pt.putaway_no AS putaway_no, rc.receipt_no AS receipt_no,
			p.sku AS sku, p.name AS product_name,
			pti.requested_qty AS requested_qty, pti.moved_qty AS moved_qty,
			sl.code AS source_location_code, dl.code AS destination_location_code,
			pti.status AS status
	` + base + where + `
		ORDER BY pt.id DESC, pti.id ASC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			TaskID: row.TaskID, ItemID: row.ItemID, PutawayNo: row.PutawayNo, ReceiptNo: row.ReceiptNo,
			SKU: row.SKU, ProductName: row.ProductName, RequestedQty: row.RequestedQty, MovedQty: row.MovedQty,
			SourceLocationCode: row.SourceLocationCode, DestinationLocationCode: row.DestinationLocationCode,
			Status: ItemStatus(row.Status),
		})
	}
	return responses, total, nil
}

type itemRow struct {
	ID                      int64
	ReceiptItemID           *int64
	ProductID               int64
	SKU                     string
	ProductName             string
	LotNo                   string
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

func (r *Repository) ListItems(ctx context.Context, taskID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pti.id AS id, pti.receipt_item_id AS receipt_item_id, pti.product_id AS product_id,
			p.sku AS sku, p.name AS product_name, pti.lot_no AS lot_no,
			pti.source_location_id AS source_location_id, sl.code AS source_location_code,
			pti.destination_location_id AS destination_location_id, dl.code AS destination_location_code,
			pti.requested_qty AS requested_qty, pti.moved_qty AS moved_qty, pti.uom AS uom,
			pti.status AS status, pti.remark AS remark, pti.created_at AS created_at, pti.updated_at AS updated_at
		FROM putaway_task_items pti
		JOIN products p ON p.id = pti.product_id
		JOIN locations sl ON sl.id = pti.source_location_id
		JOIN locations dl ON dl.id = pti.destination_location_id
		WHERE pti.putaway_task_id = ?
		ORDER BY pti.id ASC
	`, taskID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ReceiptItemID: row.ReceiptItemID, ProductID: row.ProductID, SKU: row.SKU,
			ProductName: row.ProductName, LotNo: row.LotNo,
			SourceLocationID: row.SourceLocationID, SourceLocationCode: row.SourceLocationCode,
			DestinationLocationID: row.DestinationLocationID, DestinationLocationCode: row.DestinationLocationCode,
			RequestedQty: row.RequestedQty, MovedQty: row.MovedQty, UOM: row.UOM,
			Status: ItemStatus(row.Status), Remark: row.Remark, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return items, nil
}

func (r *Repository) CreateItem(ctx context.Context, item *TaskItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, id int64) (*TaskItem, error) {
	var item TaskItem
	err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FindItemsByTask returns the raw TaskItem rows (not the joined
// response shape) — used internally by Confirm to compute the header's
// new status after applying deltas.
func (r *Repository) FindItemsByTask(ctx context.Context, taskID int64) ([]TaskItem, error) {
	var items []TaskItem
	err := r.db.WithContext(ctx).Where("putaway_task_id = ?", taskID).Find(&items).Error
	return items, err
}

func (r *Repository) UpdateItem(ctx context.Context, item *TaskItem) error {
	return r.db.WithContext(ctx).Model(&TaskItem{}).Where("id = ?", item.ID).Updates(map[string]any{
		"destination_location_id": item.DestinationLocationID,
		"moved_qty":               item.MovedQty,
		"status":                  item.Status,
		"remark":                  item.Remark,
	}).Error
}

// AdjustMovedQty atomically increments a line's moved_qty by delta in
// one UPDATE statement — the arithmetic happens inside the database, not
// as a Go read-then-write, the same concurrency-safety pattern as
// internal/inventory.Repository.AdjustBalance. Added in Phase 9 to fix a
// real lost-update bug Phase 8's own concurrency regression test found
// on receiving.received_qty and flagged as the same structural risk
// here: Confirm previously read moved_qty into a Go struct
// (FindItemByID), added the batch's quantity in Go, and wrote the whole
// row back (UpdateItem above) — under two concurrent Confirm calls
// against the same line, the second UPDATE's stale computed total could
// silently overwrite the first's. The inventory ledger/balance itself,
// written by MoveStockTx, is unaffected either way — only this
// display/progress column. See PROJECT_STATUS.md.
func (r *Repository) AdjustMovedQty(ctx context.Context, itemID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE putaway_task_items
		SET moved_qty = moved_qty + ?, updated_at = SYSUTCDATETIME()
		WHERE id = ?
	`, delta, itemID)
	return result.RowsAffected, result.Error
}

// UpdateItemAfterConfirm writes the fields Confirm derives after calling
// AdjustMovedQty (or, for an ISSUE line, after no quantity change at
// all) — deliberately excludes moved_qty itself (unlike UpdateItem
// above) so this write can never clobber a concurrent transaction's own
// atomic increment with a value computed from a stale read.
func (r *Repository) UpdateItemAfterConfirm(ctx context.Context, item *TaskItem) error {
	return r.db.WithContext(ctx).Model(&TaskItem{}).Where("id = ?", item.ID).Updates(map[string]any{
		"destination_location_id": item.DestinationLocationID,
		"status":                  item.Status,
		"remark":                  item.Remark,
	}).Error
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0016's doc. Must be called inside the same transaction as
// the stock mutations it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, idempotencyKey string, taskID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, PutawayTaskID: taskID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// locationInfo is what Confirm/Create need to validate a location
// before using it: which warehouse it's really in, whether it's active,
// and its zone's functional type (see Zone.Type, internal/zone/model.go
// — already exactly the RECEIVING/STORAGE/PICKING/... classification
// this phase needed; no schema extension was required, see
// PROJECT_STATUS.md).
type locationInfo struct {
	WarehouseID int64
	ZoneType    string
	Status      string
}

func (r *Repository) resolveLocation(ctx context.Context, locationID int64) (*locationInfo, error) {
	var info locationInfo
	err := r.db.WithContext(ctx).Raw(`
		SELECT w.id AS warehouse_id, z.type AS zone_type, l.status AS status
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

type sourceLineRow struct {
	ReceiptItemID      int64
	ProductID          int64
	SKU                string
	ProductName        string
	Barcode            *string
	LotNo              string
	SourceLocationID   int64
	SourceLocationCode string
	UOM                string
	ReceivedQty        float64
	AlreadyPutAwayQty  float64
}

// SourceLines returns a completed/partially-received receipt's lines
// still eligible for putaway (received_qty minus whatever's already
// been moved for that line across every putaway task, regardless of
// that task's own status — moved_qty only ever increases via a real
// confirmed move) — powers GET /putaway/sources/{receiptId}, replacing
// NewPutawayView.vue's mockSourceItems object.
func (r *Repository) SourceLines(ctx context.Context, receiptID int64) ([]SourceLineResponse, error) {
	var rows []sourceLineRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT ri.id AS receipt_item_id, ri.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			p.barcode AS barcode, ri.lot_no AS lot_no, ri.location_id AS source_location_id, l.code AS source_location_code,
			ri.uom AS uom, ri.received_qty AS received_qty,
			COALESCE((SELECT SUM(pti.moved_qty) FROM putaway_task_items pti WHERE pti.receipt_item_id = ri.id), 0) AS already_put_away_qty
		FROM receipt_items ri
		JOIN products p ON p.id = ri.product_id
		JOIN locations l ON l.id = ri.location_id
		WHERE ri.receipt_id = ? AND ri.received_qty > 0
		ORDER BY ri.id ASC
	`, receiptID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	lines := make([]SourceLineResponse, 0, len(rows))
	for _, row := range rows {
		remaining := row.ReceivedQty - row.AlreadyPutAwayQty
		if remaining <= 0 {
			continue
		}
		lines = append(lines, SourceLineResponse{
			ReceiptItemID: row.ReceiptItemID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName,
			Barcode: row.Barcode, LotNo: row.LotNo, SourceLocationID: row.SourceLocationID,
			SourceLocationCode: row.SourceLocationCode, UOM: row.UOM, ReceivedQty: row.ReceivedQty,
			AlreadyPutAwayQty: row.AlreadyPutAwayQty, RemainingQty: remaining,
		})
	}
	return lines, nil
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) instead of matching
// substrings in the error message — Phase 6 shipped this same
// idempotency pattern (internal/receiving) with only "2627"/"unique"
// checks and found live that a PRIMARY KEY violation's real message
// matches neither; this package's own original fix added a "duplicate
// key" substring check, which the centralized classifier makes
// unnecessary — see sqlerr's doc.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

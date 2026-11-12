package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("order: not found")
var ErrDuplicateExternalReference = errors.New("order: external_reference is already used by another order")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the CreatePickingTask orchestration) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both the
// raw *gorm.DB (so callers can pass it straight into
// picking.Service.CreateTx, running Order's own claim and Picking's own
// task creation as one atomic unit — see Service.CreatePickingTask) and
// a Repository bound to it for order's own queries — the same pattern
// every other workflow package's own Transaction already establishes.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateOrder inserts the header, then derives order_no from the new
// IDENTITY id in the same transaction — the same race-free-by-
// construction approach as pick_no/task_no/shipment_no.
func (r *Repository) CreateOrder(ctx context.Context, o *Order) error {
	if err := r.db.WithContext(ctx).Create(o).Error; err != nil {
		return err
	}
	o.OrderNo = fmt.Sprintf("ORD-%06d", o.ID)
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ?", o.ID).
		Update("order_no", o.OrderNo).Error
}

func (r *Repository) CreateItem(ctx context.Context, item *Item) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// FindByID returns the plain GORM model — used internally by Update/
// CreatePickingTask/Cancel where the joined display fields FindDetail
// resolves aren't needed.
func (r *Repository) FindByID(ctx context.Context, id int64) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).First(&o, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// FindByOrderNo supports lookup by the server-generated order_no —
// order_no is never client-supplied (see CreateRequest's doc), so this
// exists for completeness/future use (e.g. a barcode/QR scan flow)
// rather than any path this phase's own handlers call today.
func (r *Repository) FindByOrderNo(ctx context.Context, orderNo string) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).First(&o, "order_no = ?", orderNo).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// FindByExternalReference supports lookup by the client-supplied
// idempotency key a future ERP integration would re-POST against (see
// migrations/0038's own external_reference doc). Not wired to a
// dedicated handler route this phase — GET /orders?search= already
// matches external_reference — but kept available at the repository
// layer for that future integration.
func (r *Repository) FindByExternalReference(ctx context.Context, ref string) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).First(&o, "external_reference = ?", ref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) FindItemsByOrder(ctx context.Context, orderID int64) ([]Item, error) {
	var items []Item
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("id ASC").Find(&items).Error
	return items, err
}

// DeleteItemsByOrder removes every line for orderID — Update's full-
// replace semantics (delete then reinsert) rather than a per-line diff,
// safe only because a PENDING order (the only status Update ever runs
// against — see Status.CanUpdate) has never had CreatePickingTask
// reserve anything against these lines yet.
func (r *Repository) DeleteItemsByOrder(ctx context.Context, orderID int64) error {
	return r.db.WithContext(ctx).Where("order_id = ?", orderID).Delete(&Item{}).Error
}

// UpdateHeader writes the fields Update may change — never touches
// status/picking_task_id/cancel* columns, which have their own guarded
// writers (ClaimForPickingTask, LinkPickingTask, CancelOrder).
func (r *Repository) UpdateHeader(ctx context.Context, o *Order) error {
	err := r.db.WithContext(ctx).Model(&Order{}).Where("id = ?", o.ID).Updates(map[string]any{
		"customer":           o.Customer,
		"external_reference": o.ExternalReference,
		"warehouse_id":       o.WarehouseID,
		"remark":             o.Remark,
		"updated_at":         time.Now().UTC(),
	}).Error
	return err
}

// ClaimForPickingTask is CreatePickingTask's idempotency/concurrency
// guard — an atomic conditional UPDATE, the same ClaimStatus shape
// internal/stockcount's own CreateAdjustments established (Phase 16):
// 0 rows affected means the order is not PENDING (already has a picking
// task, was cancelled, or a concurrent call won the race), and
// Service.CreatePickingTask treats that uniformly as ErrNotReleasable —
// a clean 409 instead of the raw UNIQUE-constraint violation
// orders.picking_task_id would otherwise raise. Claiming BEFORE calling
// picking.Service.CreateTx, in the same transaction, is what makes a
// failed picking-task creation roll back this claim automatically too —
// no separate compensating update is needed.
func (r *Repository) ClaimForPickingTask(ctx context.Context, orderID int64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE orders
		SET status = ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND status = ? AND picking_task_id IS NULL
	`, string(StatusPicking), orderID, string(StatusPending))
	return result.RowsAffected, result.Error
}

// LinkPickingTask records which picking task CreatePickingTask created
// for this order — called only after picking.Service.CreateTx has
// already succeeded, inside the same transaction as ClaimForPickingTask.
func (r *Repository) LinkPickingTask(ctx context.Context, orderID, pickingTaskID int64) error {
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ?", orderID).
		Update("picking_task_id", pickingTaskID).Error
}

// CancelOrder persists a successful Cancel — status plus the
// cancellation-audit columns, in one explicit UPDATE, the same
// dedicated-writer shape every other workflow package's own CancelTask
// uses.
func (r *Repository) CancelOrder(ctx context.Context, o *Order) error {
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ?", o.ID).Updates(map[string]any{
		"status":        o.Status,
		"cancelled_at":  o.CancelledAt,
		"cancelled_by":  o.CancelledBy,
		"cancel_remark": o.CancelRemark,
	}).Error
}

// detailRow is FindDetail's scan target — a single query joining
// warehouse/user display names plus, via LEFT JOINs to picking_tasks and
// shipments (both at most one row per order: orders.picking_task_id and
// shipments.picking_task_id are each UNIQUE), the linked task's pick_no
// and the live picking/shipment status FulfillmentStatus is derived
// from — matching every other workflow package's own header-detail
// query shape, extended here per this phase's own orchestration-only
// design (Order never stores picking/shipment state itself).
type detailRow struct {
	ID                int64
	OrderNo           string
	Customer          *string
	ExternalReference *string
	WarehouseID       int64
	WarehouseCode     string
	PickingTaskID     *int64
	PickNo            *string
	PickingStatus     *string
	ShipmentStatus    *string
	Status            string
	Remark            *string
	CreatedBy         *int64
	CreatedByName     *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CancelledAt       *time.Time
	CancelledBy       *int64
	CancelledByName   *string
	CancelRemark      *string
}

func (r *Repository) FindDetail(ctx context.Context, id int64) (*detailRow, error) {
	var row detailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT o.id AS id, o.order_no AS order_no, o.customer AS customer, o.external_reference AS external_reference,
			o.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			o.picking_task_id AS picking_task_id, pt.pick_no AS pick_no, pt.status AS picking_status,
			sh.status AS shipment_status,
			o.status AS status, o.remark AS remark,
			o.created_by AS created_by, cu.full_name AS created_by_name,
			o.created_at AS created_at, o.updated_at AS updated_at,
			o.cancelled_at AS cancelled_at, o.cancelled_by AS cancelled_by, xu.full_name AS cancelled_by_name,
			o.cancel_remark AS cancel_remark
		FROM orders o
		JOIN warehouses w ON w.id = o.warehouse_id
		LEFT JOIN picking_tasks pt ON pt.id = o.picking_task_id
		LEFT JOIN shipments sh ON sh.picking_task_id = pt.id
		LEFT JOIN users cu ON cu.id = o.created_by
		LEFT JOIN users xu ON xu.id = o.cancelled_by
		WHERE o.id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

type itemDetailRow struct {
	ID          int64
	ProductID   int64
	SKU         string
	ProductName string
	OrderedQty  float64
	UOM         string
}

func (r *Repository) FindItemsDetail(ctx context.Context, orderID int64) ([]ItemResponse, error) {
	var rows []itemDetailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT oi.id AS id, oi.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			oi.ordered_qty AS ordered_qty, oi.uom AS uom
		FROM order_items oi
		JOIN products p ON p.id = oi.product_id
		WHERE oi.order_id = ?
		ORDER BY oi.id ASC
	`, orderID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName,
			OrderedQty: row.OrderedQty, UOM: row.UOM,
		})
	}
	return items, nil
}

type listRow struct {
	ID                int64
	OrderNo           string
	Customer          *string
	ExternalReference *string
	WarehouseCode     string
	ItemCount         int
	Status            string
	PickingStatus     *string
	ShipmentStatus    *string
	CreatedAt         time.Time
}

// List returns order-level rows matching a real Order Management list
// screen (one row per order, with an aggregated item count) — the
// picking_status/shipment_status LEFT JOINs are resolved here, in bulk,
// rather than via a per-row call into picking.Service/shipping.Service
// (which Service.Get uses instead — see its own doc) specifically to
// avoid an N+1 lookup across a page of orders; both paths feed the exact
// same pure deriveFulfillmentStatus function.
func (r *Repository) List(ctx context.Context, params ListParams) ([]listRow, int64, error) {
	base := `
		FROM orders o
		JOIN warehouses w ON w.id = o.warehouse_id
		LEFT JOIN order_items oi ON oi.order_id = o.id
		LEFT JOIN picking_tasks pt ON pt.id = o.picking_task_id
		LEFT JOIN shipments sh ON sh.picking_task_id = pt.id
	`
	where := `WHERE (? = '' OR o.order_no LIKE ? OR o.external_reference LIKE ? OR o.customer LIKE ?)
		AND (? = '' OR o.status = ?)
		AND (? = 0 OR o.warehouse_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		params.Search, like, like, like,
		string(params.Status), string(params.Status),
		params.WarehouseID, params.WarehouseID,
	}

	var total int64
	countSQL := "SELECT COUNT(DISTINCT o.id) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT o.id AS id, o.order_no AS order_no, o.customer AS customer, o.external_reference AS external_reference,
			w.code AS warehouse_code, COUNT(DISTINCT oi.id) AS item_count,
			o.status AS status, pt.status AS picking_status, sh.status AS shipment_status, o.created_at AS created_at
	` + base + where + `
		GROUP BY o.id, o.order_no, o.customer, o.external_reference, w.code, o.status, pt.status, sh.status, o.created_at
		ORDER BY o.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier — see internal/picking's own doc for why this matters over
// substring-matching the error message.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

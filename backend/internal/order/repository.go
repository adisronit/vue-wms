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

// CreateOrder stamps tenantID onto the header, inserts it, then derives
// order_no from the new IDENTITY id in the same transaction — the same
// race-free-by-construction approach as pick_no/task_no/shipment_no.
// Phase 30 Batch 3: tenantID must be the caller's own verified tenant,
// never read from the request body.
func (r *Repository) CreateOrder(ctx context.Context, tenantID int64, o *Order) error {
	o.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(o).Error; err != nil {
		return err
	}
	o.OrderNo = fmt.Sprintf("ORD-%06d", o.ID)
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND tenant_id = ?", o.ID, tenantID).
		Update("order_no", o.OrderNo).Error
}

func (r *Repository) CreateItem(ctx context.Context, tenantID int64, item *Item) error {
	item.TenantID = tenantID
	return r.db.WithContext(ctx).Create(item).Error
}

// FindByID returns the plain GORM model, scoped to tenantID — used
// internally by Update/CreatePickingTask/Cancel where the joined display
// fields FindDetail resolves aren't needed. Phase 30 Batch 3: a
// cross-tenant id now returns ErrNotFound exactly like a nonexistent one,
// closing the IDOR this query previously had.
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).First(&o, "id = ? AND tenant_id = ?", id, tenantID).Error
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
// rather than any path this phase's own handlers call today. Scoped to
// tenantID for the same reason FindByID is.
func (r *Repository) FindByOrderNo(ctx context.Context, tenantID int64, orderNo string) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).First(&o, "order_no = ? AND tenant_id = ?", orderNo, tenantID).Error
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
// layer for that future integration. Scoped to tenantID.
func (r *Repository) FindByExternalReference(ctx context.Context, tenantID int64, ref string) (*Order, error) {
	var o Order
	err := r.db.WithContext(ctx).First(&o, "external_reference = ? AND tenant_id = ?", ref, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// FindItemsByOrder is only ever called right after a tenant-scoped
// FindByID already proved orderID belongs to tenantID, but the
// tenant_id predicate here is defense in depth, not the primary guard —
// matching internal/picking's own FindItemsByTask precedent (Batch 2).
func (r *Repository) FindItemsByOrder(ctx context.Context, tenantID, orderID int64) ([]Item, error) {
	var items []Item
	err := r.db.WithContext(ctx).Where("order_id = ? AND tenant_id = ?", orderID, tenantID).Order("id ASC").Find(&items).Error
	return items, err
}

// DeleteItemsByOrder removes every line for orderID — Update's full-
// replace semantics (delete then reinsert) rather than a per-line diff,
// safe only because a PENDING order (the only status Update ever runs
// against — see Status.CanUpdate) has never had CreatePickingTask
// reserve anything against these lines yet. Scoped to tenantID as
// defense in depth, matching FindItemsByOrder above.
func (r *Repository) DeleteItemsByOrder(ctx context.Context, tenantID, orderID int64) error {
	return r.db.WithContext(ctx).Where("order_id = ? AND tenant_id = ?", orderID, tenantID).Delete(&Item{}).Error
}

// UpdateHeader writes the fields Update may change — never touches
// status/picking_task_id/cancel* columns, which have their own guarded
// writers (ClaimForPickingTask, LinkPickingTask, CancelOrder). Phase 30
// Batch 3: scoped to tenantID — the caller's own FindByID already proved
// ownership, but the write itself is scoped too, matching every other
// workflow package's own write-layer precedent.
func (r *Repository) UpdateHeader(ctx context.Context, tenantID int64, o *Order) error {
	err := r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND tenant_id = ?", o.ID, tenantID).Updates(map[string]any{
		"customer":               o.Customer,
		"customer_id":            o.CustomerID,
		"ship_to_id":             o.ShipToID,
		"ship_to_name":           o.ShipToName,
		"ship_to_address":        o.ShipToAddress,
		"ship_to_province":       o.ShipToProvince,
		"ship_to_district":       o.ShipToDistrict,
		"ship_to_postal_code":    o.ShipToPostalCode,
		"ship_to_contact_person": o.ShipToContactPerson,
		"ship_to_contact_phone":  o.ShipToContactPhone,
		"external_reference":     o.ExternalReference,
		"order_type":             o.OrderType,
		"priority":               o.Priority,
		"required_shipping_date": o.RequiredShippingDate,
		"required_delivery_date": o.RequiredDeliveryDate,
		"carrier":                o.Carrier,
		"shipping_method":        o.ShippingMethod,
		"delivery_instructions":  o.DeliveryInstructions,
		"prepared_by":            o.PreparedBy,
		"approved_by":            o.ApprovedBy,
		"warehouse_id":           o.WarehouseID,
		"remark":                 o.Remark,
		"updated_at":             time.Now().UTC(),
	}).Error
	return err
}

// ClaimForRelease is Service.Release's own atomic idempotency/
// concurrency guard — the same ClaimForPickingTask shape (a single
// conditional UPDATE: 0 rows affected means "not PENDING" or "wrong
// tenant", uniformly ErrNotPending to the caller), writing every
// Review & Release audit column (status, released_at/released_by/
// shortage_approved) in the one guarded statement so a concurrent
// double-click can never both succeed.
func (r *Repository) ClaimForRelease(ctx context.Context, tenantID, orderID, userID int64, shortageApproved bool) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE orders
		SET status = ?, released_at = SYSUTCDATETIME(), released_by = ?, shortage_approved = ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND status = ?
	`, string(StatusReleased), userID, shortageApproved, orderID, tenantID, string(StatusPending))
	return result.RowsAffected, result.Error
}

// UpdateItemShortage stamps the per-line availability-check result
// Service.Release's own checkAvailability just computed — write-only,
// defense-in-depth scoped to tenantID like every other line writer in
// this package (DeleteItemsByOrder, etc). shortfallQty writes NULL when
// hasShortage is false, so a line that was once short and later
// re-Released (not a real path today — CanRelease only ever allows one
// Release per order — but kept correct regardless) never shows a stale
// positive shortfall once resolved.
func (r *Repository) UpdateItemShortage(ctx context.Context, tenantID, itemID int64, hasShortage bool, shortfallQty float64) error {
	var shortfall *float64
	if hasShortage {
		shortfall = &shortfallQty
	}
	return r.db.WithContext(ctx).Model(&Item{}).Where("id = ? AND tenant_id = ?", itemID, tenantID).Updates(map[string]any{
		"shortage_flag": hasShortage,
		"shortfall_qty": shortfall,
	}).Error
}

// ClaimForPickingTask is CreatePickingTask's idempotency/concurrency
// guard — an atomic conditional UPDATE, the same ClaimStatus shape
// internal/stockcount's own CreateAdjustments established (Phase 16):
// 0 rows affected means the order is not PENDING (already has a picking
// task, was cancelled, or a concurrent call won the race) OR does not
// belong to tenantID, and Service.CreatePickingTask treats both cases
// uniformly as ErrNotReleasable — a clean 409 instead of the raw
// UNIQUE-constraint violation orders.picking_task_id would otherwise
// raise, and without ever distinguishing "exists in another tenant" from
// "not releasable" in the response. Claiming BEFORE calling
// picking.Service.CreateTx, in the same transaction, is what makes a
// failed picking-task creation roll back this claim automatically too —
// no separate compensating update is needed (a rolled-back claim leaves
// the order exactly RELEASED again, not PENDING — see Status.
// CanCreatePickingTask's own Phase 33A doc). Phase 30 Batch 3: tenant_id
// now lives directly in this atomic UPDATE's WHERE clause — the same
// highest-risk fix Batch 1/2 applied to AdjustBalance/AdjustPickedQty — a
// mismatched tenant_id here would otherwise let a caller claim (and thus
// create a picking task against) another tenant's order. Phase 33A:
// the required source status changed from PENDING to RELEASED — see
// Status.CanCreatePickingTask's own doc for why Release is now the one
// and only gate in front of this claim.
func (r *Repository) ClaimForPickingTask(ctx context.Context, tenantID, orderID int64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE orders
		SET status = ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND status = ? AND picking_task_id IS NULL
	`, string(StatusPicking), orderID, tenantID, string(StatusReleased))
	return result.RowsAffected, result.Error
}

// LinkPickingTask records which picking task CreatePickingTask created
// for this order — called only after picking.Service.CreateTx has
// already succeeded, inside the same transaction as ClaimForPickingTask.
// Scoped to tenantID as defense in depth.
func (r *Repository) LinkPickingTask(ctx context.Context, tenantID, orderID, pickingTaskID int64) error {
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND tenant_id = ?", orderID, tenantID).
		Update("picking_task_id", pickingTaskID).Error
}

// CancelOrder persists a successful Cancel — status plus the
// cancellation-audit columns, in one explicit UPDATE, the same
// dedicated-writer shape every other workflow package's own CancelTask
// uses. Scoped to tenantID as defense in depth.
func (r *Repository) CancelOrder(ctx context.Context, tenantID int64, o *Order) error {
	return r.db.WithContext(ctx).Model(&Order{}).Where("id = ? AND tenant_id = ?", o.ID, tenantID).Updates(map[string]any{
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
	ID                   int64
	OrderNo              string
	OrderType            string
	Priority             string
	Customer             *string
	CustomerID           *int64
	ShipToID             *int64
	ShipToName           *string
	ShipToAddress        *string
	ShipToProvince       *string
	ShipToDistrict       *string
	ShipToPostalCode     *string
	ShipToContactPerson  *string
	ShipToContactPhone   *string
	ExternalReference    *string
	RequiredShippingDate *time.Time
	RequiredDeliveryDate *time.Time
	Carrier              *string
	ShippingMethod       *string
	DeliveryInstructions *string
	PreparedBy           *string
	ApprovedBy           *string
	WarehouseID          int64
	WarehouseCode        string
	PickingTaskID        *int64
	PickNo               *string
	PickingStatus        *string
	ShipmentStatus       *string
	Status               string
	Remark               *string
	ReleasedAt           *time.Time
	ReleasedBy           *int64
	ReleasedByName       *string
	ShortageApproved     bool
	CreatedBy            *int64
	CreatedByName        *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CancelledAt          *time.Time
	CancelledBy          *int64
	CancelledByName      *string
	CancelRemark         *string
}

// FindDetail scopes to tenantID — Phase 30 Batch 3 closes the IDOR this
// query previously had: a cross-tenant id now returns ErrNotFound exactly
// like a nonexistent one.
func (r *Repository) FindDetail(ctx context.Context, tenantID, id int64) (*detailRow, error) {
	var row detailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT o.id AS id, o.order_no AS order_no, o.order_type AS order_type, o.priority AS priority,
			o.customer AS customer, o.external_reference AS external_reference,
			o.customer_id AS customer_id, o.ship_to_id AS ship_to_id,
			o.ship_to_name AS ship_to_name, o.ship_to_address AS ship_to_address,
			o.ship_to_province AS ship_to_province, o.ship_to_district AS ship_to_district,
			o.ship_to_postal_code AS ship_to_postal_code,
			o.ship_to_contact_person AS ship_to_contact_person, o.ship_to_contact_phone AS ship_to_contact_phone,
			o.required_shipping_date AS required_shipping_date, o.required_delivery_date AS required_delivery_date,
			o.carrier AS carrier, o.shipping_method AS shipping_method, o.delivery_instructions AS delivery_instructions,
			o.prepared_by AS prepared_by, o.approved_by AS approved_by,
			o.warehouse_id AS warehouse_id, w.code AS warehouse_code,
			o.picking_task_id AS picking_task_id, pt.pick_no AS pick_no, pt.status AS picking_status,
			sh.status AS shipment_status,
			o.status AS status, o.remark AS remark,
			o.released_at AS released_at, o.released_by AS released_by, ru.full_name AS released_by_name,
			o.shortage_approved AS shortage_approved,
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
		LEFT JOIN users ru ON ru.id = o.released_by
		WHERE o.id = ? AND o.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

type itemDetailRow struct {
	ID             int64
	ProductID      int64
	SKU            string
	ProductName    string
	OrderedQty     float64
	UOM            string
	StorageType    *string
	PreferredLotNo *string
	FEFOPreferred  bool
	ShortageFlag   bool
	ShortfallQty   *float64
	LineNotes      *string
}

// FindItemsDetail is only ever called right after a tenant-scoped
// FindDetail already proved orderID belongs to tenantID, but the
// tenant_id predicate here is defense in depth, matching
// FindItemsByOrder's own reasoning above. storage_type is read-only,
// resolved from the product master at query time — never stored on the
// order line itself (see ItemResponse's own doc).
func (r *Repository) FindItemsDetail(ctx context.Context, tenantID, orderID int64) ([]ItemResponse, error) {
	var rows []itemDetailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT oi.id AS id, oi.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			oi.ordered_qty AS ordered_qty, oi.uom AS uom, p.storage_type AS storage_type,
			oi.preferred_lot_no AS preferred_lot_no, oi.fefo_preferred AS fefo_preferred,
			oi.shortage_flag AS shortage_flag, oi.shortfall_qty AS shortfall_qty, oi.line_notes AS line_notes
		FROM order_items oi
		JOIN products p ON p.id = oi.product_id
		WHERE oi.order_id = ? AND oi.tenant_id = ?
		ORDER BY oi.id ASC
	`, orderID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		storageType := ""
		if row.StorageType != nil {
			storageType = *row.StorageType
		}
		items = append(items, ItemResponse{
			ID: row.ID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName,
			OrderedQty: row.OrderedQty, UOM: row.UOM, StorageType: storageType,
			PreferredLotNo: row.PreferredLotNo, FEFOPreferred: row.FEFOPreferred,
			ShortageFlag: row.ShortageFlag, ShortfallQty: row.ShortfallQty, LineNotes: row.LineNotes,
		})
	}
	return items, nil
}

type listRow struct {
	ID                int64
	OrderNo           string
	OrderType         string
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
// same pure deriveFulfillmentStatus function. Phase 30 Batch 3: scoped to
// tenantID so one tenant's order history never leaks into another's
// list.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]listRow, int64, error) {
	base := `
		FROM orders o
		JOIN warehouses w ON w.id = o.warehouse_id
		LEFT JOIN order_items oi ON oi.order_id = o.id
		LEFT JOIN picking_tasks pt ON pt.id = o.picking_task_id
		LEFT JOIN shipments sh ON sh.picking_task_id = pt.id
	`
	where := `WHERE o.tenant_id = ?
		AND (? = '' OR o.order_no LIKE ? OR o.external_reference LIKE ? OR o.customer LIKE ?)
		AND (? = '' OR o.status = ?)
		AND (? = 0 OR o.warehouse_id = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
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
		SELECT o.id AS id, o.order_no AS order_no, o.order_type AS order_type, o.customer AS customer, o.external_reference AS external_reference,
			w.code AS warehouse_code, COUNT(DISTINCT oi.id) AS item_count,
			o.status AS status, pt.status AS picking_status, sh.status AS shipment_status, o.created_at AS created_at
	` + base + where + `
		GROUP BY o.id, o.order_no, o.order_type, o.customer, o.external_reference, w.code, o.status, pt.status, sh.status, o.created_at
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

package shipping

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("shipping: not found")
var ErrDuplicateIdempotencyKey = errors.New("shipping: idempotency key already used")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the confirm workflow) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both the
// raw *gorm.DB and a Repository bound to it, the same pattern
// internal/receiving, internal/putaway, and internal/picking established.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateShipment stamps tenantID onto the header, inserts it, then
// derives shipment_no from the new IDENTITY id in the same transaction —
// the same race-free-by-construction approach as receipt_no/putaway_no/
// pick_no. Phase 30 Batch 4: tenantID must be the caller's own verified
// tenant, never read from the request body.
func (r *Repository) CreateShipment(ctx context.Context, tenantID int64, s *Shipment) error {
	s.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return err
	}
	s.ShipmentNo = fmt.Sprintf("SHP-%06d", s.ID)
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ? AND tenant_id = ?", s.ID, tenantID).
		Update("shipment_no", s.ShipmentNo).Error
}

// FindByID scopes to tenantID — Phase 30 Batch 4 closes the IDOR this
// query previously had: a cross-tenant id now returns ErrNotFound
// exactly like a nonexistent one.
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Shipment, error) {
	var s Shipment
	err := r.db.WithContext(ctx).First(&s, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByPickingTaskID returns the shipment already linked to a picking
// task, if any — kept read-only for internal/order's own display-only
// "has this order's picking task been shipped" lookup (Service.
// FindByPickingTaskID). Phase 33D.1: no longer the uniqueness/
// idempotency guard for Create — see FindByPackingJobID, which replaced
// it in that role, and migrations/0106's own doc for why the old bare
// UNIQUE constraint this pre-check used to sit ahead of is gone. Still
// scoped to tenantID (Phase 30 Batch 4) — internal/order's own
// Service.Get passes its own verified tenantID here.
func (r *Repository) FindByPickingTaskID(ctx context.Context, tenantID, pickingTaskID int64) (*Shipment, error) {
	var s Shipment
	err := r.db.WithContext(ctx).First(&s, "picking_task_id = ? AND tenant_id = ?", pickingTaskID, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByPackingJobID returns the shipment already linked to a packing
// job, if any — Phase 33D.1's replacement for FindByPickingTaskID's own
// former role: Create's pre-check ahead of migrations/0106's own
// UQ_shipments_packing_job_id, the real uniqueness guard. Scoped to
// tenantID.
func (r *Repository) FindByPackingJobID(ctx context.Context, tenantID, packingJobID int64) (*Shipment, error) {
	var s Shipment
	err := r.db.WithContext(ctx).First(&s, "packing_job_id = ? AND tenant_id = ?", packingJobID, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

type headerRow struct {
	ID                  int64
	ShipmentNo          string
	PackingJobID        int64
	JobNo               string
	PickingTaskID       int64
	PickNo              string
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
	Carrier             *string
	TrackingNo          *string
	Packages            int
	Weight              *float64
	ShipDate            *time.Time
	Address             *string
	Remark              *string
	Status              string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DispatchedAt        *time.Time
	DeliveredAt         *time.Time
	CancelledAt         *time.Time
	CancelledBy         *int64
	CancelledByName     *string
	CancelRemark        *string
}

// FindDetail returns the header with the linked packing job's own
// order/customer/ship-to snapshot and the linked picking task's
// pick_no resolved, for GET /shipping/{id}'s response — a separate
// query from FindByID (which returns the plain GORM model, used
// internally by Update/Confirm where the joined display fields aren't
// needed). The LEFT JOIN to users (Phase 18) resolves cancelled_by to a
// display name, the same pattern Phase 14 established for
// created_by_name on stock counts.
//
// Phase 33D.1: order_no/customer/ship_to* now come from packing_jobs
// (pj), not picking_tasks (pt) — pj already carries the correct,
// order-scoped snapshot for a batch-sourced job (Phase 33C.2's own
// Job.OrderNo doc); picking_tasks' own order_no/customer would be
// whichever (arbitrary) order the shared task happened to be created
// under, wrong for every batch-sourced shipment. pt is still joined
// (via s.picking_task_id, unaffected by this change) purely for
// pick_no. Phase 30 Batch 4: scoped to tenantID on the shipment's own
// row — a cross-tenant id now returns ErrNotFound exactly like a
// nonexistent one.
func (r *Repository) FindDetail(ctx context.Context, tenantID, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS id, s.shipment_no AS shipment_no, s.packing_job_id AS packing_job_id, pj.job_no AS job_no,
			s.picking_task_id AS picking_task_id,
			pt.pick_no AS pick_no, pj.order_no AS order_no, pj.customer AS customer,
			pj.customer_id AS customer_id, pj.ship_to_id AS ship_to_id,
			pj.ship_to_name AS ship_to_name, pj.ship_to_address AS ship_to_address,
			pj.ship_to_province AS ship_to_province, pj.ship_to_district AS ship_to_district,
			pj.ship_to_postal_code AS ship_to_postal_code,
			pj.ship_to_contact_person AS ship_to_contact_person, pj.ship_to_contact_phone AS ship_to_contact_phone,
			s.carrier AS carrier, s.tracking_no AS tracking_no, s.packages AS packages, s.weight AS weight,
			s.ship_date AS ship_date, s.address AS address, s.remark AS remark, s.status AS status,
			s.created_at AS created_at, s.updated_at AS updated_at,
			s.dispatched_at AS dispatched_at, s.delivered_at AS delivered_at,
			s.cancelled_at AS cancelled_at, s.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			s.cancel_remark AS cancel_remark
		FROM shipments s
		JOIN packing_jobs pj ON pj.id = s.packing_job_id
		JOIN picking_tasks pt ON pt.id = s.picking_task_id
		LEFT JOIN users cu ON cu.id = s.cancelled_by
		WHERE s.id = ? AND s.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// UpdateHeader writes the editable header fields — used by both the PUT
// endpoint and Create (nothing else touches these columns). Scoped to
// tenantID as defense in depth — the caller's own FindByID already
// proved ownership, matching every other workflow package's own
// write-layer precedent.
func (r *Repository) UpdateHeader(ctx context.Context, tenantID int64, s *Shipment) error {
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ? AND tenant_id = ?", s.ID, tenantID).Updates(map[string]any{
		"carrier":     s.Carrier,
		"tracking_no": s.TrackingNo,
		"packages":    s.Packages,
		"weight":      s.Weight,
		"ship_date":   s.ShipDate,
		"address":     s.Address,
		"remark":      s.Remark,
	}).Error
}

// UpdateStatus writes the status transition fields Confirm derives —
// deliberately excludes every editable header field above so a confirm
// action can never clobber a concurrent header edit (the same
// field-scoped-write discipline Phase 9's race fix established). Scoped
// to tenantID as defense in depth.
func (r *Repository) UpdateStatus(ctx context.Context, tenantID int64, s *Shipment) error {
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ? AND tenant_id = ?", s.ID, tenantID).Updates(map[string]any{
		"status":        s.Status,
		"remark":        s.Remark,
		"dispatched_at": s.DispatchedAt,
		"delivered_at":  s.DeliveredAt,
	}).Error
}

// CancelShipment writes the exact columns Cancel derives — status plus
// the three Phase 18 audit columns — in one explicit UPDATE, deliberately
// separate from UpdateStatus (which Confirm's DISPATCH/DELIVER/ISSUE
// actions also call, and which has no business ever touching
// cancellation columns). Never called except from a Cancel that has
// already passed Status.CanCancel, so cancelled_at/cancelled_by/
// cancel_remark are only ever written once per shipment. Scoped to
// tenantID as defense in depth.
func (r *Repository) CancelShipment(ctx context.Context, tenantID int64, s *Shipment) error {
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ? AND tenant_id = ?", s.ID, tenantID).Updates(map[string]any{
		"status":        s.Status,
		"cancelled_at":  s.CancelledAt,
		"cancelled_by":  s.CancelledBy,
		"cancel_remark": s.CancelRemark,
	}).Error
}

// MarkLoaded writes only loaded_at/loaded_by (migrations/0107-0108,
// Phase 33D.2) — deliberately excludes every other column, mirroring
// UpdateStatus's own field-scoped-write discipline, so this call can
// never clobber a concurrent header edit or status transition. Scoped
// to tenantID as defense in depth — Service.MarkLoaded's own FindByID
// already proved ownership.
func (r *Repository) MarkLoaded(ctx context.Context, tenantID int64, s *Shipment) error {
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ? AND tenant_id = ?", s.ID, tenantID).Updates(map[string]any{
		"loaded_at": s.LoadedAt,
		"loaded_by": s.LoadedBy,
	}).Error
}

type listRow struct {
	ID         int64
	ShipmentNo string
	OrderNo    string
	Customer   *string
	Carrier    *string
	TrackingNo *string
	Packages   int
	Weight     *float64
	ShipDate   *time.Time
	Status     string
}

// List returns shipment-level rows joined with the linked packing
// job's own order_no/customer snapshot (Phase 33D.1 — see FindDetail's
// own doc for why pj, not pt) — matches ShippingView.vue's table
// directly. Phase 30 Batch 4: scoped to tenantID so one tenant's
// shipping history never leaks into another's list/export.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM shipments s
		JOIN packing_jobs pj ON pj.id = s.packing_job_id
	`
	where := `WHERE s.tenant_id = ?
		AND (? = '' OR s.shipment_no LIKE ? OR pj.order_no LIKE ? OR pj.customer LIKE ? OR s.tracking_no LIKE ?)
		AND (? = '' OR s.status = ?)
		AND (? = '' OR s.carrier = ?)
		AND (? IS NULL OR s.ship_date = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like, like,
		string(params.Status), string(params.Status),
		params.Carrier, params.Carrier,
		params.Date, params.Date,
	}

	countSQL := "SELECT COUNT(*) " + base + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT s.id AS id, s.shipment_no AS shipment_no, pj.order_no AS order_no, pj.customer AS customer,
			s.carrier AS carrier, s.tracking_no AS tracking_no, s.packages AS packages, s.weight AS weight,
			s.ship_date AS ship_date, s.status AS status
	` + base + where + `
		ORDER BY s.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			ID: row.ID, ShipmentNo: row.ShipmentNo, OrderNo: row.OrderNo, Customer: row.Customer,
			Carrier: row.Carrier, TrackingNo: row.TrackingNo, Packages: row.Packages, Weight: row.Weight,
			ShipDate: row.ShipDate, Status: Status(row.Status),
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
	Qty          float64
	UOM          string
	CreatedAt    time.Time
}

// ListItems is only ever called right after a tenant-scoped FindDetail
// already proved shipmentID belongs to tenantID, but the tenant_id
// predicate here is defense in depth, matching internal/picking's own
// ListItems precedent (Batch 2).
func (r *Repository) ListItems(ctx context.Context, tenantID, shipmentID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT si.id AS id, si.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			si.lot_no AS lot_no, si.location_id AS location_id, l.code AS location_code,
			si.qty AS qty, si.uom AS uom, si.created_at AS created_at
		FROM shipment_items si
		JOIN products p ON p.id = si.product_id
		JOIN locations l ON l.id = si.location_id
		WHERE si.shipment_id = ? AND si.tenant_id = ?
		ORDER BY si.id ASC
	`, shipmentID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ProductID: row.ProductID, SKU: row.SKU, ProductName: row.ProductName, LotNo: row.LotNo,
			LocationID: row.LocationID, LocationCode: row.LocationCode, Qty: row.Qty, UOM: row.UOM,
			CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

func (r *Repository) CreateItem(ctx context.Context, tenantID int64, item *Item) error {
	item.TenantID = tenantID
	return r.db.WithContext(ctx).Create(item).Error
}

type sourceRow struct {
	PackingJobID  int64
	JobNo         string
	PickingTaskID int64
	PickNo        string
	OrderNo       string
	Customer      *string
	ItemCount     int
	IsBatch       bool
	BatchNo       *string
}

// Sources returns COMPLETED packing jobs that have no shipment yet —
// Phase 33D.1's replacement for the old raw-picking-task listing, now
// requiring the real pack-validation gate (internal/packing's own two-
// person checker-confirm) before a job is even eligible to ship. Powers
// GET /shipping/sources, mirroring packing.Repository.Sources' own
// query shape (NOT EXISTS against this package's own join table) for
// the ordinary (non-batch) case.
//
// IsBatch/BatchNo (mirroring packing.SourceResponse's own shape) are
// derived from pj.order_id being set (Phase 33C.2: a batch-sourced job
// always has order_id, an ordinary job never does) and a LEFT JOIN to
// picking_batches on the shared picking_task_id — a batch-sourced
// source task can surface more than one packing job here (one per
// order), each becoming its own independent Sources row and,
// eventually, its own independent shipment. Scoped to tenantID
// throughout; the correlated NOT EXISTS subquery needs no separate
// predicate since a shipment can only ever be linked to a packing job
// that shares its own tenant (migrations/0106's own FK_shipments_packing_job).
func (r *Repository) Sources(ctx context.Context, tenantID int64) ([]SourceResponse, error) {
	var rows []sourceRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pj.id AS packing_job_id, pj.job_no AS job_no, pj.picking_task_id AS picking_task_id, pt.pick_no AS pick_no,
			pj.order_no AS order_no, pj.customer AS customer, COUNT(pji.id) AS item_count,
			CASE WHEN pj.order_id IS NOT NULL THEN 1 ELSE 0 END AS is_batch, pb.batch_no AS batch_no
		FROM packing_jobs pj
		JOIN picking_tasks pt ON pt.id = pj.picking_task_id
		JOIN packing_job_items pji ON pji.packing_job_id = pj.id
		LEFT JOIN picking_batches pb ON pb.picking_task_id = pj.picking_task_id AND pb.tenant_id = pj.tenant_id
		WHERE pj.tenant_id = ?
			AND pj.status = 'COMPLETED'
			AND NOT EXISTS (SELECT 1 FROM shipments sh WHERE sh.packing_job_id = pj.id)
		GROUP BY pj.id, pj.job_no, pj.picking_task_id, pt.pick_no, pj.order_no, pj.customer, pj.order_id, pb.batch_no
		ORDER BY pj.id DESC
	`, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	sources := make([]SourceResponse, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, SourceResponse{
			PackingJobID: row.PackingJobID, JobNo: row.JobNo, PickingTaskID: row.PickingTaskID, PickNo: row.PickNo,
			OrderNo: row.OrderNo, Customer: row.Customer, ItemCount: row.ItemCount,
			IsBatch: row.IsBatch, BatchNo: row.BatchNo,
		})
	}
	return sources, nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0022's doc. Must be called inside the same transaction as
// the status transition it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, shipmentID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{TenantID: tenantID, IdempotencyKey: idempotencyKey, ShipmentID: shipmentID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) — built correct from
// the start, the same as internal/picking, since Phase 8 already
// generalized the fix Receiving/Putaway each needed live.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

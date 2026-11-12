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

// CreateShipment inserts the header, then derives shipment_no from the
// new IDENTITY id in the same transaction — the same race-free-by-
// construction approach as receipt_no/putaway_no/pick_no.
func (r *Repository) CreateShipment(ctx context.Context, s *Shipment) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return err
	}
	s.ShipmentNo = fmt.Sprintf("SHP-%06d", s.ID)
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ?", s.ID).
		Update("shipment_no", s.ShipmentNo).Error
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Shipment, error) {
	var s Shipment
	err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByPickingTaskID returns the shipment already linked to a picking
// task, if any — used by Create for a friendly pre-check ahead of the
// UNIQUE constraint (migrations/0020) that makes "one shipment per
// picking task" authoritative regardless.
func (r *Repository) FindByPickingTaskID(ctx context.Context, pickingTaskID int64) (*Shipment, error) {
	var s Shipment
	err := r.db.WithContext(ctx).First(&s, "picking_task_id = ?", pickingTaskID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

type headerRow struct {
	ID              int64
	ShipmentNo      string
	PickingTaskID   int64
	PickNo          string
	OrderNo         string
	Customer        *string
	Carrier         *string
	TrackingNo      *string
	Packages        int
	Weight          *float64
	ShipDate        *time.Time
	Address         *string
	Remark          *string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DispatchedAt    *time.Time
	DeliveredAt     *time.Time
	CancelledAt     *time.Time
	CancelledBy     *int64
	CancelledByName *string
	CancelRemark    *string
}

// FindDetail returns the header with the linked picking task's order_no/
// customer/pick_no resolved, for GET /shipping/{id}'s response — a
// separate query from FindByID (which returns the plain GORM model,
// used internally by Update/Confirm where the joined display fields
// aren't needed). The LEFT JOIN to users (Phase 18) resolves
// cancelled_by to a display name, the same pattern Phase 14 established
// for created_by_name on stock counts.
func (r *Repository) FindDetail(ctx context.Context, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS id, s.shipment_no AS shipment_no, s.picking_task_id AS picking_task_id,
			pt.pick_no AS pick_no, pt.order_no AS order_no, pt.customer AS customer,
			s.carrier AS carrier, s.tracking_no AS tracking_no, s.packages AS packages, s.weight AS weight,
			s.ship_date AS ship_date, s.address AS address, s.remark AS remark, s.status AS status,
			s.created_at AS created_at, s.updated_at AS updated_at,
			s.dispatched_at AS dispatched_at, s.delivered_at AS delivered_at,
			s.cancelled_at AS cancelled_at, s.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			s.cancel_remark AS cancel_remark
		FROM shipments s
		JOIN picking_tasks pt ON pt.id = s.picking_task_id
		LEFT JOIN users cu ON cu.id = s.cancelled_by
		WHERE s.id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// UpdateHeader writes the editable header fields — used by both the PUT
// endpoint and Create (nothing else touches these columns).
func (r *Repository) UpdateHeader(ctx context.Context, s *Shipment) error {
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ?", s.ID).Updates(map[string]any{
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
// field-scoped-write discipline Phase 9's race fix established).
func (r *Repository) UpdateStatus(ctx context.Context, s *Shipment) error {
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ?", s.ID).Updates(map[string]any{
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
// cancel_remark are only ever written once per shipment.
func (r *Repository) CancelShipment(ctx context.Context, s *Shipment) error {
	return r.db.WithContext(ctx).Model(&Shipment{}).Where("id = ?", s.ID).Updates(map[string]any{
		"status":        s.Status,
		"cancelled_at":  s.CancelledAt,
		"cancelled_by":  s.CancelledBy,
		"cancel_remark": s.CancelRemark,
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

// List returns shipment-level rows joined with the linked picking
// task's order_no/customer — matches ShippingView.vue's table directly.
func (r *Repository) List(ctx context.Context, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM shipments s
		JOIN picking_tasks pt ON pt.id = s.picking_task_id
	`
	where := `WHERE (? = '' OR s.shipment_no LIKE ? OR pt.order_no LIKE ? OR pt.customer LIKE ? OR s.tracking_no LIKE ?)
		AND (? = '' OR s.status = ?)
		AND (? = '' OR s.carrier = ?)
		AND (? IS NULL OR s.ship_date = ?)`

	like := "%" + params.Search + "%"
	args := []any{
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
		SELECT s.id AS id, s.shipment_no AS shipment_no, pt.order_no AS order_no, pt.customer AS customer,
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

func (r *Repository) ListItems(ctx context.Context, shipmentID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT si.id AS id, si.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			si.lot_no AS lot_no, si.location_id AS location_id, l.code AS location_code,
			si.qty AS qty, si.uom AS uom, si.created_at AS created_at
		FROM shipment_items si
		JOIN products p ON p.id = si.product_id
		JOIN locations l ON l.id = si.location_id
		WHERE si.shipment_id = ?
		ORDER BY si.id ASC
	`, shipmentID).Scan(&rows).Error
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

func (r *Repository) CreateItem(ctx context.Context, item *Item) error {
	return r.db.WithContext(ctx).Create(item).Error
}

type sourceRow struct {
	PickingTaskID int64
	PickNo        string
	OrderNo       string
	Customer      *string
	ItemCount     int
}

// Sources returns completed (or partially-completed, i.e. SHORT_PICK
// with at least one COMPLETED line) picking tasks that have no shipment
// yet — powers GET /shipping/sources, replacing ShippingView.vue's lack
// of any source picker at all (see dto.go's SourceResponse doc).
func (r *Repository) Sources(ctx context.Context) ([]SourceResponse, error) {
	var rows []sourceRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT pt.id AS picking_task_id, pt.pick_no AS pick_no, pt.order_no AS order_no, pt.customer AS customer,
			COUNT(pti.id) AS item_count
		FROM picking_tasks pt
		JOIN picking_task_items pti ON pti.picking_task_id = pt.id AND pti.status = 'COMPLETED'
		WHERE pt.status IN ('COMPLETED', 'SHORT_PICK')
			AND NOT EXISTS (SELECT 1 FROM shipments sh WHERE sh.picking_task_id = pt.id)
		GROUP BY pt.id, pt.pick_no, pt.order_no, pt.customer
		ORDER BY pt.id DESC
	`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	sources := make([]SourceResponse, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, SourceResponse{
			PickingTaskID: row.PickingTaskID, PickNo: row.PickNo, OrderNo: row.OrderNo, Customer: row.Customer,
			ItemCount: row.ItemCount,
		})
	}
	return sources, nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0022's doc. Must be called inside the same transaction as
// the status transition it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, idempotencyKey string, shipmentID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, ShipmentID: shipmentID}).Error
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

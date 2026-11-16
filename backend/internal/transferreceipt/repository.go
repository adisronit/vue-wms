package transferreceipt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("transferreceipt: not found")
var ErrItemNotFound = errors.New("transferreceipt: item not found")
var ErrDuplicateIdempotencyKey = errors.New("transferreceipt: idempotency key already used")
var ErrLocationNotFound = errors.New("transferreceipt: location not found")

// Repository is a thin GORM wrapper — business rules live in Service.
// Every method takes an explicit tenantID and applies it to every
// query, matching internal/relocation's own tenant-scoped-from-day-one
// discipline.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both
// the raw *gorm.DB (so callers can construct an inventory.Repository
// bound to the exact same transaction) and a Repository bound to it —
// matches relocation.Repository.Transaction exactly.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateReceipt inserts the header, then derives receipt_no from the
// new IDENTITY id in the same transaction — the same race-free-by-
// construction approach as relocation_no/receipt_no.
func (r *Repository) CreateReceipt(ctx context.Context, tenantID int64, receipt *Receipt) error {
	receipt.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(receipt).Error; err != nil {
		return err
	}
	receipt.ReceiptNo = fmt.Sprintf("TRC-%06d", receipt.ID)
	return r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ? AND tenant_id = ?", receipt.ID, tenantID).
		Update("receipt_no", receipt.ReceiptNo).Error
}

func (r *Repository) FindReceiptByID(ctx context.Context, tenantID, id int64) (*Receipt, error) {
	var receipt Receipt
	err := r.db.WithContext(ctx).First(&receipt, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

// FindReceiptByShipmentID backs the one-receipt-per-shipment guard —
// returns ErrNotFound (not a sentinel bool) when none exists yet, so
// Service.Create's call site reads naturally as "if it already exists,
// reject."
func (r *Repository) FindReceiptByShipmentID(ctx context.Context, tenantID, shipmentID int64) (*Receipt, error) {
	var receipt Receipt
	err := r.db.WithContext(ctx).First(&receipt, "shipment_id = ? AND tenant_id = ?", shipmentID, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

type headerRow struct {
	ID                       int64
	ReceiptNo                string
	ShipmentID               int64
	ShipmentNo               string
	OrderID                  int64
	OrderNo                  string
	TransferReference        *string
	DestinationWarehouseID   int64
	DestinationWarehouseCode string
	Status                   string
	ReceivedBy               *int64
	ReceivedByName           *string
	ReceivedAt               *time.Time
	Remark                   *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	CancelledAt              *time.Time
	CancelledBy              *int64
	CancelledByName          *string
	CancelRemark             *string
}

// FindReceiptDetail returns the header with every joined display field
// GET /transfer-receipts/{id} needs — shipment_no, order_no,
// transfer_reference (both from orders — the source INTERNAL_TRANSFER
// order), destination warehouse code, and received_by/cancelled_by
// names.
func (r *Repository) FindReceiptDetail(ctx context.Context, tenantID, id int64) (*headerRow, error) {
	var row headerRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT tr.id AS id, tr.receipt_no AS receipt_no,
			tr.shipment_id AS shipment_id, s.shipment_no AS shipment_no,
			tr.order_id AS order_id, o.order_no AS order_no, o.transfer_reference AS transfer_reference,
			tr.destination_warehouse_id AS destination_warehouse_id, dw.code AS destination_warehouse_code,
			tr.status AS status,
			tr.received_by AS received_by, ru.full_name AS received_by_name, tr.received_at AS received_at,
			tr.remark AS remark, tr.created_at AS created_at, tr.updated_at AS updated_at,
			tr.cancelled_at AS cancelled_at, tr.cancelled_by AS cancelled_by, cu.full_name AS cancelled_by_name,
			tr.cancel_remark AS cancel_remark
		FROM transfer_receipts tr
		JOIN shipments s ON s.id = tr.shipment_id
		JOIN orders o ON o.id = tr.order_id
		JOIN warehouses dw ON dw.id = tr.destination_warehouse_id
		LEFT JOIN users ru ON ru.id = tr.received_by
		LEFT JOIN users cu ON cu.id = tr.cancelled_by
		WHERE tr.id = ? AND tr.tenant_id = ?
	`, id, tenantID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// UpdateReceipt writes the header fields Confirm/ReceiveItem derive.
func (r *Repository) UpdateReceipt(ctx context.Context, tenantID int64, receipt *Receipt) error {
	result := r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ? AND tenant_id = ?", receipt.ID, tenantID).Updates(map[string]any{
		"status":      receipt.Status,
		"remark":      receipt.Remark,
		"received_by": receipt.ReceivedBy,
		"received_at": receipt.ReceivedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CancelReceipt writes the exact columns Cancel derives — status plus
// the three audit columns — in one explicit UPDATE, deliberately
// separate from UpdateReceipt, matching relocation.Repository.
// CancelTask exactly.
func (r *Repository) CancelReceipt(ctx context.Context, tenantID int64, receipt *Receipt) error {
	result := r.db.WithContext(ctx).Model(&Receipt{}).Where("id = ? AND tenant_id = ?", receipt.ID, tenantID).Updates(map[string]any{
		"status":        receipt.Status,
		"cancelled_at":  receipt.CancelledAt,
		"cancelled_by":  receipt.CancelledBy,
		"cancel_remark": receipt.CancelRemark,
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
	ID                       int64
	ReceiptNo                string
	ShipmentID               int64
	ShipmentNo               string
	OrderNo                  string
	TransferReference        *string
	DestinationWarehouseID   int64
	DestinationWarehouseCode string
	Status                   string
	CreatedAt                time.Time
}

// List returns header-level rows for the Transfer Receiving worklist.
func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	base := `
		FROM transfer_receipts tr
		JOIN shipments s ON s.id = tr.shipment_id
		JOIN orders o ON o.id = tr.order_id
		JOIN warehouses dw ON dw.id = tr.destination_warehouse_id
	`
	where := `WHERE tr.tenant_id = ?
		AND (? = '' OR tr.receipt_no LIKE ? OR s.shipment_no LIKE ? OR o.order_no LIKE ?)
		AND (? = '' OR tr.status = ?)
		AND (? = 0 OR tr.destination_warehouse_id = ?)
		AND (? = 0 OR EXISTS (
			SELECT 1 FROM transfer_receipt_items tri
			WHERE tri.transfer_receipt_id = tr.id AND tri.tenant_id = tr.tenant_id
				AND tri.discrepancy_qty IS NOT NULL AND tri.discrepancy_qty <> 0
		))`

	// hasDiscrepancyFlag (Phase 33E.3) drives the "? = 0 OR EXISTS (...)"
	// clause above using the exact same int-flag-over-a-bool-param shape
	// DestinationWarehouseID's own "? = 0 OR ..." clause already uses --
	// SQL Server's raw driver has no native bool bind for a literal "0 OR
	// 1" comparison in this position, so every other optional filter in
	// this query already goes through either a string or an int,
	// matching that precedent rather than introducing a new parameter
	// type for this one clause.
	hasDiscrepancyFlag := 0
	if params.HasDiscrepancy {
		hasDiscrepancyFlag = 1
	}

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like,
		string(params.Status), string(params.Status),
		params.DestinationWarehouseID, params.DestinationWarehouseID,
		hasDiscrepancyFlag,
	}

	var total int64
	countSQL := "SELECT COUNT(*) " + base + where
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT tr.id AS id, tr.receipt_no AS receipt_no, tr.shipment_id AS shipment_id, s.shipment_no AS shipment_no,
			o.order_no AS order_no, o.transfer_reference AS transfer_reference,
			tr.destination_warehouse_id AS destination_warehouse_id, dw.code AS destination_warehouse_code,
			tr.status AS status, tr.created_at AS created_at
	` + base + where + `
		ORDER BY tr.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			ID: row.ID, ReceiptNo: row.ReceiptNo, ShipmentID: row.ShipmentID, ShipmentNo: row.ShipmentNo,
			OrderNo: row.OrderNo, TransferReference: row.TransferReference,
			DestinationWarehouseID: row.DestinationWarehouseID, DestinationWarehouseCode: row.DestinationWarehouseCode,
			Status: Status(row.Status), CreatedAt: row.CreatedAt,
		})
	}
	return responses, total, nil
}

type eligibleShipmentRow struct {
	ShipmentID             int64
	ShipmentNo             string
	OrderID                int64
	OrderNo                string
	TransferReference      *string
	SourceWarehouseID      int64
	SourceWarehouseCode    string
	DestinationWarehouseID int64
	ShipmentStatus         string
	ItemCount              int
}

// ListEligibleShipments finds DISPATCHED-or-later INTERNAL_TRANSFER
// shipments, scoped to destinationWarehouseID, that have no transfer
// receipt yet — the Transfer Receiving screen's "+ New Receipt"
// shipment picker. Joins packing_jobs.order_no (always populated, even
// for a non-batch job — see internal/packing's own Job.OrderNo doc) to
// orders rather than any picking_tasks column, since picking_tasks
// itself carries no order_id at all; this is the same join shape
// internal/shipping's own SourceResponse query already uses to resolve
// an order from a shipment.
func (r *Repository) ListEligibleShipments(ctx context.Context, tenantID int64, params EligibleShipmentParams) ([]EligibleShipmentResponse, error) {
	like := "%" + params.Search + "%"
	var rows []eligibleShipmentRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS shipment_id, s.shipment_no AS shipment_no,
			o.id AS order_id, o.order_no AS order_no, o.transfer_reference AS transfer_reference,
			o.warehouse_id AS source_warehouse_id, sw.code AS source_warehouse_code,
			o.destination_warehouse_id AS destination_warehouse_id,
			s.status AS shipment_status,
			(SELECT COUNT(*) FROM shipment_items si WHERE si.shipment_id = s.id) AS item_count
		FROM shipments s
		JOIN packing_jobs pj ON pj.id = s.packing_job_id
		JOIN orders o ON o.order_no = pj.order_no AND o.tenant_id = s.tenant_id
		JOIN warehouses sw ON sw.id = o.warehouse_id
		WHERE s.tenant_id = ?
			AND o.order_type = 'INTERNAL_TRANSFER'
			AND o.destination_warehouse_id = ?
			AND s.status IN ('DISPATCHED', 'IN_TRANSIT', 'DELIVERED')
			AND (? = '' OR s.shipment_no LIKE ? OR o.order_no LIKE ?)
			AND NOT EXISTS (SELECT 1 FROM transfer_receipts tr WHERE tr.shipment_id = s.id AND tr.tenant_id = s.tenant_id)
		ORDER BY s.id DESC
	`, tenantID, params.DestinationWarehouseID, params.Search, like, like).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	responses := make([]EligibleShipmentResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, EligibleShipmentResponse{
			ShipmentID: row.ShipmentID, ShipmentNo: row.ShipmentNo,
			OrderID: row.OrderID, OrderNo: row.OrderNo, TransferReference: row.TransferReference,
			SourceWarehouseID: row.SourceWarehouseID, SourceWarehouseCode: row.SourceWarehouseCode,
			DestinationWarehouseID: row.DestinationWarehouseID,
			ShipmentStatus:         row.ShipmentStatus, ItemCount: row.ItemCount,
		})
	}
	return responses, nil
}

func (r *Repository) CreateItem(ctx context.Context, tenantID int64, item *Item) error {
	item.TenantID = tenantID
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *Repository) FindItemByID(ctx context.Context, tenantID, id int64) (*Item, error) {
	var item Item
	err := r.db.WithContext(ctx).First(&item, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// FindItemsByReceipt returns the raw Item rows (not the joined
// response shape) — used internally by Confirm to check whether every
// line is settled.
func (r *Repository) FindItemsByReceipt(ctx context.Context, tenantID, receiptID int64) ([]Item, error) {
	var items []Item
	err := r.db.WithContext(ctx).Where("transfer_receipt_id = ? AND tenant_id = ?", receiptID, tenantID).Find(&items).Error
	return items, err
}

type itemRow struct {
	ID                int64
	ShipmentItemID    int64
	ProductID         int64
	SKU               string
	ProductName       string
	LotNo             string
	UOM               string
	DispatchedQty     float64
	ReceivedQty       float64
	LocationID        *int64
	LocationCode      *string
	DiscrepancyQty    *float64
	DiscrepancyReason *string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ListItems resolves display fields (SKU/product name/location code)
// for one receipt's lines.
func (r *Repository) ListItems(ctx context.Context, tenantID, receiptID int64) ([]ItemResponse, error) {
	var rows []itemRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT tri.id AS id, tri.shipment_item_id AS shipment_item_id, tri.product_id AS product_id,
			p.sku AS sku, p.name AS product_name, tri.lot_no AS lot_no, tri.uom AS uom,
			tri.dispatched_qty AS dispatched_qty, tri.received_qty AS received_qty,
			tri.location_id AS location_id, l.code AS location_code,
			tri.discrepancy_qty AS discrepancy_qty, tri.discrepancy_reason AS discrepancy_reason,
			tri.status AS status, tri.created_at AS created_at, tri.updated_at AS updated_at
		FROM transfer_receipt_items tri
		JOIN products p ON p.id = tri.product_id
		LEFT JOIN locations l ON l.id = tri.location_id
		WHERE tri.transfer_receipt_id = ? AND tri.tenant_id = ?
		ORDER BY tri.id ASC
	`, receiptID, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]ItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemResponse{
			ID: row.ID, ShipmentItemID: row.ShipmentItemID, ProductID: row.ProductID,
			SKU: row.SKU, ProductName: row.ProductName, LotNo: row.LotNo, UOM: row.UOM,
			DispatchedQty: row.DispatchedQty, ReceivedQty: row.ReceivedQty,
			LocationID: row.LocationID, LocationCode: row.LocationCode,
			DiscrepancyQty: row.DiscrepancyQty, DiscrepancyReason: row.DiscrepancyReason,
			Status: ItemStatus(row.Status), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return items, nil
}

// AdjustReceivedQty atomically increments a line's received_qty by
// delta in one UPDATE statement, bounded in the WHERE clause so the
// increment is rejected outright (never clamped) if it would push
// received_qty past dispatched_qty — the exact same "one UPDATE,
// bounded in the WHERE clause" over-receipt guard
// packing_job_items.packed_qty already uses against picked_qty
// (packing.Repository.AdjustPackedQty). affected == 0 after the owning
// item was already confirmed to exist means this call's quantity would
// have over-received the line; nothing was written.
func (r *Repository) AdjustReceivedQty(ctx context.Context, tenantID, itemID int64, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE transfer_receipt_items
		SET received_qty = received_qty + ?, updated_at = SYSUTCDATETIME()
		WHERE id = ? AND tenant_id = ? AND received_qty + ? <= dispatched_qty
	`, delta, itemID, tenantID, delta)
	return result.RowsAffected, result.Error
}

// UpdateItemAfterReceive writes the fields ReceiveItem derives after
// calling AdjustReceivedQty — deliberately excludes received_qty
// itself so this write can never clobber a concurrent transaction's
// own atomic increment with a value computed from a stale read.
func (r *Repository) UpdateItemAfterReceive(ctx context.Context, tenantID int64, item *Item) error {
	result := r.db.WithContext(ctx).Model(&Item{}).Where("id = ? AND tenant_id = ?", item.ID, tenantID).Updates(map[string]any{
		"location_id":        item.LocationID,
		"discrepancy_qty":    item.DiscrepancyQty,
		"discrepancy_reason": item.DiscrepancyReason,
		"status":             item.Status,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrItemNotFound
	}
	return nil
}

// InsertConfirmation is the idempotency guard shared by ReceiveItem and
// Confirm — see migrations/0118's own doc. Must be called inside the
// same transaction as the action it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, tenantID int64, idempotencyKey string, receiptID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, TenantID: tenantID, TransferReceiptID: receiptID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// locationInfo is what ReceiveItem needs to validate a chosen staging
// location before using it — matches relocation.locationInfo exactly.
type locationInfo struct {
	WarehouseID int64
	ZoneType    string
	Status      string
}

// resolveLocation verifies locationID belongs to tenantID and returns
// the fields ReceiveItem needs to validate it — never distinguishes
// "found but belongs to another tenant" from "doesn't exist at all"
// (tenant-isolation-security.md §6), matching
// relocation.Repository.resolveLocation's own IDOR-closing shape.
func (r *Repository) resolveLocation(ctx context.Context, tenantID, locationID int64) (*locationInfo, error) {
	var info locationInfo
	err := r.db.WithContext(ctx).Raw(`
		SELECT w.id AS warehouse_id, z.type AS zone_type, l.status AS status
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

func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

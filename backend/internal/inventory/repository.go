package inventory

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/csvexport"
)

var ErrNotFound = errors.New("inventory: not found")

// row scans one joined+computed-status result. Field names/tags don't
// matter for GORM's Raw().Scan(&[]row{}) — it maps by the SQL aliases
// below, which match these Go field names case-insensitively.
type row struct {
	ID               int64
	TenantID         int64
	ProductID        int64
	SKU              string
	ProductName      string
	Barcode          *string
	WarehouseID      int64
	WarehouseCode    string
	ZoneID           int64
	ZoneCode         string
	LocationID       int64
	LocationCode     string
	LotNo            string
	ExpiryDate       *time.Time
	Quantity         float64
	ReservedQuantity float64
	HoldQuantity     float64
	UOM              string
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (r row) toResponse() Response {
	return Response{
		ID: r.ID, ProductID: r.ProductID, SKU: r.SKU, ProductName: r.ProductName, Barcode: r.Barcode,
		WarehouseID: r.WarehouseID, WarehouseCode: r.WarehouseCode,
		ZoneID: r.ZoneID, ZoneCode: r.ZoneCode,
		LocationID: r.LocationID, LocationCode: r.LocationCode,
		LotNo: r.LotNo, ExpiryDate: r.ExpiryDate,
		AvailableQty: r.Quantity - r.ReservedQuantity - r.HoldQuantity, ReservedQty: r.ReservedQuantity,
		HoldQty: r.HoldQuantity, OnHandQty: r.Quantity,
		UOM: r.UOM, Status: Status(r.Status), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// joinedSelect is shared by List and FindByID. status is computed here
// (not stored — see dto.go's Status doc) so both filtering and display
// use the exact same rule, applied before pagination so OFFSET/FETCH
// stays correct with a status filter active. inv.tenant_id is selected
// (Phase 30 Batch 1) so the outer WHERE below can filter by it without
// re-joining — List/FindByID are the Stock Inquiry screen's own read
// path and must never return another tenant's balance row.
const joinedSelect = `
	SELECT
		inv.id AS id, inv.tenant_id AS tenant_id, inv.product_id AS product_id, p.sku AS sku, p.name AS product_name, p.barcode AS barcode,
		w.id AS warehouse_id, w.code AS warehouse_code,
		z.id AS zone_id, z.code AS zone_code,
		l.id AS location_id, l.code AS location_code,
		inv.lot_no AS lot_no, inv.expiry_date AS expiry_date,
		inv.quantity AS quantity, inv.reserved_quantity AS reserved_quantity, inv.hold_quantity AS hold_quantity,
		p.base_uom AS uom, inv.created_at AS created_at, inv.updated_at AS updated_at,
		CASE
			WHEN inv.on_hold = 1 OR inv.hold_quantity > 0 THEN 'HOLD'
			WHEN inv.reserved_quantity > 0 THEN 'RESERVED'
			WHEN (inv.quantity - inv.reserved_quantity - inv.hold_quantity) <= p.min_stock THEN 'LOW_STOCK'
			ELSE 'AVAILABLE'
		END AS status
	FROM inventory inv
	JOIN products p ON p.id = inv.product_id
	JOIN locations l ON l.id = inv.location_id
	JOIN zones z ON z.id = l.zone_id
	JOIN warehouses w ON w.id = z.warehouse_id
`

// Repository is a thin wrapper — raw SQL for the read side (the
// join + computed status need it), a guarded conditional UPDATE for the
// write side (see AdjustBalance) so stock mutation stays a single
// atomic statement rather than read-then-write in Go. Business rules
// (insufficient-stock handling, ledger pairing) live in Service.
//
// Every method below takes an explicit tenantID and applies it to every
// query (Phase 30 Batch 1) — Inventory is the leaf dependency every
// inbound/outbound workflow's stock mutation funnels through
// (AdjustBalance/AdjustReservation), so a tenant-mismatched write here
// doesn't just leak data, it silently corrupts another tenant's on-hand
// or reserved quantity. See phase27-audit-report.md §2.4 and the
// Phase 30 planning note for the full rationale.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it a
// Repository bound to the transactional *gorm.DB so every call fn makes
// participates in the same COMMIT/ROLLBACK. Deliberately takes no
// tenantID — it opens a transaction and returns a bound Repository, it
// executes no business query itself — mirroring every other module's
// own Transaction wrapper (order/picking/receiving/putaway/...), all of
// which are accepted exceptions to tenant-scoping for the same reason.
func (r *Repository) Transaction(ctx context.Context, fn func(txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

func (r *Repository) List(ctx context.Context, tenantID int64, params ListParams) ([]Response, int64, error) {
	where := `WHERE tenant_id = ?
		AND (? = '' OR sku LIKE ? OR product_name LIKE ? OR barcode LIKE ? OR location_code LIKE ? OR lot_no LIKE ?)
		AND (? = 0 OR product_id = ?)
		AND (? = 0 OR warehouse_id = ?)
		AND (? = 0 OR zone_id = ?)
		AND (? = 0 OR location_id = ?)
		AND (? = '' OR status = ?)
		AND (? = '' OR lot_no = ?)`

	like := "%" + params.Search + "%"
	args := []any{
		tenantID,
		params.Search, like, like, like, like, like,
		params.ProductID, params.ProductID,
		params.WarehouseID, params.WarehouseID,
		params.ZoneID, params.ZoneID,
		params.LocationID, params.LocationID,
		string(params.Status), string(params.Status),
		params.LotNo, params.LotNo,
	}

	countSQL := "SELECT COUNT(*) FROM (" + joinedSelect + ") j " + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := "SELECT * FROM (" + joinedSelect + ") j " + where +
		" ORDER BY sku ASC, location_code ASC OFFSET ? ROWS FETCH NEXT ? ROWS ONLY"
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []row
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(rows))
	for _, rr := range rows {
		responses = append(responses, rr.toResponse())
	}
	return responses, total, nil
}

func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Response, error) {
	sql := "SELECT * FROM (" + joinedSelect + ") j WHERE id = ? AND tenant_id = ?"
	var rows []row
	if err := r.db.WithContext(ctx).Raw(sql, id, tenantID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	resp := rows[0].toResponse()
	return &resp, nil
}

// summaryRow scans the aggregate query. GORM's Raw().Scan() maps SQL
// column aliases to Go struct fields via its naming strategy when no
// explicit gorm column tag is present, and it does not reliably guess
// "total_skus" from a field named TotalSKUs (the SKUs acronym+plural
// confuses the snake_case conversion) — explicit tags avoid that
// silently-wrong mapping rather than relying on the guess.
type summaryRow struct {
	TotalSKUs     int64   `gorm:"column:total_skus"`
	AvailableQty  float64 `gorm:"column:available_qty"`
	ReservedQty   float64 `gorm:"column:reserved_qty"`
	LowStockCount int64   `gorm:"column:low_stock_count"`
}

func (r *Repository) Summary(ctx context.Context, tenantID int64) (SummaryResponse, error) {
	var s summaryRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(DISTINCT inv.product_id) AS total_skus,
			COALESCE(SUM(inv.quantity - inv.reserved_quantity - inv.hold_quantity), 0) AS available_qty,
			COALESCE(SUM(inv.reserved_quantity), 0) AS reserved_qty,
			(SELECT COUNT(*) FROM (
				SELECT inv2.product_id
				FROM inventory inv2
				JOIN products p2 ON p2.id = inv2.product_id
				WHERE inv2.tenant_id = ?
				GROUP BY inv2.product_id, p2.min_stock
				HAVING SUM(inv2.quantity - inv2.reserved_quantity - inv2.hold_quantity) <= p2.min_stock
			) low) AS low_stock_count
		FROM inventory inv
		WHERE inv.tenant_id = ?
	`, tenantID, tenantID).Scan(&s).Error
	if err != nil {
		return SummaryResponse{}, err
	}
	return SummaryResponse{
		TotalSKUs: s.TotalSKUs, AvailableQty: s.AvailableQty,
		ReservedQty: s.ReservedQty, LowStockCount: s.LowStockCount,
	}, nil
}

func (r *Repository) Transactions(ctx context.Context, tenantID, inventoryID int64, limit int) ([]TransactionResponse, error) {
	inv, err := r.findRaw(ctx, tenantID, inventoryID)
	if err != nil {
		return nil, err
	}

	var txns []Transaction
	err = r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?", tenantID, inv.ProductID, inv.LocationID, inv.LotNo).
		Order("created_at DESC").Limit(limit).Find(&txns).Error
	if err != nil {
		return nil, err
	}

	responses := make([]TransactionResponse, 0, len(txns))
	for _, t := range txns {
		responses = append(responses, TransactionResponse{
			ID: t.ID, TransactionType: t.TransactionType, Quantity: t.Quantity,
			ReferenceType: t.ReferenceType, ReferenceID: t.ReferenceID, Reason: t.Reason,
			CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt,
		})
	}
	return responses, nil
}

// ListTransactionsForExport is Phase 22's global (not per-inventory-row)
// ledger query backing the async Inventory Transaction History export —
// see TransactionExportRow's own doc for why this report alone uses the
// async pipeline instead of a synchronous CSV handler. Capped at
// csvexport.MaxExportRows regardless of how many rows actually match:
// even the async job pipeline has a defensive ceiling, it just also
// exists to make crossing the *synchronous* export's own smaller
// tolerance for query time a non-issue. tenantID (Phase 30 Batch 1) is
// the export_jobs row's own tenant_id, supplied by internal/report —
// without it this query would dump every tenant's ledger into one CSV.
func (r *Repository) ListTransactionsForExport(ctx context.Context, tenantID int64, filters TransactionExportFilters) ([]TransactionExportRow, error) {
	where := `WHERE it.tenant_id = ?
		AND (? = 0 OR it.product_id = ?)
		AND (? = 0 OR w.id = ?)
		AND (? = '' OR it.transaction_type = ?)
		AND (? IS NULL OR it.created_at >= ?)
		AND (? IS NULL OR it.created_at <= ?)`

	args := []any{
		tenantID,
		filters.ProductID, filters.ProductID,
		filters.WarehouseID, filters.WarehouseID,
		string(filters.TransactionType), string(filters.TransactionType),
		filters.DateFrom, filters.DateFrom,
		filters.DateTo, filters.DateTo,
	}

	sql := `
		SELECT it.id AS id, it.transaction_type AS transaction_type,
			p.sku AS sku, p.name AS product_name, w.code AS warehouse_code, l.code AS location_code,
			it.lot_no AS lot_no, it.quantity AS quantity,
			it.reference_type AS reference_type, it.reference_id AS reference_id, it.reason AS reason,
			u.full_name AS created_by_name, it.created_at AS created_at
		FROM inventory_transactions it
		JOIN products p ON p.id = it.product_id
		JOIN locations l ON l.id = it.location_id
		JOIN zones z ON z.id = l.zone_id
		JOIN warehouses w ON w.id = z.warehouse_id
		LEFT JOIN users u ON u.id = it.created_by
	` + where + `
		ORDER BY it.id DESC
		OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY`

	listArgs := append(append([]any{}, args...), csvexport.MaxExportRows)

	var rows []TransactionExportRow
	if err := r.db.WithContext(ctx).Raw(sql, listArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListTransactionsByReference returns every ledger row tagged with
// referenceType/referenceID (e.g. "PUTAWAY"/task.PutawayNo) — added in
// Phase 31D so internal/inbounddoc's Completion Report can discover
// every distinct destination a putaway task's line actually moved
// stock to over its lifetime. This is necessary, not cosmetic:
// putaway_task_items.destination_location_id (Phase 31C's own
// documented limitation) holds only the MOST RECENT destination for a
// line confirmed across multiple destinations, silently losing earlier
// ones — the ledger is the one place that history survives, since
// MoveAvailableStockTx always writes one immutable row per leg.
func (r *Repository) ListTransactionsByReference(ctx context.Context, tenantID int64, referenceType, referenceID string) ([]Transaction, error) {
	var rows []Transaction
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, referenceType, referenceID).
		Order("id ASC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) findRaw(ctx context.Context, tenantID, id int64) (*Inventory, error) {
	var inv Inventory
	err := r.db.WithContext(ctx).First(&inv, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// FindBalance returns the current balance row for (product, location,
// lot) within tenantID, or ErrNotFound if none exists yet (or exists
// under a different tenant — the two cases are indistinguishable by
// design, see tenant-isolation-security.md §6).
func (r *Repository) FindBalance(ctx context.Context, tenantID, productID, locationID int64, lotNo string) (*Inventory, error) {
	var inv Inventory
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?", tenantID, productID, locationID, lotNo).
		First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// AdjustBalance atomically applies delta to an existing balance row in
// one UPDATE statement — the arithmetic and the non-negative check both
// happen inside the WHERE clause, in the database, not as a Go
// read-then-write. Returns rows affected: 0 means either the row
// doesn't exist (in this tenant), or applying delta would take quantity
// negative; Service disambiguates by calling FindBalance separately
// when this returns 0. This is the concurrency-safety mechanism — see
// PROJECT_STATUS.md. tenant_id is in the WHERE clause itself (Phase 30
// Batch 1), not just checked beforehand in Go, because this is an
// atomic, hand-written UPDATE: a tenant-mismatched write here would
// silently corrupt another tenant's on-hand quantity, not just leak it.
func (r *Repository) AdjustBalance(ctx context.Context, tenantID, productID, locationID int64, lotNo string, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE inventory
		SET quantity = quantity + ?, updated_at = SYSUTCDATETIME()
		WHERE tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ? AND quantity + ? >= 0
	`, delta, tenantID, productID, locationID, lotNo, delta)
	return result.RowsAffected, result.Error
}

// AdjustReservation atomically applies delta to reserved_quantity in one
// UPDATE statement — the arithmetic and both safety checks (a
// reservation can never exceed on-hand quantity net of anything already
// on hold; a release/consume can never take reserved_quantity negative)
// live in the WHERE clause, not as a Go read-then-write, mirroring
// AdjustBalance's mechanism exactly, including the same tenant_id-in-WHERE
// reasoning (Phase 30 Batch 1). A positive delta reserves; a negative
// delta releases or consumes. Returns rows affected: 0 means either the
// row doesn't exist (in this tenant), or applying delta would push
// reserved_quantity+hold_quantity outside [0, quantity]. This statement
// never touches quantity or hold_quantity themselves — reservation is a
// claim against an existing balance, never a movement of it. This is
// Phase 12's concurrency-safety mechanism for Picking reservation — see
// PROJECT_STATUS.md.
//
// Phase 31A: the upper-bound guard changed from
// "reserved_quantity + delta <= quantity" to
// "reserved_quantity + hold_quantity + delta <= quantity" — the exact
// enforcement point for the approved stock lifecycle's safety
// requirement that QC-held stock can never be reserved (and therefore
// never picked): Available = Quantity - ReservedQuantity - HoldQuantity
// can never go negative, because no reservation can ever push Reserved
// past Quantity-Hold. Every existing caller is unaffected for any row
// with hold_quantity = 0 (every row that existed before migrations/0067,
// and every row Receiving posts today for a non-QC-required product).
func (r *Repository) AdjustReservation(ctx context.Context, tenantID, productID, locationID int64, lotNo string, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE inventory
		SET reserved_quantity = reserved_quantity + ?, updated_at = SYSUTCDATETIME()
		WHERE tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?
			AND reserved_quantity + ? >= 0
			AND reserved_quantity + hold_quantity + ? <= quantity
	`, delta, tenantID, productID, locationID, lotNo, delta, delta)
	return result.RowsAffected, result.Error
}

// AdjustAvailableBalance atomically applies a negative delta to
// quantity, guarded so the removal can never dip into
// reserved_quantity or hold_quantity — i.e. it only ever removes from
// the currently AVAILABLE portion (Quantity - Reserved - Hold). Added
// in Phase 31C for Putaway, which relocates existing stock between
// locations and must never move reserved or QC-held units out of their
// current location. Deliberately narrower than AdjustBalance (used by
// every other caller — Stock Adjustment/Return/Count/Receiving/Picking
// — which either add stock, or reconcile to physical reality/an
// already-released reservation, and must not gain this extra
// restriction). See MoveAvailableStockTx's own doc.
func (r *Repository) AdjustAvailableBalance(ctx context.Context, tenantID, productID, locationID int64, lotNo string, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE inventory
		SET quantity = quantity + ?, updated_at = SYSUTCDATETIME()
		WHERE tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?
			AND quantity + ? >= reserved_quantity + hold_quantity
	`, delta, tenantID, productID, locationID, lotNo, delta)
	return result.RowsAffected, result.Error
}

// AdjustHold atomically applies delta to hold_quantity in one UPDATE
// statement, mirroring AdjustReservation's exact mechanism and the same
// reserved_quantity+hold_quantity<=quantity invariant (so posting a hold
// can never itself push the balance past 100% committed). A positive
// delta is Receiving's Confirm posting a QC-required line into hold
// (see internal/receiving's Confirm doc); a negative delta is a future
// QC release/reject (Phase 31B — no caller exists yet in this phase).
// Returns rows affected: 0 means either the row doesn't exist (in this
// tenant), or applying delta would push hold_quantity outside
// [0, quantity - reserved_quantity]. Never touches quantity or
// reserved_quantity themselves.
func (r *Repository) AdjustHold(ctx context.Context, tenantID, productID, locationID int64, lotNo string, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE inventory
		SET hold_quantity = hold_quantity + ?, updated_at = SYSUTCDATETIME()
		WHERE tenant_id = ? AND product_id = ? AND location_id = ? AND lot_no = ?
			AND hold_quantity + ? >= 0
			AND reserved_quantity + hold_quantity + ? <= quantity
	`, delta, tenantID, productID, locationID, lotNo, delta, delta)
	return result.RowsAffected, result.Error
}

// CreateBalance inserts the first balance row for a (product, location,
// lot) tuple — only ever called after AdjustBalance affected 0 rows
// because the row didn't exist (an AddStock of a brand new location).
// Stamps tenantID onto the row itself (Phase 30 Batch 1) rather than
// trusting the caller to have already set inv.TenantID.
func (r *Repository) CreateBalance(ctx context.Context, tenantID int64, inv *Inventory) error {
	inv.TenantID = tenantID
	return r.db.WithContext(ctx).Create(inv).Error
}

// InsertTransaction appends one immutable ledger row. Stamps tenantID
// onto the row itself (Phase 30 Batch 1), same reasoning as CreateBalance.
func (r *Repository) InsertTransaction(ctx context.Context, tenantID int64, t *Transaction) error {
	t.TenantID = tenantID
	return r.db.WithContext(ctx).Create(t).Error
}

// CountByLocation reports how many inventory rows, within tenantID,
// currently hold a positive quantity at locationID — used by
// internal/location.Service.Delete's referential-safety check before
// deleting a location. This was the first tenant-scoped method on this
// Repository, added ahead of the rest of Phase 30 Batch 1 specifically
// to answer a question posed by a module that was already correctly
// tenant-scoped (Location Master Management) without leaking a count
// across tenants back to it. Zero-quantity residual rows (a balance
// drained to exactly 0 but not yet cleaned up) don't count as "stock
// still exists."
func (r *Repository) CountByLocation(ctx context.Context, tenantID, locationID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Inventory{}).
		Where("tenant_id = ? AND location_id = ? AND quantity > 0", tenantID, locationID).
		Count(&count).Error
	return count, err
}

// availabilityRow is AvailabilityByProductWarehouse's scan target.
type availabilityRow struct {
	OnHandQty    float64
	ReservedQty  float64
	HoldQty      float64
	AvailableQty float64
}

// AvailabilityByProductWarehouse sums every (location, lot) balance row
// for productID across every location belonging to warehouseID — added
// in Phase 33A so internal/order's new Review & Release step can show a
// pure, read-only "how much of this SKU can this warehouse actually
// fulfill right now" figure without reserving or mutating anything.
// Deliberately warehouse-wide, not per-location/lot: an order line is
// location-agnostic (see order.Item's own doc) until CreatePickingTask
// picks a specific location/lot, so the figure that matters here is the
// warehouse's own total available balance, the same aggregation
// Summary's own SUM already uses, just scoped to one product and one
// warehouse instead of the whole tenant. Returns all-zero (never
// ErrNotFound) when no balance row exists at all for this
// (tenant, product, warehouse) — "zero stock" is a valid, common
// answer, not an error.
func (r *Repository) AvailabilityByProductWarehouse(ctx context.Context, tenantID, productID, warehouseID int64) (float64, float64, float64, float64, error) {
	var row availabilityRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			COALESCE(SUM(inv.quantity), 0) AS on_hand_qty,
			COALESCE(SUM(inv.reserved_quantity), 0) AS reserved_qty,
			COALESCE(SUM(inv.hold_quantity), 0) AS hold_qty,
			COALESCE(SUM(inv.quantity - inv.reserved_quantity - inv.hold_quantity), 0) AS available_qty
		FROM inventory inv
		JOIN locations l ON l.id = inv.location_id
		JOIN zones z ON z.id = l.zone_id
		WHERE inv.tenant_id = ? AND inv.product_id = ? AND z.warehouse_id = ?
	`, tenantID, productID, warehouseID).Scan(&row).Error
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return row.AvailableQty, row.ReservedQty, row.HoldQty, row.OnHandQty, nil
}

// lotAvailabilityRow is ListAvailableLotsByProductWarehouse's own scan
// target — see that method's doc.
type lotAvailabilityRow struct {
	LocationID   int64
	LocationCode string
	LotNo        string
	ExpiryDate   *time.Time
	Quantity     float64
	AvailableQty float64
	CreatedAt    time.Time
}

// ListAvailableLotsByProductWarehouse is Phase 33B's read-only FEFO/FIFO
// primitive — grepped first (per the phase plan's own instruction):
// nothing else in this codebase (including internal/putaway and
// internal/relocation) exposes a query ordering a product's lots by
// expiry/receipt date; order.ItemResponse.FEFOPreferred (migrations/
// 0080) is only ever a caller-set boolean flag on an order line, never
// a query result. This fills that gap with the narrowest possible
// addition: one SELECT, no reservation/consumption logic duplicated
// from AdjustReservation/AdjustBalance, ordered First-Expired-First-Out
// (ExpiryDate ascending, NULLs last via the CASE below) and tie-broken
// First-In-First-Out (CreatedAt ascending) — the standard combined
// FEFO/FIFO picking sequence. Only rows with a positive available
// balance (quantity - reserved - hold > 0) are returned; a lot that is
// fully reserved/held/exhausted is not a usable suggestion. Never
// mutates anything — internal/picking's own ReserveStockTx call at
// task-creation (or Repick's own re-reservation) is still the only
// thing that ever actually claims a lot this query surfaces.
func (r *Repository) ListAvailableLotsByProductWarehouse(ctx context.Context, tenantID, productID, warehouseID int64) ([]LotAvailability, error) {
	var rows []lotAvailabilityRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT inv.location_id AS location_id, l.code AS location_code, inv.lot_no AS lot_no,
			inv.expiry_date AS expiry_date, inv.quantity AS quantity,
			(inv.quantity - inv.reserved_quantity - inv.hold_quantity) AS available_qty,
			inv.created_at AS created_at
		FROM inventory inv
		JOIN locations l ON l.id = inv.location_id
		JOIN zones z ON z.id = l.zone_id
		WHERE inv.tenant_id = ? AND inv.product_id = ? AND z.warehouse_id = ?
			AND (inv.quantity - inv.reserved_quantity - inv.hold_quantity) > 0
		ORDER BY CASE WHEN inv.expiry_date IS NULL THEN 1 ELSE 0 END, inv.expiry_date ASC, inv.created_at ASC
	`, tenantID, productID, warehouseID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]LotAvailability, 0, len(rows))
	for _, row := range rows {
		out = append(out, LotAvailability{
			LocationID: row.LocationID, LocationCode: row.LocationCode, LotNo: row.LotNo,
			ExpiryDate: row.ExpiryDate, AvailableQty: row.AvailableQty, OnHandQty: row.Quantity, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

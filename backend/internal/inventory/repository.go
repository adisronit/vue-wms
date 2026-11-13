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
		AvailableQty: r.Quantity - r.ReservedQuantity, ReservedQty: r.ReservedQuantity, OnHandQty: r.Quantity,
		UOM: r.UOM, Status: Status(r.Status), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// joinedSelect is shared by List and FindByID. status is computed here
// (not stored — see dto.go's Status doc) so both filtering and display
// use the exact same rule, applied before pagination so OFFSET/FETCH
// stays correct with a status filter active.
const joinedSelect = `
	SELECT
		inv.id AS id, inv.product_id AS product_id, p.sku AS sku, p.name AS product_name, p.barcode AS barcode,
		w.id AS warehouse_id, w.code AS warehouse_code,
		z.id AS zone_id, z.code AS zone_code,
		l.id AS location_id, l.code AS location_code,
		inv.lot_no AS lot_no, inv.expiry_date AS expiry_date,
		inv.quantity AS quantity, inv.reserved_quantity AS reserved_quantity,
		p.base_uom AS uom, inv.created_at AS created_at, inv.updated_at AS updated_at,
		CASE
			WHEN inv.on_hold = 1 THEN 'HOLD'
			WHEN inv.reserved_quantity > 0 THEN 'RESERVED'
			WHEN (inv.quantity - inv.reserved_quantity) <= p.min_stock THEN 'LOW_STOCK'
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
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it a
// Repository bound to the transactional *gorm.DB so every call fn makes
// participates in the same COMMIT/ROLLBACK.
func (r *Repository) Transaction(ctx context.Context, fn func(txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	where := `WHERE (? = '' OR sku LIKE ? OR product_name LIKE ? OR barcode LIKE ? OR location_code LIKE ? OR lot_no LIKE ?)
		AND (? = 0 OR product_id = ?)
		AND (? = 0 OR warehouse_id = ?)
		AND (? = 0 OR zone_id = ?)
		AND (? = 0 OR location_id = ?)
		AND (? = '' OR status = ?)
		AND (? = '' OR lot_no = ?)`

	like := "%" + params.Search + "%"
	args := []any{
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

func (r *Repository) FindByID(ctx context.Context, id int64) (*Response, error) {
	sql := "SELECT * FROM (" + joinedSelect + ") j WHERE id = ?"
	var rows []row
	if err := r.db.WithContext(ctx).Raw(sql, id).Scan(&rows).Error; err != nil {
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

func (r *Repository) Summary(ctx context.Context) (SummaryResponse, error) {
	var s summaryRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(DISTINCT inv.product_id) AS total_skus,
			COALESCE(SUM(inv.quantity - inv.reserved_quantity), 0) AS available_qty,
			COALESCE(SUM(inv.reserved_quantity), 0) AS reserved_qty,
			(SELECT COUNT(*) FROM (
				SELECT inv2.product_id
				FROM inventory inv2
				JOIN products p2 ON p2.id = inv2.product_id
				GROUP BY inv2.product_id, p2.min_stock
				HAVING SUM(inv2.quantity - inv2.reserved_quantity) <= p2.min_stock
			) low) AS low_stock_count
		FROM inventory inv
	`).Scan(&s).Error
	if err != nil {
		return SummaryResponse{}, err
	}
	return SummaryResponse{
		TotalSKUs: s.TotalSKUs, AvailableQty: s.AvailableQty,
		ReservedQty: s.ReservedQty, LowStockCount: s.LowStockCount,
	}, nil
}

func (r *Repository) Transactions(ctx context.Context, inventoryID int64, limit int) ([]TransactionResponse, error) {
	inv, err := r.findRaw(ctx, inventoryID)
	if err != nil {
		return nil, err
	}

	var txns []Transaction
	err = r.db.WithContext(ctx).
		Where("product_id = ? AND location_id = ? AND lot_no = ?", inv.ProductID, inv.LocationID, inv.LotNo).
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
// tolerance for query time a non-issue.
func (r *Repository) ListTransactionsForExport(ctx context.Context, filters TransactionExportFilters) ([]TransactionExportRow, error) {
	where := `WHERE (? = 0 OR it.product_id = ?)
		AND (? = 0 OR w.id = ?)
		AND (? = '' OR it.transaction_type = ?)
		AND (? IS NULL OR it.created_at >= ?)
		AND (? IS NULL OR it.created_at <= ?)`

	args := []any{
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

func (r *Repository) findRaw(ctx context.Context, id int64) (*Inventory, error) {
	var inv Inventory
	err := r.db.WithContext(ctx).First(&inv, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// FindBalance returns the current balance row for (product, location,
// lot), or ErrNotFound if none exists yet.
func (r *Repository) FindBalance(ctx context.Context, productID, locationID int64, lotNo string) (*Inventory, error) {
	var inv Inventory
	err := r.db.WithContext(ctx).
		Where("product_id = ? AND location_id = ? AND lot_no = ?", productID, locationID, lotNo).
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
// doesn't exist yet, or applying delta would take quantity negative;
// Service disambiguates by calling FindBalance separately when this
// returns 0. This is the concurrency-safety mechanism — see
// PROJECT_STATUS.md.
func (r *Repository) AdjustBalance(ctx context.Context, productID, locationID int64, lotNo string, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE inventory
		SET quantity = quantity + ?, updated_at = SYSUTCDATETIME()
		WHERE product_id = ? AND location_id = ? AND lot_no = ? AND quantity + ? >= 0
	`, delta, productID, locationID, lotNo, delta)
	return result.RowsAffected, result.Error
}

// AdjustReservation atomically applies delta to reserved_quantity in one
// UPDATE statement — the arithmetic and both safety checks (a
// reservation can never exceed on-hand quantity; a release/consume can
// never take reserved_quantity negative) live in the WHERE clause, not
// as a Go read-then-write, mirroring AdjustBalance's mechanism exactly.
// A positive delta reserves; a negative delta releases or consumes.
// Returns rows affected: 0 means either the row doesn't exist, or
// applying delta would push reserved_quantity outside [0, quantity].
// This statement never touches quantity itself — reservation is a claim
// against an existing balance, never a movement of it. This is Phase
// 12's concurrency-safety mechanism for Picking reservation — see
// PROJECT_STATUS.md.
func (r *Repository) AdjustReservation(ctx context.Context, productID, locationID int64, lotNo string, delta float64) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE inventory
		SET reserved_quantity = reserved_quantity + ?, updated_at = SYSUTCDATETIME()
		WHERE product_id = ? AND location_id = ? AND lot_no = ?
			AND reserved_quantity + ? >= 0
			AND reserved_quantity + ? <= quantity
	`, delta, productID, locationID, lotNo, delta, delta)
	return result.RowsAffected, result.Error
}

// CreateBalance inserts the first balance row for a (product, location,
// lot) tuple — only ever called after AdjustBalance affected 0 rows
// because the row didn't exist (an AddStock of a brand new location).
func (r *Repository) CreateBalance(ctx context.Context, inv *Inventory) error {
	return r.db.WithContext(ctx).Create(inv).Error
}

// InsertTransaction appends one immutable ledger row.
func (r *Repository) InsertTransaction(ctx context.Context, t *Transaction) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// CountByLocation reports how many inventory rows, within tenantID,
// currently hold a positive quantity at locationID — used by
// internal/location.Service.Delete's referential-safety check before
// deleting a location. Unlike every other method on this Repository
// (which, consistent with this module's pre-existing, documented lack
// of tenant enforcement, ignore tenant_id entirely), this one new
// method filters by it: it exists specifically to answer a question
// posed by a module that IS correctly tenant-scoped (Location Master
// Management), and must not leak a count across tenants back to it.
// Zero-quantity residual rows (a balance drained to exactly 0 but not
// yet cleaned up) don't count as "stock still exists."
func (r *Repository) CountByLocation(ctx context.Context, tenantID, locationID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Inventory{}).
		Where("tenant_id = ? AND location_id = ? AND quantity > 0", tenantID, locationID).
		Count(&count).Error
	return count, err
}

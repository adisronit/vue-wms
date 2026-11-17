// Package inventoryreport implements Phase 34E's Inventory Balance
// Report. It reads inventory/inventory_transactions/locations/zones/
// warehouses/products directly via raw SQL rather than taking a Go
// dependency on internal/inventory's write-side package, the same
// shape every other Phase 34 report package already established.
//
// Available = OnHand - Reserved - Hold (the spec's own invariant) is
// always computed from the same three live columns, never stored
// separately — mirrors inventory.Inventory.Available() exactly.
//
// Low Stock has no backing threshold anywhere in this schema (no
// reorder_point/min_qty column on products or any other table,
// confirmed by direct inspection) — it is a request-time filter
// (BelowQty) the caller supplies, never a fabricated stored rule.
//
// Historical As-of Balance (ListAsOf/SummaryAsOf) reconstructs a
// balance by summing inventory_transactions.quantity (signed) up to a
// cutoff timestamp. This is reliable PROVIDED every balance's full
// history was captured in the ledger from its very first unit — this
// package cannot verify there is no balance anywhere that was seeded
// directly without a corresponding ledger entry (that would require
// auditing every migration/seed script, out of scope here). Every
// As-of response therefore carries an explicit Caveat string — this
// is a documented assumption, not a verified guarantee, per CLAUDE.md's
// "never generate fabricated stock history" rule: the uncertainty is
// surfaced, never hidden.
//
// Schema gap: no LPN/pallet column exists on the inventory table —
// Row.LPN is always nil, the frontend renders "N/A", never fabricated.
package inventoryreport

import "time"

// Row is one inventory balance — grain is one row per (product,
// location, lot), identical to the inventory table's own grain (no
// further exploding needed, unlike Receiving/Picking's split/
// allocation grains).
type Row struct {
	InventoryID      int64      `json:"inventory_id"`
	WarehouseCode    string     `json:"warehouse_code"`
	WarehouseName    string     `json:"warehouse_name"`
	ZoneCode         string     `json:"zone_code"`
	ZoneName         string     `json:"zone_name"`
	LocationCode     string     `json:"location_code"`
	LocationName     string     `json:"location_name"`
	StorageType      string     `json:"storage_type"` // locations.type: RECEIVING, STORAGE, PICKING, PACKING, SHIPPING, QC, HOLD
	SKU              string     `json:"sku"`
	ProductName      string     `json:"product_name"`
	BrandName        *string    `json:"brand_name"`
	CategoryName     *string    `json:"category_name"`
	LotNo            string     `json:"lot_no"`
	ExpiryDate       *time.Time `json:"expiry_date"`
	LPN              *string    `json:"lpn"` // always nil — no LPN column on inventory; see package doc
	UOM              string     `json:"uom"` // from products.base_uom — inventory itself carries no per-row UOM
	OnHand           float64    `json:"on_hand"`
	Reserved         float64    `json:"reserved"`
	Hold             float64    `json:"hold"`
	Available        float64    `json:"available"`          // OnHand - Reserved - Hold, always computed
	LastMovementDate *time.Time `json:"last_movement_date"` // MAX(inventory_transactions.created_at); nil if never moved
	// InventoryStatus is a display classification derived from the
	// same three live quantities every row already has — ON_HOLD when
	// Hold > 0, RESERVED when Reserved > 0 (and not on hold), ZERO when
	// OnHand <= 0, AVAILABLE otherwise. Never a stored column.
	InventoryStatus string `json:"inventory_status"`
}

// Filters is every server-side filter the Inventory Report's list/
// summary/export endpoints share, including the lens-selecting
// filters (BelowQty for Low Stock, ExpiringWithinDays/ExpiredOnly for
// Near-Expiry/Expired, InactiveDays for Slow/Non-Moving) — these are
// filters over the same Row data, not separate report endpoints.
type Filters struct {
	WarehouseID *int64
	ZoneID      *int64
	LocationID  *int64
	StorageType string
	Search      string // matches SKU, product name, or lot_no
	LotNo       string
	// BelowQty: only rows whose Available is strictly less than this
	// value — the Low Stock lens. No stored threshold exists; the
	// caller always supplies this.
	BelowQty *float64
	// ExpiringWithinDays: only rows whose expiry_date falls within the
	// next N days (and is not null) — the Near Expiry lens.
	ExpiringWithinDays *int
	// ExpiredOnly: only rows whose expiry_date is already in the past.
	ExpiredOnly bool
	// InactiveDays: only rows with no inventory_transactions row at all,
	// or whose last movement is older than N days — the Slow/Non-Moving
	// lens.
	InactiveDays *int
}

type ListParams struct {
	Filters
	Page    int
	Limit   int
	SortBy  string // on_hand (default), available, expiry_date, last_movement_date, warehouse_name, sku
	SortDir string
}

// UOMQty is one UOM's total — kept separate per UOM rather than
// summed together (CLAUDE.md's "do not mix incompatible UOMs").
type UOMQty struct {
	UOM string  `json:"uom"`
	Qty float64 `json:"qty"`
}

// NamedCount is one grouping bucket (by warehouse or zone).
type NamedCount struct {
	Name string  `json:"name"`
	Qty  float64 `json:"qty"`
}

type Charts struct {
	ByWarehouse []NamedCount `json:"by_warehouse"`
	ByZone      []NamedCount `json:"by_zone"`
}

// Summary is the Inventory Report's KPI panel. LowStockCount/
// NearExpiryCount/ExpiredCount/NonMovingCount are computed from the
// SAME lens filters as the list endpoint (BelowQty/ExpiringWithinDays/
// ExpiredOnly/InactiveDays) — if the caller does not supply the
// relevant filter, that count is nil (rendered "N/A"), never a
// fabricated 0, since there is no default threshold to apply.
type Summary struct {
	TotalOnHandByUOM    []UOMQty `json:"total_on_hand_by_uom"`
	TotalAvailableByUOM []UOMQty `json:"total_available_by_uom"`
	LowStockCount       *int64   `json:"low_stock_count"`
	NearExpiryCount     *int64   `json:"near_expiry_count"`
	ExpiredCount        *int64   `json:"expired_count"`
	NonMovingCount      *int64   `json:"non_moving_count"`
	Charts              Charts   `json:"charts"`
}

// MovementRow is one inventory_transactions row — a SEPARATE grain
// from Row (see package doc), for the Stock Movement report
// specifically.
type MovementRow struct {
	TransactionID   int64     `json:"transaction_id"`
	CreatedAt       time.Time `json:"created_at"`
	TransactionType string    `json:"transaction_type"`
	WarehouseCode   string    `json:"warehouse_code"`
	WarehouseName   string    `json:"warehouse_name"`
	LocationCode    string    `json:"location_code"`
	SKU             string    `json:"sku"`
	ProductName     string    `json:"product_name"`
	LotNo           string    `json:"lot_no"`
	Quantity        float64   `json:"quantity"` // signed: negative = debit, positive = credit
	ReferenceType   *string   `json:"reference_type"`
	ReferenceID     *string   `json:"reference_id"`
	Operator        *string   `json:"operator"`
}

type MovementFilters struct {
	From            *time.Time
	To              *time.Time
	WarehouseID     *int64
	LocationID      *int64
	TransactionType string
	Search          string // matches SKU, product name, or lot_no
}

type MovementListParams struct {
	MovementFilters
	Page  int
	Limit int
}

// AsOfRow is one reconstructed historical balance — see package doc's
// caveat on reliability.
type AsOfRow struct {
	WarehouseCode string  `json:"warehouse_code"`
	WarehouseName string  `json:"warehouse_name"`
	LocationCode  string  `json:"location_code"`
	SKU           string  `json:"sku"`
	ProductName   string  `json:"product_name"`
	LotNo         string  `json:"lot_no"`
	UOM           string  `json:"uom"`
	AsOfQuantity  float64 `json:"as_of_quantity"`
}

type AsOfFilters struct {
	AsOf        time.Time
	WarehouseID *int64
	LocationID  *int64
	Search      string
}

type AsOfListParams struct {
	AsOfFilters
	Page  int
	Limit int
}

// AsOfCaveat is returned alongside every As-of response — see package
// doc for the full explanation.
const AsOfCaveat = "Reconstructed from inventory_transactions up to the given timestamp. Reliable only if every balance's full history was captured in the ledger from its first unit; this has not been independently verified against every migration/seed script. Treat as a documented assumption, not a verified guarantee."

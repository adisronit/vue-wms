// Package stockcountreport implements Phase 34C's Stock Count Report.
// It reads stock_counts/stock_count_items/stock_adjustments/
// stock_adjustment_items directly via raw SQL rather than taking a Go
// dependency on internal/stockcount/internal/stockadjustment, the same
// shape internal/receivingreport (Phase 34B) and internal/qualitycheck
// already established, for the same reason: no import-cycle risk, and
// a report has no business rules to share with those packages' own
// write paths. Every query is read-only — see Rules below.
//
// Schema gaps (confirmed by direct model inspection, not guessed):
// stock_count_items has no LPN/pallet column, and neither
// stock_counts nor stock_adjustments has a "reviewed_by" column
// anywhere. Row.LPN and Row.ReviewedBy are always nil — they exist in
// the JSON shape because the Phase 34 spec lists them as report
// fields, but this package never fabricates a value for either; the
// frontend renders them as "N/A".
//
// "Approved By" only has real data for the subset of variance that
// was routed through stockcount's CreateAdjustments bridge into
// internal/stockadjustment's own separate approval workflow — a
// directly-Confirmed count (no adjustment generated) has no approver
// at all, matching StatusConfirmed's own "applies variance directly,
// no approval step" design.
package stockcountreport

import "time"

// Row is one line of the Stock Count Report — grain is one row per
// stock_count_item (no further sub-split exists in this schema, unlike
// Receiving's lot/pallet splits).
type Row struct {
	CountID          int64      `json:"count_id"`
	CountItemID      int64      `json:"count_item_id"`
	CountNo          string     `json:"count_no"`
	CountType        string     `json:"count_type"`
	CountDateTime    *time.Time `json:"count_date_time"`
	WarehouseCode    string     `json:"warehouse_code"`
	WarehouseName    string     `json:"warehouse_name"`
	ZoneCode         *string    `json:"zone_code"`
	ZoneName         *string    `json:"zone_name"`
	LocationCode     string     `json:"location_code"`
	LocationName     string     `json:"location_name"`
	SKU              string     `json:"sku"`
	ProductName      string     `json:"product_name"`
	BrandName        *string    `json:"brand_name"`
	CategoryName     *string    `json:"category_name"`
	LotNo            string     `json:"lot_no"`
	LPN              *string    `json:"lpn"` // always nil — no LPN column exists on stock_count_items; see package doc
	UOM              string     `json:"uom"`
	SystemQty        float64    `json:"system_qty"` // historical frozen snapshot, never the live balance
	CountedQty       *float64   `json:"counted_qty"`
	VarianceQty      *float64   `json:"variance_qty"`
	VariancePct      *float64   `json:"variance_pct"` // nil when SystemQty is 0 (division undefined) or not yet counted
	ItemStatus       string     `json:"item_status"`  // PENDING, COUNTED, RECOUNT
	CountStatus      string     `json:"count_status"` // header status: DRAFT, CONFIRMED, ADJUSTMENT_CREATED
	CountedBy        *string    `json:"counted_by"`   // the count session's creator — the schema's only real "who counted this" field
	ReviewedBy       *string    `json:"reviewed_by"`  // always nil; see package doc
	ApprovedBy       *string    `json:"approved_by"`  // from the linked stock_adjustment, when one was generated
	AdjustmentNo     *string    `json:"adjustment_no"`
	AdjustmentStatus *string    `json:"adjustment_status"`
}

// Filters is every server-side filter the Stock Count Report's list,
// summary, and export endpoints share.
type Filters struct {
	From          *time.Time // inclusive, on count_date (COALESCE'd to created_at when null)
	To            *time.Time // exclusive
	WarehouseID   *int64
	ZoneID        *int64
	LocationID    *int64
	CounterID     *int64 // the count session's CreatedBy
	Status        string // header status
	ItemStatus    string // PENDING | COUNTED | RECOUNT
	Search        string // matches count_no, sku, or product name
	VarianceOnly  bool   // only items whose variance_qty is non-zero
	HasAdjustment *bool  // true: only items with a generated adjustment; false: only items without one
}

type ListParams struct {
	Filters
	Page    int
	Limit   int
	SortBy  string // count_date (default), count_no, warehouse_name, status
	SortDir string
}

// Summary is the Stock Count Report's KPI panel. Percentages are nil
// (rendered "N/A") when there is no reliable denominator — never a
// fabricated 0 — per CLAUDE.md's "show N/A when a metric cannot be
// reliably calculated" rule.
type Summary struct {
	TotalCountSessions int64 `json:"total_count_sessions"`
	CountedLocations   int64 `json:"counted_locations"`
	CountedSKUs        int64 `json:"counted_skus"`
	// StockCountAccuracy: percentage of counted items (COUNTED or
	// RECOUNT) whose variance_qty is exactly zero. Nil if nothing in
	// scope has been counted yet.
	StockCountAccuracy *float64 `json:"stock_count_accuracy"`
	// QuantityVariance: net SUM(variance_qty) across counted items in
	// scope — positive means net overage, negative net shortage.
	QuantityVariance float64 `json:"quantity_variance"`
	// RecountRate: percentage of items in scope whose item_status is
	// RECOUNT. Nil if there are no items in scope at all.
	RecountRate *float64 `json:"recount_rate"`
	// PendingApproval: count of stock_adjustments generated from a
	// count session in scope that are still Status=PENDING — the
	// closest real proxy for "pending approval" this schema supports,
	// since stock_counts itself has no approval workflow of its own
	// (see package doc).
	PendingApproval int64 `json:"pending_approval"`
}

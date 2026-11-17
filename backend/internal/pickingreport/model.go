// Package pickingreport implements Phase 34D's Picking & Stock Issue
// Report. It reads picking_tasks/picking_task_items/picking_batches/
// picking_batch_allocations/shipment_items directly via raw SQL rather
// than taking a Go dependency on internal/picking/internal/pickingbatch/
// internal/shipping, the same shape internal/receivingreport (34B) and
// internal/stockcountreport (34C) already established.
//
// Double-counting rule (the spec's own explicit requirement): a
// consolidated picking_task_items row's PickedQty is the ONE true
// physical pick, shared across every order a batch put into it —
// each order's own share lives in picking_batch_allocations.
// AllocatedQty, not in a duplicated PickedQty. Row (below) is at
// picking_task_items grain — correct for physical-pick totals
// (Summary/Detail/Short Pick views) — and never sums AllocatedQty
// into it. BatchAllocationRow is a SEPARATE grain (one row per
// order's own share of a batch), for the Batch/Wave Picking Report
// specifically, where per-order attribution is the point.
//
// Picked vs Dispatched: PickedQty (from picking_task_items) and
// DispatchedQty (summed from shipment_items joined back via
// picking_task_item_id, only counting shipments that have actually
// left — status DISPATCHED/IN_TRANSIT/DELIVERED) are two genuinely
// different real quantities — never conflated, per CLAUDE.md's
// "distinguish picking from actual dispatched/issued stock" rule.
//
// Schema gaps (confirmed by direct model inspection): neither
// picking_task_items nor picking_batch_allocations has an LPN/pallet
// or expiry_date column — Row.LPN/Row.ExpiryDate are always nil, the
// frontend renders "N/A", never fabricated.
package pickingreport

import "time"

// Row is one line of the Picking & Stock Issue Report — grain is one
// row per picking_task_item (the physical pick, never exploded per
// order for a consolidated batch line — see package doc).
type Row struct {
	TaskID     int64  `json:"task_id"`
	TaskItemID int64  `json:"task_item_id"`
	PickNo     string `json:"pick_no"`
	OrderNo    string `json:"order_no"` // blank for a batch-created task; see BatchNo/PickingMode instead
	// PickingMode is derived from whether a picking_batches row
	// references THIS TASK — PICK_BY_ORDER (one order, the common
	// case) or BATCH (consolidated across picking_batch_orders). Note
	// orders.picking_mode (PICK_BY_ORDER/PICK_BY_PRODUCT) is a real,
	// separate column too — the order's own declared intent — but this
	// report is at the task grain, and a batch task's consolidation
	// (not an individual order's stated preference) is what actually
	// determines whether PickedQty here is shared across multiple
	// orders, which is the fact this field needs to carry.
	PickingMode   string     `json:"picking_mode"`
	BatchNo       *string    `json:"batch_no"`
	CustomerName  *string    `json:"customer_name"`
	ShipToName    *string    `json:"ship_to_name"`
	WarehouseCode string     `json:"warehouse_code"`
	WarehouseName string     `json:"warehouse_name"`
	SKU           string     `json:"sku"`
	ProductName   string     `json:"product_name"`
	BrandName     *string    `json:"brand_name"`
	CategoryName  *string    `json:"category_name"`
	LocationCode  string     `json:"location_code"`
	LocationName  string     `json:"location_name"`
	LotNo         string     `json:"lot_no"`
	ExpiryDate    *time.Time `json:"expiry_date"` // always nil — no expiry_date column on picking_task_items; see package doc
	LPN           *string    `json:"lpn"`         // always nil — no LPN column anywhere in this schema; see package doc
	UOM           string     `json:"uom"`
	RequestedQty  float64    `json:"requested_qty"` // picking_task_items.ordered_qty
	// AllocatedQty equals RequestedQty at this grain for a plain
	// PICK_BY_ORDER task (reservation always matches the order 1:1).
	// For a BATCH task, this is NOT the per-order breakdown — see the
	// Batch/Wave Picking Report (BatchAllocationRow) for that.
	AllocatedQty float64  `json:"allocated_qty"`
	PickedQty    float64  `json:"picked_qty"`
	ShortQty     *float64 `json:"short_qty"`
	ShortReason  *string  `json:"short_reason"`
	// DispatchedQty is nil when no shipment has been created for this
	// line yet at all (not zero — a real, already-shipped zero
	// quantity does not occur), summed across shipment_items whose
	// owning shipment has actually left (DISPATCHED/IN_TRANSIT/
	// DELIVERED) — a shipment still READY or flagged ISSUE is not
	// counted as dispatched.
	DispatchedQty    *float64   `json:"dispatched_qty"`
	ItemStatus       string     `json:"item_status"` // PENDING, PARTIALLY_PICKED, COMPLETED, SHORT
	TaskStatus       string     `json:"task_status"`
	AssignedPicker   *string    `json:"assigned_picker"`
	StartDateTime    *time.Time `json:"start_date_time"`    // picking_tasks.assigned_at
	CompleteDateTime *time.Time `json:"complete_date_time"` // picking_tasks.completed_at
}

// BatchAllocationRow is one order's own share of a batch-consolidated
// picking line — the Batch/Wave Picking Report's own grain, kept
// deliberately separate from Row (see package doc's double-counting
// rule). SharedTaskPickedQty is included only as cross-reference
// context (how much the shared physical line has picked in total so
// far) — it is NOT this row's own exclusive quantity and must never be
// summed across rows of the same task item.
type BatchAllocationRow struct {
	BatchID             int64    `json:"batch_id"`
	BatchNo             string   `json:"batch_no"`
	TaskPickNo          *string  `json:"task_pick_no"`
	OrderID             int64    `json:"order_id"`
	OrderNo             string   `json:"order_no"`
	CustomerName        *string  `json:"customer_name"`
	WarehouseCode       string   `json:"warehouse_code"`
	WarehouseName       string   `json:"warehouse_name"`
	SKU                 string   `json:"sku"`
	ProductName         string   `json:"product_name"`
	LocationCode        string   `json:"location_code"`
	LotNo               string   `json:"lot_no"`
	UOM                 string   `json:"uom"`
	AllocatedQty        float64  `json:"allocated_qty"`
	SortedQty           float64  `json:"sorted_qty"`
	ShortageQty         *float64 `json:"shortage_qty"`
	ExceptionReason     *string  `json:"exception_reason"`
	SharedTaskPickedQty *float64 `json:"shared_task_picked_qty"`
}

// Filters is every server-side filter the Picking Report's list,
// summary, and export endpoints share. Dates filter on the task's
// CreatedAt (when the pick was generated) — inclusive From, exclusive
// To.
type Filters struct {
	From        *time.Time
	To          *time.Time
	WarehouseID *int64
	Status      string // task header status
	ItemStatus  string // task_item status; ShortOnly below is a convenience alias for ItemStatus == SHORT
	ShortOnly   bool
	PickerID    *int64 // assigned_picker_id
	Search      string // matches pick_no, order_no, SKU, or product name
	LotNo       string
}

type ListParams struct {
	Filters
	Page    int
	Limit   int
	SortBy  string // created_at (default), pick_no, warehouse_name, status
	SortDir string
}

type BatchAllocationFilters struct {
	From        *time.Time
	To          *time.Time
	WarehouseID *int64
	Search      string // matches batch_no or order_no
}

type BatchAllocationListParams struct {
	BatchAllocationFilters
	Page  int
	Limit int
}

// Summary is the Picking Report's KPI panel — intentionally a small
// set (totals, short-pick rate); the full Picking KPI Dashboard
// (productivity, cycle time, on-time rate, ...) is Phase 34F's own
// scope, not duplicated here.
type Summary struct {
	TotalTasks        int64   `json:"total_tasks"`
	CompletedTasks    int64   `json:"completed_tasks"`
	TotalRequestedQty float64 `json:"total_requested_qty"`
	TotalPickedQty    float64 `json:"total_picked_qty"`
	TotalShortQty     float64 `json:"total_short_qty"`
	// ShortPickRate: percentage of lines in scope whose item_status is
	// SHORT. Nil if there are no lines in scope at all.
	ShortPickRate *float64 `json:"short_pick_rate"`
}

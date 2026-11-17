// Package receivingreport implements Phase 34B's Receiving Report — the
// first of the Phase 34 Reports & Analytics family. It reads receipts,
// receipt_items, receipt_item_splits, qc_inspection_items,
// qc_dispositions, and putaway_task_items directly via raw SQL rather
// than taking a Go dependency on internal/receiving/qualitycheck/
// putaway, mirroring internal/qualitycheck's own documented precedent
// for the same reason (no import cycle risk, and a report is read-only
// so there is no business-rule logic to share with those packages'
// own write paths). Every query is read-only: a Receiving Report
// request can never move stock or change a receipt/inspection/putaway
// task's status.
package receivingreport

import "time"

// Row is one line of the Receiving Report — grain is one row per
// (receipt_item, receipt_item_split), so a line received across
// multiple lots/pallets produces multiple rows sharing the same
// ExpectedQty/ReceivedQty (both are per-receipt-item, not per-split —
// only Lot/MfgDate/ExpiryDate/Pallet/Temperature are split-level). A
// line not yet received at all (no splits exist yet) still produces
// exactly one row, with every split-level field nil.
type Row struct {
	ReceiptID       int64      `json:"receipt_id"`
	ReceiptItemID   int64      `json:"receipt_item_id"`
	SplitID         *int64     `json:"split_id"`
	ReceiptNo       string     `json:"receipt_no"`
	ReceivingDate   *time.Time `json:"receiving_date"`
	ReceivingType   string     `json:"receiving_type"`
	ReferenceNo     *string    `json:"reference_no"`
	SupplierName    *string    `json:"supplier_name"`
	InvoiceNo       *string    `json:"invoice_no"`
	DeliveryNoteNo  *string    `json:"delivery_note_no"`
	WarehouseCode   string     `json:"warehouse_code"`
	WarehouseName   string     `json:"warehouse_name"`
	SKU             string     `json:"sku"`
	ProductName     string     `json:"product_name"`
	BrandName       *string    `json:"brand_name"`
	CategoryName    *string    `json:"category_name"`
	ExpectedQty     float64    `json:"expected_qty"`
	ReceivedQty     float64    `json:"received_qty"`
	ShortOverQty    float64    `json:"short_over_qty"` // received - expected: negative = short, positive = over
	UOM             string     `json:"uom"`
	LotNo           *string    `json:"lot_no"`
	MfgDate         *time.Time `json:"mfg_date"`
	ExpiryDate      *time.Time `json:"expiry_date"`
	PalletID        *string    `json:"pallet_id"`
	TemperatureC    *float64   `json:"temperature_c"`
	ReceivingStatus string     `json:"receiving_status"`
	// QCStatus is one of NOT_REQUIRED, PENDING, PARTIAL, PASSED,
	// REJECTED, MIXED — see repository.go's query comment for the exact
	// derivation rule.
	QCStatus string `json:"qc_status"`
	// PutawayStatus is one of NOT_STARTED, PENDING, IN_PROGRESS,
	// COMPLETED, ISSUE — line-level only (putaway_task_items has no
	// split-level FK), a pre-existing, documented limitation (see
	// internal/putaway's own Sources-still-line-level note), not
	// introduced by this report.
	PutawayStatus string  `json:"putaway_status"`
	OperatorName  *string `json:"operator_name"`
}

// Filters is every server-side filter the Receiving Report's list,
// summary, and export endpoints share — the exact same filter set
// produces the exact same scope for all three, per CLAUDE.md's
// "exports must respect exactly the same filters as the displayed
// report" rule.
type Filters struct {
	From        *time.Time // inclusive, on receiving_date (COALESCE'd to created_at when receiving_date is null)
	To          *time.Time // exclusive
	WarehouseID *int64
	SupplierID  *int64
	Status      string // receipt header status
	Search      string // matches receipt_no, reference_no, sku, or product name
	LotNo       string
	OperatorID  *int64
}

// ListParams adds pagination/sorting to Filters for the list endpoint.
type ListParams struct {
	Filters
	Page    int
	Limit   int
	SortBy  string // receiving_date (default), receipt_no, warehouse_name, status
	SortDir string // asc | desc
}

// UOMQty is one UOM's total received quantity — kept separate per UOM
// rather than summed together, since quantities in different UOMs are
// not comparable (CLAUDE.md's "do not mix quantities from incompatible
// UOMs").
type UOMQty struct {
	UOM string  `json:"uom"`
	Qty float64 `json:"qty"`
}

// TrendPoint is one day's receiving activity, for the Receiving Trend
// chart.
type TrendPoint struct {
	Date         string  `json:"date"` // YYYY-MM-DD, Bangkok-local (see repository.go)
	ReceiptCount int64   `json:"receipt_count"`
	ReceivedQty  float64 `json:"received_qty"`
}

// NamedCount is one grouping bucket (by supplier, warehouse, or
// operator) for the corresponding chart.
type NamedCount struct {
	Name         string  `json:"name"`
	ReceiptCount int64   `json:"receipt_count"`
	ReceivedQty  float64 `json:"received_qty"`
}

// Charts bundles every Receiving Report chart's data, computed from
// the exact same filtered scope as the list/export endpoints.
type Charts struct {
	Trend       []TrendPoint `json:"trend"`
	BySupplier  []NamedCount `json:"by_supplier"`
	ByWarehouse []NamedCount `json:"by_warehouse"`
	ByOperator  []NamedCount `json:"by_operator"`
}

// Summary is the Receiving Report's KPI panel. CompletionRate and
// AvgReceivingLeadTimeHours are nil (rendered "N/A" by the frontend)
// when there is no reliable basis to compute them — never a fabricated
// 0 — per CLAUDE.md's "show N/A when a metric cannot be reliably
// calculated" rule.
type Summary struct {
	TotalReceipts             int64    `json:"total_receipts"`
	TotalReceivedQtyByUOM     []UOMQty `json:"total_received_qty_by_uom"`
	CompletionRate            *float64 `json:"completion_rate"`               // percentage 0-100: COMPLETED receipts / total receipts in scope
	AvgReceivingLeadTimeHours *float64 `json:"avg_receiving_lead_time_hours"` // nil unless at least one COMPLETED receipt in scope has a recorded confirmation
	PendingQCQty              float64  `json:"pending_qc_qty"`
	PendingPutawayQty         float64  `json:"pending_putaway_qty"`
	// SupplierDeliveryVariance is always nil: no expected-delivery-date
	// field exists on receipts to compare against (see package doc on
	// Filters) — the spec's own "show N/A when a metric cannot be
	// reliably calculated" rule applies, this is not an oversight.
	SupplierDeliveryVariance *float64 `json:"supplier_delivery_variance"`
	Charts                   Charts   `json:"charts"`
}

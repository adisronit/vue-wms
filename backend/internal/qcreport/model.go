// Package qcreport implements Phase 34G's QC Report. It reads
// qc_inspections, qc_inspection_items, qc_dispositions, receipts,
// products, and locations directly via raw SQL rather than taking a Go
// dependency on internal/qualitycheck, mirroring every other Phase 34
// report package's own precedent (no import cycle risk, and a report
// has no business rules to share with a write path). Every query is
// read-only: a QC Report request can never move stock, resolve a hold,
// or change an inspection's status.
//
// Row grain is one row per qc_inspection_item — i.e. "QC was at least
// started" for this (product, lot, location) tuple. A receipt split
// that requires QC but has no qc_inspection_items row yet (inspection
// never started) does not appear as a Row; it is counted separately in
// Summary.PendingQCNotStarted*, which is computed from receipts/
// receipt_item_splits directly, not from this package's own Row grain
// — see repository.go's notStarted query.
//
// Key formula decisions, audited directly against
// internal/qualitycheck's model.go/service.go rather than assumed:
//
//   - Resolution Status (PENDING/PARTIAL/MIXED/PASSED/REJECTED): a HOLD
//     disposition performs no inventory mutation (service.go's
//     DispositionHold case) and never resolves anything by itself — an
//     item can receive a HOLD now and a PASS/REJECT later via a second
//     Confirm call against the same (not-yet-COMPLETED) inspection.
//     Resolution Status is therefore derived from
//     SUM(pass_qty)+SUM(reject_qty) vs held_qty_snapshot, reusing
//     exactly the CASE expression internal/receivingreport's own
//     qcStatusCase already established for this exact arithmetic.
//
//   - Defect Code: qc_dispositions.defect_code is free text with no
//     lookup/enum table anywhere in the codebase. There is no separate
//     "Defect Description" field — this report shows DefectCode
//     verbatim and never fabricates a description. DefectCode here is
//     taken from an inspection item's MOST RECENT disposition only; if
//     an item went through more than one reject/hold cycle, only the
//     latest disposition's defect code is shown, not a history of all
//     of them (documented limitation, not an oversight).
//
//   - Resolution Status / Corrective Action: "Corrective Action" is
//     NOT IMPLEMENTED — no such field exists anywhere in the schema.
//     Only a free-text Remark exists (on both qc_inspections and
//     qc_dispositions); it is surfaced as Remark, never relabeled.
//
//   - Evidence Reference: qc_dispositions.evidence_reference is a real
//     schema column with a real writer path (service.go wires
//     ConfirmRequest.Dispositions[].EvidenceReference into the
//     INSERT), but no currently-known UI path actually populates it —
//     expect this field to be empty in practice today even though the
//     column is not merely theoretical.
//
//   - QC Turnaround Time: hours between qc_inspections.created_at and
//     the item's most recent disposition's created_at, computed ONLY
//     once the item is resolved (PASSED/REJECTED/MIXED) — nil while
//     PENDING/PARTIAL, never a fabricated in-progress duration.
//
//   - Hold Aging: hours between an anchor and "now", computed ONLY
//     while an item is still open (PENDING/PARTIAL). No "hold started
//     at" timestamp exists anywhere on qc_inspection_items or
//     receipt_item_splits, so the anchor is the item's own first HOLD
//     disposition's created_at when one has been recorded, falling
//     back to qc_inspections.created_at when the item is still PENDING
//     with no disposition at all yet — both are approximations and the
//     frontend should present this as such, not as an exact SLA clock.
//
//   - Pending QC: this report distinguishes the two sub-cases the
//     Receiving Report's own "Pending QC Qty" collapses together: (a)
//     PendingQCNotStarted — a receipt split that requires QC with zero
//     qc_inspection_items rows at all, and (b) PendingQCAwaitingDecision
//     — a qc_inspection_items row exists (Resolution Status PENDING or
//     PARTIAL) but is not yet fully resolved. See repository.go.
package qcreport

import "time"

// Filters is every server-side filter the QC Report's list, summary,
// and export endpoints share. From/To/WarehouseID/SupplierID/QCType/
// InspectorID/Search scope which inspections are considered at all
// (applied identically everywhere, including Summary's KPI
// aggregates); Disposition/DefectCode are row-level presentational
// refinements of the List/Export output only (e.g. "show me just the
// REJECTED rows") and are not applied to Summary's own KPI
// denominators, so a KPI's definition never silently shifts underneath
// a drill-down filter.
type Filters struct {
	From        *time.Time // inclusive, on qc_inspections.created_at
	To          *time.Time // exclusive
	WarehouseID *int64
	SupplierID  *int64
	QCType      string
	InspectorID *int64
	Search      string // matches inspection_no, receipt_no, sku, or product name

	Disposition string // list-only: filter to one Resolution Status (PENDING, PARTIAL, MIXED, PASSED, REJECTED)
	DefectCode  string // list-only: filter to a defect code (exact or partial match)
}

// ListParams adds pagination/sorting to Filters for the list endpoint.
type ListParams struct {
	Filters
	Page    int
	Limit   int
	SortBy  string // inspection_date (default), inspection_no, warehouse_name, resolution_status
	SortDir string // asc | desc
}

// Row is one QC Report line — see package doc for its exact grain.
type Row struct {
	InspectionID        int64     `json:"inspection_id"`
	InspectionNo        string    `json:"inspection_no"`
	ReceiptID           int64     `json:"receipt_id"`
	ReceiptNo           string    `json:"receipt_no"`
	WarehouseCode       string    `json:"warehouse_code"`
	WarehouseName       string    `json:"warehouse_name"`
	SupplierName        *string   `json:"supplier_name"`
	InspectorName       *string   `json:"inspector_name"`
	QCType              string    `json:"qc_type"`
	InspectionCreatedAt time.Time `json:"inspection_created_at"`

	InspectionItemID int64      `json:"inspection_item_id"`
	SKU              string     `json:"sku"`
	ProductName      string     `json:"product_name"`
	LotNo            string     `json:"lot_no"`
	ExpiryDate       *time.Time `json:"expiry_date"`
	LocationCode     *string    `json:"location_code"`
	HeldQtySnapshot  float64    `json:"held_qty_snapshot"`
	SampleQty        float64    `json:"sample_qty"`
	TemperatureC     *float64   `json:"temperature_c"`
	PackagingOK      *bool      `json:"packaging_ok"`
	LabelOK          *bool      `json:"label_ok"`

	PassQty          float64 `json:"pass_qty"`
	RejectQty        float64 `json:"reject_qty"`
	RemainingHoldQty float64 `json:"remaining_hold_qty"` // held_qty_snapshot - pass_qty - reject_qty, floored at 0
	// ResolutionStatus is one of PENDING, PARTIAL, MIXED, PASSED,
	// REJECTED — see package doc for the exact derivation rule.
	ResolutionStatus string `json:"resolution_status"`

	// DefectCode/EvidenceReference/Remark all come from this item's most
	// recent disposition only — see package doc.
	DefectCode        *string `json:"defect_code"`
	EvidenceReference *string `json:"evidence_reference"`
	Remark            *string `json:"remark"`

	QCTurnaroundHours *float64 `json:"qc_turnaround_hours"` // nil until resolved
	HoldAgingHours    *float64 `json:"hold_aging_hours"`    // nil once resolved
}

// RateWithDenominator pairs a percentage (nil when the denominator is
// zero) with the quantities that produced it, so the frontend never
// shows a bare percentage with a hidden exclusion — same intent as
// internal/pickingkpi's own RateWithDenominator, but quantity-valued
// (float64) rather than count-valued here, since Pass/Reject/Hold are
// all measured in stock quantity, not a row count.
type RateWithDenominator struct {
	RatePct     *float64 `json:"rate_pct"`
	Numerator   float64  `json:"numerator"`
	Denominator float64  `json:"denominator"`
}

// DefectCount is one bucket of the Top Defect Types chart/table, over
// REJECT dispositions' defect_code in scope — free text, grouped
// verbatim (see package doc on why there is no description to attach).
type DefectCount struct {
	DefectCode string  `json:"defect_code"`
	Count      int64   `json:"count"`
	Qty        float64 `json:"qty"`
}

// SupplierRejectionPoint is one (date, supplier) bucket of the
// Supplier Rejection Trend chart.
type SupplierRejectionPoint struct {
	Date      string  `json:"date"` // YYYY-MM-DD, UTC
	Supplier  string  `json:"supplier"`
	RejectQty float64 `json:"reject_qty"`
}

// NamedCount is one grouping bucket (by QC type) for its chart.
type NamedCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// Charts bundles every QC Report chart's data, computed from the same
// filtered scope as Summary (Filters only, not the list-only
// Disposition/DefectCode refinements).
type Charts struct {
	TopDefects             []DefectCount            `json:"top_defects"`
	SupplierRejectionTrend []SupplierRejectionPoint `json:"supplier_rejection_trend"`
	ByQCType               []NamedCount             `json:"by_qc_type"`
}

// Summary is the QC Report's KPI panel. Every nullable field is nil
// (render "N/A"), never a fabricated 0, when its denominator is zero —
// see package doc for each formula's exact basis.
type Summary struct {
	TotalInspectionItems int64 `json:"total_inspection_items"`

	PassRate      RateWithDenominator `json:"pass_rate"`      // pass_qty / (pass_qty+reject_qty) across items in scope
	RejectionRate RateWithDenominator `json:"rejection_rate"` // reject_qty / (pass_qty+reject_qty) across items in scope
	HoldRate      RateWithDenominator `json:"hold_rate"`      // currently-open remaining_hold_qty / total held_qty_snapshot across items in scope

	// PendingQCNotStarted* and PendingQCAwaitingDecision* are the two
	// distinct sub-cases "Pending QC" can mean — see package doc.
	PendingQCNotStartedCount       int64   `json:"pending_qc_not_started_count"`
	PendingQCNotStartedQty         float64 `json:"pending_qc_not_started_qty"`
	PendingQCAwaitingDecisionCount int64   `json:"pending_qc_awaiting_decision_count"`
	PendingQCAwaitingDecisionQty   float64 `json:"pending_qc_awaiting_decision_qty"`

	AvgTurnaroundHours *float64 `json:"avg_turnaround_hours"` // nil when zero resolved items in scope
	AvgHoldAgingHours  *float64 `json:"avg_hold_aging_hours"` // nil when zero currently-open items in scope
	MaxHoldAgingHours  *float64 `json:"max_hold_aging_hours"`

	Charts Charts `json:"charts"`
}

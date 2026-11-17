// Package deliveryreport implements Phase 34H's Delivery & Shipping
// Report. It reads shipments/shipment_items/packing_jobs/orders/
// loading_trips/loading_trip_shipments/loading_trip_cartons directly
// via raw SQL rather than taking a Go dependency on internal/shipping/
// internal/loading/internal/packing/internal/order, mirroring every
// other Phase 34 report package's own precedent. Every query is
// read-only: a request here can never dispatch, mark delivered, or
// move any shipment/trip through its own workflow.
//
// CRITICAL, audited directly against internal/shipping's own
// service.go rather than assumed: "Dispatched is NOT Delivered."
// DELIVERED is a real, storable status with a real DeliveredAt
// timestamp (shipments.delivered_at) — but Service.Confirm's own
// DELIVER action (service.go) is a manual, self-reported button click
// that stamps "now" with ZERO supporting evidence: no proof-of-
// delivery reference, no receiver name or signature, no carrier
// webhook or tracking-event integration anywhere in this schema. This
// report therefore shows real status=DELIVERED/delivered_at values
// (never fabricates them, and never infers delivery from
// DISPATCHED/IN_TRANSIT), but documents everywhere it surfaces this
// that it represents "someone in this application clicked Mark
// Delivered," not carrier-verified proof of delivery — see
// Summary.DeliveryConfirmationRate's own doc.
//
// Key formula/schema decisions:
//
//   - Delivery Exception: the only real exception signal anywhere in
//     this schema is shipments.status = 'ISSUE', with a single shared
//     free-text Remark column and no closed reason taxonomy and no
//     dedicated "issue reported at" timestamp (ISSUE only touches the
//     generic UpdatedAt). A loading-dock short-carton exception
//     (loading_trip_cartons.short_reason, DAMAGED/MISSING/OTHER) is a
//     SEPARATE, earlier-stage event — it can occur before a shipment
//     is ever dispatched and is surfaced only in the Loading & Dispatch
//     view (TripRow), never conflated with a delivery exception.
//
//   - Delivery Lead Time: delivered_at - dispatched_at, computed ONLY
//     when both are non-null (i.e. only for shipments actually marked
//     delivered) — never estimated for a shipment still sitting at
//     DISPATCHED/IN_TRANSIT from an assumed transit time, since no
//     such assumption exists anywhere in this system.
//
//   - On-time dispatch: there is no "promised ship date" distinct from
//     ship_date itself (a user-entered planning field set before
//     dispatch, not a confirmed SLA commitment) — this report does
//     NOT compute an on-time-dispatch KPI, since doing so would compare
//     a confirmed event against an unconfirmed plan and misrepresent
//     the comparison as an SLA measurement.
//
//   - Order Type / double-counting transfers: internal/transferreceipt
//     is created from an already-dispatched internal/shipping Shipment
//     — inter-warehouse transfers flow through the SAME shipments
//     table as ordinary sales shipments, distinguished only via
//     orders.order_type (SALES_ORDER default, INTERNAL_TRANSFER).
//     OrderType is surfaced on every Row/Summary bucket so a caller can
//     separate the two; Summary's own totals are NOT pre-filtered to
//     exclude transfers (both are real dispatched/delivered shipments),
//     but the OrderType filter lets a caller isolate either one.
//
//   - IN_TRANSIT is a valid shipments.status enum value with no code
//     path that ever sets it (Confirm only implements DISPATCH/DELIVER/
//     ISSUE) — it is included in every status list here for
//     completeness but should be expected to never appear in practice.
package deliveryreport

import "time"

// Filters is every server-side filter the Delivery & Shipping Report's
// list, summary, and export endpoints share.
type Filters struct {
	From        *time.Time // inclusive, on shipments.created_at
	To          *time.Time // exclusive
	WarehouseID *int64     // via the linked packing job
	Carrier     string
	Status      string // shipment status
	OrderType   string // SALES_ORDER | INTERNAL_TRANSFER, via the linked order
	Search      string // matches shipment_no, order_no, tracking_no, sku, or product name
}

// ListParams adds pagination/sorting to Filters for the list endpoint.
type ListParams struct {
	Filters
	Page    int
	Limit   int
	SortBy  string // ship_date (default), shipment_no, status
	SortDir string // asc | desc
}

// Row is one Shipment Item Detail line — grain is one row per
// shipment_item, the correct grain for line-level export, analogous to
// receivingreport's split-grain Row. A shipment's header fields
// (status, carrier, dispatched_at, delivered_at, ...) repeat across
// every item line it has, by design — summing Qty across rows of the
// same shipment reconstructs the shipment's own total without
// double-counting, but summing header-level counts (e.g. "how many
// shipments are DISPATCHED") from this grain WOULD double-count by
// however many item lines a shipment has; Summary's own counts are
// computed from a separate shipment-grain query, never from this one.
type Row struct {
	ShipmentID    int64      `json:"shipment_id"`
	ShipmentNo    string     `json:"shipment_no"`
	OrderNo       string     `json:"order_no"`
	OrderType     string     `json:"order_type"`
	CustomerName  *string    `json:"customer_name"`
	ShipToName    *string    `json:"ship_to_name"`
	ShipToAddress *string    `json:"ship_to_address"`
	WarehouseCode string     `json:"warehouse_code"`
	WarehouseName string     `json:"warehouse_name"`
	Carrier       *string    `json:"carrier"`
	TrackingNo    *string    `json:"tracking_no"`
	Packages      int        `json:"packages"`
	Weight        *float64   `json:"weight"`
	Status        string     `json:"status"`
	ShipDate      *time.Time `json:"ship_date"`
	DispatchedAt  *time.Time `json:"dispatched_at"`
	// DeliveredAt is real when present — see package doc — but
	// represents a manual "Mark Delivered" click, not carrier-verified
	// proof of delivery.
	DeliveredAt *time.Time `json:"delivered_at"`

	ShipmentItemID int64   `json:"shipment_item_id"`
	ProductID      int64   `json:"product_id"`
	SKU            string  `json:"sku"`
	ProductName    string  `json:"product_name"`
	LotNo          string  `json:"lot_no"`
	LocationCode   *string `json:"location_code"`
	Qty            float64 `json:"qty"`
	UOM            string  `json:"uom"`

	// DeliveryLeadTimeHours is nil unless both DispatchedAt and
	// DeliveredAt are set — never estimated.
	DeliveryLeadTimeHours *float64 `json:"delivery_lead_time_hours"`
	IsException           bool     `json:"is_exception"` // status == ISSUE
	ExceptionRemark       *string  `json:"exception_remark"`
}

// TripRow is one Loading & Dispatch Report line — trip grain, a
// genuinely different grain than Row (one loading_trips row can carry
// many shipments). Export-only (CSV), no XLSX/PDF — a deliberate scope
// decision matching every other Phase 34 secondary view's own
// precedent, not an oversight.
type TripRow struct {
	TripID               int64      `json:"trip_id"`
	TripNo               string     `json:"trip_no"`
	VehicleNo            *string    `json:"vehicle_no"`
	DriverName           *string    `json:"driver_name"`
	Carrier              *string    `json:"carrier"`
	LoadingDock          *string    `json:"loading_dock"`
	Route                *string    `json:"route"`
	Status               string     `json:"status"`
	ScheduledLoadingTime *time.Time `json:"scheduled_loading_time"`
	ActualLoadingStartAt *time.Time `json:"actual_loading_start_at"`
	ActualLoadingEndAt   *time.Time `json:"actual_loading_end_at"`
	CompletedAt          *time.Time `json:"completed_at"`
	ShipmentCount        int64      `json:"shipment_count"`
	CartonCount          int64      `json:"carton_count"`
	ShortCartonCount     int64      `json:"short_carton_count"`
	// LoadingDurationHours is nil unless both ActualLoadingStartAt and
	// ActualLoadingEndAt are set.
	LoadingDurationHours *float64 `json:"loading_duration_hours"`
}

// RateWithDenominator pairs a percentage (nil when the denominator is
// zero) with the counts that produced it.
type RateWithDenominator struct {
	RatePct     *float64 `json:"rate_pct"`
	Numerator   int64    `json:"numerator"`
	Denominator int64    `json:"denominator"`
}

// NamedCount is one grouping bucket (by status, carrier, or warehouse).
type NamedCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// TrendPoint is one day's shipment activity, for the Shipment Trend
// chart.
type TrendPoint struct {
	Date          string `json:"date"`
	ShipmentCount int64  `json:"shipment_count"`
}

// Charts bundles every Delivery & Shipping Report chart's data.
type Charts struct {
	Trend       []TrendPoint `json:"trend"`
	ByStatus    []NamedCount `json:"by_status"`
	ByCarrier   []NamedCount `json:"by_carrier"`
	ByWarehouse []NamedCount `json:"by_warehouse"`
}

// Summary is the Delivery & Shipping Report's KPI panel — every count
// here is computed from a shipment-grain query (never the exploded
// item grain Row uses), so a shipment with five lines is never counted
// five times.
type Summary struct {
	TotalShipments int64 `json:"total_shipments"`

	// DispatchRate = (DISPATCHED + IN_TRANSIT + DELIVERED + ISSUE-after-
	// dispatch) / (every non-CANCELLED shipment in scope). Shipments
	// still READY count toward the denominator but not the numerator.
	DispatchRate RateWithDenominator `json:"dispatch_rate"`
	// DeliveryConfirmationRate = DELIVERED / (every shipment that was
	// actually dispatched, in scope) — see package doc: this reflects a
	// manual "Mark Delivered" click, not carrier-verified proof of
	// delivery. Never compared against the full shipment population,
	// since a never-dispatched shipment can never be delivered.
	DeliveryConfirmationRate RateWithDenominator `json:"delivery_confirmation_rate"`
	// ExceptionRate = ISSUE / (every non-CANCELLED shipment in scope).
	ExceptionRate RateWithDenominator `json:"exception_rate"`

	// AvgDeliveryLeadTimeHours is nil when zero shipments in scope have
	// both DispatchedAt and DeliveredAt set.
	AvgDeliveryLeadTimeHours *float64 `json:"avg_delivery_lead_time_hours"`

	TotalTrips     int64 `json:"total_trips"`
	CompletedTrips int64 `json:"completed_trips"`
	// AvgLoadingDurationHours is nil when zero COMPLETED trips in scope
	// have both ActualLoadingStartAt and ActualLoadingEndAt set.
	AvgLoadingDurationHours *float64 `json:"avg_loading_duration_hours"`

	Charts Charts `json:"charts"`
}

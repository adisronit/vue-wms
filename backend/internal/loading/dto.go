package loading

import "time"

// CartonResponse is one carton on a trip's own scan-to-load worklist.
type CartonResponse struct {
	ID           int64        `json:"id,string" example:"1"`
	CartonID     int64        `json:"carton_id,string" example:"1"`
	LPN          string       `json:"lpn" example:"CTN-000001"`
	ShipmentID   int64        `json:"shipment_id,string" example:"1"`
	ShipmentNo   string       `json:"shipment_no" example:"SHP-000001"`
	Status       CartonStatus `json:"status" example:"PLANNED"`
	LoadedAt     *time.Time   `json:"loaded_at"`
	LoadedBy     *int64       `json:"loaded_by,string"`
	LoadedByName *string      `json:"loaded_by_name"`
	ShortReason  *string      `json:"short_reason"`
	ShortRemark  *string      `json:"short_remark"`
	CreatedAt    time.Time    `json:"created_at"`
} //	@name	LoadingTripCartonResponse

// ShipmentSummaryResponse is one shipment assigned to a trip, with its
// own planned/loaded/short carton counts.
type ShipmentSummaryResponse struct {
	ShipmentID   int64  `json:"shipment_id,string" example:"1"`
	ShipmentNo   string `json:"shipment_no" example:"SHP-000001"`
	OrderNo      string `json:"order_no" example:"SO-20260608-001"`
	Customer     *string `json:"customer" example:"ABC Customer"`
	PackingJobID int64  `json:"packing_job_id,string" example:"1"`
	JobNo        string `json:"job_no" example:"PACK-000001"`
	PlannedCount int    `json:"planned_count" example:"3"`
	LoadedCount  int    `json:"loaded_count" example:"2"`
	ShortCount   int    `json:"short_count" example:"0"`
} //	@name	LoadingTripShipmentResponse

// Response is a full loading trip with its assigned shipments and
// carton worklist — GET /loading-trips/{id}.
type Response struct {
	ID                   int64                     `json:"id,string" example:"1"`
	TripNo               string                    `json:"trip_no" example:"TRIP-000001"`
	VehicleNo            *string                   `json:"vehicle_no"`
	DriverName           *string                   `json:"driver_name"`
	Carrier              *string                   `json:"carrier"`
	LoadingDock          *string                   `json:"loading_dock"`
	Route                *string                   `json:"route"`
	ScheduledLoadingTime *time.Time                `json:"scheduled_loading_time"`
	ActualLoadingStartAt *time.Time                `json:"actual_loading_start_at"`
	ActualLoadingEndAt   *time.Time                `json:"actual_loading_end_at"`
	VehicleSeal          *string                   `json:"vehicle_seal"`
	Status               Status                    `json:"status" example:"PLANNED"`
	Remark               *string                   `json:"remark"`
	PlannedCount         int                       `json:"planned_count" example:"6"`
	LoadedCount          int                       `json:"loaded_count" example:"4"`
	ShortCount           int                       `json:"short_count" example:"0"`
	Shipments            []ShipmentSummaryResponse `json:"shipments"`
	Cartons              []CartonResponse          `json:"cartons"`
	CreatedAt            time.Time                 `json:"created_at"`
	UpdatedAt            time.Time                 `json:"updated_at"`
	CompletedAt          *time.Time                `json:"completed_at"`
	CancelledAt          *time.Time                `json:"cancelled_at"`
	CancelledBy          *int64                    `json:"cancelled_by,string"`
	CancelledByName      *string                   `json:"cancelled_by_name"`
	CancelRemark         *string                   `json:"cancel_remark"`
} //	@name	LoadingTripResponse

// ListResponse is one row of GET /loading-trips.
type ListResponse struct {
	ID                   int64      `json:"id,string" example:"1"`
	TripNo               string     `json:"trip_no" example:"TRIP-000001"`
	VehicleNo            *string    `json:"vehicle_no"`
	DriverName           *string    `json:"driver_name"`
	Carrier              *string    `json:"carrier"`
	Status               Status     `json:"status" example:"PLANNED"`
	ShipmentCount        int        `json:"shipment_count" example:"2"`
	PlannedCount         int        `json:"planned_count" example:"6"`
	LoadedCount          int        `json:"loaded_count" example:"4"`
	ShortCount           int        `json:"short_count" example:"0"`
	ScheduledLoadingTime *time.Time `json:"scheduled_loading_time"`
	CreatedAt            time.Time  `json:"created_at"`
} //	@name	LoadingTripListResponse

// SourceResponse is one candidate shipment for GET /loading-trips/sources
// — a READY shipment not yet assigned to any active (non-CANCELLED)
// trip. Powers the "assign shipment to trip" picker.
type SourceResponse struct {
	ShipmentID   int64   `json:"shipment_id,string" example:"1"`
	ShipmentNo   string  `json:"shipment_no" example:"SHP-000001"`
	OrderNo      string  `json:"order_no" example:"SO-20260608-001"`
	Customer     *string `json:"customer" example:"ABC Customer"`
	PackingJobID int64   `json:"packing_job_id,string" example:"1"`
	JobNo        string  `json:"job_no" example:"PACK-000001"`
	CartonCount  int     `json:"carton_count" example:"2"`
} //	@name	LoadingTripSourceResponse

// CreateRequest is POST /loading-trips — the trip header. Every field
// is optional at creation time (a dispatcher may open a trip before
// every detail is confirmed) except none are actually required by the
// database either — the trip exists as soon as it has an id/trip_no.
type CreateRequest struct {
	VehicleNo            *string    `json:"vehicle_no" example:"1กก-1234"`
	DriverName           *string    `json:"driver_name" example:"Somchai Jaidee"`
	Carrier              *string    `json:"carrier" example:"DHL"`
	LoadingDock          *string    `json:"loading_dock" example:"DOCK-01"`
	Route                *string    `json:"route" example:"Bangkok - Chiang Mai"`
	ScheduledLoadingTime *time.Time `json:"scheduled_loading_time"`
	VehicleSeal          *string    `json:"vehicle_seal" example:"SEAL-000123"`
	Remark               *string    `json:"remark"`
} //	@name	CreateLoadingTripRequest

// AssignShipmentRequest is POST /loading-trips/{id}/shipments.
type AssignShipmentRequest struct {
	ShipmentID int64 `json:"shipment_id,string" binding:"required" example:"1"`
} //	@name	AssignShipmentRequest

// ScanCartonRequest is POST /loading-trips/{id}/scan — one "scan this
// carton as loaded" action. LPN is matched against this trip's own
// planned cartons (cartons.lpn, resolved tenant-scoped) — a carton that
// belongs to a different trip, or was never planned for any trip at
// all, is rejected (see Service.ScanCarton's own doc).
type ScanCartonRequest struct {
	LPN string `json:"lpn" binding:"required" example:"CTN-000001"`
} //	@name	ScanCartonRequest

// ShortCartonRequest is POST /loading-trips/{id}/short — flags a
// planned carton as unable to be loaded (damaged/missing), instead of
// LOADED.
type ShortCartonRequest struct {
	LPN    string  `json:"lpn" binding:"required" example:"CTN-000001"`
	Reason string  `json:"reason" binding:"required,oneof=DAMAGED MISSING OTHER" example:"DAMAGED"`
	Remark *string `json:"remark" example:"Carton crushed on the dock while staging"`
} //	@name	ShortLoadCartonRequest

// ConfirmRequest is POST /loading-trips/{id}/confirm — transitions the
// trip to COMPLETED once every planned carton is LOADED or SHORT, and
// calls shipping.Service.MarkLoaded for every shipment on the trip.
type ConfirmRequest struct {
	Remark *string `json:"remark" example:"Verified vehicle seal SEAL-000123 applied"`
} //	@name	ConfirmLoadingTripRequest

// CancelRequest is POST /loading-trips/{id}/cancel.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Vehicle did not show up"`
} //	@name	CancelLoadingTripRequest

// ListParams narrows GET /loading-trips to its search box plus status
// filter, with pagination.
type ListParams struct {
	Search string
	Status Status
	Page   int
	Limit  int
}

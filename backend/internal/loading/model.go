// Package loading implements Phase 33D.2 — the "load" half of the
// original Phase 33 "Dispatch re-sequencing behind pack/load
// validation" requirement (33D.1 already added the "pack" half: a
// shipment cannot even be CREATED from a packing job that is not
// COMPLETED). Loading Trip -> Trip Shipments -> Trip Cartons -> scan
// each carton loaded (or flag it short) -> Confirm Trip is the real
// physical-handling/traceability layer that proves a shipment's own
// cartons actually left the dock on a vehicle before internal/shipping
// will allow it to be DISPATCHED.
//
// Like internal/packing before it, this package performs NO inventory
// mutation of its own at all — stock already left the building at
// Picking.Confirm(PICKED), and internal/shipping performs no inventory
// mutation either (see that package's own doc); Loading is purely a
// physical-handling/traceability record of which cartons were put on
// which vehicle, and when.
//
// Dependency direction (the one-way shape this phase's own
// instructions require, mirroring internal/packing -> internal/picking
// and internal/shipping -> internal/packing before it):
//
//	internal/loading -> internal/shipping -> internal/packing -> internal/picking
//
// internal/loading depends on internal/shipping's own Get (read-only,
// to resolve a shipment's status and its own packing_job_id) and
// internal/packing's own Get (read-only, to resolve that packing job's
// own cartons) through narrow, structurally-satisfied interfaces
// (shippingChecker/packingChecker below) — the same pattern packing's
// own pickingChecker and shipping's own packingChecker already
// established. internal/shipping is NEVER modified to call into this
// package — its own dispatch gate (Status.CanDispatch plus the new
// LoadedAt != nil check) is a plain local field check against two new
// columns (migrations/0107-0108) that ONLY this package's own
// ConfirmTrip action ever writes, via the new, narrow
// shipping.Service.MarkLoaded mutator. Grepped to confirm
// internal/shipping does not import internal/loading anywhere — doing
// so would be a real Go import cycle (loading -> shipping -> loading).
package loading

import "time"

// Status is the loading trip header's state machine: PLANNED (header
// created, shipments may already be assigned — see TripShipment — but
// no carton has been scanned loaded or flagged short yet) -> LOADING
// (at least one carton has been resolved — ActualLoadingStartAt is
// stamped at that same moment, the first time this trip's own status
// moves off PLANNED) -> COMPLETED (every planned carton for this trip
// is LOADED or SHORT, and the explicit ConfirmTrip action has run) /
// CANCELLED (abandoned before completion).
type Status string

const (
	StatusPlanned   Status = "PLANNED"
	StatusLoading   Status = "LOADING"
	StatusCompleted Status = "COMPLETED"
	StatusCancelled Status = "CANCELLED"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPlanned, StatusLoading, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// CanAssignShipment reports whether AssignShipment may run — any
// non-terminal status. A shipment may still be added to a trip that is
// already LOADING (multi-drop trips are typically built up over time,
// and a late-added shipment's own cartons simply start out PLANNED like
// every other carton already on the trip).
func (s Status) CanAssignShipment() bool {
	switch s {
	case StatusPlanned, StatusLoading:
		return true
	default:
		return false
	}
}

// CanScan reports whether ScanCarton/ShortCarton may run — the same
// non-terminal predicate as CanAssignShipment.
func (s Status) CanScan() bool {
	return s.CanAssignShipment()
}

// CanConfirm reports whether ConfirmTrip may run — only once the trip
// has actually moved into LOADING (i.e., at least one carton has
// already been resolved). A still-PLANNED trip (zero scan activity at
// all) has nothing to confirm yet.
func (s Status) CanConfirm() bool {
	return s == StatusLoading
}

// CanCancel reports whether CancelTrip may run — before either
// terminal state, matching every other module's own CanCancel shape.
func (s Status) CanCancel() bool {
	switch s {
	case StatusPlanned, StatusLoading:
		return true
	default:
		return false
	}
}

// CartonStatus is one loading_trip_cartons row's own state —
// independent of the trip header. PLANNED -> LOADED (ScanCarton) /
// SHORT (ShortCarton) is terminal-once-resolved, the same two-terminal-
// outcome shape packing_job_items.status (PACKED/EXCEPTION) and
// picking_task_items.status (COMPLETED/SHORT) already established.
type CartonStatus string

const (
	CartonStatusPlanned CartonStatus = "PLANNED"
	CartonStatusLoaded  CartonStatus = "LOADED"
	CartonStatusShort   CartonStatus = "SHORT"
)

func (s CartonStatus) IsValid() bool {
	switch s {
	case CartonStatusPlanned, CartonStatusLoaded, CartonStatusShort:
		return true
	default:
		return false
	}
}

// cartonResolved reports whether a trip carton has reached a terminal
// outcome (LOADED or SHORT) — the precondition ConfirmTrip checks
// across every carton on the trip.
func cartonResolved(s CartonStatus) bool {
	return s == CartonStatusLoaded || s == CartonStatusShort
}

// ShortReason is a closed set for the "unable to load" exception —
// mirrors packing.ExceptionReason's own closed-set shape, adapted to
// this phase's own loading-dock discovery (damaged in transit from the
// packing face to the dock, or physically missing from the staging
// area), deliberately separate from both packing.ExceptionReason and
// picking.ShortReasonCode — a loading-stage exception is a different
// event at a different workflow stage than either of those.
type ShortReason string

const (
	ShortReasonDamaged ShortReason = "DAMAGED"
	ShortReasonMissing ShortReason = "MISSING"
	ShortReasonOther   ShortReason = "OTHER"
)

func (r ShortReason) IsValid() bool {
	switch r {
	case ShortReasonDamaged, ShortReasonMissing, ShortReasonOther:
		return true
	default:
		return false
	}
}

// Trip is the GORM model for the loading_trips table (migrations/0109).
type Trip struct {
	ID                    int64      `gorm:"column:id;primaryKey"`
	TenantID              int64      `gorm:"column:tenant_id;not null"`
	TripNo                string     `gorm:"column:trip_no"`
	VehicleNo             *string    `gorm:"column:vehicle_no"`
	DriverName            *string    `gorm:"column:driver_name"`
	Carrier               *string    `gorm:"column:carrier"`
	LoadingDock           *string    `gorm:"column:loading_dock"`
	Route                 *string    `gorm:"column:route"`
	ScheduledLoadingTime  *time.Time `gorm:"column:scheduled_loading_time"`
	ActualLoadingStartAt  *time.Time `gorm:"column:actual_loading_start_at"`
	ActualLoadingEndAt    *time.Time `gorm:"column:actual_loading_end_at"`
	VehicleSeal           *string    `gorm:"column:vehicle_seal"`
	Status                Status     `gorm:"column:status"`
	Remark                *string    `gorm:"column:remark"`
	CreatedBy             *int64     `gorm:"column:created_by"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
	CompletedAt           *time.Time `gorm:"column:completed_at"`
	CancelledAt           *time.Time `gorm:"column:cancelled_at"`
	CancelledBy           *int64     `gorm:"column:cancelled_by"`
	CancelRemark          *string    `gorm:"column:cancel_remark"`
}

func (Trip) TableName() string { return "loading_trips" }

// TripShipment is the GORM model for the loading_trip_shipments table
// (migrations/0110) — which shipments are assigned to a trip.
type TripShipment struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	TenantID   int64     `gorm:"column:tenant_id;not null"`
	TripID     int64     `gorm:"column:trip_id"`
	ShipmentID int64     `gorm:"column:shipment_id"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (TripShipment) TableName() string { return "loading_trip_shipments" }

// TripCarton is the GORM model for the loading_trip_cartons table
// (migrations/0111) — the scan-to-load worklist.
type TripCarton struct {
	ID          int64        `gorm:"column:id;primaryKey"`
	TenantID    int64        `gorm:"column:tenant_id;not null"`
	TripID      int64        `gorm:"column:trip_id"`
	CartonID    int64        `gorm:"column:carton_id"`
	ShipmentID  int64        `gorm:"column:shipment_id"`
	Status      CartonStatus `gorm:"column:status"`
	LoadedAt    *time.Time   `gorm:"column:loaded_at"`
	LoadedBy    *int64       `gorm:"column:loaded_by"`
	ShortReason *string      `gorm:"column:short_reason"`
	ShortRemark *string      `gorm:"column:short_remark"`
	CreatedAt   time.Time    `gorm:"column:created_at"`
	UpdatedAt   time.Time    `gorm:"column:updated_at"`
}

func (TripCarton) TableName() string { return "loading_trip_cartons" }

// Package shipping implements the fourth and final real transactional
// workflow in the outbound flow: Picking Task -> Shipment -> Shipment
// Items -> Confirm (Dispatch/Deliver/Issue). Unlike every prior
// workflow, it performs NO inventory mutation of its own: Picking
// (Phase 9) already called RemoveStock and wrote the inventory ledger
// when it picked the stock, so Shipping represents outbound completion
// / dispatch confirmation only — it never calls AddStock, RemoveStock,
// or MoveStock. Grepped to confirm: neither internal/inventory nor its
// Service type is even imported by this package.
//
// Matches ShippingView.vue — a single list+drawer screen, the same
// shape Picking (Phase 9) already established (there is no
// NewShippingView.vue). See PROJECT_STATUS.md for the one-shipment-
// per-picking-task, no-partial-shipment, and status-machine decisions.
package shipping

import "time"

// Status is the shipment header's state machine.
type Status string

const (
	StatusReady      Status = "READY"
	StatusDispatched Status = "DISPATCHED"
	StatusInTransit  Status = "IN_TRANSIT"
	StatusDelivered  Status = "DELIVERED"
	StatusIssue      Status = "ISSUE"
	// StatusCancelled is Phase 15's addition — unlike Receiving/Putaway/
	// Picking, this package never had a CANCELLED forward-compatibility
	// placeholder (see migrations/0029's doc: this is the first phase to
	// add a shipment cancel action, and the first migration in the
	// project to ALTER an existing CHECK constraint rather than only
	// adding new tables).
	StatusCancelled Status = "CANCELLED"
)

// CanEdit reports whether header fields (carrier/tracking/packages/
// weight/ship_date/address/remark) may still be changed — only before
// dispatch, the same "can't edit out from under a stock movement"
// reasoning Receiving/Putaway apply to their own documents, adapted
// here to "can't edit out from under a confirmed dispatch."
func (s Status) CanEdit() bool {
	return s == StatusReady
}

// CanDispatch reports whether Confirm's DISPATCH action may run.
func (s Status) CanDispatch() bool {
	return s == StatusReady
}

// CanDeliver reports whether Confirm's DELIVER action may run — only
// after a real dispatch, matching ShippingView.vue's own button set (a
// shipment that hasn't been dispatched has no tracking number yet, so
// "delivered" would be meaningless).
func (s Status) CanDeliver() bool {
	return s == StatusDispatched
}

// CanReportIssue reports whether Confirm's ISSUE action may run —
// mirrors ShippingView.vue's "Report Issue" button being available
// regardless of whether the shipment has already been dispatched
// (matches Putaway's ISSUE/Picking's SHORT precedent: a problem can
// surface at any non-terminal point). DELIVERED is terminal; ISSUE
// itself is terminal (no double-reporting).
func (s Status) CanReportIssue() bool {
	return s == StatusReady || s == StatusDispatched
}

func (s Status) IsValid() bool {
	switch s {
	case StatusReady, StatusDispatched, StatusInTransit, StatusDelivered, StatusIssue, StatusCancelled:
		return true
	default:
		return false
	}
}

// CanCancel reports whether Cancel may run against a shipment in this
// status — Phase 15's addition. Only READY, i.e. before dispatch:
// Picking already removed the stock for every line when it was picked
// (see package doc — Shipping performs no inventory mutation of its
// own at all), so cancelling a shipment never needs to touch inventory
// either way; the boundary is purely "has this actually been handed to
// a carrier yet," matching CanEdit/CanDispatch's own boundary exactly.
func (s Status) CanCancel() bool {
	return s == StatusReady
}

// Carrier mirrors ShippingView.vue's Carrier <select> exactly.
type Carrier string

const (
	CarrierDHL      Carrier = "DHL"
	CarrierKerry    Carrier = "KERRY"
	CarrierFlash    Carrier = "FLASH"
	CarrierInternal Carrier = "INTERNAL"
)

func (c Carrier) IsValid() bool {
	switch c {
	case CarrierDHL, CarrierKerry, CarrierFlash, CarrierInternal:
		return true
	default:
		return false
	}
}

// Shipment is the GORM model for the shipments table (migrations/0020).
// CancelledAt/CancelledBy/CancelRemark (migrations/0032, Phase 18) are
// nil until a successful Cancel — see Service.Cancel's own doc.
type Shipment struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID      int64      `gorm:"column:tenant_id;not null;default:1"`
	ShipmentNo    string     `gorm:"column:shipment_no"`
	PickingTaskID int64      `gorm:"column:picking_task_id"`
	Carrier       *string    `gorm:"column:carrier"`
	TrackingNo    *string    `gorm:"column:tracking_no"`
	Packages      int        `gorm:"column:packages"`
	Weight        *float64   `gorm:"column:weight"`
	ShipDate      *time.Time `gorm:"column:ship_date"`
	Address       *string    `gorm:"column:address"`
	Remark        *string    `gorm:"column:remark"`
	Status        Status     `gorm:"column:status"`
	CreatedBy     *int64     `gorm:"column:created_by"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
	DispatchedAt  *time.Time `gorm:"column:dispatched_at"`
	DeliveredAt   *time.Time `gorm:"column:delivered_at"`
	CancelledAt   *time.Time `gorm:"column:cancelled_at"`
	CancelledBy   *int64     `gorm:"column:cancelled_by"`
	CancelRemark  *string    `gorm:"column:cancel_remark"`
}

func (Shipment) TableName() string { return "shipments" }

// applyCancel mutates the shipment in place for a successful cancellation:
// sets Status to CANCELLED and stamps CancelledAt/CancelledBy, storing
// remark in the dedicated CancelRemark column only when a non-empty
// remark was supplied. Callers (Service.Cancel) only ever invoke this
// after Status.CanCancel() has already gated the guard, so it never runs
// twice for the same shipment and never overwrites an existing
// cancellation's audit trail.
func (s *Shipment) applyCancel(userID int64, remark *string, now time.Time) {
	s.Status = StatusCancelled
	s.CancelledAt = &now
	s.CancelledBy = &userID
	if remark != nil && *remark != "" {
		s.CancelRemark = remark
	}
}

// Item is the GORM model for the shipment_items table (migrations/0021)
// — immutable once created: qty is copied from the source picking
// task's picked_qty at shipment-creation time and never edited
// afterward (see package doc).
type Item struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID          int64     `gorm:"column:tenant_id;not null;default:1"`
	ShipmentID        int64     `gorm:"column:shipment_id"`
	PickingTaskItemID int64     `gorm:"column:picking_task_item_id"`
	ProductID         int64     `gorm:"column:product_id"`
	LotNo             string    `gorm:"column:lot_no"`
	LocationID        int64     `gorm:"column:location_id"`
	Qty               float64   `gorm:"column:qty"`
	UOM               string    `gorm:"column:uom"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (Item) TableName() string { return "shipment_items" }

// Confirmation is the GORM model for the shipment_confirmations table
// (migrations/0022) — the idempotency guard for Confirm, the same
// pattern as receiving/putaway/picking's own Confirmation types.
type Confirmation struct {
	IdempotencyKey string `gorm:"column:idempotency_key;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID   int64     `gorm:"column:tenant_id;not null;default:1"`
	ShipmentID int64     `gorm:"column:shipment_id"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "shipment_confirmations" }

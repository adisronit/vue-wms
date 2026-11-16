// Package picking implements the third real transactional workflow:
// Picking Task -> Picking Task Items -> Confirm -> Inventory RemoveStock
// -> Inventory Ledger. It calls Phase 5's internal/inventory
// (RemoveStockTx) for every stock mutation — no balance or ledger logic
// is duplicated here, and it never calls AddStock/MoveStock: picking
// only ever removes stock from a location for outbound fulfillment.
//
// Matches PickingView.vue — a single list+drawer screen, unlike
// Receiving/Putaway's separate list/create views (there is no
// NewPickingView.vue). See PROJECT_STATUS.md for the order/customer
// free-text, source-location, atomic-quantity, RBAC, and idempotency
// decisions.
package picking

import "time"

// Status is the picking task header's state machine.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
	StatusShortPick  Status = "SHORT_PICK"
	StatusCancelled  Status = "CANCELLED"
)

// CanConfirm reports whether Confirm may run against a task in this
// status. A task reaches COMPLETED or SHORT_PICK only once every line is
// itself terminal (see computeTaskStatus) — from either state there is
// nothing left to confirm. CANCELLED is likewise terminal (schema
// forward-compatibility only, unreachable this phase, matching
// receiving/putaway's own precedent).
func (s Status) CanConfirm() bool {
	switch s {
	case StatusPending, StatusInProgress:
		return true
	default:
		return false
	}
}

// CanCancel reports whether Cancel may run against a task in this
// status. Same predicate as CanConfirm today (PENDING/IN_PROGRESS only)
// — a task that has already reached a terminal state (COMPLETED,
// SHORT_PICK, or CANCELLED itself) has nothing left to cancel: per
// Phase 13's own instructions, cancellation is never allowed once stock
// has already been fully picked, and CANCELLED is itself terminal so a
// second cancel attempt is rejected rather than silently no-op'd. Kept
// as its own method (not a direct reuse of CanConfirm) so a future
// business rule could diverge the two predicates without disturbing
// Confirm's own gate.
func (s Status) CanCancel() bool {
	switch s {
	case StatusPending, StatusInProgress:
		return true
	default:
		return false
	}
}

// CanAssign reports whether AssignPicker may run against a task in this
// status — same predicate as CanConfirm: assigning (or reassigning) an
// operator only makes sense while a task still has work left to do.
func (s Status) CanAssign() bool {
	return s.CanConfirm()
}

// CanRepick reports whether a SHORT line on a task in this status may
// be reopened via Repick. Unlike CanConfirm/CanAssign, SHORT_PICK itself
// must be allowed here — that header status is exactly what a task
// reaches once every line is terminal and at least one was short (see
// computeTaskStatus), so it is the normal starting point for a re-pick
// attempt, not an exception to it. CANCELLED is still rejected: a
// cancelled task's reservations have already all been released and
// re-opening a line on it would have nothing backing a fresh
// reservation's own business context.
func (s Status) CanRepick() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusShortPick:
		return true
	default:
		return false
	}
}

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted, StatusShortPick, StatusCancelled:
		return true
	default:
		return false
	}
}

// Priority mirrors PickingView.vue's Priority filter <select>.
type Priority string

const (
	PriorityLow    Priority = "LOW"
	PriorityNormal Priority = "NORMAL"
	PriorityHigh   Priority = "HIGH"
)

func (p Priority) IsValid() bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh:
		return true
	default:
		return false
	}
}

// ShortReasonCode (migrations/0084-0085, Phase 33B) is a closed set
// replacing free-text remark as the authoritative reason a line was
// flagged SHORT. remark itself is untouched — still an optional general
// note any confirm action may carry — but a SHORT action now requires
// one of these codes, not an arbitrary string, so a report or dashboard
// can group/filter short-picks by reason without parsing prose.
type ShortReasonCode string

const (
	ShortReasonOutOfStock    ShortReasonCode = "OUT_OF_STOCK"
	ShortReasonDamaged       ShortReasonCode = "DAMAGED"
	ShortReasonLocationEmpty ShortReasonCode = "LOCATION_EMPTY"
	ShortReasonWrongLot      ShortReasonCode = "WRONG_LOT"
	ShortReasonMiscount      ShortReasonCode = "MISCOUNT"
	ShortReasonOther         ShortReasonCode = "OTHER"
)

func (r ShortReasonCode) IsValid() bool {
	switch r {
	case ShortReasonOutOfStock, ShortReasonDamaged, ShortReasonLocationEmpty, ShortReasonWrongLot, ShortReasonMiscount, ShortReasonOther:
		return true
	default:
		return false
	}
}

// ScanResult is a closed set for picking_scan_log.result — see
// Service.ScanConfirm's own doc.
type ScanResult string

const (
	ScanResultMatch    ScanResult = "MATCH"
	ScanResultMismatch ScanResult = "MISMATCH"
)

// ItemStatus is a task line's own state, independent of the header.
type ItemStatus string

const (
	ItemStatusPending         ItemStatus = "PENDING"
	ItemStatusPartiallyPicked ItemStatus = "PARTIALLY_PICKED"
	ItemStatusCompleted       ItemStatus = "COMPLETED"
	ItemStatusShort           ItemStatus = "SHORT"
)

// Task is the GORM model for the picking_tasks table (migrations/0017).
// CancelledAt/CancelledBy/CancelRemark (migrations/0032, Phase 18) are
// nil until a successful Cancel — see Service.Cancel's own doc.
type Task struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID int64   `gorm:"column:tenant_id;not null;default:1"`
	PickNo   string  `gorm:"column:pick_no"`
	OrderNo  string  `gorm:"column:order_no"`
	Customer *string `gorm:"column:customer"`
	// CustomerID/ShipTo* (migrations/0062) are a one-way copy from
	// internal/order's own CreatePickingTask — see that method's own doc
	// and Service.createCore's own doc here. Never written by any other
	// path in this package (Confirm/Cancel never touch them), matching
	// Customer's own existing immutable-once-set convention.
	CustomerID          *int64     `gorm:"column:customer_id"`
	ShipToID            *int64     `gorm:"column:ship_to_id"`
	ShipToName          *string    `gorm:"column:ship_to_name"`
	ShipToAddress       *string    `gorm:"column:ship_to_address"`
	ShipToProvince      *string    `gorm:"column:ship_to_province"`
	ShipToDistrict      *string    `gorm:"column:ship_to_district"`
	ShipToPostalCode    *string    `gorm:"column:ship_to_postal_code"`
	ShipToContactPerson *string    `gorm:"column:ship_to_contact_person"`
	ShipToContactPhone  *string    `gorm:"column:ship_to_contact_phone"`
	WarehouseID         int64      `gorm:"column:warehouse_id"`
	Status              Status     `gorm:"column:status"`
	Priority            Priority   `gorm:"column:priority"`
	DueDate             *time.Time `gorm:"column:due_date"`
	Remark              *string    `gorm:"column:remark"`
	CreatedBy           *int64     `gorm:"column:created_by"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	CompletedAt         *time.Time `gorm:"column:completed_at"`
	CancelledAt         *time.Time `gorm:"column:cancelled_at"`
	CancelledBy         *int64     `gorm:"column:cancelled_by"`
	CancelRemark        *string    `gorm:"column:cancel_remark"`
	// AssignedPickerID/AssignedAt (migrations/0082-0083, Phase 33B) are
	// a one-way "claim" written only by Service.AssignPicker — nil until
	// a task is assigned, and simply overwritten (both fields together,
	// always in the same call) on a reassignment. No ownership
	// verification against AssignedPickerID is required of any other
	// mutating action (Confirm/Cancel/Repick) — matching CreatedBy/
	// CancelledBy's own existing precedent of being a display/audit
	// field, not an authorization gate; any caller holding the
	// appropriate picking.* permission may still act on the task
	// regardless of who it is assigned to.
	AssignedPickerID *int64     `gorm:"column:assigned_picker_id"`
	AssignedAt       *time.Time `gorm:"column:assigned_at"`
}

func (Task) TableName() string { return "picking_tasks" }

// applyCancel mutates the task in place for a successful cancellation:
// sets Status to CANCELLED and stamps CancelledAt/CancelledBy, storing
// remark in the dedicated CancelRemark column only when a non-empty
// remark was supplied. Callers (Service.Cancel) only ever invoke this
// after Status.CanCancel() has already gated the guard, so it never runs
// twice for the same task and never overwrites an existing
// cancellation's audit trail.
func (t *Task) applyCancel(userID int64, remark *string, now time.Time) {
	t.Status = StatusCancelled
	t.CancelledAt = &now
	t.CancelledBy = &userID
	if remark != nil && *remark != "" {
		t.CancelRemark = remark
	}
}

// TaskItem is the GORM model for the picking_task_items table
// (migrations/0018). PickedQty is cumulative across every confirm call
// for this line, not one batch's delta — the same convention Phase 6's
// ReceiptItem.ReceivedQty and Phase 7's TaskItem.MovedQty established,
// written via an atomic guarded UPDATE from the start (see
// repository.go's AdjustPickedQty) rather than a Go read-modify-write.
type TaskItem struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID      int64      `gorm:"column:tenant_id;not null;default:1"`
	PickingTaskID int64      `gorm:"column:picking_task_id"`
	ProductID     int64      `gorm:"column:product_id"`
	LotNo         string     `gorm:"column:lot_no"`
	LocationID    int64      `gorm:"column:location_id"`
	OrderedQty    float64    `gorm:"column:ordered_qty"`
	PickedQty     float64    `gorm:"column:picked_qty"`
	UOM           string     `gorm:"column:uom"`
	Status        ItemStatus `gorm:"column:status"`
	Remark        *string    `gorm:"column:remark"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
	// ShortQty/ShortReason (migrations/0084-0085, Phase 33B) are set
	// together, only by the SHORT branch of Service.Confirm, and cleared
	// together, only by a successful Service.Repick — see each method's
	// own doc. ShortQty is a frozen snapshot of "ordered_qty - picked_qty
	// at the moment this line was flagged short", not a live
	// recomputation, so it keeps its meaning even after a later re-pick
	// changes picked_qty.
	ShortQty    *float64 `gorm:"column:short_qty"`
	ShortReason *string  `gorm:"column:short_reason"`
	RepickCount int      `gorm:"column:repick_count"`
}

func (TaskItem) TableName() string { return "picking_task_items" }

// ScanLog is the GORM model for the picking_scan_log table
// (migrations/0086) — the persisted audit trail for Service.ScanConfirm.
// Append-only: nothing in this package updates or deletes a row here.
type ScanLog struct {
	ID                  int64      `gorm:"column:id;primaryKey"`
	TenantID            int64      `gorm:"column:tenant_id;not null"`
	PickingTaskID       int64      `gorm:"column:picking_task_id"`
	PickingTaskItemID   int64      `gorm:"column:picking_task_item_id"`
	ScannedLocationCode string     `gorm:"column:scanned_location_code"`
	ScannedSKU          string     `gorm:"column:scanned_sku"`
	ScannedLotNo        string     `gorm:"column:scanned_lot_no"`
	Result              ScanResult `gorm:"column:result"`
	MismatchDetail      *string    `gorm:"column:mismatch_detail"`
	ScannedBy           *int64     `gorm:"column:scanned_by"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
}

func (ScanLog) TableName() string { return "picking_scan_log" }

// Confirmation is the GORM model for the picking_confirmations table
// (migrations/0019) — the idempotency guard for Confirm, the same
// pattern as internal/receiving.Confirmation and internal/putaway.Confirmation.
type Confirmation struct {
	IdempotencyKey string `gorm:"column:idempotency_key;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID      int64     `gorm:"column:tenant_id;not null;default:1"`
	PickingTaskID int64     `gorm:"column:picking_task_id"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "picking_confirmations" }

// itemStatus derives a line's status from its own quantities and
// whether it has been explicitly flagged short — mirrors putaway's
// itemStatus/ISSUE pattern, using SHORT (matching PickingView.vue's
// "Short Pick" button) instead of ISSUE: a line flagged SHORT is
// terminal regardless of picked_qty, since a reported short pick means
// no further picking is expected for this line this task.
func itemStatus(orderedQty, pickedQty float64, flaggedShort bool) ItemStatus {
	if flaggedShort {
		return ItemStatusShort
	}
	if pickedQty <= 0 {
		return ItemStatusPending
	}
	if orderedQty > 0 && pickedQty < orderedQty {
		return ItemStatusPartiallyPicked
	}
	return ItemStatusCompleted
}

// computeTaskStatus derives the header status from its lines: once every
// line has reached a terminal state (COMPLETED or SHORT), the header is
// COMPLETED if none of them were short, or SHORT_PICK if at least one
// was (matching PickingView.vue's "Short Pick" summary card). While any
// line is still workable (PENDING/PARTIALLY_PICKED), the header stays
// IN_PROGRESS as soon as anything has happened at all, or PENDING if
// nothing has.
func computeTaskStatus(items []TaskItem) Status {
	if len(items) == 0 {
		return StatusPending
	}

	allTerminal := true
	anyShort := false
	anyActivity := false
	for _, it := range items {
		if it.Status == ItemStatusShort {
			anyShort = true
		}
		if it.Status != ItemStatusCompleted && it.Status != ItemStatusShort {
			allTerminal = false
		}
		if it.PickedQty > 0 || it.Status == ItemStatusShort {
			anyActivity = true
		}
	}

	if allTerminal {
		if anyShort {
			return StatusShortPick
		}
		return StatusCompleted
	}
	if anyActivity {
		return StatusInProgress
	}
	return StatusPending
}

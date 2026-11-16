// Package relocation implements Phase 32's Stock Relocation workflow:
// Relocation Task -> Relocation Task Items -> Confirm -> Inventory
// MoveAvailableStockTx -> Inventory Ledger. It moves stock that is
// already sitting somewhere in the warehouse to a different bin within
// the SAME warehouse, independently of Receiving/Putaway — it never
// creates new stock (no AddStock call anywhere in this package) and the
// total system-wide quantity for any (product, lot) is unchanged by any
// relocation operation, only which location holds it. Architecturally
// this package mirrors internal/putaway closely (same tenant-scoped,
// hold-aware, idempotent-confirm shape); see service.go's Confirm doc
// for the one deliberate behavioral difference (Cancel while
// IN_PROGRESS).
package relocation

import "time"

// Status is the relocation task header's state machine:
// DRAFT -> READY -> IN_PROGRESS -> COMPLETED / CANCELLED. READY is a
// deliberate, explicit transition (Service.Ready) rather than derived
// from line activity the way putaway's IN_PROGRESS/COMPLETED are —
// Phase 32 asked for lines to be lockable for review before any confirm
// call is allowed at all.
type Status string

const (
	StatusDraft      Status = "DRAFT"
	StatusReady      Status = "READY"
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
	StatusCancelled  Status = "CANCELLED"
)

// CanAddItems reports whether lines may still be added/the task edited
// — only before it has been marked Ready.
func (s Status) CanAddItems() bool {
	return s == StatusDraft
}

// CanReady reports whether Service.Ready may transition this task.
func (s Status) CanReady() bool {
	return s == StatusDraft
}

// CanConfirm reports whether Confirm may run against a task in this
// status — once READY (lines locked) through IN_PROGRESS (some lines
// still outstanding). A task reaches COMPLETED only once every line is
// itself COMPLETE — from there nothing is left to confirm.
func (s Status) CanConfirm() bool {
	switch s {
	case StatusReady, StatusInProgress:
		return true
	default:
		return false
	}
}

// CanCancel reports whether Cancel may run against a task in this
// status. Unlike putaway's stricter CanCancel (which Service.Cancel
// additionally rejects if any line has already moved stock), relocation
// deliberately allows cancelling an IN_PROGRESS task — see
// Service.Cancel's own doc for why "abandon the unconfirmed remainder,
// keep what already moved" is the correct behavior here given Phase
// 32's explicit partial-relocation requirement.
func (s Status) CanCancel() bool {
	switch s {
	case StatusDraft, StatusReady, StatusInProgress:
		return true
	default:
		return false
	}
}

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusReady, StatusInProgress, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// ItemStatus is a task line's own state, independent of the header. No
// ISSUE state (unlike putaway.ItemStatus) — Phase 32 did not ask for a
// per-line "report issue" action.
type ItemStatus string

const (
	ItemStatusPending            ItemStatus = "PENDING"
	ItemStatusPartiallyCompleted ItemStatus = "PARTIALLY_COMPLETED"
	ItemStatusCompleted          ItemStatus = "COMPLETED"
)

// Task is the GORM model for the relocation_tasks table
// (migrations/0076). tenant_id is NOT NULL with no default — unlike
// putaway.Task's retrofit-era `default:1`, this table was created after
// tenant scoping was already standard, so every row must always carry
// an explicit tenant.
type Task struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	TenantID     int64      `gorm:"column:tenant_id;not null"`
	RelocationNo string     `gorm:"column:relocation_no"`
	WarehouseID  int64      `gorm:"column:warehouse_id"`
	Status       Status     `gorm:"column:status"`
	Remark       *string    `gorm:"column:remark"`
	CreatedBy    *int64     `gorm:"column:created_by"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
	ReadyAt      *time.Time `gorm:"column:ready_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at"`
	CancelledAt  *time.Time `gorm:"column:cancelled_at"`
	CancelledBy  *int64     `gorm:"column:cancelled_by"`
	CancelRemark *string    `gorm:"column:cancel_remark"`
}

func (Task) TableName() string { return "relocation_tasks" }

// applyCancel mutates the task in place for a successful cancellation —
// matches putaway.Task.applyCancel exactly.
func (t *Task) applyCancel(userID int64, remark *string, now time.Time) {
	t.Status = StatusCancelled
	t.CancelledAt = &now
	t.CancelledBy = &userID
	if remark != nil && *remark != "" {
		t.CancelRemark = remark
	}
}

// TaskItem is the GORM model for the relocation_task_items table
// (migrations/0077). MovedQty is cumulative across every confirm call
// for this line, not one batch's delta — the same convention
// putaway_task_items/receipt_items established. LPN is pure display/
// audit metadata (pallet/license-plate number) — never consulted by
// inventory.MoveAvailableStockTx, which has no LPN concept.
type TaskItem struct {
	ID                    int64      `gorm:"column:id;primaryKey"`
	TenantID              int64      `gorm:"column:tenant_id;not null"`
	RelocationTaskID      int64      `gorm:"column:relocation_task_id"`
	ProductID             int64      `gorm:"column:product_id"`
	LotNo                 string     `gorm:"column:lot_no"`
	LPN                   *string    `gorm:"column:lpn"`
	SourceLocationID      int64      `gorm:"column:source_location_id"`
	DestinationLocationID int64      `gorm:"column:destination_location_id"`
	RequestedQty          float64    `gorm:"column:requested_qty"`
	MovedQty              float64    `gorm:"column:moved_qty"`
	UOM                   string     `gorm:"column:uom"`
	Status                ItemStatus `gorm:"column:status"`
	Remark                *string    `gorm:"column:remark"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (TaskItem) TableName() string { return "relocation_task_items" }

// Confirmation is the GORM model for the relocation_confirmations table
// (migrations/0078) — the idempotency guard for Confirm, the same
// pattern as putaway.Confirmation.
type Confirmation struct {
	IdempotencyKey   string    `gorm:"column:idempotency_key;primaryKey"`
	TenantID         int64     `gorm:"column:tenant_id;not null"`
	RelocationTaskID int64     `gorm:"column:relocation_task_id"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "relocation_confirmations" }

// itemStatus derives a line's status from its own quantities — mirrors
// putaway's itemStatus minus the ISSUE override (no such flag exists on
// this line shape).
func itemStatus(requestedQty, movedQty float64) ItemStatus {
	if movedQty <= 0 {
		return ItemStatusPending
	}
	if requestedQty > 0 && movedQty < requestedQty {
		return ItemStatusPartiallyCompleted
	}
	return ItemStatusCompleted
}

// computeTaskStatus derives the header status from its lines once the
// task is already READY/IN_PROGRESS — callers (Service.Confirm) never
// invoke this from DRAFT (Ready is the only transition out of DRAFT,
// and it is explicit, not derived). COMPLETED only once every line has
// reached ItemStatusCompleted; IN_PROGRESS otherwise, since Confirm
// having run at all means something happened.
func computeTaskStatus(items []TaskItem) Status {
	for _, it := range items {
		if it.Status != ItemStatusCompleted {
			return StatusInProgress
		}
	}
	return StatusCompleted
}

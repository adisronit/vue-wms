// Package putaway implements the second real transactional workflow:
// Putaway Task -> Putaway Task Items -> Confirm -> Inventory MoveStock
// -> Inventory Ledger. It moves stock a Phase 6 receipt already placed
// in a receiving/staging location to a storage bin — it never creates
// new stock (no AddStock call anywhere in this package) and the total
// system-wide quantity for any (product, lot) is unchanged by any
// putaway operation, only which location holds it.
//
// Matches PutawayView.vue / NewPutawayView.vue. See PROJECT_STATUS.md
// for the location-classification, task-origin, ledger-reference-type,
// and idempotency decisions.
package putaway

import "time"

// Status is the putaway task header's state machine.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
	StatusIssue      Status = "ISSUE"
	StatusCancelled  Status = "CANCELLED"
)

// CanConfirm reports whether Confirm may run against a task in this
// status. A task reaches COMPLETED or ISSUE only once every line is
// itself terminal (see computeTaskStatus) — from either state there is
// nothing left to confirm. CANCELLED is likewise terminal (schema
// forward-compatibility only, unreachable this phase).
func (s Status) CanConfirm() bool {
	switch s {
	case StatusPending, StatusInProgress:
		return true
	default:
		return false
	}
}

// CanCancel reports whether Cancel may run against a task in this
// status — Phase 15's addition. Same header-level predicate as
// CanConfirm (PENDING/IN_PROGRESS), but this alone is NOT sufficient:
// unlike Receiving/Picking, IN_PROGRESS here can be reached purely by a
// line being flagged ISSUE (no stock movement at all — see
// computeTaskStatus's own doc), so Service.Cancel additionally checks
// every line's actual MovedQty before allowing cancellation, per this
// phase's own "based on actual task state" instruction. This method
// only gates the header-level boundary that COMPLETED/ISSUE/CANCELLED
// (all terminal) can never be cancelled.
func (s Status) CanCancel() bool {
	switch s {
	case StatusPending, StatusInProgress:
		return true
	default:
		return false
	}
}

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted, StatusIssue, StatusCancelled:
		return true
	default:
		return false
	}
}

// Priority mirrors NewPutawayView.vue's Priority <select>.
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

// ItemStatus is a task line's own state, independent of the header.
type ItemStatus string

const (
	ItemStatusPending            ItemStatus = "PENDING"
	ItemStatusPartiallyCompleted ItemStatus = "PARTIALLY_COMPLETED"
	ItemStatusCompleted          ItemStatus = "COMPLETED"
	ItemStatusIssue              ItemStatus = "ISSUE"
)

// Task is the GORM model for the putaway_tasks table (migrations/0014).
// CancelledAt/CancelledBy/CancelRemark (migrations/0032, Phase 18) are
// nil until a successful Cancel — see Service.Cancel's own doc.
type Task struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID     int64      `gorm:"column:tenant_id;not null;default:1"`
	PutawayNo    string     `gorm:"column:putaway_no"`
	ReceiptID    *int64     `gorm:"column:receipt_id"`
	WarehouseID  int64      `gorm:"column:warehouse_id"`
	Status       Status     `gorm:"column:status"`
	Priority     Priority   `gorm:"column:priority"`
	AssignedTo   *string    `gorm:"column:assigned_to"`
	Remark       *string    `gorm:"column:remark"`
	CreatedBy    *int64     `gorm:"column:created_by"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at"`
	CancelledAt  *time.Time `gorm:"column:cancelled_at"`
	CancelledBy  *int64     `gorm:"column:cancelled_by"`
	CancelRemark *string    `gorm:"column:cancel_remark"`
}

func (Task) TableName() string { return "putaway_tasks" }

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

// TaskItem is the GORM model for the putaway_task_items table
// (migrations/0015). MovedQty is cumulative across every confirm call
// for this line, not one batch's delta — the same convention Phase 6's
// ReceiptItem.ReceivedQty established.
type TaskItem struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID              int64      `gorm:"column:tenant_id;not null;default:1"`
	PutawayTaskID         int64      `gorm:"column:putaway_task_id"`
	ReceiptItemID         *int64     `gorm:"column:receipt_item_id"`
	ProductID             int64      `gorm:"column:product_id"`
	LotNo                 string     `gorm:"column:lot_no"`
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

func (TaskItem) TableName() string { return "putaway_task_items" }

// Confirmation is the GORM model for the putaway_confirmations table
// (migrations/0016) — the idempotency guard for Confirm, the same
// pattern as internal/receiving.Confirmation.
type Confirmation struct {
	IdempotencyKey string `gorm:"column:idempotency_key;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID      int64     `gorm:"column:tenant_id;not null;default:1"`
	PutawayTaskID int64     `gorm:"column:putaway_task_id"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "putaway_confirmations" }

// itemStatus derives a line's status from its own quantities and
// whether it has been explicitly flagged — mirrors receiving's
// itemStatus, plus the ISSUE override: an item flagged ISSUE (via
// Confirm's per-line "ISSUE" action, matching PutawayView.vue's
// "Report Issue" button / NewPutawayView.vue's per-item Issue status)
// is terminal regardless of moved_qty, since a reported issue means no
// further movement is expected for this line this task.
func itemStatus(requestedQty, movedQty float64, flaggedIssue bool) ItemStatus {
	if flaggedIssue {
		return ItemStatusIssue
	}
	if movedQty <= 0 {
		return ItemStatusPending
	}
	if requestedQty > 0 && movedQty < requestedQty {
		return ItemStatusPartiallyCompleted
	}
	return ItemStatusCompleted
}

// computeTaskStatus derives the header status from its lines: once
// every line has reached a terminal state (COMPLETED or ISSUE), the
// header is COMPLETED if none of them were issues, or ISSUE if at
// least one was (surfacing it for review — matches the "Location
// Issues" summary card on PutawayView.vue). While any line is still
// workable (PENDING/PARTIALLY_COMPLETED), the header stays IN_PROGRESS
// as soon as anything has happened at all, or PENDING if nothing has.
func computeTaskStatus(items []TaskItem) Status {
	if len(items) == 0 {
		return StatusPending
	}

	allTerminal := true
	anyIssue := false
	anyActivity := false
	for _, it := range items {
		if it.Status == ItemStatusIssue {
			anyIssue = true
		}
		if it.Status != ItemStatusCompleted && it.Status != ItemStatusIssue {
			allTerminal = false
		}
		if it.MovedQty > 0 || it.Status == ItemStatusIssue {
			anyActivity = true
		}
	}

	if allTerminal {
		if anyIssue {
			return StatusIssue
		}
		return StatusCompleted
	}
	if anyActivity {
		return StatusInProgress
	}
	return StatusPending
}

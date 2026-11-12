// Package replenishment implements Phase 24's move-stock-to-the-pick-
// face workflow: a single-line task (one product, one source location,
// one destination location, one quantity) that reserves stock at the
// source on Create (Phase 12's own ReserveStockTx pattern, preventing
// two tasks from oversubscribing the same available stock) and, on
// Confirm, physically relocates it via Phase 7's inventory.MoveStockTx
// — the same "call into internal/inventory for every balance/ledger
// change, never duplicate that logic" rule every other workflow package
// in this codebase already follows.
package replenishment

import "time"

// Status is the task's state machine.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
	StatusCancelled  Status = "CANCELLED"
	StatusIssue      Status = "ISSUE"
)

// CanStart reports whether Start may run — only ever the very first
// transition out of PENDING.
func (s Status) CanStart() bool {
	return s == StatusPending
}

// CanConfirm reports whether Confirm may run — PENDING (skipping Start
// is allowed; Start exists only to let staff signal "I'm working this
// now", not to gate Confirm) or IN_PROGRESS.
func (s Status) CanConfirm() bool {
	switch s {
	case StatusPending, StatusInProgress:
		return true
	default:
		return false
	}
}

// CanCancel reports whether Cancel may run — same open set as
// CanConfirm. COMPLETED/CANCELLED/ISSUE are all terminal.
func (s Status) CanCancel() bool {
	switch s {
	case StatusPending, StatusInProgress:
		return true
	default:
		return false
	}
}

// CanReportIssue reports whether ReportIssue may run — same open set.
func (s Status) CanReportIssue() bool {
	switch s {
	case StatusPending, StatusInProgress:
		return true
	default:
		return false
	}
}

// Task is the GORM model for the replenishment_tasks table
// (migrations/0036).
type Task struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0045, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID       int64      `gorm:"column:tenant_id;not null;default:1"`
	TaskNo         string     `gorm:"column:task_no"`
	ProductID      int64      `gorm:"column:product_id"`
	FromLocationID int64      `gorm:"column:from_location_id"`
	ToLocationID   int64      `gorm:"column:to_location_id"`
	LotNo          string     `gorm:"column:lot_no"`
	Quantity       float64    `gorm:"column:quantity"`
	WarehouseID    int64      `gorm:"column:warehouse_id"`
	Status         Status     `gorm:"column:status"`
	Remark         *string    `gorm:"column:remark"`
	IssueRemark    *string    `gorm:"column:issue_remark"`
	CreatedBy      *int64     `gorm:"column:created_by"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	StartedAt      *time.Time `gorm:"column:started_at"`
	CompletedAt    *time.Time `gorm:"column:completed_at"`
	CancelledAt    *time.Time `gorm:"column:cancelled_at"`
	CancelledBy    *int64     `gorm:"column:cancelled_by"`
	CancelRemark   *string    `gorm:"column:cancel_remark"`
}

func (Task) TableName() string { return "replenishment_tasks" }

// applyCancel populates the cancellation-audit columns — same shape as
// picking.Task's own applyCancel (Phase 18).
func (t *Task) applyCancel(userID int64, remark string, now time.Time) {
	t.Status = StatusCancelled
	t.CancelledAt = &now
	t.CancelledBy = &userID
	if remark != "" {
		t.CancelRemark = &remark
	}
}

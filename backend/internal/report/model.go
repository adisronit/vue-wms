// Package report is Phase 22's async export-job pipeline. Scope is
// deliberately narrow: today the only job_type is
// INVENTORY_TRANSACTIONS (the one report in this phase's required list
// expected to genuinely outgrow a synchronous request — an append-only,
// ever-growing ledger). Every other report (Inventory Stock, Receiving/
// Putaway/Picking/Shipping/Stock Adjustment/Stock Count History) is a
// synchronous CSV handler living directly on its own owning domain
// package (see internal/receiving's own Export handler for the
// pattern) — this package exists only for the one report that needed
// the async job/RabbitMQ/worker shape.
package report

import "time"

// Status is the job's own state machine — the same PENDING/PROCESSING/
// COMPLETED/FAILED shape CLAUDE.md's own Background Job Status section
// recommends.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusCompleted  Status = "COMPLETED"
	StatusFailed     Status = "FAILED"
)

// JobType is a closed set — see the package doc for why only one exists
// today. A plain string column in the DB (not a CHECK-constrained
// enum), so a future async report type is a Go-side addition only, no
// migration required to widen it.
type JobType string

const (
	JobTypeInventoryTransactions JobType = "INVENTORY_TRANSACTIONS"
)

// ExportJob is the GORM model for the export_jobs table (migrations/0035).
type ExportJob struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	JobType      JobType    `gorm:"column:job_type"`
	Status       Status     `gorm:"column:status"`
	RequestedBy  int64      `gorm:"column:requested_by"`
	FiltersJSON  *string    `gorm:"column:filters_json"`
	FileName     *string    `gorm:"column:file_name"`
	FilePath     *string    `gorm:"column:file_path"`
	RowCount     *int       `gorm:"column:row_count"`
	ErrorMessage *string    `gorm:"column:error_message"`
	MessageID    string     `gorm:"column:message_id"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	StartedAt    *time.Time `gorm:"column:started_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at"`
}

func (ExportJob) TableName() string { return "export_jobs" }

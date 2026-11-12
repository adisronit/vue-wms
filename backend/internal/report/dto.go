package report

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Response is GET /reports/export-jobs/{id} — lets the frontend poll a
// job's own status/row_count/error_message. file_name is included so
// the frontend can label the download link; file_path (a server
// filesystem path) is never exposed.
type Response struct {
	ID           int64      `json:"id,string" example:"1"`
	JobType      JobType    `json:"job_type" example:"INVENTORY_TRANSACTIONS"`
	Status       Status     `json:"status" example:"COMPLETED"`
	FileName     *string    `json:"file_name"`
	RowCount     *int       `json:"row_count"`
	ErrorMessage *string    `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
} //	@name	ExportJobResponse

// InventoryTransactionsExportRequest is POST
// /reports/inventory-transactions/export's body — the filters this one
// async report supports, deliberately the same shape as
// inventory.TransactionExportFilters (this package's own copy, not an
// import of it, matching this codebase's established narrow-shape-per-
// package convention for cross-package data).
type InventoryTransactionsExportRequest struct {
	ProductID       int64      `json:"product_id,string" example:"1"`
	WarehouseID     int64      `json:"warehouse_id,string" example:"1"`
	TransactionType string     `json:"transaction_type" example:"ADJUST"`
	DateFrom        *time.Time `json:"date_from"`
	DateTo          *time.Time `json:"date_to"`
} //	@name	InventoryTransactionsExportRequest

// CreateJobResponse is the 202 Accepted body for a newly queued export
// job — the standard async-acceptance shape CLAUDE.md's own API
// contract already established for every other job-accepted response
// in this codebase.
type CreateJobResponse struct {
	JobID int64 `json:"job_id,string" example:"1"`
} //	@name	CreateExportJobResponse

// Message is the RabbitMQ envelope for one export job — matches
// CLAUDE.md's own recommended shape (message_id/event_type/created_at/
// correlation_id/payload), the same envelope
// internal/stockadjustment.ReminderMessage already established for
// Phase 21's own async job.
type Message struct {
	MessageID     string         `json:"message_id"`
	EventType     string         `json:"event_type"`
	CreatedAt     time.Time      `json:"created_at"`
	CorrelationID string         `json:"correlation_id"`
	Payload       MessagePayload `json:"payload"`
}

// MessagePayload carries only the job's own ID — the worker reads
// everything else (job_type, filters_json) back from export_jobs itself
// rather than duplicating it into the message body, so there is exactly
// one place filters can ever be read from.
type MessagePayload struct {
	JobID int64 `json:"job_id,string"`
}

const EventTypeExportJob = "report.export"

// NewMessage builds a fresh envelope for jobID — a new MessageID/
// CorrelationID per call, the same convention every message constructor
// in this codebase already uses (DevHeartbeat, stockadjustment's own
// ReminderMessage).
func NewMessage(jobID int64) Message {
	return Message{
		MessageID:     uuid.NewString(),
		EventType:     EventTypeExportJob,
		CreatedAt:     time.Now().UTC(),
		CorrelationID: uuid.NewString(),
		Payload:       MessagePayload{JobID: jobID},
	}
}

func (m Message) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

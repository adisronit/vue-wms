package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"wms-backend/internal/inventory"
	"wms-backend/internal/platform/csvexport"
)

var (
	// ErrNotReady is returned by Download when the job hasn't reached
	// COMPLETED yet (still PENDING/PROCESSING, or ended FAILED).
	ErrNotReady = errors.New("report: export job is not ready for download")
)

// transactionExporter is the narrow slice of internal/inventory's own
// Service this package needs. Unlike internal/notification's own
// cross-package payload (a narrow local copy, decoded from a RabbitMQ
// message by cmd/worker before ever reaching that package), this
// package calls directly into inventory.Service — the same "import the
// owning package's own types for an interface that calls into it"
// precedent internal/stockcount's own adjustmentCreator already
// established for internal/stockadjustment.
type transactionExporter interface {
	ListTransactionsForExport(ctx context.Context, filters inventory.TransactionExportFilters) ([]inventory.TransactionExportRow, error)
}

// messagePublisher is the narrow slice of
// internal/messaging/rabbitmq.Publisher this package needs — CLAUDE.md's
// own rule ("Do not publish RabbitMQ messages directly from HTTP
// handlers... Handler -> Service -> Database + Message Publisher") is
// followed literally here: the handler only ever calls
// CreateInventoryTransactionsJob, publishing itself happens inside this
// package, after the export_jobs row has durably committed.
type messagePublisher interface {
	Publish(ctx context.Context, exchange, routingKey, contentType string, body []byte) error
}

// Service holds the export-job business rules: creating (+ publishing)
// a job, answering status/ownership queries, and (called from
// cmd/worker) the claim-then-generate pipeline.
type Service struct {
	repo         *Repository
	transactions transactionExporter
	publisher    messagePublisher
	exchange     string
	routingKey   string
	exportDir    string
}

// NewService builds a Service. exportDir is the shared filesystem path
// (the export_files Docker volume, mounted at the same path in both
// backend and worker — see docker-compose.yml) generated files are
// written to and served from. exchange/routingKey are the topology
// constants from internal/messaging/rabbitmq, passed in rather than
// imported directly so this package never depends on that one beyond
// the narrow messagePublisher/Handler shape it already needs.
func NewService(repo *Repository, transactions transactionExporter, publisher messagePublisher, exchange, routingKey, exportDir string) *Service {
	return &Service{repo: repo, transactions: transactions, publisher: publisher, exchange: exchange, routingKey: routingKey, exportDir: exportDir}
}

// CreateInventoryTransactionsJob queues a new export job: persists a
// PENDING export_jobs row (with a fresh message_id), then publishes the
// corresponding RabbitMQ message — the row commits before the publish
// is even attempted, so a job is never referenced by a message that
// doesn't durably exist yet. A publish failure here is returned to the
// caller as a 500 (the row still exists, PENDING forever, visible via
// GET /reports/export-jobs/{id} — this phase does not add a retry-
// publish sweep for that edge case, matching Phase 21's own documented
// best-effort stance on a non-critical async path).
func (s *Service) CreateInventoryTransactionsJob(ctx context.Context, userID int64, filters inventory.TransactionExportFilters) (*ExportJob, error) {
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}
	filtersStr := string(filtersJSON)

	msg := NewMessage(0) // job ID filled in after Create assigns one
	job := &ExportJob{
		JobType: JobTypeInventoryTransactions, Status: StatusPending,
		RequestedBy: userID, FiltersJSON: &filtersStr, MessageID: msg.MessageID,
	}
	if err := s.repo.Create(ctx, job); err != nil {
		return nil, err
	}
	msg.Payload.JobID = job.ID

	body, err := msg.Marshal()
	if err != nil {
		return nil, err
	}
	if err := s.publisher.Publish(ctx, s.exchange, s.routingKey, "application/json", body); err != nil {
		return nil, err
	}

	return job, nil
}

// GetJob returns job id, only for its own requester — another user's
// job is reported as ErrNotFound, the same no-existence-leak pattern
// Phase 21's notification.Service.MarkRead already established.
func (s *Service) GetJob(ctx context.Context, id, userID int64) (*Response, error) {
	job, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.RequestedBy != userID {
		return nil, ErrNotFound
	}
	return toResponse(job), nil
}

// ResolveDownload returns the server-side file path and a safe download
// filename for a COMPLETED job the caller owns.
func (s *Service) ResolveDownload(ctx context.Context, id, userID int64) (filePath, fileName string, err error) {
	job, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	if job.RequestedBy != userID {
		return "", "", ErrNotFound
	}
	if job.Status != StatusCompleted || job.FilePath == nil {
		return "", "", ErrNotReady
	}
	name := "export.csv"
	if job.FileName != nil {
		name = *job.FileName
	}
	return *job.FilePath, name, nil
}

// ProcessJob is the worker's own entry point (cmd/worker calls this
// once per consumed message). Claims the job (see Repository.Claim's
// own doc for the redelivery-safety guarantee); if the claim doesn't
// succeed, this is a no-op success (already processed or already
// in flight) — the caller ACKs either way. On a claimed job, decodes
// filters_json, queries the ledger, writes the CSV to exportDir, and
// marks the job COMPLETED — or MarkFailed with the error, never left
// silently stuck.
func (s *Service) ProcessJob(ctx context.Context, jobID int64) error {
	now := time.Now().UTC()
	claimed, err := s.repo.Claim(ctx, jobID, now)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	job, err := s.repo.FindByID(ctx, jobID)
	if err != nil {
		return err
	}

	var filters inventory.TransactionExportFilters
	if job.FiltersJSON != nil {
		if err := json.Unmarshal([]byte(*job.FiltersJSON), &filters); err != nil {
			return s.fail(ctx, jobID, "invalid filters: "+err.Error())
		}
	}

	rows, err := s.transactions.ListTransactionsForExport(ctx, filters)
	if err != nil {
		return s.fail(ctx, jobID, "query failed: "+err.Error())
	}

	fileName := csvexport.SafeFilename(fmt.Sprintf("inventory_transactions_%d_%s.csv", jobID, now.Format("20060102_150405")))
	filePath := filepath.Join(s.exportDir, fileName)

	if err := s.writeCSV(filePath, rows); err != nil {
		return s.fail(ctx, jobID, "file write failed: "+err.Error())
	}

	if err := s.repo.MarkCompleted(ctx, jobID, fileName, filePath, len(rows), time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

func (s *Service) fail(ctx context.Context, jobID int64, message string) error {
	return s.repo.MarkFailed(ctx, jobID, message, time.Now().UTC())
}

func (s *Service) writeCSV(path string, rows []inventory.TransactionExportRow) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	headers := []string{"transaction_type", "sku", "product_name", "warehouse_code", "location_code", "lot_no", "quantity", "reference_type", "reference_id", "reason", "created_by_name", "created_at"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			string(r.TransactionType), r.SKU, r.ProductName, r.WarehouseCode, r.LocationCode, r.LotNo,
			csvexport.FormatFloat(r.Quantity), csvexport.FormatString(r.ReferenceType),
			csvexport.FormatString(r.ReferenceID), csvexport.FormatString(r.Reason),
			csvexport.FormatString(r.CreatedByName), csvexport.FormatTimeValue(r.CreatedAt),
		})
	}
	return csvexport.WriteTo(f, headers, csvRows)
}

func toResponse(job *ExportJob) *Response {
	return &Response{
		ID: job.ID, JobType: job.JobType, Status: job.Status,
		FileName: job.FileName, RowCount: job.RowCount, ErrorMessage: job.ErrorMessage,
		CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, CompletedAt: job.CompletedAt,
	}
}

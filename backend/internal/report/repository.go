package report

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("report: not found")

// Repository is a thin GORM wrapper — the claim decision, filter
// marshaling, and CSV generation itself all live in Service/the worker.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, job *ExportJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*ExportJob, error) {
	var job ExportJob
	err := r.db.WithContext(ctx).First(&job, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// Claim atomically transitions job id from PENDING to PROCESSING,
// stamping started_at — reports true only if this call actually made
// that transition. A RabbitMQ redelivery of the same message calls this
// again and gets false (the job is already PROCESSING or terminal), so
// the worker never regenerates a file it already started or finished —
// the same conditional-UPDATE claim pattern Phase 21's own
// ClaimReminder already established for this codebase's other async
// consumer.
func (r *Repository) Claim(ctx context.Context, id int64, startedAt time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE export_jobs SET status = 'PROCESSING', started_at = ?
		WHERE id = ? AND status = 'PENDING'
	`, startedAt, id)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// MarkCompleted records a successful generation.
func (r *Repository) MarkCompleted(ctx context.Context, id int64, fileName, filePath string, rowCount int, completedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&ExportJob{}).Where("id = ?", id).Updates(map[string]any{
		"status": StatusCompleted, "file_name": fileName, "file_path": filePath,
		"row_count": rowCount, "completed_at": completedAt,
	}).Error
}

// MarkFailed records a failed generation — "failed jobs must be
// visible" (requirement): the job row stays queryable via GET
// /reports/export-jobs/{id} with status=FAILED and error_message set,
// never silently dropped.
func (r *Repository) MarkFailed(ctx context.Context, id int64, errMsg string, completedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&ExportJob{}).Where("id = ?", id).Updates(map[string]any{
		"status": StatusFailed, "error_message": errMsg, "completed_at": completedAt,
	}).Error
}

// ListForUser returns id's own export jobs, newest first — backs a
// simple "my exports" list if the frontend needs one beyond polling a
// single job it just created.
func (r *Repository) ListForUser(ctx context.Context, userID int64, limit int) ([]ExportJob, error) {
	var jobs []ExportJob
	err := r.db.WithContext(ctx).Where("requested_by = ?", userID).
		Order("id DESC").Limit(limit).Find(&jobs).Error
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

package notification

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

// ErrNotFound is returned by repository lookups that find no matching row.
var ErrNotFound = errors.New("notification: not found")

// Repository is a thin GORM wrapper — business rules (ownership checks,
// recipient resolution, the idempotent-create decision) live in Service.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a Repository bound to the shared *gorm.DB.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateIdempotent inserts n. On a (message_id, user_id) unique
// violation — this exact recipient already has a notification for this
// exact message, i.e. a redelivery — it reports created=false with no
// error rather than propagating ErrDuplicate: the caller (Service.
// NotifyStockAdjustmentReminder) treats "already exists" as a
// successful idempotent no-op, not a failure, the same shape every
// other idempotency guard in this codebase (e.g. stockadjustment's own
// InsertConfirmation) already uses.
func (r *Repository) CreateIdempotent(ctx context.Context, n *Notification) (bool, error) {
	err := r.db.WithContext(ctx).Create(n).Error
	if err != nil {
		if isUniqueViolation(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Notification, error) {
	var n Notification
	err := r.db.WithContext(ctx).First(&n, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// SetRead unconditionally marks id read at readAt — Service.MarkRead
// only ever calls this after its own ownership + already-read checks,
// the same guard-in-Service-then-write-in-Repository split every other
// package in this codebase uses.
func (r *Repository) SetRead(ctx context.Context, id int64, readAt time.Time) error {
	return r.db.WithContext(ctx).Model(&Notification{}).Where("id = ?", id).Updates(map[string]any{
		"is_read": true,
		"read_at": readAt,
	}).Error
}

// List returns the caller's own notifications, newest first.
func (r *Repository) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	query := r.db.WithContext(ctx).Model(&Notification{}).Where("user_id = ?", params.UserID)
	if params.IsRead != nil {
		query = query.Where("is_read = ?", *params.IsRead)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Notification
	offset := (params.Page - 1) * params.Limit
	if err := query.Order("id DESC").Offset(offset).Limit(params.Limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(rows))
	for _, n := range rows {
		responses = append(responses, Response{
			ID: n.ID, Type: n.Type, ReferenceType: n.ReferenceType, ReferenceID: n.ReferenceID,
			Title: n.Title, Message: n.Message, IsRead: n.IsRead, CreatedAt: n.CreatedAt,
		})
	}
	return responses, total, nil
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr), the same as every
// other package's own idempotency guard in this codebase.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

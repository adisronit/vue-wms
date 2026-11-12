package notification

import (
	"context"
	"fmt"
	"time"
)

// recipientResolver is the narrow slice of internal/user's own
// Repository this package needs — the same narrow-interface-over-
// another-package's-Repository pattern every cross-package dependency
// in this codebase already uses (e.g. stockadjustment's productChecker/
// locationChecker).
type recipientResolver interface {
	FindActiveIDsWithPermission(ctx context.Context, permissionCode string) ([]int64, error)
}

// StockAdjustmentApprovePermission defines who receives a stock-
// adjustment pending-reminder notification. stock_adjustment.approve,
// not .view: it is the exact permission POST
// /stock-adjustments/{id}/confirm itself requires (internal/
// stockadjustment/handler.go), so a recipient is always someone who can
// actually resolve the adjustment, never a spectator who can only look.
// Inspected internal/rbac's own catalog before choosing this — only
// ADMIN and WAREHOUSE_MANAGER are granted it today (RECEIVING/QC/
// PUTAWAY/PICKING/SHIPPING have no stock_adjustment.* grant at all, per
// Phase 2's own seed data), giving a clean, existing approval-role
// mapping with nothing hardcoded to a specific role code.
const StockAdjustmentApprovePermission = "stock_adjustment.approve"

// Service holds notification business rules — recipient resolution,
// the idempotent-create decision for the reminder consumer, and the
// ownership check for mark-read.
type Service struct {
	repo  *Repository
	users recipientResolver
}

// NewService builds a Service bound to repo and a recipient resolver.
func NewService(repo *Repository, users recipientResolver) *Service {
	return &Service{repo: repo, users: users}
}

// List returns the authenticated caller's own notifications.
func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	params.Page, params.Limit = page, limit
	return s.repo.List(ctx, params)
}

// MarkRead marks notification id read, only for its own owner (userID).
// A notification belonging to someone else is reported as ErrNotFound,
// not a permission error — this deliberately does not confirm to the
// caller that a given notification ID exists at all for another user.
// Already-read is a no-op success (idempotent, matching requirement 11),
// not an error.
func (s *Service) MarkRead(ctx context.Context, id, userID int64) (*Response, error) {
	n, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if n.UserID != userID {
		return nil, ErrNotFound
	}

	if !n.IsRead {
		now := time.Now().UTC()
		if err := s.repo.SetRead(ctx, id, now); err != nil {
			return nil, err
		}
		n.IsRead = true
		n.ReadAt = &now
	}

	return &Response{
		ID: n.ID, Type: n.Type, ReferenceType: n.ReferenceType, ReferenceID: n.ReferenceID,
		Title: n.Title, Message: n.Message, IsRead: n.IsRead, CreatedAt: n.CreatedAt,
	}, nil
}

// StockAdjustmentReminderPayload is this package's own narrow view of
// the fields a reminder notification needs to render — deliberately not
// internal/stockadjustment's own ReminderPayload type, matching this
// codebase's established convention (see e.g. internal/stockadjustment's
// own SourceCountSummary) of a narrow package-local shape instead of
// importing another domain package's type directly, so internal/
// notification never depends on internal/stockadjustment. cmd/worker
// maps the decoded RabbitMQ message into this before calling
// NotifyStockAdjustmentReminder.
type StockAdjustmentReminderPayload struct {
	AdjustmentID int64
	AdjustmentNo string
	CreatedBy    *int64
	PendingSince time.Time
	AgeHours     int
}

// NotifyStockAdjustmentReminder is the reminder consumer's entry point
// (cmd/worker calls this once per consumed message, before acking).
// Idempotent by construction: resolves every ACTIVE user holding
// StockAdjustmentApprovePermission, then attempts one CreateIdempotent
// insert per recipient tagged with the message's own messageID — a
// redelivery of the same message re-attempts the identical set of
// inserts, each independently rejected as an already-processed
// duplicate by the (message_id, user_id) unique constraint (migrations/
// 0033), never producing a second notification for the same recipient.
//
// Known limitation: if an insert fails partway through the recipient
// list (a transient DB error, not a duplicate), this returns an error
// and the caller NACKs the whole message without requeue (matching this
// codebase's existing no-DLQ consumer pattern — see
// internal/messaging/rabbitmq/consumer.go's own doc) — recipients
// already inserted keep their notification (idempotency protects them
// from ever getting a second one), but recipients after the failure
// point get none until the adjustment's next cooldown-eligible reminder
// cycle. Documented, not solved with per-recipient retry, to keep this
// phase's own scope to "narrowly scoped reminders," not a generic
// guaranteed-delivery framework.
func (s *Service) NotifyStockAdjustmentReminder(ctx context.Context, messageID string, payload StockAdjustmentReminderPayload) error {
	recipients, err := s.users.FindActiveIDsWithPermission(ctx, StockAdjustmentApprovePermission)
	if err != nil {
		return err
	}

	refType := "STOCK_ADJUSTMENT"
	title, message := buildReminderNotificationText(payload)

	for _, userID := range recipients {
		if _, err := s.repo.CreateIdempotent(ctx, &Notification{
			Type: TypeStockAdjustmentReminder, UserID: userID,
			ReferenceType: &refType, ReferenceID: &payload.AdjustmentID,
			Title: title, Message: message, MessageID: messageID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// buildReminderNotificationText is the pure text-construction piece of
// NotifyStockAdjustmentReminder, extracted so the exact wording is
// unit-testable independent of the DB/recipient-resolution it's used
// inside — the same "extract the pure part" precedent Phase 18/20's own
// applyCancel/pendingAging already established in this codebase.
func buildReminderNotificationText(payload StockAdjustmentReminderPayload) (title, message string) {
	title = "Pending Stock Adjustment Reminder"
	message = fmt.Sprintf("%s has been pending for %dh and is overdue.", payload.AdjustmentNo, payload.AgeHours)
	return title, message
}

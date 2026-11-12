// Phase 21 — overdue PENDING stock-adjustment reminder delivery. This
// file owns the message shape and the scan-and-claim Service method;
// internal/messaging/rabbitmq owns the queue/exchange plumbing (see its
// topology.go), and internal/notification owns turning a consumed
// message into a persisted, per-recipient notification row. Nothing
// here talks to RabbitMQ directly — cmd/server/main.go's scanner
// publishes whatever ClaimAndListOverdueForReminder returns.
package stockadjustment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ReminderEventType identifies this message shape on the wire — the
// EventType CLAUDE.md's own recommended envelope calls for.
const ReminderEventType = "stock_adjustment.pending_reminder"

// ReminderCandidate is the lean shape Phase 21's reminder pipeline
// carries end to end — deliberately narrower than Response/ListResponse:
// this phase's own instruction is that a reminder message must contain
// only the minimum identifiers/context needed, never a full adjustment
// row.
type ReminderCandidate struct {
	ID           int64
	AdjustmentNo string
	CreatedBy    *int64
	PendingSince time.Time
	AgeHours     int
}

// ReminderMessage is the RabbitMQ envelope for one reminder job,
// matching CLAUDE.md's own recommended shape (message_id/event_type/
// created_at/correlation_id/payload) — the same envelope
// internal/messaging/rabbitmq.DevHeartbeat already established as this
// codebase's one prior message type.
type ReminderMessage struct {
	MessageID     string          `json:"message_id"`
	EventType     string          `json:"event_type"`
	CreatedAt     time.Time       `json:"created_at"`
	CorrelationID string          `json:"correlation_id"`
	Payload       ReminderPayload `json:"payload"`
}

// ReminderPayload is intentionally minimal — see ReminderMessage's own
// doc. CreatedBy is the adjustment's own creator (context for the
// notification text), not a recipient list: the worker resolves who
// actually gets notified (every user holding stock_adjustment.approve)
// itself at consume time, nothing about recipients is published here.
type ReminderPayload struct {
	AdjustmentID int64     `json:"adjustment_id,string"`
	AdjustmentNo string    `json:"adjustment_no"`
	CreatedBy    *int64    `json:"created_by,string"`
	PendingSince time.Time `json:"pending_since"`
	AgeHours     int       `json:"age_hours"`
}

// NewReminderMessage builds a fresh envelope for one candidate — a new
// MessageID/CorrelationID per call, the same per-call-fresh-IDs
// convention rabbitmq.NewDevHeartbeat already uses.
func NewReminderMessage(c ReminderCandidate) ReminderMessage {
	return ReminderMessage{
		MessageID:     uuid.NewString(),
		EventType:     ReminderEventType,
		CreatedAt:     time.Now().UTC(),
		CorrelationID: uuid.NewString(),
		Payload: ReminderPayload{
			AdjustmentID: c.ID,
			AdjustmentNo: c.AdjustmentNo,
			CreatedBy:    c.CreatedBy,
			PendingSince: c.PendingSince,
			AgeHours:     c.AgeHours,
		},
	}
}

// Marshal encodes the message as the JSON body a Publisher sends.
func (m ReminderMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// ClaimAndListOverdueForReminder is Phase 21's scan-and-claim step:
// finds every PENDING adjustment overdue by the exact same
// pendingWarningHours threshold Phase 20's own is_overdue uses (calls
// the identical overdueCutoff helper — no duplicated aging calculation),
// then attempts to claim each one (see Repository.ClaimReminder for the
// cooldown rule: the same pendingWarningHours value doubles as the
// reminder cooldown, so a still-unresolved adjustment is reminded again
// every time it accumulates another full warning-hours period beyond
// its last reminder). Returns only the candidates that were actually
// claimed — the ones the caller should publish; a candidate that lost
// its claim (still within cooldown) is silently omitted, not an error.
func (s *Service) ClaimAndListOverdueForReminder(ctx context.Context) ([]ReminderCandidate, error) {
	now := time.Now().UTC()
	cutoff := overdueCutoff(now, s.pendingWarningHours)

	candidates, err := s.repo.FindOverduePending(ctx, cutoff)
	if err != nil {
		return nil, err
	}

	cooldown := time.Duration(s.pendingWarningHours) * time.Hour
	claimed := make([]ReminderCandidate, 0, len(candidates))
	for _, c := range candidates {
		ok, err := s.repo.ClaimReminder(ctx, c.ID, cooldown)
		if err != nil {
			return nil, err
		}
		if ok {
			claimed = append(claimed, c)
		}
	}
	return claimed, nil
}

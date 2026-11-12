package notification

import (
	"testing"
	"time"
)

// --- pure text-construction test (no DB) — the DB-touching parts of
// NotifyStockAdjustmentReminder/MarkRead/List (recipient resolution,
// the idempotent-create decision, the ownership check's actual read)
// are verified live against the real SQL Server and RabbitMQ instead,
// the same split every DB-facing phase in this codebase uses (see
// PROJECT_STATUS.md's own Phase 17/18/20 "Tests" sections for the exact
// same reasoning). ---

func TestBuildReminderNotificationText(t *testing.T) {
	payload := StockAdjustmentReminderPayload{
		AdjustmentID: 123,
		AdjustmentNo: "ADJ-000123",
		PendingSince: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		AgeHours:     52,
	}

	title, message := buildReminderNotificationText(payload)

	if title != "Pending Stock Adjustment Reminder" {
		t.Errorf("title = %q, want %q", title, "Pending Stock Adjustment Reminder")
	}
	wantMessage := "ADJ-000123 has been pending for 52h and is overdue."
	if message != wantMessage {
		t.Errorf("message = %q, want %q", message, wantMessage)
	}
}

func TestBuildReminderNotificationText_DifferentAdjustment(t *testing.T) {
	payload := StockAdjustmentReminderPayload{AdjustmentNo: "ADJ-999999", AgeHours: 100}

	_, message := buildReminderNotificationText(payload)

	wantMessage := "ADJ-999999 has been pending for 100h and is overdue."
	if message != wantMessage {
		t.Errorf("message = %q, want %q", message, wantMessage)
	}
}

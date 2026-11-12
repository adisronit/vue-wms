package report

import "testing"

// --- pure message-construction tests (no DB/RabbitMQ needed — the
// claim/idempotency logic itself (Repository.Claim) and CSV generation
// are verified live, the same split every DB/queue-facing change in
// this codebase already uses). ---

func TestNewMessage(t *testing.T) {
	msg := NewMessage(123)

	if msg.EventType != EventTypeExportJob {
		t.Errorf("EventType = %q, want %q", msg.EventType, EventTypeExportJob)
	}
	if msg.MessageID == "" || msg.CorrelationID == "" {
		t.Error("expected non-empty MessageID and CorrelationID")
	}
	if msg.Payload.JobID != 123 {
		t.Errorf("Payload.JobID = %d, want 123", msg.Payload.JobID)
	}
}

func TestNewMessage_UniqueIDsPerCall(t *testing.T) {
	first := NewMessage(1)
	second := NewMessage(1)

	if first.MessageID == second.MessageID {
		t.Error("expected a fresh MessageID per call, got the same value twice")
	}
}

func TestMessage_Marshal(t *testing.T) {
	msg := NewMessage(1)

	body, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty marshaled body")
	}
}

package stockadjustment

import (
	"context"
	"errors"
	"testing"
	"time"

	"wms-backend/internal/location"
	"wms-backend/internal/product"
)

// --- pure state-machine tests (no DB, no fakes needed) ---

func TestStatus_CanEdit(t *testing.T) {
	cases := map[Status]bool{
		StatusDraft: true, StatusPending: false, StatusApproved: false, StatusRejected: false,
	}
	for status, want := range cases {
		if got := status.CanEdit(); got != want {
			t.Errorf("Status(%q).CanEdit() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanConfirm(t *testing.T) {
	cases := map[Status]bool{
		StatusDraft: false, StatusPending: true, StatusApproved: false, StatusRejected: false,
	}
	for status, want := range cases {
		if got := status.CanConfirm(); got != want {
			t.Errorf("Status(%q).CanConfirm() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusDraft: true, StatusPending: true, StatusApproved: true, StatusRejected: true,
		"NOT_REAL": false, "": false,
	}
	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

func TestAdjustType_IsValid(t *testing.T) {
	cases := map[AdjustType]bool{
		AdjustIncrease: true, AdjustDecrease: true, "SET": false, "": false,
	}
	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("AdjustType(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

func TestReason_IsValid(t *testing.T) {
	cases := map[Reason]bool{
		ReasonCycleCountVariance: true, ReasonDamagedGoods: true, ReasonLostGoods: true,
		ReasonFoundGoods: true, ReasonSystemCorrection: true, "Made Up Reason": false, "": false,
	}
	for reason, want := range cases {
		if got := reason.IsValid(); got != want {
			t.Errorf("Reason(%q).IsValid() = %v, want %v", reason, got, want)
		}
	}
}

// --- Phase 20: pendingAging / cutoff helpers (no DB needed — the
// filters that use these cutoffs are verified live against the real
// SQL Server instead, the same split every prior phase's own DB-facing
// change uses: pure arithmetic unit-tested here, the query itself
// verified live). ---

func TestPendingAging_NonPendingNeverOverdue(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	veryOld := now.Add(-1000 * time.Hour)

	for _, status := range []Status{StatusDraft, StatusApproved, StatusRejected} {
		aging := pendingAging(status, veryOld, now, 48)
		if aging.IsOverdue {
			t.Errorf("Status(%q): IsOverdue = true, want false regardless of age", status)
		}
		if aging.PendingSince != nil || aging.AgeHours != nil || aging.AgeDays != nil {
			t.Errorf("Status(%q): expected nil PendingSince/AgeHours/AgeDays, got %v/%v/%v",
				status, aging.PendingSince, aging.AgeHours, aging.AgeDays)
		}
		if aging.PendingWarningHours != 48 {
			t.Errorf("Status(%q): PendingWarningHours = %d, want 48 (still echoed even when not pending)", status, aging.PendingWarningHours)
		}
	}
}

func TestPendingAging_AgeCalculation(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(-30 * time.Hour) // 1 day 6 hours ago

	aging := pendingAging(StatusPending, updatedAt, now, 48)

	if aging.PendingSince == nil || !aging.PendingSince.Equal(updatedAt) {
		t.Errorf("PendingSince = %v, want %v", aging.PendingSince, updatedAt)
	}
	if aging.AgeHours == nil || *aging.AgeHours != 30 {
		t.Errorf("AgeHours = %v, want 30", aging.AgeHours)
	}
	if aging.AgeDays == nil || *aging.AgeDays != 1.3 {
		t.Errorf("AgeDays = %v, want 1.3 (30h/24)", aging.AgeDays)
	}
}

func TestPendingAging_OverdueThreshold(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		hoursPending int
		wantOverdue  bool
	}{
		{"one hour under threshold", 47, false},
		{"exactly at threshold", 48, true},
		{"one hour over threshold", 49, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updatedAt := now.Add(-time.Duration(tc.hoursPending) * time.Hour)
			aging := pendingAging(StatusPending, updatedAt, now, 48)
			if aging.IsOverdue != tc.wantOverdue {
				t.Errorf("IsOverdue = %v, want %v (pending %dh, threshold 48h)", aging.IsOverdue, tc.wantOverdue, tc.hoursPending)
			}
		})
	}
}

func TestOverdueCutoff(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	got := overdueCutoff(now, 48)
	want := time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("overdueCutoff(now, 48) = %v, want %v", got, want)
	}
}

func TestOlderThanCutoffTime(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	got := olderThanCutoffTime(now, 3)
	want := time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("olderThanCutoffTime(now, 3) = %v, want %v", got, want)
	}
}

// --- Phase 21: reminder message construction (no DB/RabbitMQ needed —
// FindOverduePending/ClaimReminder's own SQL and the actual publish are
// verified live, the same split as every other DB/queue-facing change
// in this codebase). ---

func TestNewReminderMessage(t *testing.T) {
	createdBy := int64(7)
	pendingSince := time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC)
	candidate := ReminderCandidate{
		ID: 123, AdjustmentNo: "ADJ-000123", CreatedBy: &createdBy,
		PendingSince: pendingSince, AgeHours: 50,
	}

	msg := NewReminderMessage(candidate)

	if msg.EventType != ReminderEventType {
		t.Errorf("EventType = %q, want %q", msg.EventType, ReminderEventType)
	}
	if msg.MessageID == "" || msg.CorrelationID == "" {
		t.Error("expected non-empty MessageID and CorrelationID")
	}
	if msg.Payload.AdjustmentID != 123 || msg.Payload.AdjustmentNo != "ADJ-000123" {
		t.Errorf("Payload adjustment fields = %+v, want ID=123 No=ADJ-000123", msg.Payload)
	}
	if msg.Payload.CreatedBy == nil || *msg.Payload.CreatedBy != 7 {
		t.Errorf("Payload.CreatedBy = %v, want 7", msg.Payload.CreatedBy)
	}
	if !msg.Payload.PendingSince.Equal(pendingSince) {
		t.Errorf("Payload.PendingSince = %v, want %v", msg.Payload.PendingSince, pendingSince)
	}
	if msg.Payload.AgeHours != 50 {
		t.Errorf("Payload.AgeHours = %d, want 50", msg.Payload.AgeHours)
	}
}

func TestNewReminderMessage_UniqueIDsPerCall(t *testing.T) {
	candidate := ReminderCandidate{ID: 1, AdjustmentNo: "ADJ-000001"}

	first := NewReminderMessage(candidate)
	second := NewReminderMessage(candidate)

	if first.MessageID == second.MessageID {
		t.Error("expected a fresh MessageID per call, got the same value twice")
	}
}

func TestReminderMessage_Marshal(t *testing.T) {
	candidate := ReminderCandidate{ID: 1, AdjustmentNo: "ADJ-000001", AgeHours: 60}
	msg := NewReminderMessage(candidate)

	body, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty marshaled body")
	}
}

// --- Create's validation-only paths: run before s.inventory/s.repo are
// ever touched (validateItemFields checks adjust_type/qty/reason and
// product/location existence first, all pure or fake-backed), so
// they're safe to test with repo/inventory left nil. ---

type fakeProducts struct {
	err error
}

func (f *fakeProducts) FindByID(context.Context, int64) (*product.Product, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &product.Product{ID: 1}, nil
}

type fakeLocations struct {
	err error
}

func (f *fakeLocations) FindByID(context.Context, int64) (*location.Location, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &location.Location{ID: 1}, nil
}

func TestCreate_RejectsInvalidAdjustType(t *testing.T) {
	svc := NewService(nil, &fakeProducts{}, &fakeLocations{}, nil, 48)

	_, err := svc.Create(context.Background(), CreateRequest{
		ProductID: 1, LocationID: 1, AdjustType: "SET", AdjustQty: 10, Reason: ReasonFoundGoods,
	}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsZeroAdjustQty(t *testing.T) {
	svc := NewService(nil, &fakeProducts{}, &fakeLocations{}, nil, 48)

	_, err := svc.Create(context.Background(), CreateRequest{
		ProductID: 1, LocationID: 1, AdjustType: AdjustIncrease, AdjustQty: 0, Reason: ReasonFoundGoods,
	}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidReason(t *testing.T) {
	svc := NewService(nil, &fakeProducts{}, &fakeLocations{}, nil, 48)

	_, err := svc.Create(context.Background(), CreateRequest{
		ProductID: 1, LocationID: 1, AdjustType: AdjustIncrease, AdjustQty: 10, Reason: "Not A Real Reason",
	}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsUnknownProduct(t *testing.T) {
	svc := NewService(nil, &fakeProducts{err: errors.New("not found")}, &fakeLocations{}, nil, 48)

	_, err := svc.Create(context.Background(), CreateRequest{
		ProductID: 999, LocationID: 1, AdjustType: AdjustIncrease, AdjustQty: 10, Reason: ReasonFoundGoods,
	}, 1)

	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("error = %v, want ErrProductNotFound", err)
	}
}

func TestCreate_RejectsUnknownLocation(t *testing.T) {
	svc := NewService(nil, &fakeProducts{}, &fakeLocations{err: errors.New("not found")}, nil, 48)

	_, err := svc.Create(context.Background(), CreateRequest{
		ProductID: 1, LocationID: 999, AdjustType: AdjustIncrease, AdjustQty: 10, Reason: ReasonFoundGoods,
	}, 1)

	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("error = %v, want ErrLocationNotFound", err)
	}
}

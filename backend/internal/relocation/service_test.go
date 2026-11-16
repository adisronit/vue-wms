package relocation

import (
	"testing"
	"time"
)

func TestStatus_Transitions(t *testing.T) {
	cases := []struct {
		status      Status
		canAddItems bool
		canReady    bool
		canConfirm  bool
		canCancel   bool
	}{
		{StatusDraft, true, true, false, true},
		{StatusReady, false, false, true, true},
		{StatusInProgress, false, false, true, true},
		{StatusCompleted, false, false, false, false},
		{StatusCancelled, false, false, false, false},
	}
	for _, c := range cases {
		if got := c.status.CanAddItems(); got != c.canAddItems {
			t.Errorf("%s.CanAddItems() = %v, want %v", c.status, got, c.canAddItems)
		}
		if got := c.status.CanReady(); got != c.canReady {
			t.Errorf("%s.CanReady() = %v, want %v", c.status, got, c.canReady)
		}
		if got := c.status.CanConfirm(); got != c.canConfirm {
			t.Errorf("%s.CanConfirm() = %v, want %v", c.status, got, c.canConfirm)
		}
		if got := c.status.CanCancel(); got != c.canCancel {
			t.Errorf("%s.CanCancel() = %v, want %v", c.status, got, c.canCancel)
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	for _, s := range []Status{StatusDraft, StatusReady, StatusInProgress, StatusCompleted, StatusCancelled} {
		if !s.IsValid() {
			t.Errorf("%s.IsValid() = false, want true", s)
		}
	}
	if Status("BOGUS").IsValid() {
		t.Error(`Status("BOGUS").IsValid() = true, want false`)
	}
}

func TestItemStatus(t *testing.T) {
	cases := []struct {
		name         string
		requestedQty float64
		movedQty     float64
		want         ItemStatus
	}{
		{"nothing moved", 100, 0, ItemStatusPending},
		{"partially moved", 100, 40, ItemStatusPartiallyCompleted},
		{"fully moved", 100, 100, ItemStatusCompleted},
		{"over-moved (defensive)", 100, 100.0001, ItemStatusCompleted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := itemStatus(c.requestedQty, c.movedQty); got != c.want {
				t.Errorf("itemStatus(%v, %v) = %v, want %v", c.requestedQty, c.movedQty, got, c.want)
			}
		})
	}
}

func TestComputeTaskStatus(t *testing.T) {
	cases := []struct {
		name  string
		items []TaskItem
		want  Status
	}{
		{"no items", nil, StatusCompleted},
		{"all pending", []TaskItem{{Status: ItemStatusPending}, {Status: ItemStatusPending}}, StatusInProgress},
		{"one partial", []TaskItem{{Status: ItemStatusCompleted}, {Status: ItemStatusPartiallyCompleted}}, StatusInProgress},
		{"all completed", []TaskItem{{Status: ItemStatusCompleted}, {Status: ItemStatusCompleted}}, StatusCompleted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := computeTaskStatus(c.items); got != c.want {
				t.Errorf("computeTaskStatus(%v) = %v, want %v", c.items, got, c.want)
			}
		})
	}
}

func TestTask_ApplyCancel(t *testing.T) {
	task := &Task{Status: StatusInProgress}
	remark := "no longer needed"
	now, err := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	task.applyCancel(42, &remark, now)

	if task.Status != StatusCancelled {
		t.Errorf("Status = %v, want CANCELLED", task.Status)
	}
	if task.CancelledBy == nil || *task.CancelledBy != 42 {
		t.Errorf("CancelledBy = %v, want 42", task.CancelledBy)
	}
	if task.CancelledAt == nil || !task.CancelledAt.Equal(now) {
		t.Errorf("CancelledAt = %v, want %v", task.CancelledAt, now)
	}
	if task.CancelRemark == nil || *task.CancelRemark != remark {
		t.Errorf("CancelRemark = %v, want %q", task.CancelRemark, remark)
	}
}

func TestTask_ApplyCancel_EmptyRemarkLeavesExistingUntouched(t *testing.T) {
	existing := "pre-existing remark"
	task := &Task{Status: StatusDraft, CancelRemark: &existing}
	empty := ""
	now, err := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	task.applyCancel(1, &empty, now)

	if task.CancelRemark == nil || *task.CancelRemark != existing {
		t.Errorf("CancelRemark = %v, want unchanged %q", task.CancelRemark, existing)
	}
}

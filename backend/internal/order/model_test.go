package order

import (
	"testing"
	"time"
)

func TestStatus_CanCreatePickingTask(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusPicking: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanCreatePickingTask(); got != want {
			t.Errorf("Status(%q).CanCreatePickingTask() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanCancel(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusPicking: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanCancel(); got != want {
			t.Errorf("Status(%q).CanCancel() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanUpdate(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusPicking: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanUpdate(); got != want {
			t.Errorf("Status(%q).CanUpdate() = %v, want %v", status, got, want)
		}
	}
}

func TestOrder_ApplyCancel(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("sets status and audit fields from a non-empty remark", func(t *testing.T) {
		o := &Order{Status: StatusPending}
		o.applyCancel(42, "customer changed mind", now)

		if o.Status != StatusCancelled {
			t.Errorf("Status = %q, want %q", o.Status, StatusCancelled)
		}
		if o.CancelledAt == nil || !o.CancelledAt.Equal(now) {
			t.Errorf("CancelledAt = %v, want %v", o.CancelledAt, now)
		}
		if o.CancelledBy == nil || *o.CancelledBy != 42 {
			t.Errorf("CancelledBy = %v, want 42", o.CancelledBy)
		}
		if o.CancelRemark == nil || *o.CancelRemark != "customer changed mind" {
			t.Errorf("CancelRemark = %v, want \"customer changed mind\"", o.CancelRemark)
		}
	})

	t.Run("empty remark leaves CancelRemark unset", func(t *testing.T) {
		o := &Order{Status: StatusPending}
		o.applyCancel(7, "", now)
		if o.CancelRemark != nil {
			t.Errorf("CancelRemark = %v, want nil", o.CancelRemark)
		}
	})
}

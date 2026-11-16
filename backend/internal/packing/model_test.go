package packing

import (
	"testing"
	"time"
)

func TestStatus_Transitions(t *testing.T) {
	cases := []struct {
		status            Status
		canAssign         bool
		canAddCarton      bool
		canPack           bool
		canCheckerConfirm bool
		canCancel         bool
	}{
		{StatusDraft, true, true, true, false, true},
		{StatusInProgress, true, true, true, true, true},
		{StatusCompleted, false, false, false, false, false},
		{StatusCancelled, false, false, false, false, false},
	}
	for _, c := range cases {
		if got := c.status.CanAssign(); got != c.canAssign {
			t.Errorf("%s.CanAssign() = %v, want %v", c.status, got, c.canAssign)
		}
		if got := c.status.CanAddCarton(); got != c.canAddCarton {
			t.Errorf("%s.CanAddCarton() = %v, want %v", c.status, got, c.canAddCarton)
		}
		if got := c.status.CanPack(); got != c.canPack {
			t.Errorf("%s.CanPack() = %v, want %v", c.status, got, c.canPack)
		}
		if got := c.status.CanCheckerConfirm(); got != c.canCheckerConfirm {
			t.Errorf("%s.CanCheckerConfirm() = %v, want %v", c.status, got, c.canCheckerConfirm)
		}
		if got := c.status.CanCancel(); got != c.canCancel {
			t.Errorf("%s.CanCancel() = %v, want %v", c.status, got, c.canCancel)
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	for _, s := range []Status{StatusDraft, StatusInProgress, StatusCompleted, StatusCancelled} {
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
		name      string
		pickedQty float64
		packedQty float64
		exception bool
		want      ItemStatus
	}{
		{"nothing packed", 100, 0, false, ItemStatusPending},
		{"partially packed", 100, 40, false, ItemStatusPartiallyPacked},
		{"fully packed", 100, 100, false, ItemStatusPacked},
		{"exception overrides activity", 100, 40, true, ItemStatusException},
		{"exception with zero packed", 100, 0, true, ItemStatusException},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := itemStatus(c.pickedQty, c.packedQty, c.exception); got != c.want {
				t.Errorf("itemStatus(%v, %v, %v) = %v, want %v", c.pickedQty, c.packedQty, c.exception, got, c.want)
			}
		})
	}
}

func TestItemTerminal(t *testing.T) {
	cases := []struct {
		status ItemStatus
		want   bool
	}{
		{ItemStatusPending, false},
		{ItemStatusPartiallyPacked, false},
		{ItemStatusPacked, true},
		{ItemStatusException, true},
	}
	for _, c := range cases {
		if got := itemTerminal(c.status); got != c.want {
			t.Errorf("itemTerminal(%v) = %v, want %v", c.status, got, c.want)
		}
	}
}

func TestComputeJobStatus(t *testing.T) {
	cases := []struct {
		name  string
		items []Item
		want  Status
	}{
		{"no items", nil, StatusDraft},
		{"no activity", []Item{{PackedQty: 0, ExceptionFlag: false}, {PackedQty: 0, ExceptionFlag: false}}, StatusDraft},
		{"one packed", []Item{{PackedQty: 10}, {PackedQty: 0}}, StatusInProgress},
		{"one exception, zero packed", []Item{{PackedQty: 0, ExceptionFlag: true}}, StatusInProgress},
		{"every line terminal — still never auto-COMPLETED", []Item{{PackedQty: 10, PickedQty: 10}, {ExceptionFlag: true}}, StatusInProgress},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := computeJobStatus(c.items); got != c.want {
				t.Errorf("computeJobStatus(%v) = %v, want %v", c.items, got, c.want)
			}
		})
	}
}

func TestAllItemsTerminal(t *testing.T) {
	cases := []struct {
		name  string
		items []Item
		want  bool
	}{
		{"no items", nil, false},
		{"all packed", []Item{{Status: ItemStatusPacked}, {Status: ItemStatusPacked}}, true},
		{"mixed packed and exception", []Item{{Status: ItemStatusPacked}, {Status: ItemStatusException}}, true},
		{"one still pending", []Item{{Status: ItemStatusPacked}, {Status: ItemStatusPending}}, false},
		{"one partially packed", []Item{{Status: ItemStatusPacked}, {Status: ItemStatusPartiallyPacked}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := allItemsTerminal(c.items); got != c.want {
				t.Errorf("allItemsTerminal(%v) = %v, want %v", c.items, got, c.want)
			}
		})
	}
}

func TestJob_ApplyCancel(t *testing.T) {
	job := &Job{Status: StatusInProgress}
	remark := "customer cancelled the order"
	now, err := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	job.applyCancel(42, &remark, now)

	if job.Status != StatusCancelled {
		t.Errorf("Status = %v, want CANCELLED", job.Status)
	}
	if job.CancelledBy == nil || *job.CancelledBy != 42 {
		t.Errorf("CancelledBy = %v, want 42", job.CancelledBy)
	}
	if job.CancelledAt == nil || !job.CancelledAt.Equal(now) {
		t.Errorf("CancelledAt = %v, want %v", job.CancelledAt, now)
	}
	if job.CancelRemark == nil || *job.CancelRemark != remark {
		t.Errorf("CancelRemark = %v, want %q", job.CancelRemark, remark)
	}
}

func TestJob_ApplyCancel_EmptyRemarkLeavesExistingUntouched(t *testing.T) {
	existing := "pre-existing remark"
	job := &Job{Status: StatusDraft, CancelRemark: &existing}
	empty := ""
	now := time.Now().UTC()
	job.applyCancel(1, &empty, now)

	if job.CancelRemark == nil || *job.CancelRemark != existing {
		t.Errorf("CancelRemark = %v, want unchanged %q", job.CancelRemark, existing)
	}
}

func TestExceptionReason_IsValid(t *testing.T) {
	for _, r := range []ExceptionReason{ExceptionDamaged, ExceptionShortage, ExceptionWrongItem, ExceptionOther} {
		if !r.IsValid() {
			t.Errorf("%s.IsValid() = false, want true", r)
		}
	}
	if ExceptionReason("BOGUS").IsValid() {
		t.Error(`ExceptionReason("BOGUS").IsValid() = true, want false`)
	}
}

func TestPackagingType_IsValid(t *testing.T) {
	for _, p := range []PackagingType{PackagingBox, PackagingPallet, PackagingBag, PackagingCrate, PackagingOther} {
		if !p.IsValid() {
			t.Errorf("%s.IsValid() = false, want true", p)
		}
	}
	if PackagingType("BOGUS").IsValid() {
		t.Error(`PackagingType("BOGUS").IsValid() = true, want false`)
	}
}

func TestCartonStatus_IsValid(t *testing.T) {
	if !CartonStatusOpen.IsValid() || !CartonStatusClosed.IsValid() {
		t.Error("OPEN/CLOSED should be valid")
	}
	if CartonStatus("SEALED").IsValid() {
		t.Error(`CartonStatus("SEALED").IsValid() = true, want false`)
	}
}

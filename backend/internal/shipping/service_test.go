package shipping

import (
	"context"
	"errors"
	"testing"
	"time"

	"wms-backend/internal/picking"
)

// --- pure state-machine tests (no DB, no fakes needed) ---

func TestStatus_CanEdit(t *testing.T) {
	cases := map[Status]bool{
		StatusReady: true, StatusDispatched: false, StatusInTransit: false,
		StatusDelivered: false, StatusIssue: false,
	}
	for status, want := range cases {
		if got := status.CanEdit(); got != want {
			t.Errorf("Status(%q).CanEdit() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanDispatch(t *testing.T) {
	cases := map[Status]bool{
		StatusReady: true, StatusDispatched: false, StatusInTransit: false,
		StatusDelivered: false, StatusIssue: false,
	}
	for status, want := range cases {
		if got := status.CanDispatch(); got != want {
			t.Errorf("Status(%q).CanDispatch() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanDeliver(t *testing.T) {
	cases := map[Status]bool{
		StatusReady: false, StatusDispatched: true, StatusInTransit: false,
		StatusDelivered: false, StatusIssue: false,
	}
	for status, want := range cases {
		if got := status.CanDeliver(); got != want {
			t.Errorf("Status(%q).CanDeliver() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanReportIssue(t *testing.T) {
	cases := map[Status]bool{
		StatusReady: true, StatusDispatched: true, StatusInTransit: false,
		StatusDelivered: false, StatusIssue: false,
	}
	for status, want := range cases {
		if got := status.CanReportIssue(); got != want {
			t.Errorf("Status(%q).CanReportIssue() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusReady: true, StatusDispatched: true, StatusInTransit: true,
		StatusDelivered: true, StatusIssue: true, StatusCancelled: true, "NOT_REAL": false, "": false,
	}
	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanCancel(t *testing.T) {
	cases := map[Status]bool{
		StatusReady: true, StatusDispatched: false, StatusInTransit: false,
		StatusDelivered: false, StatusIssue: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanCancel(); got != want {
			t.Errorf("Status(%q).CanCancel() = %v, want %v", status, got, want)
		}
	}
}

// TestShipment_ApplyCancel is Phase 18's own coverage for the audit-column
// mutation Service.Cancel delegates to — Cancel itself needs a live DB
// (via s.repo.Transaction), so this isolates the pure part: does the
// shipment end up with the right status/audit fields, and is an
// empty/nil remark correctly left out of CancelRemark rather than
// stored as an empty string.
func TestShipment_ApplyCancel(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("sets status and audit fields from a non-empty remark", func(t *testing.T) {
		s := &Shipment{Status: StatusReady}
		remark := "carrier booking cancelled"
		s.applyCancel(42, &remark, now)

		if s.Status != StatusCancelled {
			t.Errorf("Status = %q, want %q", s.Status, StatusCancelled)
		}
		if s.CancelledAt == nil || !s.CancelledAt.Equal(now) {
			t.Errorf("CancelledAt = %v, want %v", s.CancelledAt, now)
		}
		if s.CancelledBy == nil || *s.CancelledBy != 42 {
			t.Errorf("CancelledBy = %v, want 42", s.CancelledBy)
		}
		if s.CancelRemark == nil || *s.CancelRemark != "carrier booking cancelled" {
			t.Errorf("CancelRemark = %v, want \"carrier booking cancelled\"", s.CancelRemark)
		}
	})

	t.Run("nil remark leaves CancelRemark unset", func(t *testing.T) {
		s := &Shipment{Status: StatusReady}
		s.applyCancel(7, nil, now)
		if s.CancelRemark != nil {
			t.Errorf("CancelRemark = %v, want nil", s.CancelRemark)
		}
	})

	t.Run("empty remark leaves CancelRemark unset", func(t *testing.T) {
		s := &Shipment{Status: StatusReady}
		empty := ""
		s.applyCancel(7, &empty, now)
		if s.CancelRemark != nil {
			t.Errorf("CancelRemark = %v, want nil", s.CancelRemark)
		}
	})

	t.Run("does not touch pre-existing CancelRemark when new remark omitted", func(t *testing.T) {
		existing := "original reason"
		s := &Shipment{Status: StatusReady, CancelRemark: &existing}
		s.applyCancel(7, nil, now)
		if s.CancelRemark == nil || *s.CancelRemark != "original reason" {
			t.Errorf("CancelRemark = %v, want \"original reason\" untouched", s.CancelRemark)
		}
	})
}

func TestCarrier_IsValid(t *testing.T) {
	cases := map[Carrier]bool{
		CarrierDHL: true, CarrierKerry: true, CarrierFlash: true, CarrierInternal: true,
		"UPS": false, "": false,
	}
	for c, want := range cases {
		if got := c.IsValid(); got != want {
			t.Errorf("Carrier(%q).IsValid() = %v, want %v", c, got, want)
		}
	}
}

// --- Create's validation-only paths: run before s.repo is ever touched
// (see service.go's Create — the duplicate-shipment repo check happens
// only after every pure validation step), so they're safe to test with
// repo left nil, using a fake for the interface-typed pickingChecker
// dependency. ---

type fakePicking struct {
	resp *picking.Response
	err  error
}

func (f *fakePicking) Get(context.Context, int64) (*picking.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestCreate_RejectsUnknownPickingTask(t *testing.T) {
	svc := NewService(nil, &fakePicking{err: errors.New("not found")})

	_, err := svc.Create(context.Background(), CreateRequest{PickingTaskID: 999}, 1)

	if !errors.Is(err, ErrPickingTaskNotFound) {
		t.Fatalf("error = %v, want ErrPickingTaskNotFound", err)
	}
}

func TestCreate_RejectsNonShippableTask(t *testing.T) {
	svc := NewService(nil, &fakePicking{resp: &picking.Response{ID: 1, Status: picking.StatusInProgress}})

	_, err := svc.Create(context.Background(), CreateRequest{PickingTaskID: 1}, 1)

	if !errors.Is(err, ErrNotShippable) {
		t.Fatalf("error = %v, want ErrNotShippable", err)
	}
}

func TestCreate_RejectsTaskWithNoCompletedLines(t *testing.T) {
	svc := NewService(nil, &fakePicking{resp: &picking.Response{
		ID: 1, Status: picking.StatusShortPick,
		Items: []picking.ItemResponse{{ID: 1, Status: picking.ItemStatusShort, PickedQty: 0}},
	}})

	_, err := svc.Create(context.Background(), CreateRequest{PickingTaskID: 1}, 1)

	if !errors.Is(err, ErrNoShippableItems) {
		t.Fatalf("error = %v, want ErrNoShippableItems", err)
	}
}

func TestCreate_RejectsInvalidCarrier(t *testing.T) {
	svc := NewService(nil, &fakePicking{resp: &picking.Response{
		ID: 1, Status: picking.StatusCompleted,
		Items: []picking.ItemResponse{{ID: 1, Status: picking.ItemStatusCompleted, PickedQty: 10}},
	}})
	badCarrier := "UPS"

	_, err := svc.Create(context.Background(), CreateRequest{PickingTaskID: 1, Carrier: &badCarrier}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

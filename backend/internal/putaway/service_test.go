package putaway

import (
	"context"
	"errors"
	"testing"
	"time"

	"wms-backend/internal/receiving"
	"wms-backend/internal/warehouse"
)

// --- pure state-machine tests (no DB, no fakes needed) ---

func TestStatus_CanConfirm(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusInProgress: true,
		StatusCompleted: false, StatusIssue: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanConfirm(); got != want {
			t.Errorf("Status(%q).CanConfirm() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanCancel(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusInProgress: true,
		StatusCompleted: false, StatusIssue: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanCancel(); got != want {
			t.Errorf("Status(%q).CanCancel() = %v, want %v", status, got, want)
		}
	}
}

// TestTask_ApplyCancel is Phase 18's own coverage for the audit-column
// mutation Service.Cancel delegates to — Cancel itself needs a live DB
// (via s.repo.Transaction), so this isolates the pure part: does the
// task end up with the right status/audit fields, and is an empty/nil
// remark correctly left out of CancelRemark rather than stored as an
// empty string.
func TestTask_ApplyCancel(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("sets status and audit fields from a non-empty remark", func(t *testing.T) {
		task := &Task{Status: StatusPending}
		remark := "wrong location assigned"
		task.applyCancel(42, &remark, now)

		if task.Status != StatusCancelled {
			t.Errorf("Status = %q, want %q", task.Status, StatusCancelled)
		}
		if task.CancelledAt == nil || !task.CancelledAt.Equal(now) {
			t.Errorf("CancelledAt = %v, want %v", task.CancelledAt, now)
		}
		if task.CancelledBy == nil || *task.CancelledBy != 42 {
			t.Errorf("CancelledBy = %v, want 42", task.CancelledBy)
		}
		if task.CancelRemark == nil || *task.CancelRemark != "wrong location assigned" {
			t.Errorf("CancelRemark = %v, want \"wrong location assigned\"", task.CancelRemark)
		}
	})

	t.Run("nil remark leaves CancelRemark unset", func(t *testing.T) {
		task := &Task{Status: StatusInProgress}
		task.applyCancel(7, nil, now)
		if task.CancelRemark != nil {
			t.Errorf("CancelRemark = %v, want nil", task.CancelRemark)
		}
	})

	t.Run("empty remark leaves CancelRemark unset", func(t *testing.T) {
		task := &Task{Status: StatusInProgress}
		empty := ""
		task.applyCancel(7, &empty, now)
		if task.CancelRemark != nil {
			t.Errorf("CancelRemark = %v, want nil", task.CancelRemark)
		}
	})

	t.Run("does not touch pre-existing CancelRemark when new remark omitted", func(t *testing.T) {
		existing := "original reason"
		task := &Task{Status: StatusInProgress, CancelRemark: &existing}
		task.applyCancel(7, nil, now)
		if task.CancelRemark == nil || *task.CancelRemark != "original reason" {
			t.Errorf("CancelRemark = %v, want \"original reason\" untouched", task.CancelRemark)
		}
	})
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusInProgress: true, StatusCompleted: true,
		StatusIssue: true, StatusCancelled: true, "NOT_REAL": false, "": false,
	}
	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

func TestPriority_IsValid(t *testing.T) {
	cases := map[Priority]bool{
		PriorityLow: true, PriorityNormal: true, PriorityHigh: true,
		"NOT_REAL": false, "": false,
	}
	for p, want := range cases {
		if got := p.IsValid(); got != want {
			t.Errorf("Priority(%q).IsValid() = %v, want %v", p, got, want)
		}
	}
}

func TestItemStatus_Derivation(t *testing.T) {
	cases := []struct {
		name         string
		requestedQty float64
		movedQty     float64
		flaggedIssue bool
		want         ItemStatus
	}{
		{"nothing moved", 100, 0, false, ItemStatusPending},
		{"partially moved", 100, 40, false, ItemStatusPartiallyCompleted},
		{"exactly fully moved", 100, 100, false, ItemStatusCompleted},
		{"over-moved somehow still completes", 100, 120, false, ItemStatusCompleted},
		{"flagged issue overrides any quantity", 100, 40, true, ItemStatusIssue},
		{"flagged issue even with nothing moved", 100, 0, true, ItemStatusIssue},
	}
	for _, tc := range cases {
		if got := itemStatus(tc.requestedQty, tc.movedQty, tc.flaggedIssue); got != tc.want {
			t.Errorf("%s: itemStatus(%v, %v, %v) = %v, want %v",
				tc.name, tc.requestedQty, tc.movedQty, tc.flaggedIssue, got, tc.want)
		}
	}
}

func TestComputeTaskStatus(t *testing.T) {
	cases := []struct {
		name  string
		items []TaskItem
		want  Status
	}{
		{"no lines yet", nil, StatusPending},
		{"nothing moved", []TaskItem{{RequestedQty: 100, MovedQty: 0, Status: ItemStatusPending}}, StatusPending},
		{
			"one of two lines completed", []TaskItem{
				{RequestedQty: 100, MovedQty: 100, Status: ItemStatusCompleted},
				{RequestedQty: 50, MovedQty: 0, Status: ItemStatusPending},
			}, StatusInProgress,
		},
		{
			"one issue, one still pending", []TaskItem{
				{RequestedQty: 100, MovedQty: 0, Status: ItemStatusIssue},
				{RequestedQty: 50, MovedQty: 0, Status: ItemStatusPending},
			}, StatusInProgress,
		},
		{
			"all lines completed, no issues", []TaskItem{
				{RequestedQty: 100, MovedQty: 100, Status: ItemStatusCompleted},
				{RequestedQty: 50, MovedQty: 50, Status: ItemStatusCompleted},
			}, StatusCompleted,
		},
		{
			"all lines terminal, at least one issue", []TaskItem{
				{RequestedQty: 100, MovedQty: 100, Status: ItemStatusCompleted},
				{RequestedQty: 50, MovedQty: 0, Status: ItemStatusIssue},
			}, StatusIssue,
		},
		{
			"all lines issue", []TaskItem{
				{RequestedQty: 100, MovedQty: 0, Status: ItemStatusIssue},
			}, StatusIssue,
		},
	}
	for _, tc := range cases {
		if got := computeTaskStatus(tc.items); got != tc.want {
			t.Errorf("%s: computeTaskStatus(...) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// --- Create's validation-only paths: run before s.repo is ever touched,
// so they're safe to test with repo left nil, using fakes for the
// interface-typed dependencies (warehouseChecker/receiptChecker). ---

type fakeWarehouses struct {
	err error
}

func (f *fakeWarehouses) FindByID(context.Context, int64) (*warehouse.Warehouse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &warehouse.Warehouse{ID: 1}, nil
}

type fakeReceipts struct {
	warehouseID int64
	err         error
}

func (f *fakeReceipts) Get(context.Context, int64) (*receiving.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &receiving.Response{ID: 1, WarehouseID: f.warehouseID}, nil
}

func TestCreate_RejectsUnknownWarehouse(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{err: errors.New("not found")}, nil, nil)

	_, err := svc.Create(context.Background(), CreateRequest{WarehouseID: 999}, 1)

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

func TestCreate_RejectsUnknownReceipt(t *testing.T) {
	receiptID := int64(999)
	svc := NewService(nil, nil, &fakeWarehouses{}, &fakeReceipts{err: errors.New("not found")}, nil)

	_, err := svc.Create(context.Background(), CreateRequest{WarehouseID: 1, ReceiptID: &receiptID}, 1)

	if !errors.Is(err, ErrReceiptNotFound) {
		t.Fatalf("error = %v, want ErrReceiptNotFound", err)
	}
}

func TestCreate_RejectsReceiptWarehouseMismatch(t *testing.T) {
	receiptID := int64(1)
	svc := NewService(nil, nil, &fakeWarehouses{}, &fakeReceipts{warehouseID: 2}, nil)

	_, err := svc.Create(context.Background(), CreateRequest{WarehouseID: 1, ReceiptID: &receiptID}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidPriority(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{}, &fakeReceipts{}, nil)

	_, err := svc.Create(context.Background(), CreateRequest{WarehouseID: 1, Priority: "URGENT"}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestConfirm_RejectsEmptyItems(t *testing.T) {
	svc := &Service{}

	_, err := svc.Confirm(context.Background(), 1, ConfirmRequest{IdempotencyKey: "key-1"}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

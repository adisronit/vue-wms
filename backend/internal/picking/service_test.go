package picking

import (
	"context"
	"errors"
	"testing"
	"time"

	"wms-backend/internal/warehouse"
)

// --- pure state-machine tests (no DB, no fakes needed) ---

func TestStatus_CanConfirm(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusInProgress: true,
		StatusCompleted: false, StatusShortPick: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanConfirm(); got != want {
			t.Errorf("Status(%q).CanConfirm() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusInProgress: true, StatusCompleted: true,
		StatusShortPick: true, StatusCancelled: true, "NOT_REAL": false, "": false,
	}
	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanCancel(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: true, StatusInProgress: true,
		StatusCompleted: false, StatusShortPick: false, StatusCancelled: false,
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
		remark := "customer cancelled order"
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
		if task.CancelRemark == nil || *task.CancelRemark != "customer cancelled order" {
			t.Errorf("CancelRemark = %v, want \"customer cancelled order\"", task.CancelRemark)
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

func TestPriority_IsValid(t *testing.T) {
	cases := map[Priority]bool{
		PriorityLow: true, PriorityNormal: true, PriorityHigh: true,
		"URGENT": false, "": false,
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
		orderedQty   float64
		pickedQty    float64
		flaggedShort bool
		want         ItemStatus
	}{
		{"nothing picked", 20, 0, false, ItemStatusPending},
		{"partially picked", 20, 12, false, ItemStatusPartiallyPicked},
		{"exactly fully picked", 20, 20, false, ItemStatusCompleted},
		{"over-picked somehow still completes", 20, 25, false, ItemStatusCompleted},
		{"flagged short overrides any quantity", 20, 12, true, ItemStatusShort},
		{"flagged short even with nothing picked", 20, 0, true, ItemStatusShort},
	}
	for _, tc := range cases {
		if got := itemStatus(tc.orderedQty, tc.pickedQty, tc.flaggedShort); got != tc.want {
			t.Errorf("%s: itemStatus(%v, %v, %v) = %v, want %v",
				tc.name, tc.orderedQty, tc.pickedQty, tc.flaggedShort, got, tc.want)
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
		{"nothing picked", []TaskItem{{OrderedQty: 20, PickedQty: 0, Status: ItemStatusPending}}, StatusPending},
		{
			"one of two lines completed", []TaskItem{
				{OrderedQty: 20, PickedQty: 20, Status: ItemStatusCompleted},
				{OrderedQty: 10, PickedQty: 0, Status: ItemStatusPending},
			}, StatusInProgress,
		},
		{
			"one short, one still pending", []TaskItem{
				{OrderedQty: 20, PickedQty: 0, Status: ItemStatusShort},
				{OrderedQty: 10, PickedQty: 0, Status: ItemStatusPending},
			}, StatusInProgress,
		},
		{
			"all lines completed, none short", []TaskItem{
				{OrderedQty: 20, PickedQty: 20, Status: ItemStatusCompleted},
				{OrderedQty: 10, PickedQty: 10, Status: ItemStatusCompleted},
			}, StatusCompleted,
		},
		{
			"all lines terminal, at least one short", []TaskItem{
				{OrderedQty: 20, PickedQty: 20, Status: ItemStatusCompleted},
				{OrderedQty: 10, PickedQty: 0, Status: ItemStatusShort},
			}, StatusShortPick,
		},
		{
			"all lines short", []TaskItem{
				{OrderedQty: 20, PickedQty: 0, Status: ItemStatusShort},
			}, StatusShortPick,
		},
	}
	for _, tc := range cases {
		if got := computeTaskStatus(tc.items); got != tc.want {
			t.Errorf("%s: computeTaskStatus(...) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// --- Create's validation-only paths: run before s.repo is ever touched,
// so they're safe to test with repo left nil, using a fake for the
// interface-typed warehouseChecker dependency. ---

type fakeWarehouses struct {
	err error
}

func (f *fakeWarehouses) FindByID(context.Context, int64) (*warehouse.Warehouse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &warehouse.Warehouse{ID: 1}, nil
}

func TestCreate_RejectsUnknownWarehouse(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{err: errors.New("not found")}, nil)

	_, err := svc.Create(context.Background(), CreateRequest{WarehouseID: 999, OrderNo: "SO-1"}, 1)

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

func TestCreate_RejectsInvalidPriority(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{}, nil)

	_, err := svc.Create(context.Background(), CreateRequest{WarehouseID: 1, OrderNo: "SO-1", Priority: "URGENT"}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// TestCreateTx_RejectsUnknownWarehouse covers Phase 25's addition —
// CreateTx runs the identical validateCreate pre-check as Create, before
// ever touching the caller-supplied tx, so this is safe to test the same
// way with tx left nil.
func TestCreateTx_RejectsUnknownWarehouse(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{err: errors.New("not found")}, nil)

	_, err := svc.CreateTx(context.Background(), nil, CreateRequest{WarehouseID: 999, OrderNo: "SO-1"}, 1)

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
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

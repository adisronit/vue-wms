package receiving

import (
	"context"
	"errors"
	"testing"
	"time"

	"wms-backend/internal/location"
	"wms-backend/internal/warehouse"
)

// --- pure state-machine tests (no DB, no fakes needed) ---

func TestStatus_CanConfirm(t *testing.T) {
	cases := map[Status]bool{
		StatusDraft: true, StatusWaiting: true, StatusPartiallyReceived: true,
		StatusCompleted: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanConfirm(); got != want {
			t.Errorf("Status(%q).CanConfirm() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanEdit(t *testing.T) {
	cases := map[Status]bool{
		StatusDraft: true, StatusWaiting: true,
		StatusPartiallyReceived: false, StatusCompleted: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanEdit(); got != want {
			t.Errorf("Status(%q).CanEdit() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanCancel(t *testing.T) {
	cases := map[Status]bool{
		StatusDraft: true, StatusWaiting: true,
		StatusPartiallyReceived: false, StatusCompleted: false, StatusCancelled: false,
	}
	for status, want := range cases {
		if got := status.CanCancel(); got != want {
			t.Errorf("Status(%q).CanCancel() = %v, want %v", status, got, want)
		}
	}
}

// TestReceipt_ApplyCancel is Phase 18's own coverage for the audit-column
// mutation Service.Cancel delegates to — Cancel itself needs a live DB
// (via s.repo.Transaction), so this isolates the pure part: does the
// receipt end up with the right status/audit fields, and is an
// empty/nil remark correctly left out of CancelRemark rather than
// stored as an empty string.
func TestReceipt_ApplyCancel(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	t.Run("sets status and audit fields from a non-empty remark", func(t *testing.T) {
		r := &Receipt{Status: StatusDraft}
		remark := "wrong vendor"
		r.applyCancel(42, &remark, now)

		if r.Status != StatusCancelled {
			t.Errorf("Status = %q, want %q", r.Status, StatusCancelled)
		}
		if r.CancelledAt == nil || !r.CancelledAt.Equal(now) {
			t.Errorf("CancelledAt = %v, want %v", r.CancelledAt, now)
		}
		if r.CancelledBy == nil || *r.CancelledBy != 42 {
			t.Errorf("CancelledBy = %v, want 42", r.CancelledBy)
		}
		if r.CancelRemark == nil || *r.CancelRemark != "wrong vendor" {
			t.Errorf("CancelRemark = %v, want \"wrong vendor\"", r.CancelRemark)
		}
	})

	t.Run("nil remark leaves CancelRemark unset", func(t *testing.T) {
		r := &Receipt{Status: StatusWaiting}
		r.applyCancel(7, nil, now)
		if r.CancelRemark != nil {
			t.Errorf("CancelRemark = %v, want nil", r.CancelRemark)
		}
	})

	t.Run("empty remark leaves CancelRemark unset", func(t *testing.T) {
		r := &Receipt{Status: StatusWaiting}
		empty := ""
		r.applyCancel(7, &empty, now)
		if r.CancelRemark != nil {
			t.Errorf("CancelRemark = %v, want nil", r.CancelRemark)
		}
	})

	t.Run("does not touch pre-existing CancelRemark when new remark omitted", func(t *testing.T) {
		existing := "original reason"
		r := &Receipt{Status: StatusWaiting, CancelRemark: &existing}
		r.applyCancel(7, nil, now)
		if r.CancelRemark == nil || *r.CancelRemark != "original reason" {
			t.Errorf("CancelRemark = %v, want \"original reason\" untouched", r.CancelRemark)
		}
	})
}

func TestReceivingType_IsValid(t *testing.T) {
	cases := map[ReceivingType]bool{
		TypePO: true, TypeASN: true, TypeReturn: true, TypeTransfer: true,
		"NOT_REAL": false, "": false,
	}
	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("ReceivingType(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

func TestItemStatus_Derivation(t *testing.T) {
	cases := []struct {
		name        string
		expectedQty float64
		receivedQty float64
		want        ItemStatus
	}{
		{"nothing received", 100, 0, ItemStatusPending},
		{"partially received", 100, 40, ItemStatusPartiallyReceived},
		{"exactly fully received", 100, 100, ItemStatusCompleted},
		{"over-received", 100, 120, ItemStatusCompleted},
		{"no expected qty, any receipt completes it", 0, 5, ItemStatusCompleted},
	}
	for _, tc := range cases {
		if got := itemStatus(tc.expectedQty, tc.receivedQty); got != tc.want {
			t.Errorf("%s: itemStatus(%v, %v) = %v, want %v", tc.name, tc.expectedQty, tc.receivedQty, got, tc.want)
		}
	}
}

func TestComputeReceiptStatus(t *testing.T) {
	cases := []struct {
		name  string
		items []ReceiptItem
		want  Status
	}{
		{"no lines yet", nil, StatusWaiting},
		{"nothing received", []ReceiptItem{{ExpectedQty: 100, ReceivedQty: 0, Status: ItemStatusPending}}, StatusWaiting},
		{
			"one of two lines received", []ReceiptItem{
				{ExpectedQty: 100, ReceivedQty: 100, Status: ItemStatusCompleted},
				{ExpectedQty: 50, ReceivedQty: 0, Status: ItemStatusPending},
			}, StatusPartiallyReceived,
		},
		{
			"both lines partially received", []ReceiptItem{
				{ExpectedQty: 100, ReceivedQty: 40, Status: ItemStatusPartiallyReceived},
				{ExpectedQty: 50, ReceivedQty: 10, Status: ItemStatusPartiallyReceived},
			}, StatusPartiallyReceived,
		},
		{
			"all lines fully received", []ReceiptItem{
				{ExpectedQty: 100, ReceivedQty: 100, Status: ItemStatusCompleted},
				{ExpectedQty: 50, ReceivedQty: 60, Status: ItemStatusCompleted},
			}, StatusCompleted,
		},
	}
	for _, tc := range cases {
		if got := computeReceiptStatus(tc.items); got != tc.want {
			t.Errorf("%s: computeReceiptStatus(...) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// --- Create's validation-only paths: run before s.repo is ever touched,
// so they're safe to test with repo left nil, using fakes for the
// interface-typed dependencies (warehouseChecker/locationChecker). ---

type fakeWarehouses struct {
	err error
}

func (f *fakeWarehouses) FindByID(context.Context, int64) (*warehouse.Warehouse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &warehouse.Warehouse{ID: 1}, nil
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

func TestCreate_RejectsInvalidReceivingType(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{ReceivingType: "NOT_REAL"}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidInitialStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{ReceivingType: TypePO, Status: StatusCompleted}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsUnknownWarehouse(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{err: errors.New("not found")}, nil, nil)

	_, err := svc.Create(context.Background(), CreateRequest{ReceivingType: TypePO, WarehouseID: 999}, 1)

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

func TestCreate_RejectsUnknownStagingLocation(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{}, &fakeLocations{err: errors.New("not found")}, nil)

	_, err := svc.Create(context.Background(), CreateRequest{
		ReceivingType: TypePO, WarehouseID: 1, StagingLocationID: 999,
	}, 1)

	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("error = %v, want ErrLocationNotFound", err)
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

package stockcount

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// --- pure state-machine tests (no DB, no fakes needed) ---

func TestStatus_CanEdit(t *testing.T) {
	cases := map[Status]bool{StatusDraft: true, StatusConfirmed: false, StatusAdjustmentCreated: false}
	for status, want := range cases {
		if got := status.CanEdit(); got != want {
			t.Errorf("Status(%q).CanEdit() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_CanConfirm(t *testing.T) {
	cases := map[Status]bool{StatusDraft: true, StatusConfirmed: false, StatusAdjustmentCreated: false}
	for status, want := range cases {
		if got := status.CanConfirm(); got != want {
			t.Errorf("Status(%q).CanConfirm() = %v, want %v", status, got, want)
		}
	}
}

// Phase 16: CanCreateAdjustments has the identical predicate to
// CanConfirm today — in particular, ADJUSTMENT_CREATED (a count that
// already went through Create Adjustment once) must reject a second
// call, and CONFIRMED (a count already directly confirmed) must reject
// it too — this is the Go-side mirror of what Repository.ClaimStatus's
// guarded UPDATE actually enforces at the database level.
func TestStatus_CanCreateAdjustments(t *testing.T) {
	cases := map[Status]bool{StatusDraft: true, StatusConfirmed: false, StatusAdjustmentCreated: false}
	for status, want := range cases {
		if got := status.CanCreateAdjustments(); got != want {
			t.Errorf("Status(%q).CanCreateAdjustments() = %v, want %v", status, got, want)
		}
	}
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusDraft: true, StatusConfirmed: true, StatusAdjustmentCreated: true,
		"NOT_REAL": false, "": false,
	}
	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

func TestCountType_IsValid(t *testing.T) {
	cases := map[CountType]bool{
		CountTypeCycle: true, CountTypeFull: true, CountTypeLocation: true, CountTypeSKU: true,
		"ANNUAL_COUNT": false, "": false,
	}
	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("CountType(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

func TestCondition_IsValid(t *testing.T) {
	cases := map[Condition]bool{ConditionGood: true, ConditionDamaged: true, ConditionHold: true, "BROKEN": false, "": false}
	for c, want := range cases {
		if got := c.IsValid(); got != want {
			t.Errorf("Condition(%q).IsValid() = %v, want %v", c, got, want)
		}
	}
}

func TestItemStatus_IsValid(t *testing.T) {
	cases := map[ItemStatus]bool{ItemStatusPending: true, ItemStatusCounted: true, ItemStatusRecount: true, "DONE": false, "": false}
	for s, want := range cases {
		if got := s.IsValid(); got != want {
			t.Errorf("ItemStatus(%q).IsValid() = %v, want %v", s, got, want)
		}
	}
}

// --- Create's validation-only paths: run before s.repo/s.inventory are
// ever touched (count_type validity and warehouse/zone existence are
// all checked, via fakes, before any transaction opens), so they're
// safe to test with repo/inventory left nil. ---

type fakeWarehouses struct {
	err error
}

func (f *fakeWarehouses) FindByID(context.Context, int64) (*warehouse.Warehouse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &warehouse.Warehouse{ID: 1}, nil
}

type fakeZones struct {
	err error
}

func (f *fakeZones) FindByID(context.Context, int64) (*zone.Zone, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &zone.Zone{ID: 1}, nil
}

func TestCreate_RejectsInvalidCountType(t *testing.T) {
	svc := NewService(nil, &fakeWarehouses{}, &fakeZones{}, nil, nil, nil, nil)

	_, err := svc.Create(context.Background(), CreateRequest{CountType: "ANNUAL_COUNT", WarehouseID: 1}, 1)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsUnknownWarehouse(t *testing.T) {
	svc := NewService(nil, &fakeWarehouses{err: errors.New("not found")}, &fakeZones{}, nil, nil, nil, nil)

	_, err := svc.Create(context.Background(), CreateRequest{CountType: CountTypeCycle, WarehouseID: 999}, 1)

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

func TestCreate_RejectsUnknownZone(t *testing.T) {
	zoneID := int64(999)
	svc := NewService(nil, &fakeWarehouses{}, &fakeZones{err: errors.New("not found")}, nil, nil, nil, nil)

	_, err := svc.Create(context.Background(), CreateRequest{CountType: CountTypeCycle, WarehouseID: 1, ZoneID: &zoneID}, 1)

	if !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("error = %v, want ErrZoneNotFound", err)
	}
}

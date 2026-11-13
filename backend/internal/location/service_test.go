package location

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/zone"
)

type fakeZones struct {
	found *zone.Zone
	err   error
}

func (f *fakeZones) FindByID(_ context.Context, _ int64) (*zone.Zone, error) {
	return f.found, f.err
}

const testTenantID int64 = 42

// Status/Type validation runs before any zone lookup or repository
// access, so it's testable against a zero-value Service.
func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 1, Code: "A-01-01-01", Status: "SUSPENDED",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidType(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 1, Code: "A-01-01-01", Type: "NOT_REAL",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsNegativeCapacity(t *testing.T) {
	svc := &Service{}
	negative := -1

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 1, Code: "A-01-01-01", Capacity: &negative,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// TestCreate_ZoneNotFound exercises the zone-existence check with a fake
// zoneChecker (an interface, so no real database is needed) — it fails
// before ever reaching the concrete repository.
func TestCreate_ZoneNotFound(t *testing.T) {
	svc := NewService(nil, &fakeZones{err: errors.New("not found")}, nil)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 999, Code: "A-01-01-01",
	})

	if !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("error = %v, want ErrZoneNotFound", err)
	}
}

// TestCreate_RejectsZoneBelongingToAnotherTenant proves
// validateZoneOwnership is an ownership check, not a bare existence
// check: a zone that exists but belongs to a different tenant must be
// rejected exactly like a nonexistent one (same error, same status —
// tenant-isolation-security.md §6).
func TestCreate_RejectsZoneBelongingToAnotherTenant(t *testing.T) {
	otherTenantZone := &zone.Zone{ID: 5, TenantID: testTenantID + 1}
	svc := NewService(nil, &fakeZones{found: otherTenantZone}, nil)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 5, Code: "A-01-01-01",
	})

	if !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("error = %v, want ErrZoneNotFound (cross-tenant zone must read as not-found)", err)
	}
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusActive:   true,
		StatusInactive: true,
		"SUSPENDED":    false,
		"":             false,
	}

	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

func TestType_IsValid(t *testing.T) {
	cases := map[Type]bool{
		TypeReceiving: true, TypeStorage: true, TypePicking: true, TypePacking: true,
		TypeShipping: true, TypeQC: true, TypeHold: true,
		"":         true, // optional field — unset is valid
		"NOT_REAL": false,
	}

	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("Type(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

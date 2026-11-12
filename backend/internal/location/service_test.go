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

// Status validation runs before any zone lookup or repository access,
// so it's testable against a zero-value Service.
func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		ZoneID: 1, Code: "A-01-01-01", Status: "SUSPENDED",
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
	svc := NewService(nil, &fakeZones{err: errors.New("not found")})

	_, err := svc.Create(context.Background(), CreateRequest{
		ZoneID: 999, Code: "A-01-01-01",
	})

	if !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("error = %v, want ErrZoneNotFound", err)
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

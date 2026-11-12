package warehouse

import (
	"context"
	"errors"
	"testing"
)

// Create's status validation runs before any repository or zone-count
// access, so it's testable against a zero-value Service — no database
// needed. The success/duplicate-code paths (which do touch the
// repository) were verified live against real SQL Server — see
// PROJECT_STATUS.md.
func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		Code: "WH-99", Name: "Bad Status Warehouse", Type: "Distribution Center",
		Status: "SUSPENDED",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
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

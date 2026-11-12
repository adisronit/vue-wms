package product

import (
	"context"
	"errors"
	"testing"
)

// Create's validation runs before any repository access, so it's
// testable against a zero-value Service — no database needed. The
// success/duplicate-SKU/duplicate-barcode paths (which do touch the
// repository) were verified live against real SQL Server — see
// PROJECT_STATUS.md.

func TestCreate_RejectsInvalidCategory(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		SKU: "SKU-X", Name: "Bad Category", Category: "NOT_REAL", BaseUOM: UOMPieces,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidBaseUOM(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		SKU: "SKU-X", Name: "Bad UOM", Category: CategoryPackaging, BaseUOM: "NOT_REAL",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		SKU: "SKU-X", Name: "Bad Status", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: "SUSPENDED",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsNegativeStock(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		SKU: "SKU-X", Name: "Bad Stock", Category: CategoryPackaging, BaseUOM: UOMPieces, MinStock: -1,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsMinGreaterThanMax(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		SKU: "SKU-X", Name: "Bad Range", Category: CategoryPackaging, BaseUOM: UOMPieces,
		MinStock: 100, MaxStock: 10,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCategory_IsValid(t *testing.T) {
	cases := map[Category]bool{
		CategoryPackaging: true, CategoryLabel: true, CategoryRFID: true, CategoryRawMaterial: true,
		"NOT_REAL": false, "": false,
	}
	for category, want := range cases {
		if got := category.IsValid(); got != want {
			t.Errorf("Category(%q).IsValid() = %v, want %v", category, got, want)
		}
	}
}

func TestUOM_IsValid(t *testing.T) {
	cases := map[UOM]bool{
		UOMPieces: true, UOMBox: true, UOMRoll: true, UOMKg: true,
		"NOT_REAL": false, "": false,
	}
	for uom, want := range cases {
		if got := uom.IsValid(); got != want {
			t.Errorf("UOM(%q).IsValid() = %v, want %v", uom, got, want)
		}
	}
}

func TestNormalizeBarcode(t *testing.T) {
	empty := ""
	value := "12345"

	if got := normalizeBarcode(nil); got != nil {
		t.Errorf("normalizeBarcode(nil) = %v, want nil", got)
	}
	if got := normalizeBarcode(&empty); got != nil {
		t.Errorf("normalizeBarcode(\"\") = %v, want nil", got)
	}
	if got := normalizeBarcode(&value); got == nil || *got != "12345" {
		t.Errorf("normalizeBarcode(%q) = %v, want pointer to %q", value, got, value)
	}
}

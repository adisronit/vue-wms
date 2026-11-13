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

// fakeCategoryChecker is a test-local stand-in for categoryChecker —
// validateCategoryID's unit tests below need to control Exists'
// return value/error without a real internal/productcategory
// repository or database connection.
type fakeCategoryChecker struct {
	existsResult bool
	err          error
	called       bool
}

func (f *fakeCategoryChecker) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	f.called = true
	return f.existsResult, f.err
}

func TestValidateCategoryID_NilCategoryIDSkipsCheck(t *testing.T) {
	checker := &fakeCategoryChecker{}
	svc := &Service{categories: checker}

	if err := svc.validateCategoryID(context.Background(), nil); err != nil {
		t.Fatalf("validateCategoryID(nil) = %v, want nil", err)
	}
	if checker.called {
		t.Error("Exists should not be called when categoryID is nil")
	}
}

func TestValidateCategoryID_NoCheckerConfiguredSkipsCheck(t *testing.T) {
	svc := &Service{} // categories is nil — the nil-safe fallback existing tests' &Service{} literals rely on.
	id := int64(5)

	if err := svc.validateCategoryID(context.Background(), &id); err != nil {
		t.Fatalf("validateCategoryID with no categoryChecker configured = %v, want nil (defensive no-op)", err)
	}
}

func TestValidateCategoryID_RequiresTenantHint(t *testing.T) {
	checker := &fakeCategoryChecker{existsResult: true}
	svc := &Service{categories: checker}
	id := int64(5)

	err := svc.validateCategoryID(context.Background(), &id) // no WithCategoryTenantHint attached

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation (missing tenant hint)", err)
	}
	if checker.called {
		t.Error("Exists should not be called without a tenant hint — must fail closed")
	}
}

func TestValidateCategoryID_RejectsWhenCategoryDoesNotExist(t *testing.T) {
	checker := &fakeCategoryChecker{existsResult: false}
	svc := &Service{categories: checker}
	id := int64(5)
	ctx := WithCategoryTenantHint(context.Background(), 1)

	_, err := svc.Create(ctx, CreateRequest{
		SKU: "SKU-X", Name: "Has Bad Category", Category: CategoryPackaging, BaseUOM: UOMPieces, CategoryID: &id,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create error = %v, want ErrValidation", err)
	}
	if !checker.called {
		t.Error("Exists should have been called")
	}
}

func TestValidateCategoryID_AcceptsWhenCategoryExists(t *testing.T) {
	checker := &fakeCategoryChecker{existsResult: true}
	svc := &Service{categories: checker}
	id := int64(5)
	ctx := WithCategoryTenantHint(context.Background(), 1)

	if err := svc.validateCategoryID(ctx, &id); err != nil {
		t.Fatalf("validateCategoryID = %v, want nil", err)
	}
	if !checker.called {
		t.Error("Exists should have been called")
	}
}

func TestValidateCategoryID_PropagatesCheckerError(t *testing.T) {
	wantErr := errors.New("db unavailable")
	checker := &fakeCategoryChecker{err: wantErr}
	svc := &Service{categories: checker}
	id := int64(5)
	ctx := WithCategoryTenantHint(context.Background(), 1)

	err := svc.validateCategoryID(ctx, &id)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
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

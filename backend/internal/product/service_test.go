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

const testTenantID = int64(1)

func TestCreate_RejectsInvalidCategory(t *testing.T) {
	svc := &Service{}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "Bad Category", Category: "NOT_REAL", BaseUOM: UOMPieces, CategoryID: &id,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidBaseUOM(t *testing.T) {
	svc := &Service{}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "Bad UOM", Category: CategoryPackaging, BaseUOM: "NOT_REAL", CategoryID: &id,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "Bad Status", Category: CategoryPackaging, BaseUOM: UOMPieces, Status: "SUSPENDED", CategoryID: &id,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidStorageType(t *testing.T) {
	svc := &Service{}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "Bad Storage", Category: CategoryPackaging, BaseUOM: UOMPieces, CategoryID: &id,
		StorageType: "HOT",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidRotationRule(t *testing.T) {
	svc := &Service{}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "Bad Rotation", Category: CategoryPackaging, BaseUOM: UOMPieces, CategoryID: &id,
		RotationRule: "LIFO",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsMissingCategoryID(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "No Category", Category: CategoryPackaging, BaseUOM: UOMPieces,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation (category_id required)", err)
	}
}

func TestCreate_RejectsNegativeStock(t *testing.T) {
	svc := &Service{}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "Bad Stock", Category: CategoryPackaging, BaseUOM: UOMPieces, MinStock: -1, CategoryID: &id,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsMinGreaterThanMax(t *testing.T) {
	svc := &Service{}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		SKU: "SKU-X", Name: "Bad Range", Category: CategoryPackaging, BaseUOM: UOMPieces,
		MinStock: 100, MaxStock: 10, CategoryID: &id,
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
		UOMPieces: true, UOMBox: true, UOMRoll: true, UOMKg: true, UOMGram: true, UOMLiter: true, UOMMilliliter: true,
		"NOT_REAL": false, "": false,
	}
	for uom, want := range cases {
		if got := uom.IsValid(); got != want {
			t.Errorf("UOM(%q).IsValid() = %v, want %v", uom, got, want)
		}
	}
}

func TestStorageType_IsValid(t *testing.T) {
	cases := map[StorageType]bool{
		StorageAmbient: true, StorageChilled: true, StorageFrozen: true, "": true,
		"HOT": false,
	}
	for st, want := range cases {
		if got := st.IsValid(); got != want {
			t.Errorf("StorageType(%q).IsValid() = %v, want %v", st, got, want)
		}
	}
}

func TestRotationRule_IsValid(t *testing.T) {
	cases := map[RotationRule]bool{
		RotationFIFO: true, RotationFEFO: true, "": true,
		"LIFO": false,
	}
	for rr, want := range cases {
		if got := rr.IsValid(); got != want {
			t.Errorf("RotationRule(%q).IsValid() = %v, want %v", rr, got, want)
		}
	}
}

// fakeExistsChecker is a test-local stand-in satisfying every
// categoryChecker/brandChecker/familyChecker/supplierChecker
// interface — they're all structurally identical (Exists(ctx,
// tenantID, id) (bool, error)) — used to control the return value/
// error without a real repository or database connection.
type fakeExistsChecker struct {
	existsResult   bool
	err            error
	called         bool
	calledTenantID int64
}

func (f *fakeExistsChecker) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	f.called = true
	f.calledTenantID = tenantID
	return f.existsResult, f.err
}

func TestValidateCategoryID_NilCategoryIDSkipsCheck(t *testing.T) {
	checker := &fakeExistsChecker{}
	svc := &Service{categories: checker}

	if err := svc.validateCategoryID(context.Background(), testTenantID, nil); err != nil {
		t.Fatalf("validateCategoryID(nil) = %v, want nil", err)
	}
	if checker.called {
		t.Error("Exists should not be called when categoryID is nil")
	}
}

func TestValidateCategoryID_NoCheckerConfiguredSkipsCheck(t *testing.T) {
	svc := &Service{} // categories is nil — the nil-safe fallback existing tests' &Service{} literals rely on.
	id := int64(5)

	if err := svc.validateCategoryID(context.Background(), testTenantID, &id); err != nil {
		t.Fatalf("validateCategoryID with no categoryChecker configured = %v, want nil (defensive no-op)", err)
	}
}

func TestValidateCategoryID_PassesTenantIDThrough(t *testing.T) {
	checker := &fakeExistsChecker{existsResult: true}
	svc := &Service{categories: checker}
	id := int64(5)

	if err := svc.validateCategoryID(context.Background(), 42, &id); err != nil {
		t.Fatalf("validateCategoryID = %v, want nil", err)
	}
	if !checker.called {
		t.Error("Exists should have been called")
	}
	if checker.calledTenantID != 42 {
		t.Errorf("Exists called with tenantID = %d, want 42", checker.calledTenantID)
	}
}

func TestValidateCategoryID_RejectsWhenCategoryDoesNotExist(t *testing.T) {
	checker := &fakeExistsChecker{existsResult: false}
	svc := &Service{categories: checker}
	id := int64(5)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
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
	checker := &fakeExistsChecker{existsResult: true}
	svc := &Service{categories: checker}
	id := int64(5)

	if err := svc.validateCategoryID(context.Background(), testTenantID, &id); err != nil {
		t.Fatalf("validateCategoryID = %v, want nil", err)
	}
	if !checker.called {
		t.Error("Exists should have been called")
	}
}

func TestValidateCategoryID_PropagatesCheckerError(t *testing.T) {
	wantErr := errors.New("db unavailable")
	checker := &fakeExistsChecker{err: wantErr}
	svc := &Service{categories: checker}
	id := int64(5)

	err := svc.validateCategoryID(context.Background(), testTenantID, &id)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestValidateBrandID_RejectsWhenBrandDoesNotExist(t *testing.T) {
	checker := &fakeExistsChecker{existsResult: false}
	svc := &Service{brands: checker}
	id := int64(5)

	err := svc.validateBrandID(context.Background(), testTenantID, &id)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateFamilyID_RejectsWhenFamilyDoesNotExist(t *testing.T) {
	checker := &fakeExistsChecker{existsResult: false}
	svc := &Service{families: checker}
	id := int64(5)

	err := svc.validateFamilyID(context.Background(), testTenantID, &id)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestValidateSupplierID_RejectsWhenSupplierDoesNotExist(t *testing.T) {
	checker := &fakeExistsChecker{existsResult: false}
	svc := &Service{suppliers: checker}
	id := int64(5)

	err := svc.validateSupplierID(context.Background(), testTenantID, &id)

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
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

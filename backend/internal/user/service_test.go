package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wms-backend/internal/role"
	"wms-backend/internal/warehouse"
)

// These cover the validation paths in Create that run before any
// repository or role lookup, so they're testable against a zero-value
// Service — no database needed. Update's status/role validation happens
// after a repository.FindByID call and is therefore covered by SQL
// Server verification (PROJECT_STATUS.md) rather than a unit test here.

func TestCreate_RejectsShortPassword(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		Username: "newuser", Password: "short", FullName: "New User",
		Email: "new@example.com", RoleCode: "RECEIVING",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		Username: "newuser", Password: "long-enough-password", FullName: "New User",
		Email: "new@example.com", RoleCode: "RECEIVING", Status: "SUSPENDED",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// TestPasswordHashing_RoundTrip documents and verifies the exact bcrypt
// calls Create/Update rely on: GenerateFromPassword produces a hash that
// is not the plaintext and that CompareHashAndPassword accepts only the
// original password.
func TestPasswordHashing_RoundTrip(t *testing.T) {
	plaintext := "ChangeMe123!"

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword failed: %v", err)
	}

	if string(hash) == plaintext {
		t.Fatal("hash must not equal the plaintext password")
	}

	if err := bcrypt.CompareHashAndPassword(hash, []byte(plaintext)); err != nil {
		t.Fatalf("CompareHashAndPassword rejected the correct password: %v", err)
	}

	if err := bcrypt.CompareHashAndPassword(hash, []byte("wrong-password")); err == nil {
		t.Fatal("CompareHashAndPassword accepted an incorrect password")
	}
}

// fakeRoleLookup/fakeWarehouseLookup are narrow-interface fakes for
// toResponse's cross-package lookups — the same pattern
// PROJECT_STATUS.md documents for warehouse/zone/location's own
// existence checks, so these run without a real database.
type fakeRoleLookup struct {
	byID map[int64]*role.Role
}

func (f *fakeRoleLookup) FindRoleByCode(_ context.Context, code string) (*role.Role, error) {
	for _, r := range f.byID {
		if r.Code == code {
			return r, nil
		}
	}
	return nil, role.ErrNotFound
}

func (f *fakeRoleLookup) FindRoleByID(_ context.Context, id int64) (*role.Role, error) {
	r, ok := f.byID[id]
	if !ok {
		return nil, role.ErrNotFound
	}
	return r, nil
}

type fakeWarehouseLookup struct {
	byID map[int64]*warehouse.Warehouse
}

func (f *fakeWarehouseLookup) FindByID(_ context.Context, id int64) (*warehouse.Warehouse, error) {
	w, ok := f.byID[id]
	if !ok {
		return nil, warehouse.ErrNotFound
	}
	return w, nil
}

func newTestServiceForResponses() *Service {
	return &Service{
		roles: &fakeRoleLookup{byID: map[int64]*role.Role{
			1: {ID: 1, Code: "ADMIN", Name: "Administrator"},
		}},
		warehouses: &fakeWarehouseLookup{byID: map[int64]*warehouse.Warehouse{
			5: {ID: 5, Code: "WH-01", Name: "Bangkok Warehouse"},
		}},
	}
}

// TestToResponse_IncludesWarehouseWhenAssigned covers the User
// Management list/detail response actually carrying warehouse_code/
// warehouse_name once a user has a warehouse_id — the fix for
// "User Management does not display the user's Warehouse."
func TestToResponse_IncludesWarehouseWhenAssigned(t *testing.T) {
	svc := newTestServiceForResponses()
	warehouseID := int64(5)
	u := &User{ID: 1, RoleID: 1, WarehouseID: &warehouseID, Status: StatusActive}

	resp, err := svc.toResponse(context.Background(), u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.WarehouseID == nil || *resp.WarehouseID != 5 {
		t.Fatalf("WarehouseID = %v, want 5", resp.WarehouseID)
	}
	if resp.WarehouseCode == nil || *resp.WarehouseCode != "WH-01" {
		t.Fatalf("WarehouseCode = %v, want WH-01", resp.WarehouseCode)
	}
	if resp.WarehouseName == nil || *resp.WarehouseName != "Bangkok Warehouse" {
		t.Fatalf("WarehouseName = %v, want Bangkok Warehouse", resp.WarehouseName)
	}
}

// TestToResponse_NilWarehouseDoesNotError covers a user with no
// warehouse assigned (e.g. ADMIN, or any pre-migration user) rendering
// cleanly — nil WarehouseID/Code/Name, no lookup error — rather than
// the response failing.
func TestToResponse_NilWarehouseDoesNotError(t *testing.T) {
	svc := newTestServiceForResponses()
	u := &User{ID: 1, RoleID: 1, WarehouseID: nil, Status: StatusActive}

	resp, err := svc.toResponse(context.Background(), u)
	if err != nil {
		t.Fatalf("unexpected error for a user with no warehouse: %v", err)
	}
	if resp.WarehouseID != nil || resp.WarehouseCode != nil || resp.WarehouseName != nil {
		t.Fatalf("expected nil warehouse fields, got id=%v code=%v name=%v",
			resp.WarehouseID, resp.WarehouseCode, resp.WarehouseName)
	}
}

// TestToResponse_IncludesLastLoginAt covers the response actually
// carrying last_login_at — the fix for "User Management does not
// display Last Login time" — and that a never-logged-in user (nil)
// renders as nil rather than a zero-value timestamp.
func TestToResponse_IncludesLastLoginAt(t *testing.T) {
	svc := newTestServiceForResponses()
	loggedInAt := time.Date(2026, 9, 3, 14, 34, 10, 0, time.UTC)
	u := &User{ID: 1, RoleID: 1, Status: StatusActive, LastLoginAt: &loggedInAt}

	resp, err := svc.toResponse(context.Background(), u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.LastLoginAt == nil || !resp.LastLoginAt.Equal(loggedInAt) {
		t.Fatalf("LastLoginAt = %v, want %v", resp.LastLoginAt, loggedInAt)
	}

	u2 := &User{ID: 2, RoleID: 1, Status: StatusActive, LastLoginAt: nil}
	resp2, err := svc.toResponse(context.Background(), u2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.LastLoginAt != nil {
		t.Fatalf("LastLoginAt = %v, want nil for a user who never logged in", resp2.LastLoginAt)
	}
}

// TestCreate_RejectsUnknownWarehouseID covers Create's warehouse_id
// validation: a warehouse_id that doesn't match any real warehouse must
// be rejected (422), not silently stored or left to fail as a raw FK
// violation at the database.
func TestCreate_RejectsUnknownWarehouseID(t *testing.T) {
	svc := &Service{
		roles: &fakeRoleLookup{byID: map[int64]*role.Role{
			1: {ID: 1, Code: "RECEIVING", Name: "Receiving"},
		}},
		warehouses: &fakeWarehouseLookup{byID: map[int64]*warehouse.Warehouse{}},
	}
	missingWarehouseID := int64(999)

	_, err := svc.Create(context.Background(), CreateRequest{
		Username: "newuser", Password: "long-enough-password", FullName: "New User",
		Email: "new@example.com", RoleCode: "RECEIVING", WarehouseID: &missingWarehouseID,
	})

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusActive:   true,
		StatusInactive: true,
		StatusLocked:   true,
		"SUSPENDED":    false,
		"":             false,
	}

	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

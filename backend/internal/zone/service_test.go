package zone

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/warehouse"
)

type fakeWarehouses struct {
	found *warehouse.Warehouse
	err   error
}

func (f *fakeWarehouses) FindByID(_ context.Context, _ int64) (*warehouse.Warehouse, error) {
	return f.found, f.err
}

type fakeLocations struct {
	count int64
}

func (f *fakeLocations) CountByZone(_ context.Context, _ int64) (int64, error) {
	return f.count, nil
}

// Type/status validation runs before any warehouse lookup or repository
// access, so it's testable against a zero-value Service.
func TestCreate_RejectsInvalidType(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		WarehouseID: 1, Code: "X", Name: "Bad Type Zone", Type: "NOT_REAL",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), CreateRequest{
		WarehouseID: 1, Code: "X", Name: "Bad Status Zone", Type: TypeStorage, Status: "SUSPENDED",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// TestCreate_WarehouseNotFound exercises the warehouse-existence check
// with a fake warehouseChecker (an interface, so no real database is
// needed) — it fails before ever reaching the concrete repository.
func TestCreate_WarehouseNotFound(t *testing.T) {
	svc := NewService(nil, &fakeWarehouses{err: errors.New("not found")}, &fakeLocations{})

	_, err := svc.Create(context.Background(), CreateRequest{
		WarehouseID: 999, Code: "X", Name: "Orphan Zone", Type: TypeStorage,
	})

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

func TestType_IsValid(t *testing.T) {
	cases := map[Type]bool{
		TypeReceiving: true, TypeStorage: true, TypePicking: true,
		TypePacking: true, TypeShipping: true, TypeQC: true, TypeHold: true,
		"NOT_REAL": false, "": false,
	}

	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("Type(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

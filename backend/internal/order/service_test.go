package order

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/picking"
	"wms-backend/internal/warehouse"
)

// --- buildPickingItems: CreatePickingTask's pure assignment-matching
// core (no DB needed) ---

func TestBuildPickingItems_HappyPath(t *testing.T) {
	items := []Item{
		{ID: 1, ProductID: 100, OrderedQty: 10, UOM: "PCS"},
		{ID: 2, ProductID: 200, OrderedQty: 5, UOM: "BOX"},
	}
	assignments := []LocationAssignment{
		{OrderItemID: 1, LocationID: 11, LotNo: "LOT-A"},
		{OrderItemID: 2, LocationID: 22, LotNo: "LOT-B"},
	}

	got, err := buildPickingItems(items, assignments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	want := []picking.CreateItemRequest{
		{ProductID: 100, LotNo: "LOT-A", LocationID: 11, OrderedQty: 10, UOM: "PCS"},
		{ProductID: 200, LotNo: "LOT-B", LocationID: 22, OrderedQty: 5, UOM: "BOX"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBuildPickingItems_RejectsDuplicateAssignment(t *testing.T) {
	items := []Item{{ID: 1, ProductID: 100, OrderedQty: 10, UOM: "PCS"}}
	assignments := []LocationAssignment{
		{OrderItemID: 1, LocationID: 11},
		{OrderItemID: 1, LocationID: 12},
	}

	_, err := buildPickingItems(items, assignments)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestBuildPickingItems_RejectsMissingAssignment(t *testing.T) {
	items := []Item{
		{ID: 1, ProductID: 100, OrderedQty: 10, UOM: "PCS"},
		{ID: 2, ProductID: 200, OrderedQty: 5, UOM: "BOX"},
	}
	assignments := []LocationAssignment{
		{OrderItemID: 1, LocationID: 11},
	}

	_, err := buildPickingItems(items, assignments)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestBuildPickingItems_RejectsForeignOrderItemID(t *testing.T) {
	items := []Item{{ID: 1, ProductID: 100, OrderedQty: 10, UOM: "PCS"}}
	// Same count as items, but targets an order_item_id that isn't item 1
	// — must not silently pass through as if it matched.
	assignments := []LocationAssignment{
		{OrderItemID: 999, LocationID: 11},
	}

	_, err := buildPickingItems(items, assignments)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// --- Create/Update's validation-only paths: run before s.repo is ever
// touched, so they're safe to test with repo left nil, using a fake for
// the interface-typed warehouseChecker dependency — the same pattern
// internal/picking's own TestCreate_RejectsUnknownWarehouse established.

type fakeWarehouses struct {
	err error
}

func (f *fakeWarehouses) FindByID(context.Context, int64) (*warehouse.Warehouse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &warehouse.Warehouse{ID: 1}, nil
}

func TestCreate_RejectsUnknownWarehouse(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{err: errors.New("not found")}, nil, nil, nil, nil, nil)

	_, err := svc.Create(context.Background(), 1, CreateRequest{WarehouseID: 999}, 1)

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

// --- normalizeOrderTypeAndPriority: Create/Update's pure validation
// core for the new order_type/priority header fields (Phase 33A) ---

func TestNormalizeOrderTypeAndPriority_DefaultsEmptyToSalesOrderNormal(t *testing.T) {
	orderType, priority, err := normalizeOrderTypeAndPriority("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orderType != "SALES_ORDER" || priority != "NORMAL" {
		t.Errorf("got (%q, %q), want (SALES_ORDER, NORMAL)", orderType, priority)
	}
}

func TestNormalizeOrderTypeAndPriority_AcceptsInternalTransfer(t *testing.T) {
	// INTERNAL_TRANSFER must be accepted by this package's own validation
	// (and by migrations/0081's CK_orders_order_type) even though no
	// transfer-specific behavior is built around it yet — Phase 33E will
	// build on top of this same column.
	orderType, priority, err := normalizeOrderTypeAndPriority("INTERNAL_TRANSFER", "HIGH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orderType != "INTERNAL_TRANSFER" || priority != "HIGH" {
		t.Errorf("got (%q, %q), want (INTERNAL_TRANSFER, HIGH)", orderType, priority)
	}
}

func TestNormalizeOrderTypeAndPriority_RejectsUnknownValues(t *testing.T) {
	if _, _, err := normalizeOrderTypeAndPriority("BOGUS", ""); err == nil {
		t.Error("normalizeOrderTypeAndPriority(bogus order_type) = nil error, want ErrValidation")
	}
	if _, _, err := normalizeOrderTypeAndPriority("", "URGENT"); err == nil {
		t.Error("normalizeOrderTypeAndPriority(bogus priority) = nil error, want ErrValidation")
	}
}

func TestUpdate_RejectsUnknownWarehouse(t *testing.T) {
	svc := NewService(nil, nil, &fakeWarehouses{err: errors.New("not found")}, nil, nil, nil, nil, nil)

	_, err := svc.Update(context.Background(), 1, 1, UpdateRequest{WarehouseID: 999})

	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("error = %v, want ErrWarehouseNotFound", err)
	}
}

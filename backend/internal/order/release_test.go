// Phase 33A — Order Management redesign: tests for the new Review &
// Release step (Service.CheckAvailability/Release) — availability-check
// math, the PENDING -> RELEASED gate, the "Controlled shortage approval"
// override, and tenant isolation of both new endpoints' underlying
// Service methods. Uses the same real, seeded-tenant fixtures
// tenant_isolation_test.go's own orderFixture/newOrderServiceAndRepo/
// createOrder/seedOrderBalance establish.
package order

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// --- CheckAvailability: pure read, correct math ---

func TestService_CheckAvailability_NoShortage(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "avail-ok")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 20) // on-hand 100, reserved 20 -> available 80

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 50)

	resp, err := svc.CheckAvailability(context.Background(), a.tenantID, order.ID)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if resp.HasShortage {
		t.Fatalf("HasShortage = true, want false (80 available >= 50 ordered)")
	}
	if len(resp.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(resp.Lines))
	}
	line := resp.Lines[0]
	if line.AvailableQty != 80 {
		t.Errorf("AvailableQty = %v, want 80", line.AvailableQty)
	}
	if line.ReservedQty != 20 {
		t.Errorf("ReservedQty = %v, want 20", line.ReservedQty)
	}
	if line.OnHandQty != 100 {
		t.Errorf("OnHandQty = %v, want 100", line.OnHandQty)
	}
	if line.HasShortage || line.ShortfallQty != 0 {
		t.Errorf("line HasShortage/ShortfallQty = %v/%v, want false/0", line.HasShortage, line.ShortfallQty)
	}
}

func TestService_CheckAvailability_ComputesShortfall(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "avail-short")
	seedOrderBalance(t, tx, a, "LOT-A", 30, 0) // available 30

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 50) // ordered 50 > available 30

	resp, err := svc.CheckAvailability(context.Background(), a.tenantID, order.ID)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if !resp.HasShortage {
		t.Fatalf("HasShortage = false, want true")
	}
	line := resp.Lines[0]
	if !line.HasShortage {
		t.Errorf("line.HasShortage = false, want true")
	}
	if line.ShortfallQty != 20 {
		t.Errorf("ShortfallQty = %v, want 20 (50 ordered - 30 available)", line.ShortfallQty)
	}
}

// AvailabilityByProductWarehouse is warehouse-wide — sums every location
// within the warehouse, not just the one order_fixture seeds a single
// location into. This test seeds a SECOND location in the same
// warehouse via a direct zone/location insert, proving the aggregation
// spans locations, not just the one fixture location.
func TestService_CheckAvailability_SumsAcrossLocationsInWarehouse(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "avail-multi-loc")
	seedOrderBalance(t, tx, a, "LOT-A", 10, 0)

	var zoneID, locationID2 int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z2-avail-multi-loc', 'Zone2', 'STORAGE', 'ACTIVE')",
		a.tenantID, a.warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed second zone: %v", err)
	}
	if err := tx.Raw(
		"INSERT INTO locations (tenant_id, zone_id, code, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'LOC2-avail-multi-loc', 'Location2', 'ACTIVE')",
		a.tenantID, zoneID,
	).Scan(&locationID2).Error; err != nil {
		t.Fatalf("seed second location: %v", err)
	}
	if err := tx.Exec(
		"INSERT INTO inventory (tenant_id, product_id, location_id, lot_no, quantity, reserved_quantity) VALUES (?, ?, ?, 'LOT-B', 15, 0)",
		a.tenantID, a.productID, locationID2,
	).Error; err != nil {
		t.Fatalf("seed second balance: %v", err)
	}

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 20) // ordered 20, available 10+15=25 across the warehouse

	resp, err := svc.CheckAvailability(context.Background(), a.tenantID, order.ID)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if resp.HasShortage {
		t.Fatalf("HasShortage = true, want false (25 available across both locations >= 20 ordered)")
	}
	if resp.Lines[0].AvailableQty != 25 {
		t.Errorf("AvailableQty = %v, want 25 (summed across both locations in the warehouse)", resp.Lines[0].AvailableQty)
	}
}

func TestService_CheckAvailability_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "avail-cross-a")
	b := newOrderFixture(t, tx, "avail-cross-b")

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	if _, err := svc.CheckAvailability(context.Background(), a.tenantID, order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CheckAvailability(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}
}

// --- Release: the PENDING -> RELEASED gate ---

func TestService_Release_NoShortage_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "release-ok")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)

	resp, err := svc.Release(context.Background(), a.tenantID, order.ID, ReleaseRequest{}, 1)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if resp.Status != StatusReleased {
		t.Errorf("Status = %v, want RELEASED", resp.Status)
	}
	if resp.ReleasedAt == nil {
		t.Errorf("ReleasedAt = nil, want set")
	}
	if resp.ReleasedBy == nil || *resp.ReleasedBy != 1 {
		t.Errorf("ReleasedBy = %v, want 1", resp.ReleasedBy)
	}
	if resp.ShortageApproved {
		t.Errorf("ShortageApproved = true, want false (no shortage existed)")
	}
	if resp.Items[0].ShortageFlag || resp.Items[0].ShortfallQty != nil {
		t.Errorf("line shortage = %v/%v, want false/nil", resp.Items[0].ShortageFlag, resp.Items[0].ShortfallQty)
	}

	// CreatePickingTask must now be allowed — the one and only reason
	// Release exists.
	if !resp.Status.CanCreatePickingTask() {
		t.Errorf("RELEASED order's CanCreatePickingTask() = false, want true")
	}
}

func TestService_Release_ShortageWithoutApproval_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "rel-short-noappr")
	seedOrderBalance(t, tx, a, "LOT-A", 5, 0) // available 5

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10) // ordered 10 > available 5

	_, err := svc.Release(context.Background(), a.tenantID, order.ID, ReleaseRequest{ApproveShortage: false}, 1)
	if !errors.Is(err, ErrShortageApprovalRequired) {
		t.Fatalf("Release(shortage, no approval) = %v, want ErrShortageApprovalRequired", err)
	}

	// The order must be left exactly PENDING — a rejected Release must
	// never half-apply the transition or stamp released_at/released_by.
	reloaded, err := svc.Get(context.Background(), a.tenantID, order.ID)
	if err != nil {
		t.Fatalf("Get after rejected Release: %v", err)
	}
	if reloaded.Status != StatusPending {
		t.Errorf("Status after rejected Release = %v, want still PENDING", reloaded.Status)
	}
	if reloaded.ReleasedAt != nil {
		t.Errorf("ReleasedAt after rejected Release = %v, want nil", reloaded.ReleasedAt)
	}
}

func TestService_Release_ShortageWithApproval_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "rel-short-appr")
	seedOrderBalance(t, tx, a, "LOT-A", 5, 0) // available 5

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10) // ordered 10 > available 5, shortfall 5

	resp, err := svc.Release(context.Background(), a.tenantID, order.ID, ReleaseRequest{ApproveShortage: true}, 1)
	if err != nil {
		t.Fatalf("Release(shortage, approved): %v", err)
	}
	if resp.Status != StatusReleased {
		t.Errorf("Status = %v, want RELEASED", resp.Status)
	}
	if !resp.ShortageApproved {
		t.Errorf("ShortageApproved = false, want true")
	}
	if !resp.Items[0].ShortageFlag {
		t.Errorf("line ShortageFlag = false, want true")
	}
	if resp.Items[0].ShortfallQty == nil || *resp.Items[0].ShortfallQty != 5 {
		t.Errorf("line ShortfallQty = %v, want 5", resp.Items[0].ShortfallQty)
	}
}

func TestService_Release_NotPending_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "rel-notpending")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)

	if _, err := svc.Release(context.Background(), a.tenantID, order.ID, ReleaseRequest{}, 1); err != nil {
		t.Fatalf("first Release: %v", err)
	}

	// A second Release call against an already-RELEASED order must be
	// rejected cleanly, not silently re-run.
	if _, err := svc.Release(context.Background(), a.tenantID, order.ID, ReleaseRequest{}, 1); !errors.Is(err, ErrNotPending) {
		t.Errorf("second Release call = %v, want ErrNotPending", err)
	}

	cancelled := newOrderFixture(t, tx, "rel-notpend-cxl")
	order2 := createOrder(t, svc, cancelled.tenantID, cancelled, 10)
	if _, err := svc.Cancel(context.Background(), cancelled.tenantID, order2.ID, CancelRequest{}, 1); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if _, err := svc.Release(context.Background(), cancelled.tenantID, order2.ID, ReleaseRequest{}, 1); !errors.Is(err, ErrNotPending) {
		t.Errorf("Release(cancelled order) = %v, want ErrNotPending", err)
	}
}

func TestService_Release_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "release-cross-a")
	b := newOrderFixture(t, tx, "release-cross-b")
	seedOrderBalance(t, tx, b, "LOT-B", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	if _, err := svc.Release(context.Background(), a.tenantID, order.ID, ReleaseRequest{}, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Release(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}

	reloaded, err := svc.Get(context.Background(), b.tenantID, order.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Release: %v", err)
	}
	if reloaded.Status != StatusPending {
		t.Errorf("order status after rejected cross-tenant Release = %v, want still PENDING", reloaded.Status)
	}
}

// --- CanUpdate is now PENDING-only, strictly narrower than CanCancel ---

func TestService_Update_RejectsOnceReleased(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "upd-after-rel")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)
	releaseOrder(t, svc, a.tenantID, order.ID, false)

	_, err := svc.Update(context.Background(), a.tenantID, order.ID, UpdateRequest{
		WarehouseID: a.warehouseID,
		Items:       []CreateItemRequest{{ProductID: a.productID, OrderedQty: 99, UOM: "PCS"}},
	})
	if !errors.Is(err, ErrNotEditable) {
		t.Fatalf("Update(RELEASED order) = %v, want ErrNotEditable", err)
	}
}

// Cancel must still work after Release (before a picking task exists)
// — Status.CanCancel covers both PENDING and RELEASED.
func TestService_Cancel_AllowedAfterRelease(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "cxl-after-rel")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)
	releaseOrder(t, svc, a.tenantID, order.ID, false)

	resp, err := svc.Cancel(context.Background(), a.tenantID, order.ID, CancelRequest{}, 1)
	if err != nil {
		t.Fatalf("Cancel(RELEASED order): %v", err)
	}
	if resp.Status != StatusCancelled {
		t.Errorf("Status = %v, want CANCELLED", resp.Status)
	}
}

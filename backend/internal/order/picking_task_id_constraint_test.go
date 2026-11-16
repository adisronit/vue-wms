// Regression tests for migrations/0060_fix_orders_picking_task_id_unique_constraint.
// Before that migration, orders.picking_task_id's own
// "CONSTRAINT UQ_orders_picking_task_id UNIQUE (picking_task_id)" was a
// plain UNIQUE constraint, not a filtered index — SQL Server treats
// multiple NULLs in a plain UNIQUE constraint as duplicates, so no two
// orders anywhere in the table could ever both be PENDING/unlinked at
// once (see tenant_isolation_test.go's own
// TestRepository_List_ScopedToTenant doc comment for the original
// discovery and its workaround). These tests exercise the fix directly:
// multiple simultaneous unlinked orders must now succeed, while the
// original intent — at most one order may ever link to the same real
// picking task — must still be enforced for non-NULL values.
package order

import (
	"context"
	"testing"

	"wms-backend/internal/platform/database/sqlerr"
	"wms-backend/internal/platform/testsupport"
)

// createOrderWithRef is createOrder plus an explicit, caller-chosen
// external_reference — needed whenever a test creates more than one
// order for the SAME tenant that must not rely on migrations/0061's own
// fix (kept explicit here for clarity/isolation even though plain
// createOrder would now work too — see external_reference_constraint_test.go
// for the dedicated regression tests covering that fix directly).
// orders.external_reference had its own separate bug of the identical
// shape this file's own migrations/0060 fix addressed for
// picking_task_id: migrations/0048's "CONSTRAINT
// UQ_orders_tenant_external_reference UNIQUE (tenant_id,
// external_reference)" was a plain (non-filtered) composite UNIQUE
// constraint, and SQL Server compares the full key tuple including
// NULLs — so two orders in the SAME tenant both leaving
// external_reference unset collided exactly like the picking_task_id
// bug did. Fixed by migrations/0061.
func createOrderWithRef(t *testing.T, svc *Service, tenantID int64, f orderFixture, orderedQty float64, externalRef string) *Response {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		WarehouseID:       f.warehouseID,
		ExternalReference: &externalRef,
		Items:             []CreateItemRequest{{ProductID: f.productID, OrderedQty: orderedQty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("createOrderWithRef: Create: %v", err)
	}
	return resp
}

// TestOrdersConstraint_MultiplePendingOrders_SameTenant_NoLongerConflict
// is the direct regression test for the bug: three orders, same tenant,
// same warehouse, all left PENDING/unlinked (picking_task_id NULL) at
// once. Before migrations/0060 this failed on the second INSERT with
// "Violation of UNIQUE KEY constraint 'UQ_orders_picking_task_id'...
// duplicate key value is (<NULL>)".
func TestOrdersConstraint_MultiplePendingOrders_SameTenant_NoLongerConflict(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "multi-pending")
	svc, repo := newOrderServiceAndRepo(tx)

	for i := 0; i < 3; i++ {
		order := createOrderWithRef(t, svc, a.tenantID, a, 5, "EXT-MULTI-"+string(rune('A'+i)))
		if order.PickingTaskID != nil {
			t.Fatalf("order %d: PickingTaskID = %v, want nil (not yet linked)", i, *order.PickingTaskID)
		}
	}

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 3 simultaneous PENDING orders", len(rows), total)
	}
	for _, row := range rows {
		if row.Status != string(StatusPending) {
			t.Errorf("order %d status = %q, want PENDING", row.ID, row.Status)
		}
	}
}

// TestOrdersConstraint_DuplicateNonNullPickingTaskID_StillRejected
// proves the filtered index (migrations/0060) preserves the constraint's
// original intent for real values: a picking task may still only ever
// be linked from a single order. No application code path ever attempts
// this (CreatePickingTask always links to a picking task it just created
// itself, which by construction cannot already be linked to anything
// else), so this reaches past Service/Repository to exercise the raw SQL
// UPDATE directly against the constraint.
func TestOrdersConstraint_DuplicateNonNullPickingTaskID_StillRejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "dup-link")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, repo := newOrderServiceAndRepo(tx)

	orderOne := createOrderWithRef(t, svc, a.tenantID, a, 10, "EXT-DUP-1")
	releaseOrder(t, svc, a.tenantID, orderOne.ID, false)
	resp, err := svc.CreatePickingTask(context.Background(), a.tenantID, orderOne.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: orderOne.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1)
	if err != nil {
		t.Fatalf("CreatePickingTask(orderOne): %v", err)
	}
	if resp.PickingTaskID == nil {
		t.Fatalf("orderOne.PickingTaskID is nil after CreatePickingTask")
	}
	realPickingTaskID := *resp.PickingTaskID

	orderTwo := createOrderWithRef(t, svc, a.tenantID, a, 5, "EXT-DUP-2")

	// Bypass LinkPickingTask entirely — simulate the only way a second
	// order could ever end up pointing at an already-linked picking task
	// (a bug elsewhere, or direct data manipulation), to prove the
	// database itself — not just application logic — still refuses it.
	err = tx.Exec(`UPDATE orders SET picking_task_id = ? WHERE id = ?`, realPickingTaskID, orderTwo.ID).Error
	if err == nil {
		t.Fatalf("UPDATE orderTwo.picking_task_id to orderOne's already-linked task succeeded, want a unique constraint violation")
	}
	if !sqlerr.IsUniqueViolation(err) {
		t.Fatalf("UPDATE orderTwo.picking_task_id error = %v, want a unique constraint violation", err)
	}

	// orderTwo must be left exactly as it was (still unlinked) — the
	// rejected UPDATE must not have partially applied.
	reloaded, err := repo.FindByID(context.Background(), a.tenantID, orderTwo.ID)
	if err != nil {
		t.Fatalf("FindByID(orderTwo) after rejected UPDATE: %v", err)
	}
	if reloaded.PickingTaskID != nil {
		t.Errorf("orderTwo.PickingTaskID = %v after rejected UPDATE, want still nil", *reloaded.PickingTaskID)
	}
}

// TestOrdersConstraint_ExistingWorkflow_Unaffected is an end-to-end
// regression check that the normal Create -> CreatePickingTask ->
// Create-another-order workflow still behaves exactly as designed after
// the fix: a second, unrelated order in the same tenant can be created
// and remains independently listable/cancellable while a first order's
// own picking task linkage is unaffected by it.
func TestOrdersConstraint_ExistingWorkflow_Unaffected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "workflow-ok")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)

	linked := createOrderWithRef(t, svc, a.tenantID, a, 10, "EXT-WORKFLOW-1")
	releaseOrder(t, svc, a.tenantID, linked.ID, false)
	linkedResp, err := svc.CreatePickingTask(context.Background(), a.tenantID, linked.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: linked.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1)
	if err != nil {
		t.Fatalf("CreatePickingTask(linked order): %v", err)
	}

	unrelated := createOrderWithRef(t, svc, a.tenantID, a, 7, "EXT-WORKFLOW-2")
	if unrelated.PickingTaskID != nil {
		t.Fatalf("unrelated order's PickingTaskID = %v, want nil", *unrelated.PickingTaskID)
	}

	cancelled, err := svc.Cancel(context.Background(), a.tenantID, unrelated.ID, CancelRequest{}, 1)
	if err != nil {
		t.Fatalf("Cancel(unrelated order): %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Errorf("unrelated order status = %v, want CANCELLED", cancelled.Status)
	}

	reloadedLinked, err := svc.Get(context.Background(), a.tenantID, linked.ID)
	if err != nil {
		t.Fatalf("Get(linked order) after unrelated order's cancel: %v", err)
	}
	if reloadedLinked.Status != StatusPicking || reloadedLinked.PickingTaskID == nil || *reloadedLinked.PickingTaskID != *linkedResp.PickingTaskID {
		t.Errorf("linked order after unrelated order's cancel = %+v, want unaffected PICKING with its own picking_task_id", reloadedLinked)
	}
}

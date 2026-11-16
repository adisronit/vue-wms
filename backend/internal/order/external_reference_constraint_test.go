// Regression tests for migrations/0061_fix_orders_external_reference_unique_constraint.
// Before that migration, orders.external_reference's own
// "CONSTRAINT UQ_orders_tenant_external_reference UNIQUE (tenant_id,
// external_reference)" was a plain composite UNIQUE constraint — SQL
// Server compares the full key tuple including NULLs, so two orders in
// the SAME tenant that both left external_reference unset (the normal
// case) collided (see picking_task_id_constraint_test.go's own
// createOrderWithRef doc comment for the original discovery). These
// tests exercise the fix directly: multiple simultaneous orders with no
// external_reference, in the same tenant, must now succeed; a genuine
// duplicate non-NULL reference within one tenant must still be
// rejected; and the same reference string reused across two different
// tenants must never have collided in the first place (tenant_id is
// part of the key) and must still not collide now.
package order

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// TestOrdersConstraint_MultipleNullExternalReferences_SameTenant_NoLongerConflict
// is the direct regression test for the bug: three orders, same tenant,
// none of them setting external_reference, all created successfully at
// once. Before migrations/0061 the second Create failed with
// "order: external_reference is already used by another order".
func TestOrdersConstraint_MultipleNullExternalReferences_SameTenant_NoLongerConflict(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "multi-null-ref")
	svc, repo := newOrderServiceAndRepo(tx)

	for i := 0; i < 3; i++ {
		order := createOrder(t, svc, a.tenantID, a, 5) // createOrder never sets ExternalReference
		if order.ExternalReference != nil {
			t.Fatalf("order %d: ExternalReference = %v, want nil", i, *order.ExternalReference)
		}
	}

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 3 orders with no external_reference", len(rows), total)
	}
}

// TestOrdersConstraint_DuplicateNonNullExternalReference_SameTenant_StillRejected
// proves the filtered index preserves the constraint's original intent
// for real values: within one tenant, a given external_reference may
// still only ever be used by one order.
func TestOrdersConstraint_DuplicateNonNullExternalReference_SameTenant_StillRejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "dup-ref")
	svc, _ := newOrderServiceAndRepo(tx)

	createOrderWithRef(t, svc, a.tenantID, a, 10, "ERP-DUP-REF")

	ref := "ERP-DUP-REF"
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID:       a.warehouseID,
		ExternalReference: &ref,
		Items:             []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrDuplicateExternalReference) {
		t.Fatalf("Create(duplicate external_reference, same tenant) = %v, want ErrDuplicateExternalReference", err)
	}
}

// TestOrdersConstraint_SameExternalReference_DifferentTenants_Succeeds
// proves tenant_id's own place in the composite key still works
// correctly: the identical reference string used by two different
// tenants was never a conflict (tenant_id differs) and must not become
// one as a side effect of this fix.
func TestOrdersConstraint_SameExternalReference_DifferentTenants_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "shared-ref-a")
	b := newOrderFixture(t, tx, "shared-ref-b")
	svc, _ := newOrderServiceAndRepo(tx)

	const sharedRef = "ERP-SHARED-REF"
	orderA := createOrderWithRef(t, svc, a.tenantID, a, 10, sharedRef)
	orderB := createOrderWithRef(t, svc, b.tenantID, b, 10, sharedRef)

	if orderA.ExternalReference == nil || *orderA.ExternalReference != sharedRef {
		t.Fatalf("tenant A order's ExternalReference = %v, want %q", orderA.ExternalReference, sharedRef)
	}
	if orderB.ExternalReference == nil || *orderB.ExternalReference != sharedRef {
		t.Fatalf("tenant B order's ExternalReference = %v, want %q", orderB.ExternalReference, sharedRef)
	}
}

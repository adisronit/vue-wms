// Phase 33C.2 -- Sortation + Packing Integration tests, non-batch
// regression half. The order-dependent half of this scenario (the real
// SO-001/SO-002/SO-003 three-independent-jobs flow, which needs a real
// internal/order.Service) lives in batch_integration_external_test.go
// as an external "package packing_test" instead — see that file's own
// doc for why: Phase 33D.1 made internal/shipping import internal/
// packing, and internal/order itself already imports internal/shipping
// (order/dto.go), so importing internal/order from THIS (internal,
// "package packing") test file would now be a real import cycle
// (packing (test) -> order -> shipping -> packing). This file needs no
// internal/order at all, so it stays exactly as it always was.
package packing

import (
	"context"
	"errors"
	"testing"

	"wms-backend/internal/pickingbatch"
	"wms-backend/internal/warehouse"

	"wms-backend/internal/platform/testsupport"
)

// TestService_Create_NonBatch_Unchanged is the hard regression bar: the
// ordinary (non-batch) flow must behave byte-for-byte as before Phase
// 33C.2 — order_id stays nil on the resulting job, and supplying one
// against a plain PICK_BY_ORDER task is rejected outright.
func TestService_Create_NonBatch_Unchanged(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "nb-unchg")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	// Exercise the exact same helper shape this package's own pre-
	// existing tenant_isolation_test.go already uses for the ordinary
	// flow (newPackingServiceAndRepo/createCompletedPickingTask), but
	// with batches wired up too — proving SetBatches being present
	// changes NOTHING about this path.
	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	batchRepo := pickingbatch.NewRepository(tx)
	batchSvc := pickingbatch.NewService(batchRepo, warehouse.NewRepository(tx), pickingSvc)
	svc.SetBatches(batchSvc)

	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)

	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PickingTaskID: pickTaskID}, 1)
	if err != nil {
		t.Fatalf("Create (non-batch): %v", err)
	}
	if resp.OrderID != nil {
		t.Errorf("OrderID = %v, want nil for a non-batch job", resp.OrderID)
	}

	// order_id supplied against a non-batch task is rejected.
	someOrderID := int64(1)
	if _, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PickingTaskID: pickTaskID, OrderID: &someOrderID}, 1); !errors.Is(err, ErrOrderIDNotAllowed) {
		// This second call also hits ErrAlreadyPacked territory for the
		// same task, but the order_id guard fires BEFORE any duplicate
		// check runs, so ErrOrderIDNotAllowed is the one and only
		// expected error here.
		t.Fatalf("Create with order_id against non-batch task: err = %v, want ErrOrderIDNotAllowed", err)
	}
}

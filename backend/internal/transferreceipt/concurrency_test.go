// Phase 33E.2 — the concurrency variant: multiple goroutines race to
// receive the last remaining units of one transfer receipt line — at
// most dispatched_qty may ever be received, matching every prior
// phase's own "one UPDATE, bounded in the WHERE clause" concurrency-
// test precedent (packing.Service.PackItem/relocation.Service.Confirm/
// receiving's own AdjustReceivedQty).
package transferreceipt

import (
	"context"
	"math"
	"sync"
	"testing"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
)

func TestService_ReceiveItem_ConcurrentReceiving_NeverOverReceives(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "cc")
	st := newTRStack(tx)
	const transferQty = 100.0
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-CC", 200, transferQty, "cc")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	itemID := receipt.Items[0].ID

	// Commit this setup for real — goroutines below each open their own
	// transaction against the shared connection pool (db), which cannot
	// see tx's uncommitted rows otherwise. Matches
	// packing.TestService_PackItem_ConcurrentPacking_NeverOverpacks'
	// own commit-then-cleanup shape.
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit fixture setup: %v", err)
	}
	t.Cleanup(func() {
		cleanup := db.Begin()
		cleanup.Exec("DELETE FROM transfer_receipt_confirmations WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM transfer_receipt_items WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM transfer_receipts WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM loading_trip_cartons WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM loading_trip_shipments WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM loading_trips WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM shipment_confirmations WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM shipment_items WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM shipments WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM carton_contents WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM cartons WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM packing_job_items WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM packing_confirmations WHERE tenant_id = ?", f.tenantID)
		// packing_jobs.order_id references orders, and shipments.
		// picking_task_id/packing_jobs.picking_task_id reference
		// picking_tasks — packing_jobs must therefore be deleted BEFORE
		// both orders and picking_tasks (the earlier, broken ordering
		// that deleted picking_tasks/orders first left packing_jobs'
		// own FK blocking everything above it, which in turn left
		// picking_tasks/warehouses/tenants undeleted — exactly the
		// dangling-row bug this reordering fixes; see sys.foreign_keys
		// against picking_tasks for the full dependency list this
		// ordering was derived from).
		cleanup.Exec("DELETE FROM packing_jobs WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM order_items WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM orders WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM picking_scan_log WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM picking_confirmations WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM picking_task_items WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM picking_tasks WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM inventory WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM locations WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM zones WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM warehouses WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM products WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM product_categories WHERE tenant_id = ?", f.tenantID)
		cleanup.Exec("DELETE FROM tenants WHERE id = ?", f.tenantID)
		_ = cleanup.Commit().Error
	})

	// A fresh Service bound directly to db (the connection pool), not
	// tx — each ReceiveItem call below opens its own independent
	// transaction via Repository.Transaction. shipments/orders are nil:
	// ReceiveItem never dereferences them (only Create does).
	liveProductRepo := product.NewRepository(db)
	liveLocationRepo := location.NewRepository(db)
	liveInvSvc := inventory.NewService(inventory.NewRepository(db), liveProductRepo, liveLocationRepo)
	liveSvc := NewService(NewRepository(db), nil, nil, liveInvSvc)

	const goroutines = 6
	const perCall = 30.0 // 6 * 30 = 180 > transferQty (100): at most 3 can win.
	results := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := liveSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, itemID, ReceiveItemRequest{
				IdempotencyKey: "cc-key-" + string(rune('A'+i)),
				LocationID:     f.destLocationID, Quantity: perCall,
			}, 1)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			t.Logf("ReceiveItem goroutine result: %v", err)
		}
	}

	// Verify the real safety invariant directly from committed DB state
	// — dispatched_qty is 100, perCall is 30, so at most 3 of the 6
	// goroutines may ever succeed; received_qty must never exceed 100
	// and must be an exact multiple of perCall (no partial-write
	// corruption / lost updates).
	itemRepo := NewRepository(db)
	finalItem, err := itemRepo.FindItemByID(context.Background(), f.tenantID, itemID)
	if err != nil {
		t.Fatalf("FindItemByID: %v", err)
	}
	if finalItem.ReceivedQty > transferQty {
		t.Errorf("received_qty = %v, must never exceed dispatched_qty (%v) — over-received!", finalItem.ReceivedQty, transferQty)
	}
	if math.Mod(finalItem.ReceivedQty, perCall) != 0 {
		t.Errorf("received_qty = %v, want an exact multiple of %v (no double-counting or partial-write corruption)", finalItem.ReceivedQty, perCall)
	}
	if successes*int(perCall) != int(finalItem.ReceivedQty) {
		t.Errorf("successes (%d) * perCall (%v) = %v, want equal to committed received_qty %v", successes, perCall, successes*int(perCall), finalItem.ReceivedQty)
	}
	if finalItem.ReceivedQty == 0 {
		t.Error("received_qty = 0, test is vacuous if nothing was received")
	}

	// The destination warehouse's own balance matches exactly what was
	// actually, successfully received — no phantom credit from a
	// rejected over-receipt attempt.
	destBalance := sumInventoryQtyAtWarehouse(t, db, f.tenantID, f.productID, f.destWarehouseID)
	if destBalance != finalItem.ReceivedQty {
		t.Errorf("destination warehouse balance = %v, want equal to received_qty %v", destBalance, finalItem.ReceivedQty)
	}
}

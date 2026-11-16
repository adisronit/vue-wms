// Phase 33C — Packing tenant isolation, overpacking prevention, and
// inventory-untouched proofs. Mirrors internal/relocation's own
// tenant_isolation_test.go shape (fresh tenant per test, real seeded
// rows via the real product/location/warehouse/inventory/picking
// repositories, testsupport.ConnectDB + BeginRollbackTx) but drives a
// real Picking Create->Confirm(PICKED) first — a packing job can only
// ever be created from an already-terminal picking task, so every test
// here builds on top of that real upstream workflow rather than seeding
// picking_task_items rows directly.
package packing

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
)

type packingFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newPackingFixture(t *testing.T, tx *gorm.DB, label string) packingFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"pack-iso-"+label, "Packing Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newPackingFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newPackingFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newPackingFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newPackingFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newPackingFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newPackingFixture(%s): seed location: %v", label, err)
	}

	return packingFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

func seedPackingBalance(t *testing.T, tx *gorm.DB, f packingFixture, lotNo string, quantity, reserved float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedPackingBalance: %v", err)
	}
}

// newPackingServiceAndRepo wires a *Service bound to tx, plus a real
// *picking.Service bound to the exact same tx — Create's
// pickingChecker.Get call must see the same uncommitted picking rows
// this test just seeded.
func newPackingServiceAndRepo(tx *gorm.DB) (*Service, *Repository, *picking.Service) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, location.NewRepository(tx))
	pickingSvc := picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
	pickingSvc.SetUsers(user.NewRepository(tx))

	svc := NewService(repo, pickingSvc)
	svc.SetUsers(user.NewRepository(tx))
	return svc, repo, pickingSvc
}

// createCompletedPickingTask drives a real Picking Create ->
// Confirm(PICKED) for one line, returning the terminal task's ID — the
// only legitimate way a packing job may ever be created, per
// Service.Create's own doc.
func createCompletedPickingTask(t *testing.T, pickingSvc *picking.Service, f packingFixture, lotNo string, qty float64) int64 {
	t.Helper()
	resp, err := pickingSvc.Create(context.Background(), f.tenantID, picking.CreateRequest{
		OrderNo: "SO-PACK-TEST", WarehouseID: f.warehouseID,
		Items: []picking.CreateItemRequest{
			{ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo, OrderedQty: qty, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("createCompletedPickingTask: picking Create: %v", err)
	}
	itemID := resp.Items[0].ID

	confirmed, err := pickingSvc.Confirm(context.Background(), f.tenantID, resp.ID, picking.ConfirmRequest{
		IdempotencyKey: "pack-test-pick-" + lotNo,
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: qty}},
	}, 1)
	if err != nil {
		t.Fatalf("createCompletedPickingTask: picking Confirm: %v", err)
	}
	if confirmed.Status != picking.StatusCompleted {
		t.Fatalf("createCompletedPickingTask: picking task status = %v, want COMPLETED", confirmed.Status)
	}
	return resp.ID
}

// createPackingJob drives Service.Create against an already-completed
// picking task and returns the new job's ID.
func createPackingJob(t *testing.T, svc *Service, tenantID, pickingTaskID int64) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{PickingTaskID: pickingTaskID}, 1)
	if err != nil {
		t.Fatalf("createPackingJob: Create: %v", err)
	}
	return resp.ID
}

// --- IDOR ---

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "get-a")
	b := newPackingFixture(t, tx, "get-b")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)

	if _, err := svc.Get(context.Background(), b.tenantID, jobID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(wrong tenant) error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(context.Background(), a.tenantID, jobID); err != nil {
		t.Errorf("Get(own tenant) unexpected error: %v", err)
	}
}

func TestService_Create_RejectsCrossTenantPickingTask(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "create-a")
	b := newPackingFixture(t, tx, "create-b")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)

	// b's own tenant must never be able to create a packing job against
	// a's picking task — the cross-tenant id is rejected as
	// ErrPickingTaskNotFound exactly like a nonexistent one.
	if _, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PickingTaskID: pickTaskID}, 1); !errors.Is(err, ErrPickingTaskNotFound) {
		t.Errorf("Create(wrong tenant) error = %v, want ErrPickingTaskNotFound", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "list-a")
	b := newPackingFixture(t, tx, "list-b")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)
	seedPackingBalance(t, tx, b, "LOT-B", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	aPick := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	bPick := createCompletedPickingTask(t, pickingSvc, b, "LOT-B", 10)
	createPackingJob(t, svc, a.tenantID, aPick)
	createPackingJob(t, svc, b.tenantID, bPick)

	rows, total, err := svc.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (tenant a's own job only)", total)
	}
	for _, r := range rows {
		if r.OrderNo != "SO-PACK-TEST" {
			t.Errorf("unexpected row leaked into tenant a's list: %+v", r)
		}
	}
}

func TestService_Sources_ExcludesAlreadyPackedAndScopesToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "sources-a")
	b := newPackingFixture(t, tx, "sources-b")
	seedPackingBalance(t, tx, a, "LOT-A1", 100, 0)
	seedPackingBalance(t, tx, a, "LOT-A2", 100, 0)
	seedPackingBalance(t, tx, b, "LOT-B1", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	unpackedTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A1", 5)
	alreadyPackedTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A2", 5)
	createPackingJob(t, svc, a.tenantID, alreadyPackedTaskID)
	createCompletedPickingTask(t, pickingSvc, b, "LOT-B1", 5)

	sources, err := svc.Sources(context.Background(), a.tenantID)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1 (only the unpacked task, scoped to tenant a)", len(sources))
	}
	if sources[0].PickingTaskID != unpackedTaskID {
		t.Errorf("sources[0].PickingTaskID = %d, want %d", sources[0].PickingTaskID, unpackedTaskID)
	}
}

// --- Create validation ---

func TestService_Create_RejectsNonTerminalPickingTask(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "nonterm")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	resp, err := pickingSvc.Create(context.Background(), a.tenantID, picking.CreateRequest{
		OrderNo: "SO-PENDING", WarehouseID: a.warehouseID,
		Items: []picking.CreateItemRequest{{ProductID: a.productID, LocationID: a.locationID, LotNo: "LOT-A", OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("picking Create: %v", err)
	}

	if _, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PickingTaskID: resp.ID}, 1); !errors.Is(err, ErrNotPackable) {
		t.Errorf("Create(PENDING picking task) error = %v, want ErrNotPackable", err)
	}
}

func TestService_Create_RejectsDuplicateJobForSamePickingTask(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "dup")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	createPackingJob(t, svc, a.tenantID, pickTaskID)

	if _, err := svc.Create(context.Background(), a.tenantID, CreateRequest{PickingTaskID: pickTaskID}, 1); !errors.Is(err, ErrAlreadyPacked) {
		t.Errorf("Create(duplicate) error = %v, want ErrAlreadyPacked", err)
	}
}

func TestService_Create_CopiesPickedQtyVerbatim(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "copy")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 37)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)

	job, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(job.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(job.Items))
	}
	if job.Items[0].PickedQty != 37 {
		t.Errorf("PickedQty = %v, want 37 (copied verbatim from picking)", job.Items[0].PickedQty)
	}
	if job.Items[0].PackedQty != 0 {
		t.Errorf("PackedQty = %v, want 0 (nothing packed yet)", job.Items[0].PackedQty)
	}
	if job.Status != StatusDraft {
		t.Errorf("Status = %v, want DRAFT", job.Status)
	}
}

// --- PackItem / overpacking prevention ---

func TestService_PackItem_RejectsOverpacking(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "overpack")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)

	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID

	cartonResp, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}

	// Pack exactly the full picked_qty (10) — must succeed.
	updated, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "pack-1", ItemID: itemID, CartonID: cartonResp.ID,
		ScannedSKU: "SKU-overpack", Quantity: 10,
	}, 1)
	if err != nil {
		t.Fatalf("PackItem(exactly picked_qty): %v", err)
	}
	if updated.Items[0].PackedQty != 10 {
		t.Errorf("PackedQty = %v, want 10", updated.Items[0].PackedQty)
	}
	if updated.Items[0].Status != ItemStatusPacked {
		t.Errorf("Status = %v, want PACKED", updated.Items[0].Status)
	}

	// Any further pack attempt — even 1 more unit, even into a second
	// carton — must be rejected outright, never clamped.
	carton2, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateCarton (second): %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "pack-2", ItemID: itemID, CartonID: carton2.ID,
		ScannedSKU: "SKU-overpack", Quantity: 1,
	}, 1); !errors.Is(err, ErrOverpack) {
		t.Errorf("PackItem(1 unit over picked_qty) error = %v, want ErrOverpack", err)
	}

	// Confirm packed_qty was NOT silently clamped or partially applied —
	// it must still read exactly 10, not 10 then rejected-but-written, or
	// 11.
	final, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get after rejected overpack: %v", err)
	}
	if final.Items[0].PackedQty != 10 {
		t.Errorf("PackedQty after rejected overpack = %v, want unchanged 10 (not clamped, not partially applied)", final.Items[0].PackedQty)
	}
}

func TestService_PackItem_RejectsOverpackingAcrossMultipleCartons(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "overpack-multi")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID

	carton1, _ := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)
	carton2, _ := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)

	// Split 10 units: 6 into carton1, then try to pack 6 more into
	// carton2 — only 4 remain, so this must be rejected (not clamped to
	// 4).
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "split-1", ItemID: itemID, CartonID: carton1.ID, ScannedSKU: "SKU-overpack-multi", Quantity: 6,
	}, 1); err != nil {
		t.Fatalf("PackItem(6 into carton1): %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "split-2", ItemID: itemID, CartonID: carton2.ID, ScannedSKU: "SKU-overpack-multi", Quantity: 6,
	}, 1); !errors.Is(err, ErrOverpack) {
		t.Errorf("PackItem(6 more, only 4 remain) error = %v, want ErrOverpack", err)
	}

	final, _ := svc.Get(context.Background(), a.tenantID, jobID)
	if final.Items[0].PackedQty != 6 {
		t.Errorf("PackedQty = %v, want 6 (the rejected second call must not have applied any partial amount)", final.Items[0].PackedQty)
	}

	// The remaining 4 units can still legitimately be packed.
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "split-3", ItemID: itemID, CartonID: carton2.ID, ScannedSKU: "SKU-overpack-multi", Quantity: 4,
	}, 1); err != nil {
		t.Fatalf("PackItem(remaining 4): %v", err)
	}
	final, _ = svc.Get(context.Background(), a.tenantID, jobID)
	if final.Items[0].PackedQty != 10 {
		t.Errorf("PackedQty = %v, want 10 after packing the exact remainder", final.Items[0].PackedQty)
	}
	if final.Items[0].Status != ItemStatusPacked {
		t.Errorf("Status = %v, want PACKED", final.Items[0].Status)
	}
	if len(final.Items[0].CartonLPNs) != 2 {
		t.Errorf("CartonLPNs = %v, want 2 distinct cartons", final.Items[0].CartonLPNs)
	}
}

// TestService_PackItem_ConcurrentPacking_NeverOverpacks is the
// concurrency variant: two goroutines race to pack the last remaining
// units of one line — at most the total available (picked_qty) may
// ever be packed, matching Phase 32's own relocation concurrency-test
// precedent and Phase 33B's re-pick test.
func TestService_PackItem_ConcurrentPacking_NeverOverpacks(t *testing.T) {
	db := testsupport.ConnectDB(t)

	tx := testsupport.BeginRollbackTx(t, db)
	a := newPackingFixture(t, tx, "concurrent")
	seedPackingBalance(t, tx, a, "LOT-CC", 100, 0)
	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-CC", 100)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := job.Items[0].ID
	carton, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}
	cartonID := carton.ID

	// Commit this setup for real — goroutines below each open their own
	// transaction against the shared connection pool (db), which cannot
	// see tx's uncommitted rows otherwise.
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit fixture setup: %v", err)
	}
	t.Cleanup(func() {
		cleanup := db.Begin()
		cleanup.Exec("DELETE FROM packing_confirmations WHERE packing_job_id = ?", jobID)
		cleanup.Exec("DELETE FROM carton_contents WHERE carton_id = ?", cartonID)
		cleanup.Exec("DELETE FROM cartons WHERE packing_job_id = ?", jobID)
		cleanup.Exec("DELETE FROM packing_job_items WHERE packing_job_id = ?", jobID)
		cleanup.Exec("DELETE FROM packing_jobs WHERE id = ?", jobID)
		cleanup.Exec("DELETE FROM picking_confirmations WHERE picking_task_id = ?", pickTaskID)
		cleanup.Exec("DELETE FROM picking_task_items WHERE picking_task_id = ?", pickTaskID)
		cleanup.Exec("DELETE FROM picking_tasks WHERE id = ?", pickTaskID)
		cleanup.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ?", a.tenantID)
		cleanup.Exec("DELETE FROM inventory WHERE tenant_id = ?", a.tenantID)
		cleanup.Exec("DELETE FROM locations WHERE tenant_id = ?", a.tenantID)
		cleanup.Exec("DELETE FROM zones WHERE tenant_id = ?", a.tenantID)
		cleanup.Exec("DELETE FROM warehouses WHERE tenant_id = ?", a.tenantID)
		cleanup.Exec("DELETE FROM products WHERE tenant_id = ?", a.tenantID)
		cleanup.Exec("DELETE FROM product_categories WHERE tenant_id = ?", a.tenantID)
		cleanup.Exec("DELETE FROM tenants WHERE id = ?", a.tenantID)
		_ = cleanup.Commit().Error
	})

	// A fresh Service bound directly to db (the connection pool), not
	// tx — each PackItem call below opens its own independent
	// transaction via Repository.Transaction.
	liveRepo := NewRepository(db)
	livePickingSvc := picking.NewService(picking.NewRepository(db), product.NewRepository(db), warehouse.NewRepository(db), inventory.NewService(inventory.NewRepository(db), product.NewRepository(db), location.NewRepository(db)))
	liveSvc := NewService(liveRepo, livePickingSvc)

	const goroutines = 6
	const perCall = 20.0
	results := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := liveSvc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
				IdempotencyKey: "concurrent-key-" + string(rune('A'+i)),
				ItemID:         itemID, CartonID: cartonID, ScannedSKU: "SKU-concurrent", Quantity: perCall,
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
			t.Logf("PackItem goroutine result: %v", err)
		}
	}

	// Verify the real safety invariant directly from committed DB state
	// — picked_qty is 100, perCall is 20, so at most 5 of the 6
	// goroutines may ever succeed; packed_qty must never exceed 100 and
	// must be an exact multiple of perCall (no partial-write corruption).
	itemRepo := NewRepository(db)
	finalItem, err := itemRepo.FindItemByID(context.Background(), a.tenantID, itemID)
	if err != nil {
		t.Fatalf("FindItemByID: %v", err)
	}
	if finalItem.PackedQty > 100 {
		t.Errorf("packed_qty = %v, must never exceed picked_qty (100) — overpacked!", finalItem.PackedQty)
	}
	if math.Mod(finalItem.PackedQty, perCall) != 0 {
		t.Errorf("packed_qty = %v, want an exact multiple of %v (no double-counting or partial-write corruption)", finalItem.PackedQty, perCall)
	}
	if successes*int(perCall) != int(finalItem.PackedQty) {
		t.Errorf("successes (%d) * perCall (%v) = %v, want equal to committed packed_qty %v", successes, perCall, successes*int(perCall), finalItem.PackedQty)
	}
	if finalItem.PackedQty == 0 {
		t.Error("packed_qty = 0, test is vacuous if nothing packed")
	}
}

// --- PackItem scan verification ---

func TestService_PackItem_RejectsScanMismatch(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "scanmismatch")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID
	carton, _ := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)

	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "mismatch-1", ItemID: itemID, CartonID: carton.ID,
		ScannedSKU: "WRONG-SKU", Quantity: 5,
	}, 1); !errors.Is(err, ErrScanMismatch) {
		t.Errorf("PackItem(wrong SKU) error = %v, want ErrScanMismatch", err)
	}

	final, _ := svc.Get(context.Background(), a.tenantID, jobID)
	if final.Items[0].PackedQty != 0 {
		t.Errorf("PackedQty after rejected scan mismatch = %v, want 0", final.Items[0].PackedQty)
	}
}

func TestService_PackItem_RejectsClosedCarton(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "closedcarton")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID
	carton, _ := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)

	if _, err := svc.UpdateCarton(context.Background(), a.tenantID, jobID, carton.ID, UpdateCartonRequest{
		PackagingType: "BOX", WeightUnit: "KG", DimensionUnit: "CM", Status: "CLOSED",
	}); err != nil {
		t.Fatalf("UpdateCarton(close): %v", err)
	}

	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "closed-1", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-closedcarton", Quantity: 5,
	}, 1); !errors.Is(err, ErrCartonNotOpen) {
		t.Errorf("PackItem(closed carton) error = %v, want ErrCartonNotOpen", err)
	}
}

func TestService_PackItem_DuplicateIdempotencyKey_DoesNotDoublePack(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "idempotent")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID
	carton, _ := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)

	req := PackItemRequest{IdempotencyKey: "same-key", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-idempotent", Quantity: 5}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, req, 1); err != nil {
		t.Fatalf("first PackItem: %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, req, 1); err != nil {
		t.Fatalf("duplicate-key PackItem should return the current state, not an error: %v", err)
	}

	final, _ := svc.Get(context.Background(), a.tenantID, jobID)
	if final.Items[0].PackedQty != 5 {
		t.Errorf("PackedQty after duplicate-key retry = %v, want 5 (not double-packed to 10)", final.Items[0].PackedQty)
	}
}

// --- Exception / Checker confirm / Cancel ---

func TestService_FlagException_TerminatesLine(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "exception")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID

	updated, err := svc.FlagException(context.Background(), a.tenantID, jobID, itemID, FlagExceptionRequest{
		Reason: "DAMAGED", Quantity: 3, Remark: strPtr("crushed carton"),
	}, 1)
	if err != nil {
		t.Fatalf("FlagException: %v", err)
	}
	if updated.Items[0].Status != ItemStatusException {
		t.Errorf("Status = %v, want EXCEPTION", updated.Items[0].Status)
	}
	if !updated.Items[0].ExceptionFlag {
		t.Error("ExceptionFlag = false, want true")
	}
	if updated.Status != StatusInProgress {
		t.Errorf("job Status = %v, want IN_PROGRESS", updated.Status)
	}
}

func TestService_CheckerConfirm_RequiresEveryLineTerminal(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "checker-notready")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)

	// DRAFT (no activity at all) must be rejected.
	if _, err := svc.CheckerConfirm(context.Background(), a.tenantID, jobID, CheckerConfirmRequest{}, 2); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("CheckerConfirm(DRAFT) error = %v, want ErrInvalidTransition", err)
	}

	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID
	carton, _ := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "partial-1", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-checker-notready", Quantity: 5,
	}, 1); err != nil {
		t.Fatalf("PackItem(partial): %v", err)
	}

	// IN_PROGRESS but the line is only PARTIALLY_PACKED, not terminal —
	// must still be rejected.
	if _, err := svc.CheckerConfirm(context.Background(), a.tenantID, jobID, CheckerConfirmRequest{}, 2); !errors.Is(err, ErrNotReadyToComplete) {
		t.Errorf("CheckerConfirm(line not terminal) error = %v, want ErrNotReadyToComplete", err)
	}
}

func TestService_CheckerConfirm_CompletesJobWhenEveryLineTerminal(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "checker-ok")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID
	carton, _ := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{}, 1)
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "full-1", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-checker-ok", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem(full): %v", err)
	}

	checkerUserID := int64(1)
	resp, err := svc.CheckerConfirm(context.Background(), a.tenantID, jobID, CheckerConfirmRequest{}, checkerUserID)
	if err != nil {
		t.Fatalf("CheckerConfirm: %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Errorf("Status = %v, want COMPLETED", resp.Status)
	}
	if resp.CheckerID == nil || *resp.CheckerID != checkerUserID {
		t.Errorf("CheckerID = %v, want %d", resp.CheckerID, checkerUserID)
	}
	if resp.CompletedAt == nil {
		t.Error("CompletedAt is nil, want set")
	}

	// A second CheckerConfirm call must be rejected — already COMPLETED.
	if _, err := svc.CheckerConfirm(context.Background(), a.tenantID, jobID, CheckerConfirmRequest{}, checkerUserID); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("second CheckerConfirm error = %v, want ErrInvalidTransition", err)
	}
}

func TestService_Cancel_CrossTenant_NotCancellable(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "cancel-a")
	b := newPackingFixture(t, tx, "cancel-b")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)

	if _, err := svc.Cancel(context.Background(), b.tenantID, jobID, CancelRequest{}, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Cancel(wrong tenant) error = %v, want ErrNotFound", err)
	}
}

func TestService_Cancel_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "cancel-ok")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)

	resp, err := svc.Cancel(context.Background(), a.tenantID, jobID, CancelRequest{Remark: strPtr("order cancelled")}, 1)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if resp.Status != StatusCancelled {
		t.Errorf("Status = %v, want CANCELLED", resp.Status)
	}

	if _, err := svc.Cancel(context.Background(), a.tenantID, jobID, CancelRequest{}, 1); !errors.Is(err, ErrNotCancellable) {
		t.Errorf("double-cancel error = %v, want ErrNotCancellable", err)
	}
}

// --- The inventory-untouched proof ---

// TestFullPackCycle_NeverTouchesInventory proves that a full
// pack+checker-confirm cycle leaves on-hand/reserved quantities exactly
// as Picking's own Confirm(PICKED) left them — byte-for-byte unchanged
// before and after — since Packing is a pure physical-handling/
// traceability layer over already-deducted stock and must never call
// any internal/inventory mutation function.
func TestFullPackCycle_NeverTouchesInventory(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "inv-untouched")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 25)

	invRepo := inventory.NewRepository(tx)
	before, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance(before): %v", err)
	}
	// Sanity: picking's own Confirm already deducted 25 from the
	// original 100 — this is the baseline Packing must never disturb.
	if before.Quantity != 75 {
		t.Fatalf("baseline Quantity = %v, want 75 (100 - 25 picked) — picking fixture itself is wrong", before.Quantity)
	}

	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := job.Items[0].ID

	carton1, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{PackagingType: "BOX"}, 1)
	if err != nil {
		t.Fatalf("CreateCarton 1: %v", err)
	}
	carton2, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{PackagingType: "PALLET"}, 1)
	if err != nil {
		t.Fatalf("CreateCarton 2: %v", err)
	}

	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "inv-1", ItemID: itemID, CartonID: carton1.ID, ScannedSKU: "SKU-inv-untouched", Quantity: 15,
	}, 1); err != nil {
		t.Fatalf("PackItem(15 into carton1): %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "inv-2", ItemID: itemID, CartonID: carton2.ID, ScannedSKU: "SKU-inv-untouched", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem(10 into carton2): %v", err)
	}

	if _, err := svc.CheckerConfirm(context.Background(), a.tenantID, jobID, CheckerConfirmRequest{}, 1); err != nil {
		t.Fatalf("CheckerConfirm: %v", err)
	}

	after, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance(after): %v", err)
	}

	if after.Quantity != before.Quantity {
		t.Errorf("Quantity changed by the full pack+checker-confirm cycle: before=%v after=%v, want byte-for-byte identical", before.Quantity, after.Quantity)
	}
	if after.ReservedQuantity != before.ReservedQuantity {
		t.Errorf("ReservedQuantity changed by the full pack+checker-confirm cycle: before=%v after=%v, want byte-for-byte identical", before.ReservedQuantity, after.ReservedQuantity)
	}
	if after.HoldQuantity != before.HoldQuantity {
		t.Errorf("HoldQuantity changed by the full pack+checker-confirm cycle: before=%v after=%v, want byte-for-byte identical", before.HoldQuantity, after.HoldQuantity)
	}

	// Also assert no new inventory_transactions ledger row was written
	// by anything in this entire cycle beyond Picking's own original
	// PICK row.
	var ledgerCount int64
	if err := tx.Raw(
		"SELECT COUNT(*) FROM inventory_transactions WHERE tenant_id = ? AND product_id = ? AND location_id = ?",
		a.tenantID, a.productID, a.locationID,
	).Scan(&ledgerCount).Error; err != nil {
		t.Fatalf("count inventory_transactions: %v", err)
	}
	if ledgerCount != 1 {
		t.Errorf("inventory_transactions row count = %d, want exactly 1 (Picking's own PICK row — Packing must write none of its own)", ledgerCount)
	}
}

func strPtr(s string) *string { return &s }

// --- PDF smoke test ---

func TestPDFService_RenderPackingListAndCartonLabels_ProduceValidPDFs(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "pdf-smoke")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, _ := svc.Get(context.Background(), a.tenantID, jobID)
	itemID := job.Items[0].ID
	carton, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{
		PackagingType: "BOX", GrossWeight: floatPtr(5.2), NetWeight: floatPtr(4.8),
		CartonLength: floatPtr(30), CartonWidth: floatPtr(20), CartonHeight: floatPtr(15),
	}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "pdf-1", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-pdf-smoke", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)

	data, filename, err := pdfSvc.RenderPackingList(context.Background(), a.tenantID, 1, jobID)
	if err != nil {
		t.Fatalf("RenderPackingList: %v", err)
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Error("RenderPackingList did not return a valid, non-empty PDF")
	}
	if filename == "" {
		t.Error("RenderPackingList returned an empty filename")
	}

	labelData, labelFilename, err := pdfSvc.RenderCartonLabels(context.Background(), a.tenantID, 1, jobID, 0)
	if err != nil {
		t.Fatalf("RenderCartonLabels: %v", err)
	}
	if len(labelData) == 0 || !bytes.HasPrefix(labelData, []byte("%PDF-")) {
		t.Error("RenderCartonLabels did not return a valid, non-empty PDF")
	}
	if labelFilename == "" {
		t.Error("RenderCartonLabels returned an empty filename")
	}

	singleData, _, err := pdfSvc.RenderCartonLabels(context.Background(), a.tenantID, 1, jobID, carton.ID)
	if err != nil {
		t.Fatalf("RenderCartonLabels(single carton): %v", err)
	}
	if len(singleData) == 0 || !bytes.HasPrefix(singleData, []byte("%PDF-")) {
		t.Error("RenderCartonLabels(single carton) did not return a valid, non-empty PDF")
	}
}

func floatPtr(f float64) *float64 { return &f }

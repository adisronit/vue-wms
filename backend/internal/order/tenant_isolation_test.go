// Phase 30 Batch 3 — Order tenant isolation. Mirrors
// internal/picking/tenant_isolation_test.go's own shape (Batch 2: fresh
// tenant per test, real seeded rows, testsupport.ConnectDB +
// BeginRollbackTx) but covers the surface unique to this package: IDOR
// on order lookup/list, cross-tenant Product ownership verification at
// order-item creation, the atomic ClaimForPickingTask guard, the full
// Order->Picking CreatePickingTask orchestration under real tenant
// scoping (including rollback when Picking itself rejects the request),
// and concurrent CreatePickingTask calls against the same order.
package order

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
)

// orderFixture is one fully-seeded tenant: a product, a warehouse, and a
// location within it — real rows via the real product/location/
// warehouse repositories, not stubs, so ownership verification exercises
// the actual FindByIDForTenant query this batch added to internal/order,
// and the full Order->Picking orchestration exercises Picking's own
// Batch 2 tenant scoping too.
type orderFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newOrderFixture(t *testing.T, tx *gorm.DB, label string) orderFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"order-iso-"+label, "Order Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newOrderFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newOrderFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newOrderFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newOrderFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newOrderFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newOrderFixture(%s): seed location: %v", label, err)
	}

	return orderFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

func seedOrderBalance(t *testing.T, tx *gorm.DB, f orderFixture, lotNo string, quantity, reserved float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity, ReservedQuantity: reserved,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedOrderBalance: %v", err)
	}
}

// newOrderServiceAndRepo wires the full real stack Order Management
// orchestrates through — product/warehouse (ownership checks),
// picking.Service (CreateTx/Get, Batch 2's own tenant scoping), and
// shipping.Service (FindByPickingTaskID) — exactly the dependency graph
// cmd/server/main.go wires in production, bound to the same test
// transaction throughout.
func newOrderServiceAndRepo(tx *gorm.DB) (*Service, *Repository) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	pickingRepo := picking.NewRepository(tx)
	pickingSvc := picking.NewService(pickingRepo, productRepo, warehouseRepo, invSvc)

	shippingRepo := shipping.NewRepository(tx)
	shippingSvc := shipping.NewService(shippingRepo, pickingSvc)

	return NewService(repo, productRepo, warehouseRepo, pickingSvc, shippingSvc), repo
}

// createOrder runs Service.Create end-to-end (through the real
// transaction path) and returns the full response, including each
// line's own order_item_id — needed to build a CreatePickingTaskRequest
// later.
func createOrder(t *testing.T, svc *Service, tenantID int64, f orderFixture, orderedQty float64) *Response {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		WarehouseID: f.warehouseID,
		Items:       []CreateItemRequest{{ProductID: f.productID, OrderedQty: orderedQty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("createOrder: Create: %v", err)
	}
	return resp
}

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "find-a")
	b := newOrderFixture(t, tx, "find-b")

	svc, repo := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	if _, err := repo.FindByID(context.Background(), a.tenantID, order.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByID(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByID(context.Background(), b.tenantID, order.ID); err != nil {
		t.Errorf("FindByID(tenantB, its own order) = %v, want success", err)
	}
}

func TestRepository_FindDetail_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "detail-a")
	b := newOrderFixture(t, tx, "detail-b")

	svc, repo := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	if _, err := repo.FindDetail(context.Background(), a.tenantID, order.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindDetail(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "get-a")
	b := newOrderFixture(t, tx, "get-b")

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	if _, err := svc.Get(context.Background(), a.tenantID, order.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}
}

// TestRepository_List_ScopedToTenant links tenant A's order to a real
// picking task immediately after creating it (rather than leaving two
// PENDING orders — one per tenant — both with a NULL picking_task_id at
// once). This sidesteps a genuine, pre-existing, unrelated schema bug
// this batch discovered: migrations/0038's own
// "CONSTRAINT UQ_orders_picking_task_id UNIQUE (picking_task_id)" is a
// plain UNIQUE constraint, not a filtered index (`WHERE picking_task_id
// IS NOT NULL`) — SQL Server treats multiple NULLs in a plain UNIQUE
// constraint as duplicates, so two orders anywhere in the table (even in
// different tenants, since this constraint carries no tenant_id column
// at all) can never both be PENDING/unlinked at the same time. Confirmed
// directly against the disposable test database; out of scope to fix
// here (a pre-existing correctness bug affecting every tenant equally,
// not a tenant-isolation/IDOR issue, and this batch is not authorized to
// write migrations) — reported, not silently worked around without a
// trace.
func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "list-a")
	b := newOrderFixture(t, tx, "list-b")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, repo := newOrderServiceAndRepo(tx)
	orderA := createOrder(t, svc, a.tenantID, a, 5)
	if _, err := svc.CreatePickingTask(context.Background(), a.tenantID, orderA.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: orderA.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1); err != nil {
		t.Fatalf("CreatePickingTask(tenant A's order, to give it a non-NULL picking_task_id): %v", err)
	}
	createOrder(t, svc, b.tenantID, b, 5)

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's order)", len(rows), total)
	}
}

// --- Service-layer ownership verification at order-item creation ---

func TestService_Create_RejectsCrossTenantProduct(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "cp-a")
	b := newOrderFixture(t, tx, "cp-b")

	svc, _ := newOrderServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID,
		Items:       []CreateItemRequest{{ProductID: b.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("Create(tenantA, tenantB's product_id) = %v, want ErrProductNotFound", err)
	}
}

func TestService_Create_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "create-ok")

	svc, _ := newOrderServiceAndRepo(tx)
	resp := createOrder(t, svc, a.tenantID, a, 10)
	if resp.Status != StatusPending || len(resp.Items) != 1 {
		t.Fatalf("Create response = %+v, want 1 PENDING item", resp)
	}
}

// --- Update: IDOR on the write path ---

func TestService_Update_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "upd-a")
	b := newOrderFixture(t, tx, "upd-b")

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	_, err := svc.Update(context.Background(), a.tenantID, order.ID, UpdateRequest{
		WarehouseID: b.warehouseID,
		Items:       []CreateItemRequest{{ProductID: b.productID, OrderedQty: 20, UOM: "PCS"}},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}

	// The rejected cross-tenant Update must have left tenant B's order
	// completely untouched.
	reloaded, err := svc.Get(context.Background(), b.tenantID, order.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Update: %v", err)
	}
	if reloaded.Items[0].OrderedQty != 10 {
		t.Errorf("OrderedQty after rejected cross-tenant Update = %v, want unchanged 10", reloaded.Items[0].OrderedQty)
	}
}

func TestService_Update_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "upd-ok")

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)

	updated, err := svc.Update(context.Background(), a.tenantID, order.ID, UpdateRequest{
		WarehouseID: a.warehouseID,
		Items:       []CreateItemRequest{{ProductID: a.productID, OrderedQty: 25, UOM: "PCS"}},
	})
	if err != nil {
		t.Fatalf("Update(own tenant): %v", err)
	}
	if len(updated.Items) != 1 || updated.Items[0].OrderedQty != 25 {
		t.Fatalf("Update response = %+v, want 1 item with OrderedQty 25", updated)
	}
}

// --- Cancel: IDOR ---

func TestService_Cancel_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "cancel-cross-a")
	b := newOrderFixture(t, tx, "cancel-cross-b")

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	_, err := svc.Cancel(context.Background(), a.tenantID, order.ID, CancelRequest{}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}

	reloaded, err := svc.Get(context.Background(), b.tenantID, order.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Cancel: %v", err)
	}
	if reloaded.Status != StatusPending {
		t.Errorf("order status after rejected cross-tenant Cancel = %v, want still PENDING", reloaded.Status)
	}
}

func TestService_Cancel_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "cancel-ok")

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)

	resp, err := svc.Cancel(context.Background(), a.tenantID, order.ID, CancelRequest{}, 1)
	if err != nil {
		t.Fatalf("Cancel(own tenant): %v", err)
	}
	if resp.Status != StatusCancelled {
		t.Errorf("order status = %v, want CANCELLED", resp.Status)
	}
}

// --- ClaimForPickingTask: the atomic guarded UPDATE ---

func TestRepository_ClaimForPickingTask_WrongTenant_NoRowsAffected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "claim-a")
	b := newOrderFixture(t, tx, "claim-b")

	svc, repo := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)

	rows, err := repo.ClaimForPickingTask(context.Background(), b.tenantID, order.ID)
	if err != nil {
		t.Fatalf("ClaimForPickingTask: %v", err)
	}
	if rows != 0 {
		t.Fatalf("ClaimForPickingTask(tenantB, tenantA's order) affected %d rows, want 0", rows)
	}

	reloaded, err := repo.FindByID(context.Background(), a.tenantID, order.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if reloaded.Status != StatusPending {
		t.Errorf("tenant A's order status = %v after a rejected cross-tenant claim, want unchanged PENDING", reloaded.Status)
	}
}

// --- CreatePickingTask: the full Order->Picking orchestration ---

func TestService_CreatePickingTask_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "cpt-cross-a")
	b := newOrderFixture(t, tx, "cpt-cross-b")
	seedOrderBalance(t, tx, b, "LOT-B", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, b.tenantID, b, 10)

	_, err := svc.CreatePickingTask(context.Background(), a.tenantID, order.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: order.Items[0].ID, LocationID: b.locationID, LotNo: "LOT-B"}},
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreatePickingTask(tenantA, tenantB's order) = %v, want ErrNotFound", err)
	}

	reloaded, err := svc.Get(context.Background(), b.tenantID, order.ID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant CreatePickingTask: %v", err)
	}
	if reloaded.Status != StatusPending {
		t.Errorf("order status after rejected cross-tenant CreatePickingTask = %v, want still PENDING", reloaded.Status)
	}
}

func TestService_CreatePickingTask_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "cpt-ok")
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)

	resp, err := svc.CreatePickingTask(context.Background(), a.tenantID, order.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: order.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1)
	if err != nil {
		t.Fatalf("CreatePickingTask(own tenant): %v", err)
	}
	if resp.Status != StatusPicking || resp.PickingTaskID == nil {
		t.Fatalf("CreatePickingTask response = %+v, want PICKING with a linked picking_task_id", resp)
	}
	if resp.FulfillmentStatus != FulfillmentPicking {
		t.Errorf("FulfillmentStatus = %v, want PICKING", resp.FulfillmentStatus)
	}

	// The reservation Picking placed must be scoped to tenant A's own
	// balance — proves CreatePickingTask threaded tenantID all the way
	// into picking.Service.CreateTx's own Inventory call, not
	// inventory.UnverifiedCallerTenantID.
	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after CreatePickingTask: %v", err)
	}
	if bal.ReservedQuantity != 10 {
		t.Errorf("ReservedQuantity after CreatePickingTask = %v, want 10", bal.ReservedQuantity)
	}

	// A second call must be rejected cleanly (already PICKING), not
	// allowed to create a second picking task.
	_, err = svc.CreatePickingTask(context.Background(), a.tenantID, order.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: order.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1)
	if !errors.Is(err, ErrNotReleasable) {
		t.Errorf("second CreatePickingTask call = %v, want ErrNotReleasable", err)
	}
}

// TestService_CreatePickingTask_RollsBackOnPickingFailure proves that
// when picking.Service.CreateTx itself fails (here: no stock to
// reserve), the whole transaction — including Repository.
// ClaimForPickingTask's own claim — rolls back together, leaving the
// order exactly PENDING and unlinked again, never stuck half-released.
func TestService_CreatePickingTask_RollsBackOnPickingFailure(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "cpt-rollback")
	// Deliberately no inventory balance seeded — ReserveStockTx must
	// fail with ErrNoStock inside picking.Service.CreateTx.

	svc, _ := newOrderServiceAndRepo(tx)
	order := createOrder(t, svc, a.tenantID, a, 10)

	_, err := svc.CreatePickingTask(context.Background(), a.tenantID, order.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: order.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1)
	if !errors.Is(err, picking.ErrNoSourceStock) {
		t.Fatalf("CreatePickingTask(no source stock) = %v, want picking.ErrNoSourceStock", err)
	}

	reloaded, err := svc.Get(context.Background(), a.tenantID, order.ID)
	if err != nil {
		t.Fatalf("Get after rolled-back CreatePickingTask: %v", err)
	}
	if reloaded.Status != StatusPending || reloaded.PickingTaskID != nil {
		t.Fatalf("order after rolled-back CreatePickingTask = %+v, want still PENDING with no linked picking task", reloaded)
	}

	// The order must still be claimable after the rollback — proves the
	// failed attempt left no residue blocking a legitimate retry.
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)
	resp, err := svc.CreatePickingTask(context.Background(), a.tenantID, order.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: order.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1)
	if err != nil {
		t.Fatalf("retry CreatePickingTask after rollback: %v", err)
	}
	if resp.Status != StatusPicking {
		t.Errorf("retry CreatePickingTask status = %v, want PICKING", resp.Status)
	}
}

// TestService_CreatePickingTask_ConcurrentClaims_OnlyOneWins proves
// ClaimForPickingTask's atomic guarded UPDATE under real concurrency —
// two goroutines racing CreatePickingTask against the exact same order
// must never both succeed. Deliberately does NOT use
// testsupport.BeginRollbackTx — a single *sql.Tx is not safe for
// concurrent use by multiple goroutines — so this uses the real pooled
// *gorm.DB with explicit, FK-ordered cleanup instead of rollback, the
// same pattern internal/picking's own
// TestRepository_AdjustPickedQty_Concurrent_NoLostUpdates (Batch 2)
// established.
func TestService_CreatePickingTask_ConcurrentClaims_OnlyOneWins(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('order-concurrent', 'Order Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		// orders.picking_task_id is a real FK into picking_tasks, so
		// orders (and order_items) must be deleted BEFORE picking_tasks —
		// deleting picking_tasks first silently fails the FK check (Exec
		// errors are intentionally ignored here, matching
		// internal/picking's own Batch 2 precedent, but a wrong order
		// still orphans rows up the chain, including the tenant row
		// itself, which then collides with a later run reusing this same
		// hard-coded slug).
		db.Exec("DELETE FROM order_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM orders WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_task_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM picking_tasks WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM inventory WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM locations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM zones WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM warehouses WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM product_categories WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM tenants WHERE id = ?", tenantID)
	})

	if err := db.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-OCONC', 'Category OConc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-OCONC", Name: "Concurrent Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-OCONC', 'Warehouse OConc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-OCONC', 'Zone OConc', 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-OCONC", Name: "Location OConc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	f := orderFixture{tenantID: tenantID, productID: productID, warehouseID: warehouseID, locationID: locationID}
	inv := inventory.Inventory{TenantID: tenantID, ProductID: productID, LocationID: locationID, LotNo: "LOT-OCONC", Quantity: 100, ReservedQuantity: 0}
	if err := db.Create(&inv).Error; err != nil {
		t.Fatalf("seed balance: %v", err)
	}

	svc, _ := newOrderServiceAndRepo(db)
	order := createOrder(t, svc, tenantID, f, 10)

	const attempts = 10
	results := make([]error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.CreatePickingTask(context.Background(), tenantID, order.ID, CreatePickingTaskRequest{
				Items: []LocationAssignment{{OrderItemID: order.Items[0].ID, LocationID: locationID, LotNo: "LOT-OCONC"}},
			}, 1)
			results[idx] = err
		}(i)
	}
	wg.Wait()

	successes, notReleasable := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrNotReleasable):
			notReleasable++
		default:
			t.Errorf("concurrent CreatePickingTask returned unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Errorf("successes = %d across %d concurrent CreatePickingTask calls, want exactly 1", successes, attempts)
	}
	if notReleasable != attempts-1 {
		t.Errorf("ErrNotReleasable count = %d, want %d", notReleasable, attempts-1)
	}
}

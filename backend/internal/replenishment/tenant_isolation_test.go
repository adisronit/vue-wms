// Batch 1c — Replenishment tenant isolation. Mirrors internal/
// stockadjustment's own tenant_isolation_test.go shape exactly (fresh
// tenant per test, real seeded rows via the real product/location/
// warehouse repositories, testsupport.ConnectDB + BeginRollbackTx) but
// covers the surface unique to this package: IDOR on header lookup by
// ID, cross-tenant Product/Warehouse/FromLocation/ToLocation ownership
// verification at Create, the full Create->Start->Confirm/Cancel/
// ReportIssue lifecycle under real tenant scoping, GET
// /replenishment/recommendations' own tenant scoping (a normal
// authenticated endpoint, not a background job), and — the headline
// proof for this batch — that Confirm's MoveStockTx mutation lands
// under the caller's own real tenant's inventory balance on BOTH legs
// (source decrement, destination increment), never some other tenant's,
// regardless of which tenant happens to own the task being confirmed.
package replenishment

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/warehouse"
)

// replFixture is one fully-seeded tenant: a product, a warehouse, a
// zone, and two locations within it (from/to) — real rows via the real
// product/location/warehouse repositories, not stubs, so ownership
// verification exercises the actual FindByIDForTenant/resolveLocation
// queries this batch added.
type replFixture struct {
	tenantID       int64
	productID      int64
	warehouseID    int64
	fromLocationID int64
	toLocationID   int64
}

func newReplFixture(t *testing.T, tx *gorm.DB, label string) replFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"repl-iso-"+label, "Replenishment Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newReplFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newReplFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newReplFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newReplFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newReplFixture(%s): seed zone: %v", label, err)
	}

	from := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "FROM-" + label, Name: "From Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&from).Error; err != nil {
		t.Fatalf("newReplFixture(%s): seed from location: %v", label, err)
	}

	to := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "TO-" + label, Name: "To Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&to).Error; err != nil {
		t.Fatalf("newReplFixture(%s): seed to location: %v", label, err)
	}

	return replFixture{
		tenantID: tenantID, productID: p.ID, warehouseID: warehouseID,
		fromLocationID: from.ID, toLocationID: to.ID,
	}
}

func seedReplBalance(t *testing.T, tx *gorm.DB, tenantID, productID, locationID int64, lotNo string, quantity float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: tenantID, ProductID: productID, LocationID: locationID,
		LotNo: lotNo, Quantity: quantity,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedReplBalance: %v", err)
	}
}

func newReplServiceAndRepo(tx *gorm.DB) (*Service, *Repository) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)
	svc := NewService(repo, productRepo, warehouseRepo, invSvc)
	return svc, repo
}

// createReplTask runs Service.Create end-to-end (through the real
// transaction path, including the real ReserveStockTx call) and returns
// the new task's ID. The caller must have already seeded a balance at
// f.fromLocationID for lotNo with at least qty available.
func createReplTask(t *testing.T, svc *Service, tenantID int64, f replFixture, lotNo string, qty float64) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		ProductID: f.productID, FromLocationID: f.fromLocationID, ToLocationID: f.toLocationID,
		LotNo: lotNo, Quantity: qty, WarehouseID: f.warehouseID,
	}, 1)
	if err != nil {
		t.Fatalf("createReplTask: Create: %v", err)
	}
	return resp.ID
}

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindTaskByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "find-a")
	b := newReplFixture(t, tx, "find-b")
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, repo := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := repo.FindTaskByID(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindTaskByID(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindTaskByID(context.Background(), b.tenantID, taskID); err != nil {
		t.Errorf("FindTaskByID(tenantB, its own task) = %v, want success", err)
	}
}

func TestRepository_FindTaskDetail_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "detail-a")
	b := newReplFixture(t, tx, "detail-b")
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, repo := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := repo.FindTaskDetail(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindTaskDetail(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "get-a")
	b := newReplFixture(t, tx, "get-b")
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := svc.Get(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(context.Background(), b.tenantID, taskID); err != nil {
		t.Errorf("Get(tenantB, its own task) = %v, want success", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "list-a")
	b := newReplFixture(t, tx, "list-b")
	seedReplBalance(t, tx, a.tenantID, a.productID, a.fromLocationID, "LOT-A", 50)
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, repo := newReplServiceAndRepo(tx)
	createReplTask(t, svc, a.tenantID, a, "LOT-A", 5)
	createReplTask(t, svc, b.tenantID, b, "LOT-B", 5)

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's task)", len(rows), total)
	}
}

// --- Service-layer ownership verification at Create ---

func TestService_Create_RejectsCrossTenantProduct(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "cp-a")
	b := newReplFixture(t, tx, "cp-b")

	svc, _ := newReplServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: b.productID, FromLocationID: a.fromLocationID, ToLocationID: a.toLocationID,
		LotNo: "LOT-X", Quantity: 10, WarehouseID: a.warehouseID,
	}, 1)
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("Create(tenantA, tenantB's product_id) = %v, want ErrProductNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantWarehouse(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "cw-a")
	b := newReplFixture(t, tx, "cw-b")

	svc, _ := newReplServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: a.productID, FromLocationID: a.fromLocationID, ToLocationID: a.toLocationID,
		LotNo: "LOT-X", Quantity: 10, WarehouseID: b.warehouseID,
	}, 1)
	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("Create(tenantA, tenantB's warehouse_id) = %v, want ErrWarehouseNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantFromLocation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "cfl-a")
	b := newReplFixture(t, tx, "cfl-b")

	svc, _ := newReplServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: a.productID, FromLocationID: b.fromLocationID, ToLocationID: a.toLocationID,
		LotNo: "LOT-X", Quantity: 10, WarehouseID: a.warehouseID,
	}, 1)
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("Create(tenantA, tenantB's from_location_id) = %v, want ErrLocationNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantToLocation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "ctl-a")
	b := newReplFixture(t, tx, "ctl-b")

	svc, _ := newReplServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: a.productID, FromLocationID: a.fromLocationID, ToLocationID: b.toLocationID,
		LotNo: "LOT-X", Quantity: 10, WarehouseID: a.warehouseID,
	}, 1)
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("Create(tenantA, tenantB's to_location_id) = %v, want ErrLocationNotFound", err)
	}
}

func TestService_Create_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "create-ok")
	seedReplBalance(t, tx, a.tenantID, a.productID, a.fromLocationID, "LOT-A", 50)

	svc, _ := newReplServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: a.productID, FromLocationID: a.fromLocationID, ToLocationID: a.toLocationID,
		LotNo: "LOT-A", Quantity: 10, WarehouseID: a.warehouseID,
	}, 1)
	if err != nil {
		t.Fatalf("Create(own tenant's own product/warehouse/locations): %v", err)
	}
	if resp.Status != StatusPending {
		t.Fatalf("Create response status = %v, want PENDING", resp.Status)
	}
}

// --- Start: IDOR ---

func TestService_Start_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "start-cross-a")
	b := newReplFixture(t, tx, "start-cross-b")
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := svc.Start(context.Background(), a.tenantID, taskID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Start(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}

	resp, err := svc.Get(context.Background(), b.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Start: %v", err)
	}
	if resp.Status != StatusPending {
		t.Errorf("status after rejected cross-tenant Start = %v, want still PENDING", resp.Status)
	}
}

func TestService_Start_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "start-ok")
	seedReplBalance(t, tx, a.tenantID, a.productID, a.fromLocationID, "LOT-A", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, a.tenantID, a, "LOT-A", 10)

	resp, err := svc.Start(context.Background(), a.tenantID, taskID)
	if err != nil {
		t.Fatalf("Start(own tenant): %v", err)
	}
	if resp.Status != StatusInProgress {
		t.Errorf("status after Start = %v, want IN_PROGRESS", resp.Status)
	}
}

// --- Confirm: IDOR ---

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "confirm-cross-a")
	b := newReplFixture(t, tx, "confirm-cross-b")
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, b.tenantID, b, "LOT-B", 10)

	_, err := svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cross-1",
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}

	// Confirming as the real owner must still work after the rejected
	// cross-tenant attempt — proves the cross-tenant call left no
	// residue (no idempotency row inserted under the wrong tenant that
	// could block the legitimate caller, and status still PENDING).
	resp, err := svc.Get(context.Background(), b.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Confirm: %v", err)
	}
	if resp.Status != StatusPending {
		t.Errorf("status after rejected cross-tenant Confirm = %v, want still PENDING", resp.Status)
	}

	resp2, err := svc.Confirm(context.Background(), b.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-cross-2",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(real owner, after rejected cross-tenant attempt): %v", err)
	}
	if resp2.Status != StatusCompleted {
		t.Errorf("status after real owner's Confirm = %v, want COMPLETED", resp2.Status)
	}
}

// --- Cancel: IDOR ---

func TestService_Cancel_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "cancel-cross-a")
	b := newReplFixture(t, tx, "cancel-cross-b")
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := svc.Cancel(context.Background(), a.tenantID, taskID, CancelRequest{Remark: "nope"}, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}

	// The reservation must still be intact for the real owner — a
	// cross-tenant Cancel must never release another tenant's claim.
	resp, err := svc.Get(context.Background(), b.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Cancel: %v", err)
	}
	if resp.Status != StatusPending {
		t.Errorf("status after rejected cross-tenant Cancel = %v, want still PENDING", resp.Status)
	}
}

func TestService_Cancel_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "cancel-ok")
	seedReplBalance(t, tx, a.tenantID, a.productID, a.fromLocationID, "LOT-A", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, a.tenantID, a, "LOT-A", 10)

	resp, err := svc.Cancel(context.Background(), a.tenantID, taskID, CancelRequest{Remark: "no longer needed"}, 1)
	if err != nil {
		t.Fatalf("Cancel(own tenant): %v", err)
	}
	if resp.Status != StatusCancelled {
		t.Errorf("status after Cancel = %v, want CANCELLED", resp.Status)
	}

	// The reservation must be released — the balance's own reserved
	// quantity should be back to 0, freeing the full 50 as available.
	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.fromLocationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after Cancel: %v", err)
	}
	if bal.ReservedQuantity != 0 {
		t.Errorf("ReservedQuantity after Cancel = %v, want 0", bal.ReservedQuantity)
	}
}

// --- ReportIssue: IDOR ---

func TestService_ReportIssue_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "issue-cross-a")
	b := newReplFixture(t, tx, "issue-cross-b")
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, b.tenantID, b, "LOT-B", 10)

	if _, err := svc.ReportIssue(context.Background(), a.tenantID, taskID, IssueRequest{Remark: "damaged"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ReportIssue(tenantA, tenantB's task) = %v, want ErrNotFound", err)
	}
}

func TestService_ReportIssue_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "issue-ok")
	seedReplBalance(t, tx, a.tenantID, a.productID, a.fromLocationID, "LOT-A", 50)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, a.tenantID, a, "LOT-A", 10)

	resp, err := svc.ReportIssue(context.Background(), a.tenantID, taskID, IssueRequest{Remark: "pallet damaged"})
	if err != nil {
		t.Fatalf("ReportIssue(own tenant): %v", err)
	}
	if resp.Status != StatusIssue {
		t.Errorf("status after ReportIssue = %v, want ISSUE", resp.Status)
	}
}

// --- Recommend: tenant scoping on a normal authenticated endpoint ---

func TestService_Recommend_CrossTenantWarehouse_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "rec-cross-a")
	b := newReplFixture(t, tx, "rec-cross-b")

	svc, _ := newReplServiceAndRepo(tx)
	if _, err := svc.Recommend(context.Background(), a.tenantID, b.warehouseID); !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("Recommend(tenantA, tenantB's warehouse_id) = %v, want ErrWarehouseNotFound", err)
	}
}

// TestService_Recommend_NeverLeaksAnotherTenantsRows proves
// ListForRecommend's own tenantID filter: two tenants each have a
// product that is "low" at one location with surplus at another — even
// though both warehouses could coincidentally share IDs in a buggy
// unscoped query, each tenant's own Recommend call must only ever see
// its own rows.
func TestService_Recommend_NeverLeaksAnotherTenantsRows(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "rec-leak-a")
	b := newReplFixture(t, tx, "rec-leak-b")

	// Tenant A: low at "to" (5 <= min_stock, default min/max on
	// product.Product are zero, so bump them directly).
	if err := tx.Model(&product.Product{}).Where("id = ?", a.productID).Updates(map[string]any{"min_stock": 10, "max_stock": 50}).Error; err != nil {
		t.Fatalf("set tenant A min/max stock: %v", err)
	}
	seedReplBalance(t, tx, a.tenantID, a.productID, a.fromLocationID, "LOT-A", 100)
	seedReplBalance(t, tx, a.tenantID, a.productID, a.toLocationID, "LOT-A", 5)

	// Tenant B: same shape, different tenant — must never appear in A's
	// own recommendation list.
	if err := tx.Model(&product.Product{}).Where("id = ?", b.productID).Updates(map[string]any{"min_stock": 10, "max_stock": 50}).Error; err != nil {
		t.Fatalf("set tenant B min/max stock: %v", err)
	}
	seedReplBalance(t, tx, b.tenantID, b.productID, b.fromLocationID, "LOT-B", 100)
	seedReplBalance(t, tx, b.tenantID, b.productID, b.toLocationID, "LOT-B", 5)

	svc, _ := newReplServiceAndRepo(tx)
	recs, err := svc.Recommend(context.Background(), a.tenantID, a.warehouseID)
	if err != nil {
		t.Fatalf("Recommend(tenantA): %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("Recommend(tenantA) = %d recommendations, want exactly 1 (must not include tenant B's)", len(recs))
	}
	if recs[0].ProductID != a.productID {
		t.Errorf("Recommend(tenantA) recommendation product_id = %v, want tenant A's own %v", recs[0].ProductID, a.productID)
	}
}

// --- The headline proof: Confirm's MoveStockTx mutation lands under
// the caller's own real tenant's inventory, never another tenant's, on
// BOTH legs of the move. ---

// TestService_Confirm_Move_LandsUnderOwnTenantOnly_BothLegs proves the
// core vulnerability this batch closes: tenant A creates and confirms a
// replenishment task moving stock from its own from_location to its own
// to_location. A "ghost" tenant has balances for the EXACT same lot at
// its own analogous from/to locations (same shape, different real
// tenant and IDs) — if the mutation ever landed on the wrong tenant's
// row (e.g. inventory.UnverifiedCallerTenantID's old hardcoded tenant
// 1), one of these ghost values would change. Checked on both legs:
// the source decrement and the destination increment.
func TestService_Confirm_Move_LandsUnderOwnTenantOnly_BothLegs(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "move-a")
	ghost := newReplFixture(t, tx, "move-ghost")

	seedReplBalance(t, tx, a.tenantID, a.productID, a.fromLocationID, "LOT-SHARED", 50)
	// The ghost tenant's "from" balance looks just like tenant A's own
	// source balance (same lot, same shape) and its "to" location
	// already holds some stock too — both must be completely untouched
	// by tenant A's own move.
	seedReplBalance(t, tx, ghost.tenantID, ghost.productID, ghost.fromLocationID, "LOT-SHARED", 500)
	seedReplBalance(t, tx, ghost.tenantID, ghost.productID, ghost.toLocationID, "LOT-SHARED", 200)

	svc, _ := newReplServiceAndRepo(tx)
	taskID := createReplTask(t, svc, a.tenantID, a, "LOT-SHARED", 15)

	resp, err := svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "key-move-1",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(move): %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Fatalf("status after Confirm = %v, want COMPLETED", resp.Status)
	}

	invRepo := inventory.NewRepository(tx)

	// --- Leg 1: source decrement, tenant A's own balance. ---
	balFromA, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.fromLocationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(tenant A, from) after Confirm: %v", err)
	}
	if balFromA.Quantity != 35 {
		t.Errorf("tenant A's from-location Quantity after Confirm = %v, want 35 (50-15)", balFromA.Quantity)
	}
	if balFromA.ReservedQuantity != 0 {
		t.Errorf("tenant A's from-location ReservedQuantity after Confirm = %v, want 0 (reservation released before move)", balFromA.ReservedQuantity)
	}

	// --- Leg 2: destination increment, tenant A's own balance. ---
	balToA, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.toLocationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(tenant A, to) after Confirm: %v", err)
	}
	if balToA.Quantity != 15 {
		t.Errorf("tenant A's to-location Quantity after Confirm = %v, want 15", balToA.Quantity)
	}

	// --- Ghost tenant's analogous balances, both legs, untouched. ---
	balFromGhost, err := invRepo.FindBalance(context.Background(), ghost.tenantID, ghost.productID, ghost.fromLocationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(ghost tenant, from) after tenant A's Confirm: %v", err)
	}
	if balFromGhost.Quantity != 500 {
		t.Errorf("ghost tenant's from-location Quantity after tenant A's Confirm = %v, want unchanged 500 (source leg must never cross tenants)", balFromGhost.Quantity)
	}

	balToGhost, err := invRepo.FindBalance(context.Background(), ghost.tenantID, ghost.productID, ghost.toLocationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(ghost tenant, to) after tenant A's Confirm: %v", err)
	}
	if balToGhost.Quantity != 200 {
		t.Errorf("ghost tenant's to-location Quantity after tenant A's Confirm = %v, want unchanged 200 (destination leg must never cross tenants)", balToGhost.Quantity)
	}
}

// TestService_Create_CannotReserveFromAnotherTenantsBalance proves the
// same tenant_id-in-WHERE defense in depth at Create time: tenant A has
// no balance of its own for this lot at its own from_location, even
// though a "ghost" tenant has a large balance for the identical-looking
// lot at its own from_location — ReserveStockTx's guarded UPDATE can
// never be satisfied from the ghost tenant's row, so Create must fail
// with ErrNoSourceStock rather than silently reserving against another
// tenant's stock.
func TestService_Create_CannotReserveFromAnotherTenantsBalance(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReplFixture(t, tx, "nostock-a")
	ghost := newReplFixture(t, tx, "nostock-ghost")
	seedReplBalance(t, tx, ghost.tenantID, ghost.productID, ghost.fromLocationID, "LOT-SHARED", 1000)

	svc, _ := newReplServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: a.productID, FromLocationID: a.fromLocationID, ToLocationID: a.toLocationID,
		LotNo: "LOT-SHARED", Quantity: 10, WarehouseID: a.warehouseID,
	}, 1)
	if !errors.Is(err, ErrNoSourceStock) {
		t.Fatalf("Create(tenantA, no balance of its own, ghost tenant has one) = %v, want ErrNoSourceStock", err)
	}

	invRepo := inventory.NewRepository(tx)
	balGhost, err := invRepo.FindBalance(context.Background(), ghost.tenantID, ghost.productID, ghost.fromLocationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(ghost tenant): %v", err)
	}
	if balGhost.Quantity != 1000 || balGhost.ReservedQuantity != 0 {
		t.Errorf("ghost tenant's balance after tenant A's failed Create = quantity=%v reserved=%v, want unchanged 1000/0", balGhost.Quantity, balGhost.ReservedQuantity)
	}
}

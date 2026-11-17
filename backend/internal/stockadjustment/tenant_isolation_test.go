// Batch 1a — Stock Adjustment tenant isolation. Mirrors internal/
// picking/internal/order's own tenant_isolation_test.go shape (fresh
// tenant per test, real seeded rows via the real product/location/
// inventory repositories, testsupport.ConnectDB + BeginRollbackTx) but
// covers the surface unique to this package: IDOR on header/item lookup
// by ID, cross-tenant Product/Location ownership verification at
// Create/Update, the full Create->Confirm (APPROVE/REJECT) lifecycle
// under real tenant scoping, and — the headline proof for this batch —
// that Confirm's APPROVE stock mutation lands under the caller's own
// real tenant's inventory balance, never tenant 1's, regardless of
// which tenant happens to own the row being confirmed.
package stockadjustment

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
)

// adjustmentFixture is one fully-seeded tenant: a product, a warehouse,
// and a location within it — real rows via the real product/location/
// warehouse repositories, not stubs, so ownership verification exercises
// the actual FindByIDForTenant/FindByID queries this batch added.
type adjustmentFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

func newAdjustmentFixture(t *testing.T, tx *gorm.DB, label string) adjustmentFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"adj-iso-"+label, "Stock Adjustment Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newAdjustmentFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newAdjustmentFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newAdjustmentFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newAdjustmentFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newAdjustmentFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newAdjustmentFixture(%s): seed location: %v", label, err)
	}

	return adjustmentFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

func seedAdjustmentBalance(t *testing.T, tx *gorm.DB, f adjustmentFixture, lotNo string, quantity float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedAdjustmentBalance: %v", err)
	}
}

func newAdjustmentServiceAndRepo(tx *gorm.DB) (*Service, *Repository) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)
	svc := NewService(repo, productRepo, locationRepo, invSvc, 48)
	return svc, repo
}

// createAdjustment runs Service.Create end-to-end (through the real
// transaction path) and returns the new adjustment's ID.
func createAdjustment(t *testing.T, svc *Service, tenantID int64, f adjustmentFixture, lotNo string, adjustType AdjustType, qty float64, status Status) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo,
		AdjustType: adjustType, AdjustQty: qty, Reason: ReasonFoundGoods, Status: status,
	}, 1)
	if err != nil {
		t.Fatalf("createAdjustment: Create: %v", err)
	}
	return resp.ID
}

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "find-a")
	b := newAdjustmentFixture(t, tx, "find-b")

	svc, repo := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, b.tenantID, b, "LOT-B", AdjustIncrease, 10, StatusDraft)

	if _, err := repo.FindByID(context.Background(), a.tenantID, adjID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByID(tenantA, tenantB's adjustment) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByID(context.Background(), b.tenantID, adjID); err != nil {
		t.Errorf("FindByID(tenantB, its own adjustment) = %v, want success", err)
	}
}

func TestRepository_FindDetail_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "detail-a")
	b := newAdjustmentFixture(t, tx, "detail-b")

	svc, repo := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, b.tenantID, b, "LOT-B", AdjustIncrease, 10, StatusDraft)

	if _, err := repo.FindDetail(context.Background(), a.tenantID, adjID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindDetail(tenantA, tenantB's adjustment) = %v, want ErrNotFound", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "get-a")
	b := newAdjustmentFixture(t, tx, "get-b")

	svc, _ := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, b.tenantID, b, "LOT-B", AdjustIncrease, 10, StatusDraft)

	if _, err := svc.Get(context.Background(), a.tenantID, adjID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's adjustment) = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(context.Background(), b.tenantID, adjID); err != nil {
		t.Errorf("Get(tenantB, its own adjustment) = %v, want success", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "list-a")
	b := newAdjustmentFixture(t, tx, "list-b")

	svc, repo := newAdjustmentServiceAndRepo(tx)
	createAdjustment(t, svc, a.tenantID, a, "LOT-A", AdjustIncrease, 5, StatusDraft)
	createAdjustment(t, svc, b.tenantID, b, "LOT-B", AdjustIncrease, 5, StatusDraft)

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20}, 48)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's adjustment)", len(rows), total)
	}
}

// --- Service-layer ownership verification at Create ---

func TestService_Create_RejectsCrossTenantProduct(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "cp-a")
	b := newAdjustmentFixture(t, tx, "cp-b")

	svc, _ := newAdjustmentServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: b.productID, LocationID: a.locationID, LotNo: "LOT-X",
		AdjustType: AdjustIncrease, AdjustQty: 10, Reason: ReasonFoundGoods,
	}, 1)
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("Create(tenantA, tenantB's product_id) = %v, want ErrProductNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantLocation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "cl-a")
	b := newAdjustmentFixture(t, tx, "cl-b")

	svc, _ := newAdjustmentServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: a.productID, LocationID: b.locationID, LotNo: "LOT-X",
		AdjustType: AdjustIncrease, AdjustQty: 10, Reason: ReasonFoundGoods,
	}, 1)
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("Create(tenantA, tenantB's location_id) = %v, want ErrLocationNotFound", err)
	}
}

func TestService_Create_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "create-ok")

	svc, _ := newAdjustmentServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ProductID: a.productID, LocationID: a.locationID, LotNo: "LOT-A",
		AdjustType: AdjustIncrease, AdjustQty: 10, Reason: ReasonFoundGoods,
	}, 1)
	if err != nil {
		t.Fatalf("Create(own tenant's own product/location): %v", err)
	}
	if resp.Status != StatusDraft {
		t.Fatalf("Create response status = %v, want DRAFT", resp.Status)
	}
}

// --- Update: IDOR ---

func TestService_Update_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "upd-cross-a")
	b := newAdjustmentFixture(t, tx, "upd-cross-b")

	svc, _ := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, b.tenantID, b, "LOT-B", AdjustIncrease, 10, StatusDraft)

	_, err := svc.Update(context.Background(), a.tenantID, adjID, UpdateRequest{
		AdjustType: AdjustIncrease, AdjustQty: 20, Reason: ReasonFoundGoods,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update(tenantA, tenantB's adjustment) = %v, want ErrNotFound", err)
	}

	// The real owner's row must be left untouched by the rejected
	// cross-tenant attempt.
	resp, err := svc.Get(context.Background(), b.tenantID, adjID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Update: %v", err)
	}
	if resp.AdjustQty != 10 {
		t.Errorf("AdjustQty after rejected cross-tenant Update = %v, want still 10", resp.AdjustQty)
	}
}

func TestService_Update_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "upd-ok")

	svc, _ := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, a.tenantID, a, "LOT-A", AdjustIncrease, 10, StatusDraft)

	resp, err := svc.Update(context.Background(), a.tenantID, adjID, UpdateRequest{
		AdjustType: AdjustIncrease, AdjustQty: 25, Reason: ReasonDamagedGoods, Status: StatusPending,
	})
	if err != nil {
		t.Fatalf("Update(own tenant): %v", err)
	}
	if resp.AdjustQty != 25 || resp.Status != StatusPending {
		t.Errorf("Update response = qty=%v status=%v, want qty=25 status=PENDING", resp.AdjustQty, resp.Status)
	}
}

// --- Confirm: IDOR ---

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "confirm-cross-a")
	b := newAdjustmentFixture(t, tx, "confirm-cross-b")

	svc, _ := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, b.tenantID, b, "LOT-B", AdjustIncrease, 10, StatusPending)

	_, err := svc.Confirm(context.Background(), a.tenantID, adjID, ConfirmRequest{
		IdempotencyKey: "key-cross-1", Action: "APPROVE",
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's adjustment) = %v, want ErrNotFound", err)
	}

	// Confirming as the real owner must still work after the rejected
	// cross-tenant attempt — proves the cross-tenant call left no
	// residue (no idempotency row inserted under the wrong tenant that
	// could block the legitimate caller, and status still PENDING).
	resp, err := svc.Get(context.Background(), b.tenantID, adjID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Confirm: %v", err)
	}
	if resp.Status != StatusPending {
		t.Errorf("status after rejected cross-tenant Confirm = %v, want still PENDING", resp.Status)
	}

	resp2, err := svc.Confirm(context.Background(), b.tenantID, adjID, ConfirmRequest{
		IdempotencyKey: "key-cross-2", Action: "APPROVE",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(real owner, after rejected cross-tenant attempt): %v", err)
	}
	if resp2.Status != StatusApproved {
		t.Errorf("status after real owner's Confirm = %v, want APPROVED", resp2.Status)
	}
}

// --- Confirm: REJECT action, no inventory mutation ---

func TestService_Confirm_Reject_OwnTenant_NoInventoryMutation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "reject-ok")
	seedAdjustmentBalance(t, tx, a, "LOT-A", 100)

	svc, _ := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, a.tenantID, a, "LOT-A", AdjustDecrease, 10, StatusPending)

	resp, err := svc.Confirm(context.Background(), a.tenantID, adjID, ConfirmRequest{
		IdempotencyKey: "key-reject-1", Action: "REJECT", Remark: "not approved",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(REJECT): %v", err)
	}
	if resp.Status != StatusRejected {
		t.Errorf("status after REJECT = %v, want REJECTED", resp.Status)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after REJECT: %v", err)
	}
	if bal.Quantity != 100 {
		t.Errorf("Quantity after REJECT = %v, want unchanged 100 (REJECT must never mutate inventory)", bal.Quantity)
	}
}

// --- The headline proof: Confirm's APPROVE mutation lands under the
// caller's own real tenant's inventory, never tenant 1's. ---

// TestService_Confirm_Approve_Increase_LandsUnderOwnTenantOnly proves
// the core vulnerability this batch closes: tenant A creates and
// confirms an INCREASE adjustment against its own product/location: the
// resulting AddStockTx call must land under tenant A's own inventory
// balance. A same-looking balance belonging to a DIFFERENT real tenant
// (not the legacy tenant_id=1 default — a second real tenant, "ghost
// tenant 1") must be completely unaffected, proving the mutation can
// never mis-tag onto some other tenant's row the way UnverifiedCallerTenantID
// used to force it to (always tenant_id=1) regardless of who called.
func TestService_Confirm_Approve_Increase_LandsUnderOwnTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "approve-inc-a")
	ghost := newAdjustmentFixture(t, tx, "approve-inc-ghost")
	// The "ghost" tenant has a balance for the EXACT same product/
	// location/lot combination tenant A is about to adjust — if the
	// mutation ever landed on the wrong tenant's row (e.g. a hardcoded
	// tenant), this value would change.
	seedAdjustmentBalance(t, tx, ghost, "LOT-SHARED", 500)

	svc, _ := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, a.tenantID, a, "LOT-SHARED", AdjustIncrease, 15, StatusPending)

	resp, err := svc.Confirm(context.Background(), a.tenantID, adjID, ConfirmRequest{
		IdempotencyKey: "key-approve-inc-1", Action: "APPROVE",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(APPROVE, INCREASE): %v", err)
	}
	if resp.Status != StatusApproved {
		t.Errorf("status after APPROVE = %v, want APPROVED", resp.Status)
	}
	if resp.AfterQty == nil || *resp.AfterQty != 15 {
		t.Errorf("AfterQty after APPROVE = %v, want 15 (0 + 15, tenant A had no prior balance of its own)", resp.AfterQty)
	}

	invRepo := inventory.NewRepository(tx)

	// Tenant A's own balance must now be 15.
	balA, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(tenant A) after APPROVE: %v", err)
	}
	if balA.Quantity != 15 {
		t.Errorf("tenant A's Quantity after APPROVE = %v, want 15", balA.Quantity)
	}

	// The ghost tenant's balance must be completely untouched.
	balGhost, err := invRepo.FindBalance(context.Background(), ghost.tenantID, ghost.productID, ghost.locationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(ghost tenant) after APPROVE: %v", err)
	}
	if balGhost.Quantity != 500 {
		t.Errorf("ghost tenant's Quantity after tenant A's APPROVE = %v, want unchanged 500 (mutation must never cross tenants)", balGhost.Quantity)
	}
}

// TestService_Confirm_Approve_Decrease_OwnTenant_Succeeds proves the
// DECREASE/RemoveStockTx branch is equally tenant-scoped, and that the
// before/after snapshot and resulting balance are all correct for the
// caller's own tenant.
func TestService_Confirm_Approve_Decrease_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "approve-dec-a")
	seedAdjustmentBalance(t, tx, a, "LOT-A", 100)

	svc, _ := newAdjustmentServiceAndRepo(tx)
	adjID := createAdjustment(t, svc, a.tenantID, a, "LOT-A", AdjustDecrease, 30, StatusPending)

	resp, err := svc.Confirm(context.Background(), a.tenantID, adjID, ConfirmRequest{
		IdempotencyKey: "key-approve-dec-1", Action: "APPROVE",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(APPROVE, DECREASE): %v", err)
	}
	if resp.BeforeQty == nil || *resp.BeforeQty != 100 {
		t.Errorf("BeforeQty = %v, want 100", resp.BeforeQty)
	}
	if resp.AfterQty == nil || *resp.AfterQty != 70 {
		t.Errorf("AfterQty = %v, want 70 (100-30)", resp.AfterQty)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after APPROVE: %v", err)
	}
	if bal.Quantity != 70 {
		t.Errorf("Quantity after APPROVE = %v, want 70", bal.Quantity)
	}
}

// TestService_Confirm_Approve_CrossTenant_InsufficientStock_DoesNotTouchOtherTenant
// proves that even if a cross-tenant Confirm somehow got past the
// ErrNotFound guard (it cannot — defense in depth), a DECREASE against
// a tenant with no balance of its own is rejected as insufficient stock
// rather than silently satisfied from another tenant's balance.
func TestService_Confirm_Approve_CrossTenant_InsufficientStock_DoesNotTouchOtherTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newAdjustmentFixture(t, tx, "nostock-a")
	ghost := newAdjustmentFixture(t, tx, "nostock-ghost")
	seedAdjustmentBalance(t, tx, ghost, "LOT-SHARED", 1000)

	svc, _ := newAdjustmentServiceAndRepo(tx)
	// Tenant A has no balance of its own for LOT-SHARED.
	adjID := createAdjustment(t, svc, a.tenantID, a, "LOT-SHARED", AdjustDecrease, 10, StatusPending)

	_, err := svc.Confirm(context.Background(), a.tenantID, adjID, ConfirmRequest{
		IdempotencyKey: "key-nostock-1", Action: "APPROVE",
	}, 1)
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("Confirm(DECREASE, no stock of its own) = %v, want ErrInsufficientStock (must not reach the ghost tenant's balance)", err)
	}

	invRepo := inventory.NewRepository(tx)
	balGhost, err := invRepo.FindBalance(context.Background(), ghost.tenantID, ghost.productID, ghost.locationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(ghost tenant): %v", err)
	}
	if balGhost.Quantity != 1000 {
		t.Errorf("ghost tenant's Quantity after tenant A's failed APPROVE = %v, want unchanged 1000", balGhost.Quantity)
	}
}

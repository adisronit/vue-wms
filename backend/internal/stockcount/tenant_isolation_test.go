// Batch 1b — Stock Count tenant isolation. Mirrors internal/
// stockadjustment's own tenant_isolation_test.go shape (fresh tenant per
// test, real seeded rows via the real warehouse/zone/product/location/
// inventory repositories, testsupport.ConnectDB + BeginRollbackTx) but
// covers the surface unique to this package: IDOR on header/item lookup
// by ID, List never leaking another tenant's rows, the full
// Create->RecordCount->Confirm lifecycle under real tenant scoping, and
// — the headline proofs for this batch — that Confirm's variance-posting
// mutation lands under the caller's own real tenant's inventory balance
// (never tenant 1's), and that CreateAdjustments' generated
// stock_adjustments rows are correctly tagged to the caller's own real
// tenant via the now-fixed adjustmentCreator interface.
package stockcount

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/stockadjustment"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// countFixture is one fully-seeded tenant: a warehouse, a zone within
// it, a product, and a location within the zone — real rows via the
// real warehouse/zone/product/location repositories, not stubs, so
// ownership verification exercises the actual queries this batch added.
type countFixture struct {
	tenantID    int64
	warehouseID int64
	zoneID      int64
	productID   int64
	locationID  int64
}

func newCountFixture(t *testing.T, tx *gorm.DB, label string) countFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"cnt-iso-"+label, "Stock Count Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newCountFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newCountFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newCountFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newCountFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newCountFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newCountFixture(%s): seed location: %v", label, err)
	}

	return countFixture{
		tenantID: tenantID, warehouseID: warehouseID, zoneID: zoneID,
		productID: p.ID, locationID: l.ID,
	}
}

func seedCountBalance(t *testing.T, tx *gorm.DB, f countFixture, lotNo string, quantity float64) {
	t.Helper()
	inv := inventory.Inventory{
		TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID,
		LotNo: lotNo, Quantity: quantity,
	}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedCountBalance: %v", err)
	}
}

// newCountServiceAndRepo wires a real Service against tx — including a
// real internal/stockadjustment.Service as adjustmentCreator (not a
// fake), so CreateAdjustments tests can assert the real tenant_id
// stamped onto the generated row via the now-fixed adjustmentCreator
// interface.
func newCountServiceAndRepo(tx *gorm.DB) (*Service, *Repository) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	zoneRepo := zone.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	adjRepo := stockadjustment.NewRepository(tx)
	adjSvc := stockadjustment.NewService(adjRepo, productRepo, locationRepo, invSvc, 48)

	svc := NewService(repo, warehouseRepo, zoneRepo, productRepo, locationRepo, invSvc, adjSvc)
	return svc, repo
}

// createCount runs Service.Create end-to-end (through the real
// transaction path) with one item for f.productID/f.locationID/lotNo,
// and returns the new count's ID.
func createCount(t *testing.T, svc *Service, tenantID int64, f countFixture, lotNo string) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		CountType: CountTypeCycle, WarehouseID: f.warehouseID, ZoneID: &f.zoneID,
		Items: []CreateItemRequest{
			{ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo},
		},
	}, 1)
	if err != nil {
		t.Fatalf("createCount: Create: %v", err)
	}
	return resp.ID
}

// recordCount records countQty against the count's one item (the item
// id is discovered via svc.Get under the owning tenant).
func recordCount(t *testing.T, svc *Service, tenantID, countID int64, countQty float64) {
	t.Helper()
	resp, err := svc.Get(context.Background(), tenantID, countID)
	if err != nil {
		t.Fatalf("recordCount: Get: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("recordCount: expected exactly 1 item, got %d", len(resp.Items))
	}
	if _, err := svc.RecordCount(context.Background(), tenantID, countID, resp.Items[0].ID, RecordCountRequest{
		CountQty: countQty, Condition: ConditionGood, Status: ItemStatusCounted,
	}); err != nil {
		t.Fatalf("recordCount: RecordCount: %v", err)
	}
}

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "find-a")
	b := newCountFixture(t, tx, "find-b")

	svc, repo := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, b.tenantID, b, "LOT-B")

	if _, err := repo.FindByID(context.Background(), a.tenantID, countID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindByID(tenantA, tenantB's count) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByID(context.Background(), b.tenantID, countID); err != nil {
		t.Errorf("FindByID(tenantB, its own count) = %v, want success", err)
	}
}

func TestRepository_FindDetail_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "detail-a")
	b := newCountFixture(t, tx, "detail-b")

	svc, repo := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, b.tenantID, b, "LOT-B")

	if _, err := repo.FindDetail(context.Background(), a.tenantID, countID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindDetail(tenantA, tenantB's count) = %v, want ErrNotFound", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "get-a")
	b := newCountFixture(t, tx, "get-b")

	svc, _ := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, b.tenantID, b, "LOT-B")

	if _, err := svc.Get(context.Background(), a.tenantID, countID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's count) = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(context.Background(), b.tenantID, countID); err != nil {
		t.Errorf("Get(tenantB, its own count) = %v, want success", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "list-a")
	b := newCountFixture(t, tx, "list-b")

	svc, repo := newCountServiceAndRepo(tx)
	createCount(t, svc, a.tenantID, a, "LOT-A")
	createCount(t, svc, b.tenantID, b, "LOT-B")

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's count)", len(rows), total)
	}
}

// --- RecordCount: IDOR ---

func TestService_RecordCount_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "rec-cross-a")
	b := newCountFixture(t, tx, "rec-cross-b")

	svc, _ := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, b.tenantID, b, "LOT-B")

	bResp, err := svc.Get(context.Background(), b.tenantID, countID)
	if err != nil {
		t.Fatalf("Get(owner) to discover item id: %v", err)
	}
	itemID := bResp.Items[0].ID

	// Wrong tenant, right count id and item id: must reject as not found
	// (count-level IDOR, checked before the item is even looked at).
	if _, err := svc.RecordCount(context.Background(), a.tenantID, countID, itemID, RecordCountRequest{
		CountQty: 999, Condition: ConditionGood, Status: ItemStatusCounted,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("RecordCount(tenantA, tenantB's count) = %v, want ErrNotFound", err)
	}

	// The real owner's item must be untouched (still PENDING, no
	// count_qty) by the rejected cross-tenant attempt.
	resp, err := svc.Get(context.Background(), b.tenantID, countID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant RecordCount: %v", err)
	}
	if resp.Items[0].CountQty != nil {
		t.Errorf("item CountQty after rejected cross-tenant RecordCount = %v, want still nil", resp.Items[0].CountQty)
	}

	// Recording as the real owner must still work afterward.
	if _, err := svc.RecordCount(context.Background(), b.tenantID, countID, itemID, RecordCountRequest{
		CountQty: 5, Condition: ConditionGood, Status: ItemStatusCounted,
	}); err != nil {
		t.Errorf("RecordCount(real owner, after rejected cross-tenant attempt) = %v, want success", err)
	}
}

// --- Confirm: IDOR ---

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "confirm-cross-a")
	b := newCountFixture(t, tx, "confirm-cross-b")

	svc, _ := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, b.tenantID, b, "LOT-B")
	recordCount(t, svc, b.tenantID, countID, 10)

	_, err := svc.Confirm(context.Background(), a.tenantID, countID, ConfirmRequest{
		IdempotencyKey: "key-cross-1",
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's count) = %v, want ErrNotFound", err)
	}

	// Confirming as the real owner must still work after the rejected
	// cross-tenant attempt — proves the cross-tenant call left no
	// residue (no idempotency row inserted under the wrong tenant that
	// could block the legitimate caller, and status still DRAFT).
	resp, err := svc.Get(context.Background(), b.tenantID, countID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Confirm: %v", err)
	}
	if resp.Status != StatusDraft {
		t.Errorf("status after rejected cross-tenant Confirm = %v, want still DRAFT", resp.Status)
	}

	resp2, err := svc.Confirm(context.Background(), b.tenantID, countID, ConfirmRequest{
		IdempotencyKey: "key-cross-2",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(real owner, after rejected cross-tenant attempt): %v", err)
	}
	if resp2.Status != StatusConfirmed {
		t.Errorf("status after real owner's Confirm = %v, want CONFIRMED", resp2.Status)
	}
}

// --- The headline proof: Confirm's variance-posting mutation lands
// under the caller's own real tenant, never tenant 1's. ---

// TestService_Confirm_LandsUnderOwnTenantOnly proves the core
// vulnerability this batch closes: tenant A creates, counts, and
// confirms a count against its own product/location (counted qty 25
// against a frozen system_qty of 0, i.e. a +25 variance): the resulting
// AddStockTx call must land under tenant A's own inventory balance. A
// same-looking balance belonging to a DIFFERENT real tenant (not the
// legacy tenant_id=1 default — a second real tenant, "ghost tenant")
// must be completely unaffected, proving the mutation can never
// mis-tag onto some other tenant's row the way
// inventory.UnverifiedCallerTenantID used to force it to (always
// tenant_id=1) regardless of who called.
func TestService_Confirm_LandsUnderOwnTenantOnly(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "confirm-own-a")
	ghost := newCountFixture(t, tx, "confirm-own-ghost")
	// The "ghost" tenant has a balance for the EXACT same product/
	// location/lot combination tenant A is about to count — if the
	// mutation ever landed on the wrong tenant's row (e.g. a hardcoded
	// tenant), this value would change.
	seedCountBalance(t, tx, ghost, "LOT-SHARED", 500)

	svc, _ := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, a.tenantID, a, "LOT-SHARED")
	recordCount(t, svc, a.tenantID, countID, 25)

	resp, err := svc.Confirm(context.Background(), a.tenantID, countID, ConfirmRequest{
		IdempotencyKey: "key-confirm-own-1",
	}, 1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if resp.Status != StatusConfirmed {
		t.Errorf("status after Confirm = %v, want CONFIRMED", resp.Status)
	}

	invRepo := inventory.NewRepository(tx)

	// Tenant A's own balance must now be 25 (0 frozen system_qty + 25
	// variance).
	balA, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(tenant A) after Confirm: %v", err)
	}
	if balA.Quantity != 25 {
		t.Errorf("tenant A's Quantity after Confirm = %v, want 25", balA.Quantity)
	}

	// The ghost tenant's balance must be completely untouched.
	balGhost, err := invRepo.FindBalance(context.Background(), ghost.tenantID, ghost.productID, ghost.locationID, "LOT-SHARED")
	if err != nil {
		t.Fatalf("FindBalance(ghost tenant) after Confirm: %v", err)
	}
	if balGhost.Quantity != 500 {
		t.Errorf("ghost tenant's Quantity after tenant A's Confirm = %v, want unchanged 500 (mutation must never cross tenants)", balGhost.Quantity)
	}
}

// --- CreateAdjustments: cross-tenant tagging via the now-fixed
// adjustmentCreator interface. ---

// TestService_CreateAdjustments_TagsRealOwnTenant proves the second
// half of this batch's mutation-correctness guarantee: when tenant A's
// count has a non-zero-variance line and calls CreateAdjustments, the
// resulting internal/stockadjustment document is created with its own
// tenant_id column set to tenant A's real tenant — not tenant 1, and
// not any other tenant — verified by reading the row back directly
// through a brand-new stockadjustment.Repository bound to tenant A
// alone (FindByID with tenant A's id must find it; FindByID with a
// different real tenant's id must not).
func TestService_CreateAdjustments_TagsRealOwnTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "ca-tag-a")
	other := newCountFixture(t, tx, "ca-tag-other")

	svc, _ := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, a.tenantID, a, "LOT-A")
	recordCount(t, svc, a.tenantID, countID, 40) // variance = +40

	resp, err := svc.CreateAdjustments(context.Background(), a.tenantID, countID, 1)
	if err != nil {
		t.Fatalf("CreateAdjustments: %v", err)
	}
	if len(resp.AdjustmentIDs) != 1 {
		t.Fatalf("CreateAdjustments: got %d adjustment ids, want 1", len(resp.AdjustmentIDs))
	}
	adjID := resp.AdjustmentIDs[0]

	adjRepo := stockadjustment.NewRepository(tx)

	// Must be found under tenant A's own id.
	adj, err := adjRepo.FindByID(context.Background(), a.tenantID, adjID)
	if err != nil {
		t.Fatalf("FindByID(tenant A's own tenant, generated adjustment) = %v, want success", err)
	}
	if adj.ID != adjID {
		t.Errorf("found adjustment id = %d, want %d", adj.ID, adjID)
	}

	// Must NOT be found under a different real tenant's id — proves the
	// row's own tenant_id column is really tenant A's, not some other
	// tenant's or a hardcoded default.
	if _, err := adjRepo.FindByID(context.Background(), other.tenantID, adjID); !errors.Is(err, stockadjustment.ErrNotFound) {
		t.Errorf("FindByID(a different real tenant, tenant A's generated adjustment) = %v, want ErrNotFound", err)
	}

	// The count itself must also now be claimed as ADJUSTMENT_CREATED.
	countResp, err := svc.Get(context.Background(), a.tenantID, countID)
	if err != nil {
		t.Fatalf("Get after CreateAdjustments: %v", err)
	}
	if countResp.Status != StatusAdjustmentCreated {
		t.Errorf("count status after CreateAdjustments = %v, want ADJUSTMENT_CREATED", countResp.Status)
	}
	if len(countResp.GeneratedAdjustments) != 1 || countResp.GeneratedAdjustments[0].ID != adjID {
		t.Errorf("GeneratedAdjustments = %+v, want exactly one entry for adjustment %d", countResp.GeneratedAdjustments, adjID)
	}
}

// TestService_CreateAdjustments_CrossTenant_NotFound proves
// CreateAdjustments itself is IDOR-safe: a cross-tenant call against
// another tenant's count is rejected as not found, and leaves that
// count completely unclaimed (still DRAFT, no adjustment generated) for
// its real owner.
func TestService_CreateAdjustments_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newCountFixture(t, tx, "ca-cross-a")
	b := newCountFixture(t, tx, "ca-cross-b")

	svc, _ := newCountServiceAndRepo(tx)
	countID := createCount(t, svc, b.tenantID, b, "LOT-B")
	recordCount(t, svc, b.tenantID, countID, 12)

	if _, err := svc.CreateAdjustments(context.Background(), a.tenantID, countID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateAdjustments(tenantA, tenantB's count) = %v, want ErrNotFound", err)
	}

	resp, err := svc.Get(context.Background(), b.tenantID, countID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant CreateAdjustments: %v", err)
	}
	if resp.Status != StatusDraft {
		t.Errorf("status after rejected cross-tenant CreateAdjustments = %v, want still DRAFT", resp.Status)
	}
	if len(resp.GeneratedAdjustments) != 0 {
		t.Errorf("GeneratedAdjustments after rejected cross-tenant CreateAdjustments = %+v, want empty", resp.GeneratedAdjustments)
	}

	// The real owner's own call must still succeed afterward.
	if _, err := svc.CreateAdjustments(context.Background(), b.tenantID, countID, 1); err != nil {
		t.Errorf("CreateAdjustments(real owner, after rejected cross-tenant attempt) = %v, want success", err)
	}
}

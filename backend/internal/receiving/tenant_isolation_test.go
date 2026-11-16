// Phase 31A — Receiving tenant isolation. Mirrors
// internal/picking/tenant_isolation_test.go's own shape (Batch 2: fresh
// tenant per test, real seeded rows via the real product/location/
// warehouse/supplier/inventory repositories, testsupport.ConnectDB +
// BeginRollbackTx) but covers the surface unique to this package: IDOR
// on receipt/item lookup, cross-tenant warehouse/location/product/
// supplier ownership at Create time, the hold-aware Confirm transaction
// (QC-required products posting into inventory.HoldQuantity instead of
// Available), multi-lot/pallet splits, idempotency, rollback atomicity,
// and the AdjustReceivedQty/AdjustHold concurrency guarantees.
package receiving

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/supplier"
	"wms-backend/internal/warehouse"
)

// receivingFixture is one fully-seeded tenant: a product, a warehouse,
// a location within it, and a supplier — real rows via the real
// repositories, not stubs, so ownership verification exercises the
// actual FindByIDForTenant/FindByID queries this batch added.
type receivingFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
	supplierID  int64
}

func newReceivingFixture(t *testing.T, tx *gorm.DB, label string, qcRequired bool) receivingFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"recv-iso-"+label, "Receiving Isolation Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newReceivingFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newReceivingFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive, QCRequired: qcRequired,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newReceivingFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newReceivingFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newReceivingFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newReceivingFixture(%s): seed location: %v", label, err)
	}

	sup := supplier.Supplier{TenantID: tenantID, Code: "SUP-" + label, CompanyName: "Supplier " + label, Status: supplier.StatusActive}
	if err := tx.Create(&sup).Error; err != nil {
		t.Fatalf("newReceivingFixture(%s): seed supplier: %v", label, err)
	}

	return receivingFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID, supplierID: sup.ID}
}

func newReceivingServiceAndRepo(tx *gorm.DB) (*Service, *Repository) {
	repo := NewRepository(tx)
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	supplierRepo := supplier.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)
	return NewService(repo, productRepo, warehouseRepo, locationRepo, supplierRepo, invSvc), repo
}

// createReceipt is a small helper that runs Service.Create end-to-end
// (through the real transaction path) with a single DRAFT line for
// f.productID, and returns the new receipt's ID.
func createReceipt(t *testing.T, svc *Service, f receivingFixture, expectedQty float64) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), f.tenantID, CreateRequest{
		ReceivingType: TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.locationID,
		Items: []CreateItemRequest{
			{ProductID: f.productID, LocationID: f.locationID, ExpectedQty: expectedQty, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("createReceipt: Create: %v", err)
	}
	return resp.ID
}

// --- IDOR: read-side tenant scoping ---

func TestRepository_FindReceiptByID_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "find-a", false)
	b := newReceivingFixture(t, tx, "find-b", false)

	svc, repo := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, b, 100)

	if _, err := repo.FindReceiptByID(context.Background(), a.tenantID, receiptID); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindReceiptByID(tenantA, tenantB's receipt) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindReceiptByID(context.Background(), b.tenantID, receiptID); err != nil {
		t.Errorf("FindReceiptByID(tenantB, its own receipt) = %v, want success", err)
	}
}

func TestService_Get_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "get-a", false)
	b := newReceivingFixture(t, tx, "get-b", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, b, 100)

	if _, err := svc.Get(context.Background(), a.tenantID, receiptID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(tenantA, tenantB's receipt) = %v, want ErrNotFound", err)
	}
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "list-a", false)
	b := newReceivingFixture(t, tx, "list-b", false)

	svc, repo := newReceivingServiceAndRepo(tx)
	createReceipt(t, svc, a, 10)
	createReceipt(t, svc, b, 10)

	rows, total, err := repo.ListReceipts(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("ListReceipts: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("ListReceipts(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's receipt)", len(rows), total)
	}
}

// --- Service-layer ownership verification at Create time ---

func TestService_Create_RejectsCrossTenantWarehouse(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "cw-a", false)
	b := newReceivingFixture(t, tx, "cw-b", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ReceivingType: TypePO, WarehouseID: b.warehouseID, StagingLocationID: a.locationID,
	}, 1)
	if !errors.Is(err, ErrWarehouseNotFound) {
		t.Fatalf("Create(tenantA, tenantB's warehouse_id) = %v, want ErrWarehouseNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantStagingLocation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "cl-a", false)
	b := newReceivingFixture(t, tx, "cl-b", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ReceivingType: TypePO, WarehouseID: a.warehouseID, StagingLocationID: b.locationID,
	}, 1)
	if !errors.Is(err, ErrLocationNotFound) {
		t.Fatalf("Create(tenantA, tenantB's staging_location_id) = %v, want ErrLocationNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantSupplier(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "cs-a", false)
	b := newReceivingFixture(t, tx, "cs-b", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ReceivingType: TypePO, WarehouseID: a.warehouseID, StagingLocationID: a.locationID,
		SupplierID: &b.supplierID,
	}, 1)
	if !errors.Is(err, ErrSupplierNotFound) {
		t.Fatalf("Create(tenantA, tenantB's supplier_id) = %v, want ErrSupplierNotFound", err)
	}
}

func TestService_Create_RejectsCrossTenantProductLine(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "cp-a", false)
	b := newReceivingFixture(t, tx, "cp-b", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ReceivingType: TypePO, WarehouseID: a.warehouseID, StagingLocationID: a.locationID,
		Items: []CreateItemRequest{{ProductID: b.productID, LocationID: a.locationID, ExpectedQty: 10, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("Create(tenantA, tenantB's product_id) = %v, want ErrProductNotFound", err)
	}
}

func TestService_Create_OwnTenant_WithSupplier_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "create-ok", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ReceivingType: TypePO, WarehouseID: a.warehouseID, StagingLocationID: a.locationID,
		SupplierID: &a.supplierID,
		Items:      []CreateItemRequest{{ProductID: a.productID, LocationID: a.locationID, ExpectedQty: 100, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create(own tenant): %v", err)
	}
	if resp.SupplierID == nil || *resp.SupplierID != a.supplierID {
		t.Errorf("SupplierID = %v, want %d", resp.SupplierID, a.supplierID)
	}
	if resp.SupplierName == nil || *resp.SupplierName != "Supplier create-ok" {
		t.Errorf("SupplierName = %v, want %q", resp.SupplierName, "Supplier create-ok")
	}
}

// --- Confirm: IDOR ---

func TestService_Confirm_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "confirm-cross-a", false)
	b := newReceivingFixture(t, tx, "confirm-cross-b", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, b, 100)

	_, err := svc.Confirm(context.Background(), a.tenantID, receiptID, ConfirmRequest{
		IdempotencyKey: "key-cross-1",
		Items:          []ConfirmItemRequest{{ItemID: 1, Quantity: 10, LotNo: "LOT-1"}},
	}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Confirm(tenantA, tenantB's receipt) = %v, want ErrNotFound", err)
	}

	// Confirming as the receipt's real owner must still work after the
	// rejected cross-tenant attempt — proves it left no residue (no
	// idempotency row inserted under the wrong key/tenant pairing).
	resp, err := svc.Get(context.Background(), b.tenantID, receiptID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Confirm: %v", err)
	}
	if resp.Status != StatusDraft {
		t.Errorf("receipt status after rejected cross-tenant Confirm = %v, want still DRAFT", resp.Status)
	}
}

// --- Confirm: ordinary (non-QC) product posts straight to Available ---

func TestService_Confirm_NonQCProduct_PostsToAvailable(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "confirm-nonqc", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, a, 100)
	receipt, err := svc.Get(context.Background(), a.tenantID, receiptID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := receipt.Items[0].ID

	resp, err := svc.Confirm(context.Background(), a.tenantID, receiptID, ConfirmRequest{
		IdempotencyKey: "key-nonqc-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 100, LotNo: "LOT-A"}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(non-QC product): %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Errorf("receipt status = %v, want COMPLETED", resp.Status)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance after Confirm: %v", err)
	}
	if bal.Quantity != 100 || bal.HoldQuantity != 0 {
		t.Errorf("balance after Confirm(non-QC) = quantity=%v hold=%v, want quantity=100 hold=0", bal.Quantity, bal.HoldQuantity)
	}
	if bal.Available() != 100 {
		t.Errorf("Available() = %v, want 100", bal.Available())
	}
}

// --- Confirm: QC-required product posts into Hold, and held stock
// cannot be reserved or picked — the explicit safety requirement this
// batch's approved architecture demands. ---

func TestService_Confirm_QCRequiredProduct_PostsToHold(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "confirm-qc", true)

	svc, _ := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, a, 100)
	receipt, err := svc.Get(context.Background(), a.tenantID, receiptID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := receipt.Items[0].ID

	_, err = svc.Confirm(context.Background(), a.tenantID, receiptID, ConfirmRequest{
		IdempotencyKey: "key-qc-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 100, LotNo: "LOT-QC"}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(QC-required product): %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-QC")
	if err != nil {
		t.Fatalf("FindBalance after Confirm: %v", err)
	}
	if bal.Quantity != 100 || bal.HoldQuantity != 100 {
		t.Fatalf("balance after Confirm(QC-required) = quantity=%v hold=%v, want quantity=100 hold=100", bal.Quantity, bal.HoldQuantity)
	}
	if bal.Available() != 0 {
		t.Fatalf("Available() after Confirm(QC-required) = %v, want 0 (fully held)", bal.Available())
	}

	// The explicit safety requirement: held stock must never be
	// reservable (and therefore never pickable).
	invSvc := inventory.NewService(invRepo, product.NewRepository(tx), location.NewRepository(tx))
	invRepo2 := inventory.NewRepository(tx)
	_, err = invSvc.ReserveStockTx(context.Background(), invRepo2, inventory.ReserveRequest{
		TenantID: a.tenantID, ProductID: a.productID, LocationID: a.locationID, LotNo: "LOT-QC", Quantity: 1,
	})
	if !errors.Is(err, inventory.ErrInsufficientStock) {
		t.Fatalf("ReserveStockTx against fully-held balance = %v, want ErrInsufficientStock", err)
	}
}

// --- Confirm: multiple lot splits in one call ---

func TestService_Confirm_MultipleSplits_PostsEachLotSeparately(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "confirm-splits", false)

	svc, repo := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, a, 100)
	receipt, err := svc.Get(context.Background(), a.tenantID, receiptID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := receipt.Items[0].ID

	resp, err := svc.Confirm(context.Background(), a.tenantID, receiptID, ConfirmRequest{
		IdempotencyKey: "key-splits-1",
		Items: []ConfirmItemRequest{{
			ItemID: itemID, Quantity: 100,
			Splits: []ConfirmSplitRequest{
				{Quantity: 60, LotNo: "LOT-X"},
				{Quantity: 40, LotNo: "LOT-Y"},
			},
		}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm(multi-lot splits): %v", err)
	}
	if len(resp.Items[0].Splits) != 2 {
		t.Fatalf("len(Splits) = %d, want 2", len(resp.Items[0].Splits))
	}

	invRepo := inventory.NewRepository(tx)
	balX, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-X")
	if err != nil || balX.Quantity != 60 {
		t.Errorf("LOT-X balance = %+v, err=%v, want quantity=60", balX, err)
	}
	balY, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-Y")
	if err != nil || balY.Quantity != 40 {
		t.Errorf("LOT-Y balance = %+v, err=%v, want quantity=40", balY, err)
	}

	splitsByItem, err := repo.listSplitsByReceipt(context.Background(), a.tenantID, receiptID)
	if err != nil {
		t.Fatalf("listSplitsByReceipt: %v", err)
	}
	if len(splitsByItem[itemID]) != 2 {
		t.Fatalf("persisted splits for item = %d, want 2", len(splitsByItem[itemID]))
	}
}

// --- Idempotency ---

func TestService_Confirm_DuplicateIdempotencyKey_DoesNotDoubleAddStock(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "confirm-idem", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, a, 100)
	receipt, err := svc.Get(context.Background(), a.tenantID, receiptID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := receipt.Items[0].ID

	req := ConfirmRequest{
		IdempotencyKey: "key-idem-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Quantity: 50, LotNo: "LOT-A"}},
	}
	if _, err := svc.Confirm(context.Background(), a.tenantID, receiptID, req, 1); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	// Same key again — must be treated as an already-processed no-op,
	// not a second AddStock.
	if _, err := svc.Confirm(context.Background(), a.tenantID, receiptID, req, 1); err != nil {
		t.Fatalf("duplicate Confirm: %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	bal, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance: %v", err)
	}
	if bal.Quantity != 50 {
		t.Errorf("Quantity after duplicate Confirm = %v, want 50 (must not double-add)", bal.Quantity)
	}
}

// --- Rollback / atomicity ---

// TestService_Confirm_SecondLineFailure_RollsBackFirstLinesAddStockToo
// proves Confirm's whole transaction — including the first line's
// already-successful AddStockTx — rolls back together when a later
// line in the same call fails validation.
func TestService_Confirm_SecondLineFailure_RollsBackFirstLinesAddStockToo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "confirm-rollback", false)

	// A second product created WITHOUT LotRequired (so its line can be
	// created with no lot_no at all), then flipped to LotRequired=true
	// afterward — simulating a product master update that happens after
	// a receipt line already exists with an empty lot_no. This is the
	// only way Confirm's own lot-required guard is actually reachable:
	// createItem itself already refuses to create a LotRequired
	// product's line without a lot number, so an item that reaches
	// Confirm with LotRequired=true and an empty LotNo can only have
	// gotten there via this exact sequence.
	var categoryID int64
	if err := tx.Raw(
		"SELECT category_id FROM products WHERE id = ?", a.productID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("lookup category: %v", err)
	}
	lotProduct := product.Product{
		TenantID: a.tenantID, SKU: "SKU-rollback-lot", Name: "Lot Required Product",
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&lotProduct).Error; err != nil {
		t.Fatalf("seed lot-required product: %v", err)
	}

	svc, _ := newReceivingServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		ReceivingType: TypePO, WarehouseID: a.warehouseID, StagingLocationID: a.locationID,
		Items: []CreateItemRequest{
			{ProductID: a.productID, LocationID: a.locationID, ExpectedQty: 100, UOM: "PCS"},
			{ProductID: lotProduct.ID, LocationID: a.locationID, ExpectedQty: 50, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	firstItemID, secondItemID := resp.Items[0].ID, resp.Items[1].ID

	if err := tx.Exec("UPDATE products SET lot_required = 1 WHERE id = ?", lotProduct.ID).Error; err != nil {
		t.Fatalf("flip lotProduct to LotRequired: %v", err)
	}

	_, err = svc.Confirm(context.Background(), a.tenantID, resp.ID, ConfirmRequest{
		IdempotencyKey: "key-rollback-1",
		Items: []ConfirmItemRequest{
			{ItemID: firstItemID, Quantity: 100, LotNo: "LOT-A"},
			// No LotNo supplied, and the item's own LotNo is still "" from
			// Create — now that the product requires a lot, this fails.
			{ItemID: secondItemID, Quantity: 50},
		},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Confirm(second line missing required lot) = %v, want ErrValidation", err)
	}

	// The whole transaction — including the first line's AddStockTx —
	// must have rolled back: no balance row for LOT-A may exist.
	invRepo := inventory.NewRepository(tx)
	if _, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A"); !errors.Is(err, inventory.ErrNotFound) {
		t.Errorf("FindBalance(LOT-A) after rolled-back Confirm = %v, want inventory.ErrNotFound (first line's AddStockTx must have rolled back too)", err)
	}

	updated, err := svc.Get(context.Background(), a.tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after rolled-back Confirm: %v", err)
	}
	if updated.Items[0].ReceivedQty != 0 {
		t.Errorf("first item's ReceivedQty after rolled-back Confirm = %v, want 0", updated.Items[0].ReceivedQty)
	}
	if updated.Status != StatusDraft {
		t.Errorf("receipt status after rolled-back Confirm = %v, want still DRAFT", updated.Status)
	}
}

// --- Cancel: IDOR ---

func TestService_Cancel_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "cancel-cross-a", false)
	b := newReceivingFixture(t, tx, "cancel-cross-b", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, b, 100)

	_, err := svc.Cancel(context.Background(), a.tenantID, receiptID, CancelRequest{}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel(tenantA, tenantB's receipt) = %v, want ErrNotFound", err)
	}

	resp, err := svc.Get(context.Background(), b.tenantID, receiptID)
	if err != nil {
		t.Fatalf("Get after rejected cross-tenant Cancel: %v", err)
	}
	if resp.Status != StatusDraft {
		t.Errorf("receipt status after rejected cross-tenant Cancel = %v, want still DRAFT", resp.Status)
	}
}

func TestService_Cancel_OwnTenant_Succeeds(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "cancel-ok", false)

	svc, _ := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, a, 100)

	resp, err := svc.Cancel(context.Background(), a.tenantID, receiptID, CancelRequest{}, 1)
	if err != nil {
		t.Fatalf("Cancel(own tenant): %v", err)
	}
	if resp.Status != StatusCancelled {
		t.Errorf("receipt status = %v, want CANCELLED", resp.Status)
	}
}

// --- Concurrency: AdjustReceivedQty's atomic guarded UPDATE ---

// TestRepository_AdjustReceivedQty_WrongTenant_NoRowsAffected proves a
// caller passing a mismatched tenantID against a real itemID owned by a
// different tenant affects zero rows, so a cross-tenant call can never
// corrupt another tenant's received_qty.
func TestRepository_AdjustReceivedQty_WrongTenant_NoRowsAffected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newReceivingFixture(t, tx, "adjust-a", false)
	b := newReceivingFixture(t, tx, "adjust-b", false)

	svc, repo := newReceivingServiceAndRepo(tx)
	receiptID := createReceipt(t, svc, a, 100)
	receipt, err := svc.Get(context.Background(), a.tenantID, receiptID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := receipt.Items[0].ID

	rows, err := repo.AdjustReceivedQty(context.Background(), b.tenantID, itemID, 5)
	if err != nil {
		t.Fatalf("AdjustReceivedQty: %v", err)
	}
	if rows != 0 {
		t.Fatalf("AdjustReceivedQty(tenantB, tenantA's item) affected %d rows, want 0", rows)
	}

	item, err := repo.FindItemByID(context.Background(), a.tenantID, itemID)
	if err != nil {
		t.Fatalf("FindItemByID: %v", err)
	}
	if item.ReceivedQty != 0 {
		t.Errorf("tenant A's received_qty = %v after a rejected cross-tenant AdjustReceivedQty, want unchanged 0", item.ReceivedQty)
	}
}

// TestRepository_AdjustReceivedQty_Concurrent_NoLostUpdates re-proves
// the same concurrency guarantee Phase 9/30-Batch-1 established: the
// atomic UPDATE's arithmetic happens inside the database, so N
// concurrent increments against the same row never lose an update to a
// race. Deliberately does NOT use testsupport.BeginRollbackTx — a
// single *sql.Tx is not safe for concurrent use by multiple goroutines
// — so this uses the real pooled *gorm.DB with explicit, FK-ordered
// cleanup instead of rollback.
func TestRepository_AdjustReceivedQty_Concurrent_NoLostUpdates(t *testing.T) {
	db := testsupport.ConnectDB(t)

	var tenantID, categoryID, productID, warehouseID, zoneID, locationID, receiptID, itemID int64
	if err := db.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES ('recv-concurrent', 'Receiving Concurrency Test Tenant', 'ACTIVE')",
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM receipt_item_splits WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM receipt_confirmations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM receipt_items WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM receipts WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM inventory_transactions WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM inventory WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM locations WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM zones WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM warehouses WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM products WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM product_categories WHERE tenant_id = ?", tenantID)
		db.Exec("DELETE FROM tenants WHERE id = ?", tenantID)
	})

	if err := db.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, 'CAT-RCONC', 'Category Conc')",
		tenantID,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	p := product.Product{TenantID: tenantID, SKU: "SKU-RCONC", Name: "Concurrent Product", Category: product.CategoryPackaging, CategoryID: &categoryID, BaseUOM: product.UOMPieces, Status: product.StatusActive}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	productID = p.ID
	if err := db.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, 'WH-RCONC', 'Warehouse Conc', 'Distribution Center', 'ACTIVE')",
		tenantID,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, 'Z-RCONC', 'Zone Conc', 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: "LOC-RCONC", Name: "Location Conc", Status: location.StatusActive}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	locationID = l.ID

	receipt := Receipt{TenantID: tenantID, ReceiptNo: "RCV-CONC", ReceivingType: TypePO, WarehouseID: warehouseID, StagingLocationID: locationID, Status: StatusWaiting}
	if err := db.Create(&receipt).Error; err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	receiptID = receipt.ID
	item := ReceiptItem{TenantID: tenantID, ReceiptID: receiptID, ProductID: productID, LocationID: locationID, ExpectedQty: 20, UOM: "PCS", Status: ItemStatusPending}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed item: %v", err)
	}
	itemID = item.ID

	const attempts = 20
	repo := NewRepository(db)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.AdjustReceivedQty(context.Background(), tenantID, itemID, 1); err != nil {
				t.Errorf("AdjustReceivedQty: %v", err)
			}
		}()
	}
	wg.Wait()

	updated, err := repo.FindItemByID(context.Background(), tenantID, itemID)
	if err != nil {
		t.Fatalf("FindItemByID: %v", err)
	}
	if updated.ReceivedQty != attempts {
		t.Errorf("final received_qty = %v, want %d (no lost updates across %d concurrent increments)", updated.ReceivedQty, attempts, attempts)
	}
}

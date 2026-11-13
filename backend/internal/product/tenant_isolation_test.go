package product

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
)

// newProductTestTenant inserts a fresh, empty tenant on tx, plus one
// product_categories row it owns (Create requires category_id) — every
// test below needs at least this much to exercise a real product
// end-to-end.
func newProductTestTenant(t *testing.T, tx *gorm.DB, label string) (tenantID, categoryID int64) {
	t.Helper()
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("product-iso-%s", label), fmt.Sprintf("Product Isolation Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newProductTestTenant(%s): insert tenant: %v", label, err)
	}
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newProductTestTenant(%s): insert category: %v", label, err)
	}
	return tenantID, categoryID
}

func seedProduct(t *testing.T, tx *gorm.DB, tenantID, categoryID int64, sku, name string) Product {
	t.Helper()
	p := Product{
		TenantID: tenantID, SKU: sku, Name: name, Category: CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: UOMPieces, Status: StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("seedProduct(%s): %v", sku, err)
	}
	return p
}

func TestRepository_FindByIDForTenant_RejectsCrossTenantID(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, catA := newProductTestTenant(t, tx, "cross-find-a")
	tenantB, catB := newProductTestTenant(t, tx, "cross-find-b")
	productB := seedProduct(t, tx, tenantB, catB, "SKU-CROSS-FIND", "Tenant B Product")
	_ = catA

	repo := NewRepository(tx)

	if _, err := repo.FindByIDForTenant(context.Background(), tenantA, productB.ID); err != ErrNotFound {
		t.Errorf("FindByIDForTenant(tenantA, tenantB's product) = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByIDForTenant(context.Background(), tenantB, productB.ID); err != nil {
		t.Errorf("FindByIDForTenant(tenantB, its own product) = %v, want success", err)
	}
}

func TestService_Update_RejectsCrossTenantID(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, _ := newProductTestTenant(t, tx, "cross-update-a")
	tenantB, catB := newProductTestTenant(t, tx, "cross-update-b")
	productB := seedProduct(t, tx, tenantB, catB, "SKU-CROSS-UPDATE", "Tenant B Product")

	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, "", 0)

	_, err := svc.Update(context.Background(), tenantA, productB.ID, UpdateRequest{
		SKU: "SKU-HACKED", Name: "Hacked", BaseUOM: UOMPieces, Status: StatusActive,
	})
	if err != ErrNotFound {
		t.Errorf("Update(tenantA, tenantB's product) = %v, want ErrNotFound", err)
	}

	// Confirm tenant B's product was NOT mutated.
	after, err := svc.Get(context.Background(), tenantB, productB.ID)
	if err != nil {
		t.Fatalf("Get(tenantB, its own product) after rejected cross-tenant update: %v", err)
	}
	if after.SKU != "SKU-CROSS-UPDATE" || after.Name != "Tenant B Product" {
		t.Errorf("CROSS-TENANT WRITE CONFIRMED: product mutated to %+v", after)
	}
}

func TestService_Delete_RejectsCrossTenantID(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, _ := newProductTestTenant(t, tx, "cross-delete-a")
	tenantB, catB := newProductTestTenant(t, tx, "cross-delete-b")
	productB := seedProduct(t, tx, tenantB, catB, "SKU-CROSS-DELETE", "Tenant B Product")

	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, "", 0)

	if err := svc.Delete(context.Background(), tenantA, productB.ID); err != ErrNotFound {
		t.Errorf("Delete(tenantA, tenantB's product) = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(context.Background(), tenantB, productB.ID); err != nil {
		t.Errorf("tenant B's product should still exist after the rejected cross-tenant delete: %v", err)
	}
}

func TestService_Create_RejectsCrossTenantCategoryID(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, _ := newProductTestTenant(t, tx, "cross-create-a")
	_, catB := newProductTestTenant(t, tx, "cross-create-b")

	categoryRepo := fakeCategoryRepo{tx: tx}
	svc := NewService(NewRepository(tx), categoryRepo, nil, nil, nil, nil, nil, "", 0)

	_, err := svc.Create(context.Background(), tenantA, CreateRequest{
		SKU: "SKU-CROSS-CAT", Name: "Cross Category", BaseUOM: UOMPieces, CategoryID: &catB,
	})

	var validationErr ErrValidation
	if err == nil {
		t.Fatal("expected Create to reject a category_id owned by another tenant")
	}
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// fakeCategoryRepo adapts a live product_categories table (seeded
// directly via raw SQL in these tests, since internal/productcategory
// isn't imported here to avoid a test-only import cycle risk) to the
// categoryChecker interface, so Create's real cross-tenant rejection
// path is exercised against actual rows rather than a scripted fake.
type fakeCategoryRepo struct {
	tx *gorm.DB
}

func (f fakeCategoryRepo) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	var count int64
	err := f.tx.WithContext(ctx).Table("product_categories").
		Where("id = ? AND tenant_id = ?", id, tenantID).Count(&count).Error
	return count > 0, err
}

func TestBarcodes_CRUD_ScopedToOwningProductsTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, catA := newProductTestTenant(t, tx, "barcode-a")
	tenantB, catB := newProductTestTenant(t, tx, "barcode-b")
	productA := seedProduct(t, tx, tenantA, catA, "SKU-BARCODE-A", "Product A")
	productB := seedProduct(t, tx, tenantB, catB, "SKU-BARCODE-B", "Product B")

	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, "", 0)

	created, err := svc.CreateBarcode(context.Background(), tenantA, productA.ID, CreateBarcodeRequest{
		BarcodeValue: "8850000000001", BarcodeType: BarcodeTypeBoxCode, QtyPerBarcode: 24,
	})
	if err != nil {
		t.Fatalf("CreateBarcode(own product): %v", err)
	}

	// Tenant A cannot attach a barcode to tenant B's product.
	if _, err := svc.CreateBarcode(context.Background(), tenantA, productB.ID, CreateBarcodeRequest{
		BarcodeValue: "8850000000002",
	}); err != ErrNotFound {
		t.Errorf("CreateBarcode(tenantA, tenantB's product) = %v, want ErrNotFound", err)
	}

	list, err := svc.ListBarcodes(context.Background(), tenantA, productA.ID)
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("ListBarcodes(own product) = %+v, err=%v, want exactly the one created barcode", list, err)
	}

	// ListBarcodes itself doesn't error on a cross-tenant productID (the
	// JOIN just returns zero rows) — the handler layer always calls Get
	// first (which DOES reject cross-tenant) before ever reaching
	// ListBarcodes; confirm the JOIN alone still returns no rows as
	// defense-in-depth.
	crossList, err := svc.ListBarcodes(context.Background(), tenantB, productA.ID)
	if err != nil {
		t.Fatalf("ListBarcodes(tenantB, tenantA's product): %v", err)
	}
	if len(crossList) != 0 {
		t.Errorf("ListBarcodes(tenantB, tenantA's product) = %+v, want empty (JOIN-level isolation)", crossList)
	}

	if _, err := svc.UpdateBarcode(context.Background(), tenantB, productA.ID, created.ID, UpdateBarcodeRequest{
		BarcodeValue: "HACKED", Status: StatusActive,
	}); err != ErrNotFound {
		t.Errorf("UpdateBarcode(tenantB, tenantA's barcode) = %v, want ErrNotFound", err)
	}

	if err := svc.DeleteBarcode(context.Background(), tenantB, productA.ID, created.ID); err != ErrNotFound {
		t.Errorf("DeleteBarcode(tenantB, tenantA's barcode) = %v, want ErrNotFound", err)
	}

	// Confirm it's still there after both rejected cross-tenant attempts.
	stillThere, err := svc.ListBarcodes(context.Background(), tenantA, productA.ID)
	if err != nil || len(stillThere) != 1 {
		t.Errorf("barcode should survive rejected cross-tenant update/delete, got %+v (err=%v)", stillThere, err)
	}
}

func TestBarcodes_DuplicateValueWithinSameProduct_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, catA := newProductTestTenant(t, tx, "barcode-dup")
	productA := seedProduct(t, tx, tenantA, catA, "SKU-BARCODE-DUP", "Product Dup")

	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, "", 0)

	if _, err := svc.CreateBarcode(context.Background(), tenantA, productA.ID, CreateBarcodeRequest{
		BarcodeValue: "8850000000099",
	}); err != nil {
		t.Fatalf("first CreateBarcode: %v", err)
	}

	_, err := svc.CreateBarcode(context.Background(), tenantA, productA.ID, CreateBarcodeRequest{
		BarcodeValue: "8850000000099",
	})
	if err != ErrDuplicateBarcodeMapping {
		t.Errorf("duplicate barcode on same product = %v, want ErrDuplicateBarcodeMapping", err)
	}
}

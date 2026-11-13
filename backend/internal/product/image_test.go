package product

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wms-backend/internal/platform/testsupport"
)

// tinyPNG is a real, valid 1x1 transparent PNG — enough for
// mimetype.Detect to genuinely sniff "image/png", not a fabricated
// byte sequence that merely happens to start with the right magic
// bytes.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func newImageTestService(t *testing.T) (*Service, int64, Product) {
	t.Helper()
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	// A fixed, short SKU is safe here (not t.Name(), which can exceed
	// the sku column's VARCHAR(50)) — each test runs in its own
	// rolled-back transaction against its own freshly-created tenant,
	// so there is no real cross-test collision to avoid.
	tenantID, categoryID := newProductTestTenant(t, tx, "image")
	p := seedProduct(t, tx, tenantID, categoryID, "SKU-IMG-TEST", "Image Test Product")

	dir := t.TempDir()
	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, dir, 0)
	return svc, tenantID, p
}

func TestUploadImage_AcceptsRealPNG_PersistsAndServes(t *testing.T) {
	svc, tenantID, p := newImageTestService(t)

	resp, err := svc.UploadImage(context.Background(), tenantID, p.ID, tinyPNG, "cheese.png")
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	if !resp.HasImage || resp.ImageURL == nil {
		t.Fatalf("response = %+v, want HasImage=true and a non-nil ImageURL", resp)
	}
	if *resp.ImageURL != imageURL(p.ID) {
		t.Errorf("ImageURL = %q, want %q", *resp.ImageURL, imageURL(p.ID))
	}

	absPath, mimeType, err := svc.ImageFile(context.Background(), tenantID, p.ID)
	if err != nil {
		t.Fatalf("ImageFile: %v", err)
	}
	if mimeType != "image/png" {
		t.Errorf("mimeType = %q, want image/png", mimeType)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("the uploaded file should exist on disk at %s: %v", absPath, err)
	}
	if string(data) != string(tinyPNG) {
		t.Error("stored file contents do not match the uploaded bytes")
	}
}

func TestUploadImage_RejectsNonImageContent(t *testing.T) {
	svc, tenantID, p := newImageTestService(t)

	_, err := svc.UploadImage(context.Background(), tenantID, p.ID, []byte("not an image, just text"), "fake.png")
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("error = %v (%T), want ErrValidation (content-sniffing must reject this even with a .png filename)", err, err)
	}
}

func TestUploadImage_RejectsOverSizeLimit(t *testing.T) {
	svc, tenantID, p := newImageTestService(t)

	oversized := make([]byte, maxImageUploadBytes+1)
	copy(oversized, tinyPNG)

	_, err := svc.UploadImage(context.Background(), tenantID, p.ID, oversized, "big.png")
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("error = %v (%T), want ErrValidation (over size limit)", err, err)
	}
}

func TestUploadImage_RejectsCrossTenantProductID(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, _ := newProductTestTenant(t, tx, "image-cross-a")
	tenantB, catB := newProductTestTenant(t, tx, "image-cross-b")
	productB := seedProduct(t, tx, tenantB, catB, "SKU-IMAGE-CROSS", "Tenant B Product")

	dir := t.TempDir()
	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, dir, 0)

	_, err := svc.UploadImage(context.Background(), tenantA, productB.ID, tinyPNG, "cheese.png")
	if err != ErrNotFound {
		t.Errorf("UploadImage(tenantA, tenantB's product) = %v, want ErrNotFound", err)
	}

	// No file should have been written for a rejected cross-tenant
	// upload — confirms tenant ownership is checked BEFORE any disk
	// write, not after.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, _ := os.ReadDir(filepath.Join(dir, e.Name()))
		if len(sub) > 0 {
			t.Errorf("expected no files written under %s after a rejected cross-tenant upload, found %d", dir, len(sub))
		}
	}
}

func TestUploadImage_ReplaceDeletesThePreviousFile(t *testing.T) {
	svc, tenantID, p := newImageTestService(t)

	first, err := svc.UploadImage(context.Background(), tenantID, p.ID, tinyPNG, "first.png")
	if err != nil {
		t.Fatalf("first UploadImage: %v", err)
	}
	firstAbsPath, _, err := svc.ImageFile(context.Background(), tenantID, p.ID)
	if err != nil {
		t.Fatalf("ImageFile after first upload: %v", err)
	}

	second, err := svc.UploadImage(context.Background(), tenantID, p.ID, tinyPNG, "second.png")
	if err != nil {
		t.Fatalf("second UploadImage (replace): %v", err)
	}
	secondAbsPath, _, err := svc.ImageFile(context.Background(), tenantID, p.ID)
	if err != nil {
		t.Fatalf("ImageFile after replace: %v", err)
	}

	if *first.ImageURL != *second.ImageURL {
		t.Errorf("ImageURL should stay stable across a replace (it's keyed by product id, not file name): %q vs %q", *first.ImageURL, *second.ImageURL)
	}
	if firstAbsPath == secondAbsPath {
		t.Error("replace should generate a new UUID filename, not overwrite the same path")
	}
	if _, err := os.Stat(firstAbsPath); !os.IsNotExist(err) {
		t.Errorf("the replaced file should have been deleted from disk, stat err = %v", err)
	}
	if _, err := os.Stat(secondAbsPath); err != nil {
		t.Errorf("the new file should exist on disk: %v", err)
	}
}

func TestRemoveImage_ClearsMetadataAndDeletesFile(t *testing.T) {
	svc, tenantID, p := newImageTestService(t)

	if _, err := svc.UploadImage(context.Background(), tenantID, p.ID, tinyPNG, "cheese.png"); err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	absPath, _, err := svc.ImageFile(context.Background(), tenantID, p.ID)
	if err != nil {
		t.Fatalf("ImageFile before remove: %v", err)
	}

	resp, err := svc.RemoveImage(context.Background(), tenantID, p.ID)
	if err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}
	if resp.HasImage || resp.ImageURL != nil {
		t.Errorf("response after remove = %+v, want HasImage=false and nil ImageURL", resp)
	}

	if _, _, err := svc.ImageFile(context.Background(), tenantID, p.ID); err != ErrNotFound {
		t.Errorf("ImageFile after remove = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(absPath); !os.IsNotExist(err) {
		t.Errorf("the removed file should have been deleted from disk, stat err = %v", err)
	}
}

func TestImageFile_RejectsCrossTenantAccess(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantA, catA := newProductTestTenant(t, tx, "image-serve-a")
	tenantB, _ := newProductTestTenant(t, tx, "image-serve-b")
	productA := seedProduct(t, tx, tenantA, catA, "SKU-IMAGE-SERVE", "Product A")

	dir := t.TempDir()
	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, dir, 0)
	if _, err := svc.UploadImage(context.Background(), tenantA, productA.ID, tinyPNG, "cheese.png"); err != nil {
		t.Fatalf("UploadImage: %v", err)
	}

	if _, _, err := svc.ImageFile(context.Background(), tenantB, productA.ID); err != ErrNotFound {
		t.Errorf("ImageFile(tenantB, tenantA's product) = %v, want ErrNotFound", err)
	}
}

func TestUploadImage_NotConfigured_ReturnsValidationError(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, categoryID := newProductTestTenant(t, tx, "image-unconfigured")
	p := seedProduct(t, tx, tenantID, categoryID, "SKU-IMAGE-UNCONFIGURED", "Unconfigured")

	svc := NewService(NewRepository(tx), nil, nil, nil, nil, nil, nil, "", 0)

	_, err := svc.UploadImage(context.Background(), tenantID, p.ID, tinyPNG, "cheese.png")
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("error = %v (%T), want ErrValidation when imageStorageDir is empty", err, err)
	}
}

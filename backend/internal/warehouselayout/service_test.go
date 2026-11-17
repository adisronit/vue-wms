package warehouselayout

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/user"
)

type fixture struct {
	tenantID    int64
	warehouseID int64
	zoneID      int64
	userID      int64
}

func newFixture(t *testing.T, tx *gorm.DB, label string) fixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"wl-"+label, "Warehouse Layout Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newFixture(%s): insert tenant: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed zone: %v", label, err)
	}

	// users is a global table (not tenant-scoped — see
	// wms_tenant_rbac_model memory); role_id is required and roles ARE
	// per-tenant, so a fixture-local role row is seeded first purely to
	// satisfy that FK, independent of this test's own RBAC concerns.
	var roleID int64
	if err := tx.Raw(
		"INSERT INTO roles (tenant_id, code, name, is_active) OUTPUT INSERTED.id VALUES (?, ?, ?, 1)",
		tenantID, "ROLE-"+label, "Role "+label,
	).Scan(&roleID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed role: %v", label, err)
	}

	var userID int64
	if err := tx.Raw(
		"INSERT INTO users (username, password_hash, full_name, email, role_id, status) OUTPUT INSERTED.id VALUES (?, 'x', ?, ?, ?, 'ACTIVE')",
		"user-"+label, "User "+label, "user-"+label+"@example.com", roleID,
	).Scan(&userID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed user: %v", label, err)
	}

	return fixture{tenantID: tenantID, warehouseID: warehouseID, zoneID: zoneID, userID: userID}
}

func newTestService(tx *gorm.DB, t *testing.T) *Service {
	dir, err := os.MkdirTemp("", "wms-layout-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return NewService(NewRepository(tx), user.NewRepository(tx), dir, 0)
}

func simpleObject(objType string) SaveObjectRequest {
	return SaveObjectRequest{ObjectType: objType, X: 10, Y: 20, Width: 100, Height: 50}
}

func TestService_SaveLayout_CreatesNewVersion_NeverUpdatesInPlace(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "versioning")
	svc := newTestService(tx, t)

	v1, err := svc.SaveLayout(context.Background(), f.tenantID, f.warehouseID, SaveLayoutRequest{
		Objects: []SaveObjectRequest{simpleObject("RACK")},
	}, f.userID)
	if err != nil {
		t.Fatalf("SaveLayout #1: %v", err)
	}
	if v1.VersionNo != 1 {
		t.Fatalf("VersionNo #1 = %d, want 1", v1.VersionNo)
	}

	v2, err := svc.SaveLayout(context.Background(), f.tenantID, f.warehouseID, SaveLayoutRequest{
		Objects: []SaveObjectRequest{simpleObject("RACK"), simpleObject("AISLE")},
	}, f.userID)
	if err != nil {
		t.Fatalf("SaveLayout #2: %v", err)
	}
	if v2.VersionNo != 2 {
		t.Fatalf("VersionNo #2 = %d, want 2 (must never update version 1 in place)", v2.VersionNo)
	}
	if v2.ID == v1.ID {
		t.Fatalf("version #2 reused id %d from version #1 — must be a brand-new row", v1.ID)
	}

	latest, err := svc.GetLayout(context.Background(), f.tenantID, f.warehouseID)
	if err != nil {
		t.Fatalf("GetLayout: %v", err)
	}
	if latest == nil || latest.VersionNo != 2 || len(latest.Objects) != 2 {
		t.Fatalf("GetLayout = %+v, want version 2 with 2 objects", latest)
	}

	versions, err := svc.ListVersions(context.Background(), f.tenantID, f.warehouseID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 || versions[0].VersionNo != 2 || versions[1].VersionNo != 1 {
		t.Fatalf("ListVersions = %+v, want [v2, v1] newest first", versions)
	}
	if versions[0].ObjectCount != 2 || versions[1].ObjectCount != 1 {
		t.Errorf("ObjectCounts = %d/%d, want 2/1", versions[0].ObjectCount, versions[1].ObjectCount)
	}
}

func TestService_GetLayout_NoLayoutYet_ReturnsNilNotError(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "nolayout")
	svc := newTestService(tx, t)

	resp, err := svc.GetLayout(context.Background(), f.tenantID, f.warehouseID)
	if err != nil {
		t.Fatalf("GetLayout: %v", err)
	}
	if resp != nil {
		t.Fatalf("GetLayout = %+v, want nil (no layout saved yet is not an error)", resp)
	}
}

func TestService_SaveLayout_RejectsInvalidObjectType(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "badtype")
	svc := newTestService(tx, t)

	_, err := svc.SaveLayout(context.Background(), f.tenantID, f.warehouseID, SaveLayoutRequest{
		Objects: []SaveObjectRequest{{ObjectType: "SPACESHIP", Width: 10, Height: 10}},
	}, f.userID)
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("SaveLayout(bad object_type) = %v, want ErrValidation", err)
	}
}

func TestService_SaveLayout_RejectsCrossTenantWarehouse(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "cross-a")
	b := newFixture(t, tx, "cross-b")
	svc := newTestService(tx, t)

	_, err := svc.SaveLayout(context.Background(), a.tenantID, b.warehouseID, SaveLayoutRequest{
		Objects: []SaveObjectRequest{simpleObject("WALL")},
	}, a.userID)
	if err != ErrNotFound {
		t.Fatalf("SaveLayout(tenantA, tenantB's warehouse) = %v, want ErrNotFound", err)
	}
}

func TestService_GetLayout_ReferenceLabel_ResolvedOrFallsBackWhenDangling(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "refs")
	svc := newTestService(tx, t)

	zoneRefType := "ZONE"
	danglingRefType := "ZONE"
	danglingID := int64(999999)
	label := "Free text fallback"

	_, err := svc.SaveLayout(context.Background(), f.tenantID, f.warehouseID, SaveLayoutRequest{
		Objects: []SaveObjectRequest{
			{ObjectType: "ZONE", ReferenceType: &zoneRefType, ReferenceID: &f.zoneID, Width: 100, Height: 100},
			{ObjectType: "ZONE", ReferenceType: &danglingRefType, ReferenceID: &danglingID, Width: 50, Height: 50, Label: &label},
		},
	}, f.userID)
	if err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}

	resp, err := svc.GetLayout(context.Background(), f.tenantID, f.warehouseID)
	if err != nil {
		t.Fatalf("GetLayout: %v", err)
	}
	if len(resp.Objects) != 2 {
		t.Fatalf("len(Objects) = %d, want 2", len(resp.Objects))
	}
	if resp.Objects[0].ReferenceLabel == nil {
		t.Errorf("Objects[0].ReferenceLabel = nil, want the real zone's own code/name")
	}
	if resp.Objects[1].ReferenceLabel != nil {
		t.Errorf("Objects[1].ReferenceLabel = %q, want nil (dangling reference must fall back silently, never error)", *resp.Objects[1].ReferenceLabel)
	}
	if resp.Objects[1].Label == nil || *resp.Objects[1].Label != label {
		t.Errorf("Objects[1].Label = %v, want %q (own free-text label preserved)", resp.Objects[1].Label, label)
	}
}

// newTestJPEG returns a tiny real JPEG (not a fabricated byte blob) so
// Service.UploadBackgroundImage's content-sniffing and dimension
// decoding both exercise real image library code, not a mock.
func newTestJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestService_UploadBackgroundImage_DecodesRealDimensions(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "image")
	svc := newTestService(tx, t)

	data := newTestJPEG(t, 400, 300)
	img, err := svc.UploadBackgroundImage(context.Background(), f.tenantID, f.warehouseID, data, "floorplan.jpg", f.userID)
	if err != nil {
		t.Fatalf("UploadBackgroundImage: %v", err)
	}
	if img.WidthPx != 400 || img.HeightPx != 300 {
		t.Errorf("dimensions = %dx%d, want 400x300 (must come from the real decoded image, not a client-supplied value)", img.WidthPx, img.HeightPx)
	}

	scale := 42.5
	saved, err := svc.SaveLayout(context.Background(), f.tenantID, f.warehouseID, SaveLayoutRequest{
		BackgroundImageID: &img.ID, ScalePxPerMeter: &scale,
		Objects: []SaveObjectRequest{simpleObject("WALL")},
	}, f.userID)
	if err != nil {
		t.Fatalf("SaveLayout with background image: %v", err)
	}
	if saved.BackgroundImageWidthPx == nil || *saved.BackgroundImageWidthPx != 400 {
		t.Errorf("BackgroundImageWidthPx = %v, want 400", saved.BackgroundImageWidthPx)
	}
	if saved.ScalePxPerMeter == nil || *saved.ScalePxPerMeter != scale {
		t.Errorf("ScalePxPerMeter = %v, want %v", saved.ScalePxPerMeter, scale)
	}
	if saved.BackgroundImageURL == nil {
		t.Errorf("BackgroundImageURL = nil, want a handler-mediated URL")
	}
}

func TestService_UploadBackgroundImage_RejectsNonImageData(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "badimage")
	svc := newTestService(tx, t)

	_, err := svc.UploadBackgroundImage(context.Background(), f.tenantID, f.warehouseID, []byte("not an image"), "fake.jpg", f.userID)
	if _, ok := err.(ErrValidation); !ok {
		t.Fatalf("UploadBackgroundImage(garbage bytes) = %v, want ErrValidation", err)
	}
}

func TestService_RenderPDF_NoLayoutYet_DoesNotPanic(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "pdfempty")
	svc := newTestService(tx, t)

	data, err := svc.RenderPDF(context.Background(), f.tenantID, f.warehouseID)
	if err != nil {
		t.Fatalf("RenderPDF (no layout): %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("RenderPDF returned empty bytes")
	}
}

func TestService_RenderPDF_WithObjectsAndBackgroundImage(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "pdffull")
	svc := newTestService(tx, t)

	imgData := newTestJPEG(t, 200, 150)
	img, err := svc.UploadBackgroundImage(context.Background(), f.tenantID, f.warehouseID, imgData, "bg.jpg", f.userID)
	if err != nil {
		t.Fatalf("UploadBackgroundImage: %v", err)
	}

	if _, err := svc.SaveLayout(context.Background(), f.tenantID, f.warehouseID, SaveLayoutRequest{
		BackgroundImageID: &img.ID,
		Objects:           []SaveObjectRequest{simpleObject("RACK"), simpleObject("DOCK")},
	}, f.userID); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}

	data, err := svc.RenderPDF(context.Background(), f.tenantID, f.warehouseID)
	if err != nil {
		t.Fatalf("RenderPDF: %v", err)
	}
	if len(data) < 100 {
		t.Fatalf("RenderPDF returned suspiciously small output (%d bytes)", len(data))
	}
}

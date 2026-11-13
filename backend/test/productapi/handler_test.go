// Package productapi exercises the Product Master API end to end (real
// HTTP handler, real RequireTenantContext/RequirePermission middleware,
// real SQL Server, real multipart image upload against real disk
// storage) — see test/productcategoryapi's own package doc for why
// this lives outside internal/product (testrouter imports product to
// wire its handler; a test inside that package importing testrouter
// back would be an import cycle) and why it issues its own JWT rather
// than using backend/test/isolation's stale, pre-Phase-29E fixtures.
package productapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/auth"
	"wms-backend/internal/platform/testrouter"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/role"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

const rbacTestJWTSecret = "product-api-rbac-test-secret"

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

type rbacFixture struct {
	tenantID   int64
	userID     int64
	categoryID int64
}

func seedRBACFixture(t *testing.T, tx *gorm.DB, label string) rbacFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("product-api-%s", label), fmt.Sprintf("Product API Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): insert category: %v", label, err)
	}

	r := role.Role{TenantID: tenantID, Code: "PRODUCT_TESTER", Name: "Product Tester", IsActive: true}
	if err := tx.Create(&r).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): create role: %v", label, err)
	}

	u := user.User{
		Username: fmt.Sprintf("product_rbac_%s", label), PasswordHash: "unused-this-test-issues-its-own-jwt",
		FullName: "Product RBAC Test User", Email: fmt.Sprintf("product.rbac.%s@isolation.test", label),
		RoleID: r.ID, Status: user.StatusActive,
	}
	if err := tx.Create(&u).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): create user: %v", label, err)
	}

	tu := tenant.TenantUser{TenantID: tenantID, UserID: u.ID, RoleID: r.ID, Status: tenant.TenantUserStatusActive}
	if err := tx.Create(&tu).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): create tenant_users membership: %v", label, err)
	}

	return rbacFixture{tenantID: tenantID, userID: u.ID, categoryID: categoryID}
}

func (fx rbacFixture) token(t *testing.T, permissions []string) string {
	t.Helper()
	tok, _, err := auth.IssueToken(rbacTestJWTSecret, time.Hour, fx.userID, "product_rbac_user", fx.tenantID, "PRODUCT_TESTER", permissions)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	return tok
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
}

func doRequest(t *testing.T, engine http.Handler, method, path, token string, body io.Reader, contentType string) (int, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var env envelope
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec.Code, env
}

func doJSON(t *testing.T, engine http.Handler, method, path, token string, payload any) (int, envelope) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(b)
	}
	return doRequest(t, engine, method, path, token, body, "application/json")
}

func TestCreateProduct_RequiresPermission(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "create-perm")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)

	status, _ := doJSON(t, deps.Engine, http.MethodPost, "/api/v1/products", fx.token(t, []string{}), map[string]any{
		"sku": "SKU-NOPERM", "name": "No Perm", "base_uom": "G", "category_id": fmt.Sprint(fx.categoryID),
	})
	if status != http.StatusForbidden {
		t.Errorf("POST /products with no permissions = %d, want 403", status)
	}
}

func TestCreateProduct_RequiresCategoryID(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "create-nocat")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)

	status, env := doJSON(t, deps.Engine, http.MethodPost, "/api/v1/products", fx.token(t, []string{"product.create"}), map[string]any{
		"sku": "SKU-NOCAT", "name": "No Category", "base_uom": "G",
	})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("POST /products without category_id = %d, want 422, body=%+v", status, env)
	}
}

func TestCreateAndGetProduct_EndToEnd(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "create-ok")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	token := fx.token(t, []string{"product.create", "product.view"})

	status, env := doJSON(t, deps.Engine, http.MethodPost, "/api/v1/products", token, map[string]any{
		"sku": "SKU-CHEDDAR-150G", "name": "Cheddar Block 150g", "name_th": "เชดดาร์บล็อก 150 กรัม",
		"base_uom": "G", "pack_size": 150, "pack_size_unit": "G",
		"category_id": fmt.Sprint(fx.categoryID), "storage_type": "CHILLED", "rotation_rule": "FEFO",
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /products = %d, body=%+v", status, env)
	}

	var created struct {
		ID          string `json:"id"`
		SKU         string `json:"sku"`
		HasImage    bool   `json:"has_image"`
		StorageType string `json:"storage_type"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatalf("decode created product: %v", err)
	}
	if created.SKU != "SKU-CHEDDAR-150G" || created.HasImage || created.StorageType != "CHILLED" {
		t.Errorf("created = %+v, want SKU-CHEDDAR-150G/HasImage=false/CHILLED", created)
	}

	status, env = doRequest(t, deps.Engine, http.MethodGet, "/api/v1/products/"+created.ID, token, nil, "")
	if status != http.StatusOK {
		t.Fatalf("GET /products/%s = %d, body=%+v", created.ID, status, env)
	}
}

func buildMultipartImage(t *testing.T, filename string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	part, err := w.CreateFormFile("image", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart body: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return buf, w.FormDataContentType()
}

func TestProductImage_UploadServeRemove_EndToEnd(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "image-e2e")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	token := fx.token(t, []string{"product.create", "product.view", "product.update"})

	status, env := doJSON(t, deps.Engine, http.MethodPost, "/api/v1/products", token, map[string]any{
		"sku": "SKU-IMAGE-E2E", "name": "Olive Oil 500ml", "base_uom": "ML", "pack_size": 500,
		"category_id": fmt.Sprint(fx.categoryID),
	})
	if status != http.StatusCreated {
		t.Fatalf("POST /products = %d, body=%+v", status, env)
	}
	var created struct{ ID string }
	_ = json.Unmarshal(env.Data, &created)

	body, contentType := buildMultipartImage(t, "olive-oil.png", tinyPNG)
	status, env = doRequest(t, deps.Engine, http.MethodPost, "/api/v1/products/"+created.ID+"/image", token, body, contentType)
	if status != http.StatusOK {
		t.Fatalf("POST /products/%s/image = %d, body=%+v", created.ID, status, env)
	}
	var afterUpload struct {
		HasImage bool    `json:"has_image"`
		ImageURL *string `json:"image_url"`
	}
	if err := json.Unmarshal(env.Data, &afterUpload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !afterUpload.HasImage || afterUpload.ImageURL == nil {
		t.Fatalf("after upload = %+v, want has_image=true and a non-nil image_url", afterUpload)
	}

	req := httptest.NewRequest(http.MethodGet, *afterUpload.ImageURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	deps.Engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", *afterUpload.ImageURL, rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), tinyPNG) {
		t.Error("served image bytes do not match the uploaded bytes")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}

	status, env = doRequest(t, deps.Engine, http.MethodDelete, "/api/v1/products/"+created.ID+"/image", token, nil, "")
	if status != http.StatusOK {
		t.Fatalf("DELETE image = %d, body=%+v", status, env)
	}

	status, _ = doRequest(t, deps.Engine, http.MethodGet, *afterUpload.ImageURL, token, nil, "")
	if status != http.StatusNotFound {
		t.Errorf("GET image after removal = %d, want 404", status)
	}
}

func TestProductImage_RejectsNonImageUpload(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "image-reject")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	token := fx.token(t, []string{"product.create", "product.update"})

	_, env := doJSON(t, deps.Engine, http.MethodPost, "/api/v1/products", token, map[string]any{
		"sku": "SKU-IMAGE-REJECT", "name": "Pasta 500g", "base_uom": "G", "pack_size": 500,
		"category_id": fmt.Sprint(fx.categoryID),
	})
	var created struct{ ID string }
	_ = json.Unmarshal(env.Data, &created)

	body, contentType := buildMultipartImage(t, "fake.png", []byte("not actually a png"))
	status, env := doRequest(t, deps.Engine, http.MethodPost, "/api/v1/products/"+created.ID+"/image", token, body, contentType)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("uploading non-image content = %d, want 422, body=%+v", status, env)
	}
}

func TestProductBarcodes_EndToEnd(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "barcode-e2e")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	token := fx.token(t, []string{"product.create", "product.view", "product.update"})

	_, env := doJSON(t, deps.Engine, http.MethodPost, "/api/v1/products", token, map[string]any{
		"sku": "SKU-BARCODE-E2E", "name": "Pasta 500g", "base_uom": "G", "pack_size": 500,
		"category_id": fmt.Sprint(fx.categoryID),
	})
	var created struct{ ID string }
	_ = json.Unmarshal(env.Data, &created)

	status, env := doJSON(t, deps.Engine, http.MethodPost, "/api/v1/products/"+created.ID+"/barcodes", token, map[string]any{
		"barcode_value": "8850000011111", "barcode_type": "BOX_CODE", "qty_per_barcode": 24,
	})
	if status != http.StatusCreated {
		t.Fatalf("POST barcodes = %d, body=%+v", status, env)
	}

	status, env = doRequest(t, deps.Engine, http.MethodGet, "/api/v1/products/"+created.ID+"/barcodes", token, nil, "")
	if status != http.StatusOK {
		t.Fatalf("GET barcodes = %d, body=%+v", status, env)
	}
	var list []struct {
		BarcodeValue string `json:"barcode_value"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("decode barcode list: %v", err)
	}
	if len(list) != 1 || list[0].BarcodeValue != "8850000011111" {
		t.Errorf("barcode list = %+v, want exactly [8850000011111]", list)
	}
}

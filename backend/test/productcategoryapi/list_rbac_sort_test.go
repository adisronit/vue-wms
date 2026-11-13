// Package productcategoryapi exercises GET /api/v1/product-categories
// end to end (real HTTP handler, real RequireTenantContext/RequirePermission
// middleware, real SQL Server) for the Product Categories table's sort,
// pagination, and RBAC behavior.
//
// Lives outside internal/productcategory, not alongside its other tests,
// because internal/platform/testrouter (the real Register(...) wiring
// these tests need) itself imports internal/productcategory to wire its
// handler — a test file inside that package importing testrouter back
// would be an import cycle.
package productcategoryapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/auth"
	"wms-backend/internal/platform/testrouter"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/productcategory"
	"wms-backend/internal/role"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

const rbacTestJWTSecret = "productcategory-rbac-sort-test-secret"

// rbacFixture is a tenant + one real tenant_users membership, provisioned
// directly on tx (not through POST /auth/login) — login requires a real
// password round trip, and (as of this writing) backend/test/isolation's
// own fixtures can't satisfy RequireTenantContext's membership re-check
// at all (see that package's own README), so this file issues its own
// JWT via auth.IssueToken directly instead. RequireTenantContext still
// re-verifies the membership seeded here against the database on every
// request below — only the login HTTP round trip itself is skipped, not
// the actual security check under test.
type rbacFixture struct {
	tenantID int64
	userID   int64
	roleID   int64
}

func seedRBACFixture(t *testing.T, tx *gorm.DB, label string) rbacFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("cat-rbac-%s", label), fmt.Sprintf("Category RBAC Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): insert tenant: %v", label, err)
	}

	r := role.Role{TenantID: tenantID, Code: "CAT_TESTER", Name: "Category Tester", IsActive: true}
	if err := tx.Create(&r).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): create role: %v", label, err)
	}

	u := user.User{
		Username:     fmt.Sprintf("cat_rbac_%s", label),
		PasswordHash: "unused-this-test-issues-its-own-jwt",
		FullName:     "Category RBAC Test User",
		Email:        fmt.Sprintf("cat.rbac.%s@isolation.test", label),
		RoleID:       r.ID,
		Status:       user.StatusActive,
	}
	if err := tx.Create(&u).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): create user: %v", label, err)
	}

	tu := tenant.TenantUser{TenantID: tenantID, UserID: u.ID, RoleID: r.ID, Status: tenant.TenantUserStatusActive}
	if err := tx.Create(&tu).Error; err != nil {
		t.Fatalf("seedRBACFixture(%s): create tenant_users membership: %v", label, err)
	}

	return rbacFixture{tenantID: tenantID, userID: u.ID, roleID: r.ID}
}

func seedCategory(t *testing.T, tx *gorm.DB, tenantID int64, code, name string, status productcategory.Status) {
	t.Helper()
	c := productcategory.Category{TenantID: tenantID, Code: code, Name: name, Status: status}
	if err := tx.Create(&c).Error; err != nil {
		t.Fatalf("seedCategory(%s): %v", code, err)
	}
}

// token issues a JWT for fx scoped to exactly permissions — mirrors what
// auth.Service.Login would embed in the claims, without going through
// the login HTTP round trip itself (see rbacFixture's own doc).
func (fx rbacFixture) token(t *testing.T, permissions []string) string {
	t.Helper()
	tok, _, err := auth.IssueToken(rbacTestJWTSecret, time.Hour, fx.userID, "cat_rbac_user", fx.tenantID, "CAT_TESTER", permissions)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	return tok
}

type listEnvelope struct {
	Success bool                       `json:"success"`
	Data    []productcategory.Response `json:"data"`
	Meta    listMeta                   `json:"meta"`
	Message string                     `json:"message"`
	Error   string                     `json:"error"`
}

// listMeta mirrors apihttp.ListMeta's JSON shape locally so this test
// file doesn't need to import internal/platform/http just for one
// struct — the field names/tags are the actual response contract, so a
// drift there would still be caught by json.Unmarshal silently leaving
// these at zero value and the assertions below failing.
type listMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func doListRequest(t *testing.T, engine http.Handler, token, query string) (int, listEnvelope) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/product-categories"+query, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var env listEnvelope
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, env
}

func TestList_RequiresProductCategoryViewPermission(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "no-perm")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)

	status, _ := doListRequest(t, deps.Engine, fx.token(t, []string{}), "")
	if status != http.StatusForbidden {
		t.Errorf("GET /product-categories with no permissions = %d, want 403", status)
	}
}

func TestList_WithPermission_DefaultsToCodeAscending_Limit25(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "default-list")

	seedCategory(t, tx, fx.tenantID, "CAT-C", "Charlie", productcategory.StatusActive)
	seedCategory(t, tx, fx.tenantID, "CAT-A", "Alpha", productcategory.StatusActive)
	seedCategory(t, tx, fx.tenantID, "CAT-B", "Bravo", productcategory.StatusActive)

	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	status, env := doListRequest(t, deps.Engine, fx.token(t, []string{"product_category.view"}), "")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %+v)", status, env)
	}
	if env.Meta.Limit != 25 {
		t.Errorf("meta.limit = %d, want default 25", env.Meta.Limit)
	}
	if env.Meta.Total != 3 || env.Meta.TotalPages != 1 {
		t.Errorf("meta = %+v, want total=3 total_pages=1", env.Meta)
	}
	if len(env.Data) != 3 || env.Data[0].Code != "CAT-A" || env.Data[1].Code != "CAT-B" || env.Data[2].Code != "CAT-C" {
		t.Errorf("data codes in order = %v, want [CAT-A CAT-B CAT-C] (default code ASC)", codesOf(env.Data))
	}
}

func TestList_SortQueryParams_AppliedEndToEnd(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "sort-query")

	seedCategory(t, tx, fx.tenantID, "CAT-1", "Charlie", productcategory.StatusActive)
	seedCategory(t, tx, fx.tenantID, "CAT-2", "Alpha", productcategory.StatusActive)
	seedCategory(t, tx, fx.tenantID, "CAT-3", "Bravo", productcategory.StatusActive)

	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	token := fx.token(t, []string{"product_category.view"})

	status, env := doListRequest(t, deps.Engine, token, "?sort_by=name&sort_dir=desc")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if names := namesOf(env.Data); names[0] != "Charlie" || names[1] != "Bravo" || names[2] != "Alpha" {
		t.Errorf("sort_by=name&sort_dir=desc order = %v, want [Charlie Bravo Alpha]", names)
	}
}

func TestList_InvalidSortBy_Returns400(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "invalid-sort")
	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)

	status, env := doListRequest(t, deps.Engine, fx.token(t, []string{"product_category.view"}), "?sort_by=description")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body=%+v", status, env)
	}
	if env.Error != "INVALID_SORT" {
		t.Errorf("error code = %q, want INVALID_SORT", env.Error)
	}
}

func TestList_RowsPerPage_AllFourOptionsAccepted(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "page-size")

	for i := 0; i < 12; i++ {
		seedCategory(t, tx, fx.tenantID, fmt.Sprintf("CAT-%02d", i), fmt.Sprintf("Category %02d", i), productcategory.StatusActive)
	}

	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	token := fx.token(t, []string{"product_category.view"})

	for _, limit := range []int{10, 25, 50, 100} {
		status, env := doListRequest(t, deps.Engine, token, fmt.Sprintf("?limit=%d", limit))
		if status != http.StatusOK {
			t.Fatalf("limit=%d: status = %d, want 200", limit, status)
		}
		if env.Meta.Limit != limit {
			t.Errorf("limit=%d: meta.limit = %d", limit, env.Meta.Limit)
		}
		wantRows := limit
		if wantRows > 12 {
			wantRows = 12
		}
		if len(env.Data) != wantRows {
			t.Errorf("limit=%d: got %d rows, want %d", limit, len(env.Data), wantRows)
		}
	}
}

func TestList_SearchQueryParam_StillWorks(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	fx := seedRBACFixture(t, tx, "search")

	seedCategory(t, tx, fx.tenantID, "PKG-BOX", "Boxes", productcategory.StatusActive)
	seedCategory(t, tx, fx.tenantID, "RAW-STEEL", "Steel", productcategory.StatusActive)

	deps := testrouter.BuildTestRouter(tx, rbacTestJWTSecret)
	status, env := doListRequest(t, deps.Engine, fx.token(t, []string{"product_category.view"}), "?search=PKG")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if env.Meta.Total != 1 || len(env.Data) != 1 || env.Data[0].Code != "PKG-BOX" {
		t.Errorf("search=PKG result = %+v, want exactly [PKG-BOX]", env.Data)
	}
}

func codesOf(rs []productcategory.Response) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Code
	}
	return out
}

func namesOf(rs []productcategory.Response) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

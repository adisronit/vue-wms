package isolation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"wms-backend/internal/customer"
	"wms-backend/internal/product"
	"wms-backend/internal/productcategory"
	"wms-backend/internal/supplier"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// Master Data Expansion (Product Category / Customer / Supplier) tenant
// isolation suite.
//
// Unlike this package's pre-existing fixtures.go seedTenant (which never
// sets a real TenantID — it fakes isolation via disjoint random-suffixed
// codes under the implicit default tenant, since no tenant_id column
// existed when that fixture was written), realTenantFixture below
// provisions an actual, distinct tenant row via tenant.Repository.
// CreateWithAudit (which also seeds that tenant its own full role
// catalog, including ADMIN) plus a real tenant_users membership, then
// logs in through the real /api/v1/auth/login endpoint — so
// active_tenant_id in the issued JWT is the real, distinct tenant id,
// and these tests exercise the actual RequireJWT -> RequireTenantContext
// -> RequirePermission chain product_category/customer/supplier use,
// unlike every other module this package's existing tests cover (which
// predate that chain existing at all).
//
// Expected to PASS, not documented as an expected failure like most of
// this package's pre-existing tests — product_category/customer/
// supplier are the first modules built with real tenant isolation from
// day one (see internal/productcategory/repository.go's doc).

type realTenantFixture struct {
	TenantID int64
	Token    string
}

// seedRealTenant provisions one real, isolated tenant: a tenants row
// (via CreateWithAudit, which also seeds this tenant's own role
// catalog), a global users row, and a tenant_users membership binding
// that user to this tenant's own ADMIN role — then logs in through the
// real HTTP login endpoint so h.Token is a production-issued JWT whose
// active_tenant_id claim is genuinely this tenant's id.
func (h *harness) seedRealTenant(label string) *realTenantFixture {
	h.t.Helper()
	suffix := uniqueSuffix()
	ctx := context.Background()

	t := &tenant.Tenant{
		Slug:   fmt.Sprintf("mde-%s-%s", label, suffix),
		Name:   fmt.Sprintf("MDE Tenant %s", label),
		Status: tenant.TenantStatusActive,
	}
	if err := h.deps.TenantRepo.CreateWithAudit(ctx, t, func(tenantID int64) *tenant.TenantAuditLog {
		return &tenant.TenantAuditLog{
			TenantID: &tenantID, ActorUserID: 0, ActorType: tenant.AuditActorSystem,
			Action: "isolation_test.create_tenant",
		}
	}); err != nil {
		h.t.Fatalf("seedRealTenant(%s): create tenant: %v", label, err)
	}

	adminRole, err := h.deps.TenantRepo.FindTenantRoleByCode(ctx, t.ID, "ADMIN")
	if err != nil {
		h.t.Fatalf("seedRealTenant(%s): find tenant's own ADMIN role: %v", label, err)
	}

	password := "Fixture-Pass-" + suffix + "!"
	newUser := &user.User{
		Username:     fmt.Sprintf("mde_%s_%s", label, suffix),
		PasswordHash: hashPassword(h.t, password),
		FullName:     fmt.Sprintf("MDE Tenant %s Admin", label),
		Email:        fmt.Sprintf("mde.%s.%s@isolation.test", label, suffix),
		RoleID:       adminRole.ID,
		Status:       user.StatusActive,
	}
	now := time.Now().UTC()
	membership := &tenant.TenantUser{
		RoleID:   adminRole.ID,
		Status:   tenant.TenantUserStatusActive,
		JoinedAt: &now,
	}
	if err := h.deps.TenantRepo.CreateMembershipWithNewUser(ctx, t.ID, newUser, membership, func(membershipID int64) *tenant.TenantAuditLog {
		return &tenant.TenantAuditLog{
			TenantID: &t.ID, ActorUserID: 0, ActorType: tenant.AuditActorSystem,
			Action: "isolation_test.create_membership",
		}
	}); err != nil {
		h.t.Fatalf("seedRealTenant(%s): create membership: %v", label, err)
	}

	token := h.login(newUser.Username, password)

	return &realTenantFixture{TenantID: t.ID, Token: token}
}

// seedCategory/seedCustomer/seedSupplier insert one row directly via
// GORM for tenantID — the same "arrange via direct DB access" idiom
// fixtures_test.go's own seedTenant already uses — bypassing the HTTP
// layer purely for setup so each test's "act" step is the one HTTP call
// actually under test.
func (h *harness) seedCategory(tenantID int64, code string) productcategory.Category {
	h.t.Helper()
	c := productcategory.Category{TenantID: tenantID, Code: code, Name: "Fixture Category " + code, Status: productcategory.StatusActive}
	if err := h.db.Create(&c).Error; err != nil {
		h.t.Fatalf("seedCategory: %v", err)
	}
	return c
}

func (h *harness) seedCustomer(tenantID int64, code string) customer.Customer {
	h.t.Helper()
	c := customer.Customer{TenantID: tenantID, Code: code, CompanyName: "Fixture Customer " + code, Status: customer.StatusActive}
	if err := h.db.Create(&c).Error; err != nil {
		h.t.Fatalf("seedCustomer: %v", err)
	}
	return c
}

func (h *harness) seedSupplier(tenantID int64, code string) supplier.Supplier {
	h.t.Helper()
	s := supplier.Supplier{TenantID: tenantID, Code: code, CompanyName: "Fixture Supplier " + code, Status: supplier.StatusActive}
	if err := h.db.Create(&s).Error; err != nil {
		h.t.Fatalf("seedSupplier: %v", err)
	}
	return s
}

// masterDataExpansionCase table-drives the three near-identical
// resources (category/customer/supplier) across every cross-tenant
// assertion below, the same shape idor_master_data_test.go's own
// masterDataCase uses.
type masterDataExpansionCase struct {
	name        string
	pathPrefix  string
	permPrefix  string
	seed        func(h *harness, tenantID int64, code string) int64
	updateBody  func(code string) any
	markerField string
	markerValue string
}

func masterDataExpansionCases() []masterDataExpansionCase {
	return []masterDataExpansionCase{
		{
			name: "product_category", pathPrefix: "/api/v1/product-categories", permPrefix: "product_category",
			seed: func(h *harness, tenantID int64, code string) int64 { return h.seedCategory(tenantID, code).ID },
			updateBody: func(code string) any {
				return productcategory.UpdateRequest{Code: code, Name: "HIJACKED", Status: productcategory.StatusActive}
			},
			markerField: "name", markerValue: "HIJACKED",
		},
		{
			name: "customer", pathPrefix: "/api/v1/customers", permPrefix: "customer",
			seed: func(h *harness, tenantID int64, code string) int64 { return h.seedCustomer(tenantID, code).ID },
			updateBody: func(code string) any {
				return customer.UpdateRequest{Code: code, CompanyName: "HIJACKED", Status: customer.StatusActive}
			},
			markerField: "company_name", markerValue: "HIJACKED",
		},
		{
			name: "supplier", pathPrefix: "/api/v1/suppliers", permPrefix: "supplier",
			seed: func(h *harness, tenantID int64, code string) int64 { return h.seedSupplier(tenantID, code).ID },
			updateBody: func(code string) any {
				return supplier.UpdateRequest{Code: code, CompanyName: "HIJACKED", Status: supplier.StatusActive}
			},
			markerField: "company_name", markerValue: "HIJACKED",
		},
	}
}

func TestMasterDataExpansion_TenantA_CannotReadTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")

	for _, tc := range masterDataExpansionCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bID := tc.seed(h, b.TenantID, "MDE-"+tc.name+"-"+uniqueSuffix())
			path := fmt.Sprintf("%s/%d", tc.pathPrefix, bID)

			status, resp := h.doRequest(http.MethodGet, path, a.Token, nil)
			if status != http.StatusNotFound {
				t.Errorf("Tenant A read Tenant B's %s (id=%d) via %s — got status %d (want 404); body=%s",
					tc.name, bID, path, status, resp.Data)
			}
		})
	}
}

func TestMasterDataExpansion_TenantA_CannotUpdateTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")

	for _, tc := range masterDataExpansionCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			code := "MDE-" + tc.name + "-" + uniqueSuffix()
			bID := tc.seed(h, b.TenantID, code)
			path := fmt.Sprintf("%s/%d", tc.pathPrefix, bID)

			status, _ := h.doRequest(http.MethodPut, path, a.Token, tc.updateBody(code))

			// Re-read B's row as B to see what state it's actually in now.
			getStatus, getResp := h.doRequest(http.MethodGet, path, b.Token, nil)
			if getStatus != http.StatusOK {
				t.Fatalf("re-reading tenant B's own %s after cross-tenant update attempt failed: status=%d", tc.name, getStatus)
			}
			var after map[string]any
			_ = json.Unmarshal(getResp.Data, &after)
			actualMarker, _ := after[tc.markerField].(string)

			if status == http.StatusOK || actualMarker == tc.markerValue {
				t.Errorf("Tenant A updated Tenant B's %s (id=%d) — PUT status=%d, field %q now reads %q",
					tc.name, bID, status, tc.markerField, actualMarker)
			}
		})
	}
}

func TestMasterDataExpansion_TenantA_CannotDeleteTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")

	for _, tc := range masterDataExpansionCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bID := tc.seed(h, b.TenantID, "MDE-"+tc.name+"-"+uniqueSuffix())
			path := fmt.Sprintf("%s/%d", tc.pathPrefix, bID)

			status, _ := h.doRequest(http.MethodDelete, path, a.Token, nil)

			getStatus, _ := h.doRequest(http.MethodGet, path, b.Token, nil)

			if status == http.StatusNoContent || getStatus == http.StatusNotFound {
				t.Errorf("Tenant A deleted Tenant B's %s (id=%d) — DELETE status=%d, row now reads back as status=%d",
					tc.name, bID, status, getStatus)
			}
		})
	}
}

func TestMasterDataExpansion_List_DoesNotLeakOtherTenantRows(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")

	for _, tc := range masterDataExpansionCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ownID := tc.seed(h, a.TenantID, "MDE-OWN-"+tc.name+"-"+uniqueSuffix())
			otherID := tc.seed(h, b.TenantID, "MDE-OTHER-"+tc.name+"-"+uniqueSuffix())

			status, resp := h.doRequest(http.MethodGet, tc.pathPrefix+"?limit=100", a.Token, nil)
			if status != http.StatusOK {
				t.Fatalf("unexpected status listing %s: %d", tc.name, status)
			}

			var rows []map[string]any
			if err := json.Unmarshal(resp.Data, &rows); err != nil {
				t.Fatalf("decode %s list: %v", tc.name, err)
			}

			var sawOwn, sawOther bool
			for _, row := range rows {
				idStr, _ := row["id"].(string)
				if idStr == itoa(ownID) {
					sawOwn = true
				}
				if idStr == itoa(otherID) {
					sawOther = true
				}
			}
			if !sawOwn {
				t.Errorf("Tenant A's own %s list did not include its own row (id=%d)", tc.name, ownID)
			}
			if sawOther {
				t.Errorf("Tenant A's %s list leaked Tenant B's row (id=%d)", tc.name, otherID)
			}
		})
	}
}

// TestMasterDataExpansion_RBAC_RequiresPermission proves a tenant
// member whose role has none of the product_category/customer/supplier
// permissions (every role this suite seeds besides ADMIN) is rejected
// with 403, not merely relying on the UI to hide the button.
// seedNoAccessMember creates a fresh custom role within tenantID,
// granted zero permissions, plus a user+membership using it, and
// returns a logged-in bearer token for that user — the standard
// "a tenant member whose role grants nothing" fixture for RBAC-enforces
// -it-server-side tests (not merely "the UI hides the button") across
// any RequireTenantContext-gated module's endpoints. role.Role requires
// a tenant-scoped unique code (migrations/0048), so the generated code
// (prefixed with label) cannot collide with another test or call.
func (h *harness) seedNoAccessMember(tenantID int64, label string) string {
	h.t.Helper()
	suffix := uniqueSuffix()

	noAccessRole := struct{ ID int64 }{}
	if err := h.db.Raw(
		"INSERT INTO roles (tenant_id, code, name, description, is_active) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 1)",
		tenantID, label+"_NO_ACCESS_"+suffix, "No Access", "Isolation-suite fixture role with zero grants",
	).Scan(&noAccessRole.ID).Error; err != nil {
		h.t.Fatalf("seedNoAccessMember(%s): create no-access role: %v", label, err)
	}

	password := "Fixture-Pass-" + suffix + "!"
	noAccessUser := &user.User{
		Username: "noaccess_" + label + "_" + suffix, PasswordHash: hashPassword(h.t, password),
		FullName: "No Access User " + label, Email: "noaccess." + label + "." + suffix + "@isolation.test",
		RoleID: noAccessRole.ID, Status: user.StatusActive,
	}
	now := time.Now().UTC()
	membership := &tenant.TenantUser{RoleID: noAccessRole.ID, Status: tenant.TenantUserStatusActive, JoinedAt: &now}
	if err := h.deps.TenantRepo.CreateMembershipWithNewUser(context.Background(), tenantID, noAccessUser, membership, func(membershipID int64) *tenant.TenantAuditLog {
		return &tenant.TenantAuditLog{TenantID: &tenantID, ActorUserID: 0, ActorType: tenant.AuditActorSystem, Action: "isolation_test.create_no_access_membership"}
	}); err != nil {
		h.t.Fatalf("seedNoAccessMember(%s): create no-access membership: %v", label, err)
	}

	return h.login(noAccessUser.Username, password)
}

func TestMasterDataExpansion_RBAC_RequiresPermission(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	noAccessToken := h.seedNoAccessMember(a.TenantID, "MDE")

	for _, tc := range masterDataExpansionCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			status, _ := h.doRequest(http.MethodGet, tc.pathPrefix, noAccessToken, nil)
			if status != http.StatusForbidden {
				t.Errorf("a role with no %s.view grant got status %d listing %s, want 403", tc.permPrefix, status, tc.pathPrefix)
			}
		})
	}
}

// TestProductCategory_RejectsCircularHierarchy proves Service.
// validateParent/detectCycle actually blocks a parent change that
// would create a cycle — not just a self-parent, but a two-level
// grandparent cycle (A -> B -> A).
func TestProductCategory_RejectsCircularHierarchy(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")

	parent := h.seedCategory(a.TenantID, "MDE-CYCLE-PARENT-"+uniqueSuffix())
	child := h.seedCategory(a.TenantID, "MDE-CYCLE-CHILD-"+uniqueSuffix())

	// child.parent_id = parent.id
	status, _ := h.doRequest(http.MethodPut, fmt.Sprintf("/api/v1/product-categories/%d", child.ID), a.Token,
		productcategory.UpdateRequest{Code: child.Code, Name: child.Name, ParentID: &parent.ID, Status: productcategory.StatusActive})
	if status != http.StatusOK {
		t.Fatalf("setting child's parent failed: status=%d", status)
	}

	// Now attempt parent.parent_id = child.id — a two-node cycle.
	status, resp := h.doRequest(http.MethodPut, fmt.Sprintf("/api/v1/product-categories/%d", parent.ID), a.Token,
		productcategory.UpdateRequest{Code: parent.Code, Name: parent.Name, ParentID: &child.ID, Status: productcategory.StatusActive})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("creating a circular category hierarchy got status %d, want 422; body=%s", status, resp.Data)
	}

	// Also the trivial self-parent case.
	status, resp = h.doRequest(http.MethodPut, fmt.Sprintf("/api/v1/product-categories/%d", parent.ID), a.Token,
		productcategory.UpdateRequest{Code: parent.Code, Name: parent.Name, ParentID: &parent.ID, Status: productcategory.StatusActive})
	if status != http.StatusUnprocessableEntity {
		t.Errorf("setting a category as its own parent got status %d, want 422; body=%s", status, resp.Data)
	}
}

// TestProductCategory_RejectsDuplicateCodeWithinTenant proves the
// UQ_product_categories_tenant_code constraint surfaces as a 409
// through the HTTP layer, not just at the raw DB level (already
// covered by model_tenant_integration_test.go).
func TestProductCategory_RejectsDuplicateCodeWithinTenant(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	code := "MDE-DUP-" + uniqueSuffix()

	status, _ := h.doRequest(http.MethodPost, "/api/v1/product-categories", a.Token,
		productcategory.CreateRequest{Code: code, Name: "First", Status: productcategory.StatusActive})
	if status != http.StatusCreated {
		t.Fatalf("creating first category failed: status=%d", status)
	}

	status, resp := h.doRequest(http.MethodPost, "/api/v1/product-categories", a.Token,
		productcategory.CreateRequest{Code: code, Name: "Duplicate", Status: productcategory.StatusActive})
	if status != http.StatusConflict {
		t.Errorf("creating a duplicate-code category got status %d, want 409; body=%s", status, resp.Data)
	}
}

// TestProductCategory_DeleteBlockedByChildCategory and
// TestProductCategory_DeleteBlockedByReferencingProduct prove
// Service.Delete's two referential-safety checks (ErrHasChildren,
// ErrHasProducts) both surface as 409 through the HTTP layer.
func TestProductCategory_DeleteBlockedByChildCategory(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")

	parent := h.seedCategory(a.TenantID, "MDE-HASCHILD-"+uniqueSuffix())
	child := h.seedCategory(a.TenantID, "MDE-ISCHILD-"+uniqueSuffix())
	if err := h.db.Model(&child).Update("parent_id", parent.ID).Error; err != nil {
		t.Fatalf("link child to parent: %v", err)
	}

	status, resp := h.doRequest(http.MethodDelete, fmt.Sprintf("/api/v1/product-categories/%d", parent.ID), a.Token, nil)
	if status != http.StatusConflict {
		t.Errorf("deleting a category with a child category got status %d, want 409; body=%s", status, resp.Data)
	}
}

func TestProductCategory_DeleteBlockedByReferencingProduct(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")

	cat := h.seedCategory(a.TenantID, "MDE-HASPROD-"+uniqueSuffix())
	p := product.Product{
		TenantID: a.TenantID, SKU: "MDE-PROD-" + uniqueSuffix(), Name: "Linked Product",
		Category: product.CategoryPackaging, BaseUOM: product.UOMPieces, Status: product.StatusActive,
		CategoryID: &cat.ID,
	}
	if err := h.db.Create(&p).Error; err != nil {
		t.Fatalf("create linked product: %v", err)
	}

	status, resp := h.doRequest(http.MethodDelete, fmt.Sprintf("/api/v1/product-categories/%d", cat.ID), a.Token, nil)
	if status != http.StatusConflict {
		t.Errorf("deleting a category still referenced by a product got status %d, want 409; body=%s", status, resp.Data)
	}
}

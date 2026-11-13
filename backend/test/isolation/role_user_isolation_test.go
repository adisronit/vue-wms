package isolation

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"wms-backend/internal/user"
)

// seedUserWithRole creates a bare user (no warehouse/zone/location/
// product of its own) against roleCode — used by the negative
// authorization tests below, which are about permission enforcement,
// not tenant ownership, so they don't need a full tenantFixture.
func (h *harness) seedUserWithRole(label, roleCode string) (username, password string) {
	h.t.Helper()
	suffix := uniqueSuffix()

	r, err := h.deps.RoleRepo.FindRoleByCode(context.Background(), roleCode)
	if err != nil {
		h.t.Fatalf("seedUserWithRole(%s): find role %s: %v", label, roleCode, err)
	}

	password = "Fixture-Pass-" + suffix + "!"
	username = fmt.Sprintf("plainuser_%s_%s", label, suffix)
	u := user.User{
		Username:     username,
		PasswordHash: hashPassword(h.t, password),
		FullName:     "Plain User " + label,
		Email:        fmt.Sprintf("plain.%s.%s@isolation.test", label, suffix),
		RoleID:       r.ID,
		Status:       user.StatusActive,
	}
	if err := h.db.Create(&u).Error; err != nil {
		h.t.Fatalf("seedUserWithRole(%s): create user: %v", label, err)
	}
	return username, password
}

// TestTenantA_CannotReadOrMutateTenantB_CustomRole proves the "existence
// checker, not ownership checker" pattern (phase27-audit-report.md §3c)
// reaches role.Repository.FindRoleByID too: Tenant A's ADMIN token can
// read and overwrite Tenant B's own custom role's permission grants,
// because nothing in role.Service checks which tenant a role "belongs
// to" (there is no such column yet). Expected to FAIL until roles gain
// tenant_id (tenant-er-design.md §4's unresolved roles-ownership
// question, resolved in Phase 29's implementation).
func TestTenantA_CannotReadOrMutateTenantB_CustomRole(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	permPath := "/api/v1/roles/" + itoa(tenantB.Role.ID) + "/permissions"

	t.Run("read", func(t *testing.T) {
		status, resp := h.doRequest(http.MethodGet, permPath, tenantA.Token, nil)
		if status != http.StatusNotFound {
			t.Errorf("CROSS-TENANT ROLE READ IDOR CONFIRMED: Tenant A read Tenant B's custom role (id=%d) permissions — status=%d body=%s",
				tenantB.Role.ID, status, resp.Data)
		}
	})

	t.Run("mutate", func(t *testing.T) {
		status, _ := h.doRequest(http.MethodPut, permPath, tenantA.Token, map[string]any{
			"permission_codes": []string{"warehouse.view", "warehouse.update", "warehouse.delete"},
		})

		getStatus, getResp := h.doRequest(http.MethodGet, permPath, tenantB.Token, nil)
		if getStatus != http.StatusOK {
			t.Fatalf("re-reading tenant B's own role permissions failed: status=%d", getStatus)
		}
		var after struct {
			PermissionCodes []string `json:"permission_codes"`
		}
		if err := unmarshalData(getResp.Data, &after); err != nil {
			t.Fatalf("decode role permissions: %v", err)
		}

		if status == http.StatusOK || len(after.PermissionCodes) > 0 {
			t.Errorf("CROSS-TENANT ROLE WRITE IDOR CONFIRMED: Tenant A replaced Tenant B's custom role (id=%d) permission set — PUT status=%d, role now grants %v",
				tenantB.Role.ID, status, after.PermissionCodes)
		}
	})
}

// TestMissingPermission_IsRejected is a deliberate contrast case: unlike
// every tenant-isolation test in this suite, RBAC permission
// enforcement (middleware.RequirePermission) is fully implemented today
// and this test is expected to PASS. It establishes that the suite's
// other failures are specifically about missing tenant ownership checks,
// not a blanket "authorization is broken" — a RECEIVING-role user still
// correctly cannot reach warehouse.create.
func TestMissingPermission_IsRejected(t *testing.T) {
	h := newHarness(t)
	username, password := h.seedUserWithRole("limited", "RECEIVING")
	token := h.login(username, password)

	status, resp := h.doRequest(http.MethodPost, "/api/v1/warehouses", token, map[string]any{
		"code": "SHOULD-NOT-EXIST-" + uniqueSuffix(),
		"name": "Should not be created",
		"type": "Distribution Center",
	})

	if status != http.StatusForbidden {
		t.Errorf("expected 403 FORBIDDEN for a RECEIVING-role user calling warehouse.create, got status=%d body=%s", status, resp.Data)
	}
}

// TestMissingOrInvalidJWT_IsRejected is this suite's narrowed version of
// TestMissingTenantContext_FailsClosed: there is no active_tenant_id
// claim to strip yet (the JWT carries no tenant concept at all — see
// tenant-isolation-security.md §2's "before" Claims shape), so the
// closest thing testable today is the existing JWT-presence/validity
// gate. Expected to PASS — confirms RequireJWT's fail-closed behavior is
// a solid foundation for RequireTenantContext to be inserted next to in
// Phase 29, not something Phase 29 also needs to fix.
func TestMissingOrInvalidJWT_IsRejected(t *testing.T) {
	h := newHarness(t)

	protectedRoutes := []string{
		"/api/v1/warehouses",
		"/api/v1/zones?warehouse_id=1",
		"/api/v1/locations?zone_id=1",
		"/api/v1/products",
		"/api/v1/users",
		"/api/v1/roles",
	}

	for _, path := range protectedRoutes {
		t.Run("no_token_"+path, func(t *testing.T) {
			status, _ := h.doRequest(http.MethodGet, path, "", nil)
			if status != http.StatusUnauthorized {
				t.Errorf("expected 401 for %s with no Authorization header, got %d", path, status)
			}
		})
		t.Run("garbage_token_"+path, func(t *testing.T) {
			status, _ := h.doRequest(http.MethodGet, path, "not-a-real-jwt", nil)
			if status != http.StatusUnauthorized {
				t.Errorf("expected 401 for %s with a malformed bearer token, got %d", path, status)
			}
		})
	}
}

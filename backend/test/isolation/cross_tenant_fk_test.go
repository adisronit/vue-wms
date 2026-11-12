package isolation

import (
	"net/http"
	"testing"

	"wms-backend/internal/location"
	"wms-backend/internal/user"
	"wms-backend/internal/zone"
)

// TestCrossTenantForeignKey_RejectedOnCreate is
// saas-module-roadmap.md's TestTenantA_CannotAssignTenantB_Warehouse /
// TestCrossTenantForeignKey_RejectedOnCreate combined: every "existence
// checker, not ownership checker" the Phase 27 audit found (§3c) answers
// only "does this ID exist at all," so a zone/location create that
// references another tenant's parent row succeeds today. Expected to
// FAIL until the checkers become ownership checks (tenant-isolation-
// security.md §6).
func TestCrossTenantForeignKey_RejectedOnCreate(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	t.Run("zone_references_other_tenant_warehouse", func(t *testing.T) {
		status, resp := h.doRequest(http.MethodPost, "/api/v1/zones", tenantA.Token, zone.CreateRequest{
			WarehouseID: tenantB.Warehouse.ID,
			Code:        "ATTACK-Z-" + uniqueSuffix(),
			Name:        "Attack zone",
			Type:        zone.TypeStorage,
			Status:      zone.StatusActive,
		})

		if status == http.StatusCreated {
			t.Errorf("CROSS-TENANT FK CONFIRMED: Tenant A created a zone under Tenant B's warehouse_id=%d — status=%d body=%s",
				tenantB.Warehouse.ID, status, resp.Data)
		} else if status != http.StatusNotFound && status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Fatalf("unexpected status creating cross-tenant zone: %d body=%s", status, resp.Data)
		}
	})

	t.Run("location_references_other_tenant_zone", func(t *testing.T) {
		status, resp := h.doRequest(http.MethodPost, "/api/v1/locations", tenantA.Token, location.CreateRequest{
			ZoneID: tenantB.Zone.ID,
			Code:   "ATTACK-L-" + uniqueSuffix(),
			Aisle:  "01",
			Rack:   "01",
			Bin:    "01",
			Status: location.StatusActive,
		})

		if status == http.StatusCreated {
			t.Errorf("CROSS-TENANT FK CONFIRMED: Tenant A created a location under Tenant B's zone_id=%d — status=%d body=%s",
				tenantB.Zone.ID, status, resp.Data)
		} else if status != http.StatusNotFound && status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Fatalf("unexpected status creating cross-tenant location: %d body=%s", status, resp.Data)
		}
	})

	t.Run("user_assigned_to_other_tenant_warehouse", func(t *testing.T) {
		status, resp := h.doRequest(http.MethodPost, "/api/v1/users", tenantA.Token, user.CreateRequest{
			Username:    "attack_user_" + uniqueSuffix(),
			Password:    "Attack-Pass-1!",
			FullName:    "Attack User",
			Email:       "attack." + uniqueSuffix() + "@isolation.test",
			RoleCode:    "ADMIN",
			WarehouseID: &tenantB.Warehouse.ID,
			Status:      user.StatusActive,
		})

		if status == http.StatusCreated {
			t.Errorf("CROSS-TENANT FK CONFIRMED: Tenant A created a user assigned to Tenant B's warehouse_id=%d — status=%d body=%s",
				tenantB.Warehouse.ID, status, resp.Data)
		} else if status != http.StatusNotFound && status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Fatalf("unexpected status creating cross-tenant-assigned user: %d body=%s", status, resp.Data)
		}
	})
}

// TestTenantA_CannotGrantTenantB_WarehouseToOwnRole proves
// role.Service.SetRoleWarehouseAccess's warehouse-existence check
// (phase27-audit-report.md §2.1, role row 46: "warehouses.FindByIDs
// also has no tenant scoping") lets Tenant A grant its own role access
// to Tenant B's warehouse — a privilege-escalation-flavored cross-
// tenant FK, distinct from the create-time cases above because the
// write targets A's own row (its own role), not a resource of B's.
func TestTenantA_CannotGrantTenantB_WarehouseToOwnRole(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	path := "/api/v1/roles/" + itoa(tenantA.Role.ID) + "/warehouse-access"
	status, resp := h.doRequest(http.MethodPut, path, tenantA.Token, map[string]any{
		"warehouse_ids": []int64{tenantA.Warehouse.ID, tenantB.Warehouse.ID},
	})

	if status != http.StatusOK {
		// Today's handler/service may still reject this for unrelated
		// reasons (e.g. ADMIN bypass logic) — only a 200 is the actual
		// finding; any non-200 here is reported, not asserted on, so the
		// test accurately reflects what it found either way.
		t.Logf("grant request did not succeed outright: status=%d body=%s", status, resp.Data)
		return
	}

	getStatus, getResp := h.doRequest(http.MethodGet, path, tenantA.Token, nil)
	if getStatus != http.StatusOK {
		t.Fatalf("re-reading role warehouse access failed: status=%d", getStatus)
	}

	var access struct {
		WarehouseAccess []struct {
			ID int64 `json:"id,string"`
		} `json:"warehouse_access"`
	}
	if err := unmarshalData(getResp.Data, &access); err != nil {
		t.Fatalf("decode warehouse access: %v", err)
	}

	for _, w := range access.WarehouseAccess {
		if w.ID == tenantB.Warehouse.ID {
			t.Errorf("CROSS-TENANT FK CONFIRMED: Tenant A's role (id=%d) was granted access to Tenant B's warehouse (id=%d)",
				tenantA.Role.ID, tenantB.Warehouse.ID)
		}
	}
}

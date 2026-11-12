package isolation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"wms-backend/internal/location"
	"wms-backend/internal/product"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// masterDataCase describes one tenant-owned master-data resource the
// Phase 27 audit flagged as a bare-ID-lookup IDOR candidate
// (phase27-audit-report.md §2.2) — warehouse/zone/location/product all
// follow the exact same "WHERE id = ?, no tenant filter" shape, so one
// table-driven test proves the finding across all four instead of
// hand-duplicating the same assertions four times.
type masterDataCase struct {
	name       string
	pathPrefix string
	idOf       func(f *tenantFixture) int64
	updateBody func(f *tenantFixture) any
	// markerField/markerValue identify which JSON field the update body
	// changes to a recognizable sentinel, and what that sentinel is —
	// resources don't share a common "name" field (location has none;
	// user calls it full_name), so the post-update check reads this
	// field generically rather than assuming a shape every DTO shares.
	markerField string
	markerValue string
	// leaf is true when deleting this resource type has no child-row
	// dependency that would otherwise make Delete return 409 regardless
	// of tenant — only leaf resources are safe to include in the delete
	// IDOR test below without a false negative.
	leaf bool
}

func masterDataCases() []masterDataCase {
	return []masterDataCase{
		{
			name:       "warehouse",
			pathPrefix: "/api/v1/warehouses",
			idOf:       func(f *tenantFixture) int64 { return f.Warehouse.ID },
			updateBody: func(f *tenantFixture) any {
				return warehouse.UpdateRequest{Code: f.Warehouse.Code, Name: "HIJACKED", Type: f.Warehouse.Type, Status: warehouse.StatusActive}
			},
			markerField: "name",
			markerValue: "HIJACKED",
			leaf:        false, // has a child zone
		},
		{
			name:       "zone",
			pathPrefix: "/api/v1/zones",
			idOf:       func(f *tenantFixture) int64 { return f.Zone.ID },
			updateBody: func(f *tenantFixture) any {
				return zone.UpdateRequest{Code: f.Zone.Code, Name: "HIJACKED", Type: zone.TypeStorage, Status: zone.StatusActive}
			},
			markerField: "name",
			markerValue: "HIJACKED",
			leaf:        false, // has a child location
		},
		{
			name:       "location",
			pathPrefix: "/api/v1/locations",
			idOf:       func(f *tenantFixture) int64 { return f.Location.ID },
			updateBody: func(f *tenantFixture) any {
				return location.UpdateRequest{Code: f.Location.Code, Aisle: "99", Rack: "99", Bin: "99", Status: location.StatusActive}
			},
			markerField: "aisle",
			markerValue: "99",
			leaf:        true,
		},
		{
			name:       "product",
			pathPrefix: "/api/v1/products",
			idOf:       func(f *tenantFixture) int64 { return f.Product.ID },
			updateBody: func(f *tenantFixture) any {
				return product.UpdateRequest{SKU: f.Product.SKU, Name: "HIJACKED", Category: product.CategoryPackaging, BaseUOM: product.UOMPieces, Status: product.StatusActive}
			},
			markerField: "name",
			markerValue: "HIJACKED",
			leaf:        true,
		},
		{
			name:       "user",
			pathPrefix: "/api/v1/users",
			idOf:       func(f *tenantFixture) int64 { return f.User.ID },
			updateBody: func(f *tenantFixture) any {
				return user.UpdateRequest{FullName: "HIJACKED", Email: f.User.Email, RoleCode: "ADMIN", Status: user.StatusActive}
			},
			markerField: "full_name",
			markerValue: "HIJACKED",
			leaf:        true,
		},
	}
}

// TestTenantA_CannotReadTenantB_ByID is saas-module-roadmap.md's
// TestTenantA_CannotReadTenantB_ByID, run across every master-data
// resource type. Today's handlers never check tenant ownership (there
// is no tenant_id to check), so this is expected to FAIL now — GET
// returns 200 with Tenant B's row — and must start returning 404 (never
// 403, so a guessed ID can't be distinguished from a nonexistent one;
// see tenant-isolation-security.md §6) once Phase 29 lands.
func TestTenantA_CannotReadTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	for _, tc := range masterDataCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bID := tc.idOf(tenantB)
			path := fmt.Sprintf("%s/%d", tc.pathPrefix, bID)

			status, resp := h.doRequest(http.MethodGet, path, tenantA.Token, nil)
			if status != http.StatusNotFound {
				t.Errorf("CROSS-TENANT READ IDOR CONFIRMED: Tenant A read Tenant B's %s (id=%d) via %s — got status %d (want 404); body=%s",
					tc.name, bID, path, status, resp.Data)
			}
		})
	}
}

// TestTenantA_CannotUpdateTenantB_ByID is
// TestTenantA_CannotUpdateTenantB_Inventory generalized to every
// master-data resource (the inventory-specific quantity case is covered
// separately once inventory enters scope in Phase 30 — see README.md).
// Proves both that the write is rejected AND that Tenant B's row is
// provably unchanged afterward, not merely that the HTTP call "looks"
// rejected.
func TestTenantA_CannotUpdateTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	for _, tc := range masterDataCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bID := tc.idOf(tenantB)
			path := fmt.Sprintf("%s/%d", tc.pathPrefix, bID)

			status, _ := h.doRequest(http.MethodPut, path, tenantA.Token, tc.updateBody(tenantB))

			// Re-read B's row as B to see what state it's actually in now
			// — the authoritative check, independent of what the PUT
			// call's own status code claimed.
			getStatus, getResp := h.doRequest(http.MethodGet, path, tenantB.Token, nil)
			if getStatus != http.StatusOK {
				t.Fatalf("re-reading tenant B's own %s after cross-tenant update attempt failed: status=%d", tc.name, getStatus)
			}
			var after map[string]any
			_ = json.Unmarshal(getResp.Data, &after)
			actualMarker, _ := after[tc.markerField].(string)

			if status == http.StatusOK || actualMarker == tc.markerValue {
				t.Errorf("CROSS-TENANT WRITE IDOR CONFIRMED: Tenant A updated Tenant B's %s (id=%d) — PUT status=%d, field %q now reads %q",
					tc.name, bID, status, tc.markerField, actualMarker)
			}
		})
	}
}

// TestTenantA_CannotDeleteTenantB_ByID covers only leaf resources
// (location, product, user) — warehouse/zone delete correctly return
// 409 whenever a child row exists regardless of tenant, which would
// mask whether the 409 came from the (missing) tenant check or the
// (existing) referential-safety check; see masterDataCase.leaf.
func TestTenantA_CannotDeleteTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	for _, tc := range masterDataCases() {
		if !tc.leaf {
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bID := tc.idOf(tenantB)
			path := fmt.Sprintf("%s/%d", tc.pathPrefix, bID)

			status, _ := h.doRequest(http.MethodDelete, path, tenantA.Token, nil)

			// Confirm via B's own token whether the row actually still
			// exists, independent of the delete call's reported status.
			getStatus, _ := h.doRequest(http.MethodGet, path, tenantB.Token, nil)

			if status == http.StatusNoContent || getStatus == http.StatusNotFound {
				t.Errorf("CROSS-TENANT DELETE IDOR CONFIRMED: Tenant A deleted Tenant B's %s (id=%d) — DELETE status=%d, row now reads back as status=%d",
					tc.name, bID, status, getStatus)
			}
		})
	}
}

// TestZoneList_CannotEnumerateViaWarehouseIDParam covers the
// query-param IDOR the audit found in zone/handler.go's List — the
// warehouse_id is taken directly from the query string with zero
// ownership check (phase27-audit-report.md §2.2, zone row 87).
func TestZoneList_CannotEnumerateViaWarehouseIDParam(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	path := fmt.Sprintf("/api/v1/zones?warehouse_id=%d", tenantB.Warehouse.ID)
	status, resp := h.doRequest(http.MethodGet, path, tenantA.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("unexpected status listing zones: %d", status)
	}

	var zones []zone.Response
	if err := json.Unmarshal(resp.Data, &zones); err != nil {
		t.Fatalf("decode zones: %v", err)
	}

	for _, z := range zones {
		if z.ID == tenantB.Zone.ID {
			t.Errorf("CROSS-TENANT LIST IDOR CONFIRMED: Tenant A listed Tenant B's warehouse_id=%d query param and received Tenant B's zone (id=%d, code=%q)",
				tenantB.Warehouse.ID, z.ID, z.Code)
		}
	}
}

// TestLocationList_CannotEnumerateViaZoneIDParam is the location-module
// equivalent of TestZoneList_CannotEnumerateViaWarehouseIDParam
// (phase27-audit-report.md §2.2, location row 92).
func TestLocationList_CannotEnumerateViaZoneIDParam(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	path := fmt.Sprintf("/api/v1/locations?zone_id=%d", tenantB.Zone.ID)
	status, resp := h.doRequest(http.MethodGet, path, tenantA.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("unexpected status listing locations: %d", status)
	}

	var locations []location.Response
	if err := json.Unmarshal(resp.Data, &locations); err != nil {
		t.Fatalf("decode locations: %v", err)
	}

	for _, l := range locations {
		if l.ID == tenantB.Location.ID {
			t.Errorf("CROSS-TENANT LIST IDOR CONFIRMED: Tenant A listed Tenant B's zone_id=%d query param and received Tenant B's location (id=%d, code=%q)",
				tenantB.Zone.ID, l.ID, l.Code)
		}
	}
}

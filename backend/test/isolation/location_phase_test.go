package isolation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/product"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// Location Master Management tenant isolation suite. Reuses
// realTenantFixture/seedRealTenant (masterdata_phase_test.go) — real,
// distinct tenants with real memberships — rather than this package's
// older seedTenant (fixtures_test.go), which never sets a real
// TenantID. Expected to PASS: internal/location is now tenant-scoped
// end to end (RequireJWT -> RequireTenantContext -> RequirePermission,
// every repository query filtered by tenant_id), unlike warehouse/zone
// themselves, which remain out of this phase's scope and are not
// asserted on here.

// seedWarehouseZone inserts one warehouse and one zone for tenantID
// directly via GORM — the same "arrange via direct DB access" idiom
// this package's other fixtures already use.
func (h *harness) seedWarehouseZone(tenantID int64, label string) (warehouseID, zoneID int64) {
	h.t.Helper()
	suffix := uniqueSuffix()

	wh := warehouse.Warehouse{
		TenantID: tenantID, Code: fmt.Sprintf("LMM-WH-%s-%s", label, suffix),
		Name: "LMM Warehouse " + label, Type: "Distribution Center", Status: warehouse.StatusActive,
	}
	if err := h.db.Create(&wh).Error; err != nil {
		h.t.Fatalf("seedWarehouseZone(%s): create warehouse: %v", label, err)
	}

	z := zone.Zone{
		TenantID: tenantID, WarehouseID: wh.ID, Code: fmt.Sprintf("LMM-Z-%s-%s", label, suffix),
		Name: "LMM Zone " + label, Type: zone.TypeStorage, Status: zone.StatusActive,
	}
	if err := h.db.Create(&z).Error; err != nil {
		h.t.Fatalf("seedWarehouseZone(%s): create zone: %v", label, err)
	}

	return wh.ID, z.ID
}

func (h *harness) seedLocationRow(tenantID, zoneID int64, code string) location.Location {
	h.t.Helper()
	l := location.Location{TenantID: tenantID, ZoneID: zoneID, Code: code, Status: location.StatusActive}
	if err := h.db.Create(&l).Error; err != nil {
		h.t.Fatalf("seedLocationRow: %v", err)
	}
	return l
}

func (h *harness) seedInventoryAt(tenantID, locationID int64, qty float64) {
	h.t.Helper()

	// FK_inventory_product requires a real products row — a throwaway
	// one, created fresh per call since this fixture only needs it to
	// satisfy the constraint, not to model any specific product.
	p := product.Product{
		TenantID: tenantID, SKU: "LMM-STOCK-SKU-" + uniqueSuffix(), Name: "LMM Stock Fixture Product",
		Category: product.CategoryPackaging, BaseUOM: product.UOMPieces, PackSize: 1, Status: product.StatusActive,
	}
	if err := h.db.Create(&p).Error; err != nil {
		h.t.Fatalf("seedInventoryAt: create fixture product: %v", err)
	}

	inv := inventory.Inventory{TenantID: tenantID, ProductID: p.ID, LocationID: locationID, Quantity: qty}
	if err := h.db.Create(&inv).Error; err != nil {
		h.t.Fatalf("seedInventoryAt: %v", err)
	}
}

func TestLocation_TenantA_CannotReadTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")
	_, bZoneID := h.seedWarehouseZone(b.TenantID, "B")
	bLoc := h.seedLocationRow(b.TenantID, bZoneID, "LMM-READ-"+uniqueSuffix())

	path := fmt.Sprintf("/api/v1/locations/%d", bLoc.ID)
	status, resp := h.doRequest(http.MethodGet, path, a.Token, nil)
	if status != http.StatusNotFound {
		t.Errorf("Tenant A read Tenant B's location (id=%d) — got status %d (want 404); body=%s", bLoc.ID, status, resp.Data)
	}
}

func TestLocation_TenantA_CannotUpdateTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")
	_, bZoneID := h.seedWarehouseZone(b.TenantID, "B")
	bLoc := h.seedLocationRow(b.TenantID, bZoneID, "LMM-UPD-"+uniqueSuffix())

	path := fmt.Sprintf("/api/v1/locations/%d", bLoc.ID)
	status, _ := h.doRequest(http.MethodPut, path, a.Token, location.UpdateRequest{
		Code: bLoc.Code, Status: location.StatusActive,
	})

	getStatus, getResp := h.doRequest(http.MethodGet, path, b.Token, nil)
	if getStatus != http.StatusOK {
		t.Fatalf("re-reading tenant B's own location after cross-tenant update attempt failed: status=%d", getStatus)
	}
	var after map[string]any
	_ = json.Unmarshal(getResp.Data, &after)

	if status == http.StatusOK {
		t.Errorf("Tenant A updated Tenant B's location (id=%d) — PUT status=%d", bLoc.ID, status)
	}
}

func TestLocation_TenantA_CannotDeleteTenantB_ByID(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")
	_, bZoneID := h.seedWarehouseZone(b.TenantID, "B")
	bLoc := h.seedLocationRow(b.TenantID, bZoneID, "LMM-DEL-"+uniqueSuffix())

	path := fmt.Sprintf("/api/v1/locations/%d", bLoc.ID)
	status, _ := h.doRequest(http.MethodDelete, path, a.Token, nil)

	getStatus, _ := h.doRequest(http.MethodGet, path, b.Token, nil)
	if status == http.StatusNoContent || getStatus == http.StatusNotFound {
		t.Errorf("Tenant A deleted Tenant B's location (id=%d) — DELETE status=%d, row now reads back as status=%d", bLoc.ID, status, getStatus)
	}
}

// TestLocation_List_ScopedToCallerTenant_EvenWithAnotherTenantsZoneID
// proves that passing another tenant's zone_id in the query string
// returns an empty list, never that tenant's rows — the query-param
// IDOR shape test/isolation's own
// TestZoneList_CannotEnumerateViaWarehouseIDParam already proves for
// zone/location's sibling endpoints.
func TestLocation_List_ScopedToCallerTenant_EvenWithAnotherTenantsZoneID(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")
	_, bZoneID := h.seedWarehouseZone(b.TenantID, "B")
	bLoc := h.seedLocationRow(b.TenantID, bZoneID, "LMM-LIST-"+uniqueSuffix())

	path := fmt.Sprintf("/api/v1/locations?zone_id=%d", bZoneID)
	status, resp := h.doRequest(http.MethodGet, path, a.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("unexpected status listing locations: %d", status)
	}

	var rows []map[string]any
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		t.Fatalf("decode location list: %v", err)
	}
	for _, row := range rows {
		if idStr, _ := row["id"].(string); idStr == itoa(bLoc.ID) {
			t.Errorf("Tenant A listed Tenant B's zone_id=%d query param and received Tenant B's location (id=%d)", bZoneID, bLoc.ID)
		}
	}
}

// TestLocation_RejectsCrossTenantZoneOnCreate proves
// Service.validateZoneOwnership: Tenant A cannot create a location
// under Tenant B's zone by guessing its ID.
func TestLocation_RejectsCrossTenantZoneOnCreate(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	b := h.seedRealTenant("B")
	_, bZoneID := h.seedWarehouseZone(b.TenantID, "B")

	status, resp := h.doRequest(http.MethodPost, "/api/v1/locations", a.Token, location.CreateRequest{
		ZoneID: bZoneID, Code: "LMM-ATTACK-" + uniqueSuffix(),
	})
	if status != http.StatusNotFound {
		t.Errorf("Tenant A created a location under Tenant B's zone_id=%d — status=%d body=%s", bZoneID, status, resp.Data)
	}
}

// TestLocation_DeleteBlockedByStock proves the new referential-safety
// check: a location with on-hand inventory cannot be deleted.
func TestLocation_DeleteBlockedByStock(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	_, zoneID := h.seedWarehouseZone(a.TenantID, "A")
	loc := h.seedLocationRow(a.TenantID, zoneID, "LMM-STOCK-"+uniqueSuffix())
	h.seedInventoryAt(a.TenantID, loc.ID, 10)

	path := fmt.Sprintf("/api/v1/locations/%d", loc.ID)
	status, resp := h.doRequest(http.MethodDelete, path, a.Token, nil)
	if status != http.StatusConflict {
		t.Errorf("deleting a location with on-hand stock got status %d, want 409; body=%s", status, resp.Data)
	}

	getStatus, _ := h.doRequest(http.MethodGet, path, a.Token, nil)
	if getStatus != http.StatusOK {
		t.Errorf("location should still exist after a blocked delete, got status %d", getStatus)
	}
}

// TestLocation_DeleteAllowedWithoutStock is the DeleteBlockedByStock
// test's negative control — proves the check doesn't block every
// delete indiscriminately.
func TestLocation_DeleteAllowedWithoutStock(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	_, zoneID := h.seedWarehouseZone(a.TenantID, "A")
	loc := h.seedLocationRow(a.TenantID, zoneID, "LMM-NOSTOCK-"+uniqueSuffix())

	path := fmt.Sprintf("/api/v1/locations/%d", loc.ID)
	status, resp := h.doRequest(http.MethodDelete, path, a.Token, nil)
	if status != http.StatusNoContent {
		t.Errorf("deleting a location with no stock got status %d, want 204; body=%s", status, resp.Data)
	}
}

// TestLocation_RBAC_RequiresWarehousePermission proves a tenant member
// whose role grants nothing is rejected with 403 on every Location
// endpoint, server-side — not merely relying on the UI to hide the
// Locations screen. Uses the same warehouse.view/warehouse.update
// permissions the rest of Warehouse Setup already requires (Location
// Master Management deliberately reuses them rather than introducing a
// separate location.* permission set — see internal/location/
// handler.go's own doc).
func TestLocation_RBAC_RequiresWarehousePermission(t *testing.T) {
	h := newHarness(t)
	a := h.seedRealTenant("A")
	_, zoneID := h.seedWarehouseZone(a.TenantID, "A")
	loc := h.seedLocationRow(a.TenantID, zoneID, "LMM-RBAC-"+uniqueSuffix())
	noAccessToken := h.seedNoAccessMember(a.TenantID, "LMM")

	t.Run("list", func(t *testing.T) {
		status, _ := h.doRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations?zone_id=%d", zoneID), noAccessToken, nil)
		if status != http.StatusForbidden {
			t.Errorf("listing locations with no warehouse.view grant got status %d, want 403", status)
		}
	})
	t.Run("get", func(t *testing.T) {
		status, _ := h.doRequest(http.MethodGet, fmt.Sprintf("/api/v1/locations/%d", loc.ID), noAccessToken, nil)
		if status != http.StatusForbidden {
			t.Errorf("getting a location with no warehouse.view grant got status %d, want 403", status)
		}
	})
	t.Run("create", func(t *testing.T) {
		status, _ := h.doRequest(http.MethodPost, "/api/v1/locations", noAccessToken, location.CreateRequest{ZoneID: zoneID, Code: "LMM-RBAC-NEW-" + uniqueSuffix()})
		if status != http.StatusForbidden {
			t.Errorf("creating a location with no warehouse.update grant got status %d, want 403", status)
		}
	})
	t.Run("delete", func(t *testing.T) {
		status, _ := h.doRequest(http.MethodDelete, fmt.Sprintf("/api/v1/locations/%d", loc.ID), noAccessToken, nil)
		if status != http.StatusForbidden {
			t.Errorf("deleting a location with no warehouse.update grant got status %d, want 403", status)
		}
	})
}

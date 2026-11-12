package isolation

import (
	"net/http"
	"testing"
)

// TestTransactionRollback_NoPartialWrite is a narrowed
// TestTransactionRollback_NoPartialCrossTenantWrite: with no
// tenant-owned multi-step mutation in Phase 29's own module scope (the
// receiving-confirm-style multi-table writes the roadmap's version of
// this test has in mind belong to Phase 30+), the closest equivalent
// available here is role.Repository.ReplaceRoleWarehouseAccess — a
// delete-then-insert inside one transaction (role/repository.go). This
// test forces the insert half to fail (one bogus warehouse_id mixed
// into an otherwise-valid set) and verifies the delete half was rolled
// back with it, not left partially applied. Expected to PASS — it
// is a correctness property of the existing transaction, independent of
// the missing tenant check covered by
// TestTenantA_CannotGrantTenantB_WarehouseToOwnRole in
// cross_tenant_fk_test.go.
func TestTransactionRollback_NoPartialWrite(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")

	path := "/api/v1/roles/" + itoa(tenantA.Role.ID) + "/warehouse-access"

	// Establish a known-good baseline grant first.
	status, _ := h.doRequest(http.MethodPut, path, tenantA.Token, map[string]any{
		"warehouse_ids": []int64{tenantA.Warehouse.ID},
	})
	if status != http.StatusOK {
		t.Fatalf("baseline warehouse-access grant failed: status=%d", status)
	}

	const bogusWarehouseID = int64(999_999_999)
	status, _ = h.doRequest(http.MethodPut, path, tenantA.Token, map[string]any{
		"warehouse_ids": []int64{tenantA.Warehouse.ID, bogusWarehouseID},
	})
	if status == http.StatusOK {
		t.Fatalf("expected the bogus warehouse_id to be rejected, but the request succeeded (status=%d) — cannot verify rollback behavior", status)
	}

	getStatus, getResp := h.doRequest(http.MethodGet, path, tenantA.Token, nil)
	if getStatus != http.StatusOK {
		t.Fatalf("re-reading warehouse access failed: status=%d", getStatus)
	}
	var after struct {
		WarehouseAccess []struct {
			ID int64 `json:"id,string"`
		} `json:"warehouse_access"`
	}
	if err := unmarshalData(getResp.Data, &after); err != nil {
		t.Fatalf("decode warehouse access: %v", err)
	}

	if len(after.WarehouseAccess) != 1 || after.WarehouseAccess[0].ID != tenantA.Warehouse.ID {
		t.Errorf("PARTIAL WRITE CONFIRMED: after a rejected mixed-validity warehouse-access replace, role %d's grants are %v (want exactly [%d], the pre-existing baseline untouched)",
			tenantA.Role.ID, after.WarehouseAccess, tenantA.Warehouse.ID)
	}
}

// TestWarehouseDelete_NoPartialState_WhenZonesExist is a referential-
// safety correctness check (warehouse.Service.Delete's existing
// zones.CountByWarehouse guard) rather than a tenant-isolation test —
// included because the task asked for transaction-isolation coverage
// and this is the one multi-table delete guard in Phase 29's own module
// scope. Expected to PASS: deleting a warehouse that still has a zone
// must be rejected, and the warehouse and its zone must both still
// exist afterward — not partially torn down.
func TestWarehouseDelete_NoPartialState_WhenZonesExist(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")

	status, resp := h.doRequest(http.MethodDelete, "/api/v1/warehouses/"+itoa(tenantA.Warehouse.ID), tenantA.Token, nil)
	if status != http.StatusConflict {
		t.Fatalf("expected 409 deleting a warehouse with an existing zone, got status=%d body=%s", status, resp.Data)
	}

	whStatus, _ := h.doRequest(http.MethodGet, "/api/v1/warehouses/"+itoa(tenantA.Warehouse.ID), tenantA.Token, nil)
	zoneStatus, _ := h.doRequest(http.MethodGet, "/api/v1/zones/"+itoa(tenantA.Zone.ID), tenantA.Token, nil)

	if whStatus != http.StatusOK || zoneStatus != http.StatusOK {
		t.Errorf("PARTIAL STATE CONFIRMED: after a rejected warehouse delete, warehouse status=%d zone status=%d (want both 200 — the rejected delete must leave both rows intact)",
			whStatus, zoneStatus)
	}
}

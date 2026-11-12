package isolation

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
)

// TestIDOR_SequentialIDEnumeration is saas-module-roadmap.md's
// TestIDOR_SequentialIDEnumeration: seed several independent "tenants"
// and confirm that Tenant A's token can walk every other tenant's
// warehouse ID and read it back. Reported as a count rather than one
// pass/fail so the failure is quantified (how many of N foreign IDs
// leaked), matching how an attacker would actually exploit this — a
// script walking IDs, not a single probe.
func TestIDOR_SequentialIDEnumeration(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")

	const otherTenantCount = 4
	var foreignIDs []int64
	for i := 0; i < otherTenantCount; i++ {
		other := h.seedTenant(fmt.Sprintf("Enum%d", i))
		foreignIDs = append(foreignIDs, other.Warehouse.ID)
	}

	leaked := 0
	for _, id := range foreignIDs {
		status, _ := h.doRequest(http.MethodGet, fmt.Sprintf("/api/v1/warehouses/%d", id), tenantA.Token, nil)
		if status == http.StatusOK {
			leaked++
		}
	}

	if leaked > 0 {
		t.Errorf("CROSS-TENANT ID ENUMERATION CONFIRMED: Tenant A's token successfully read %d/%d other tenants' warehouses by walking their IDs",
			leaked, otherTenantCount)
	}
}

// TestConnectionPool_NoTenantLeakage is a narrowed
// TestConnectionPool_NoTenantLeakage: tenant-isolation-security.md §9
// flags the shared GORM connection pool (SetMaxOpenConns(25), no
// per-request session binding) as a risk ONLY for a Row-Level-Security
// design that was explicitly NOT adopted — today's isolation (such as
// it is) comes entirely from each query's own bound WHERE clause
// parameters, which GORM parameterizes per call, not from any
// connection-local state. This test is expected to PASS: it is a
// baseline correctness check that interleaving concurrent requests for
// two different rows over the shared pool never cross-returns the wrong
// row — proof that the eventual tenant_id predicate Phase 29 adds will
// have a safe foundation to sit on, not evidence tenant isolation
// already exists.
func TestConnectionPool_NoTenantLeakage(t *testing.T) {
	h := newHarness(t)
	tenantA := h.seedTenant("A")
	tenantB := h.seedTenant("B")

	const iterations = 40
	var wg sync.WaitGroup
	errs := make(chan string, iterations*2)

	fire := func(f *tenantFixture) {
		defer wg.Done()
		status, resp := h.doRequest(http.MethodGet, fmt.Sprintf("/api/v1/warehouses/%d", f.Warehouse.ID), f.Token, nil)
		if status != http.StatusOK {
			errs <- fmt.Sprintf("tenant %s: unexpected status %d", f.Label, status)
			return
		}
		var got struct {
			Code string `json:"code"`
		}
		_ = unmarshalData(resp.Data, &got)
		if got.Code != f.Warehouse.Code {
			errs <- fmt.Sprintf("POOL LEAKAGE: tenant %s's request for warehouse id=%d returned code=%q, want %q",
				f.Label, f.Warehouse.ID, got.Code, f.Warehouse.Code)
		}
	}

	for i := 0; i < iterations; i++ {
		wg.Add(2)
		go fire(tenantA)
		go fire(tenantB)
	}
	wg.Wait()
	close(errs)

	for msg := range errs {
		t.Error(msg)
	}
}

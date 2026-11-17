package dashboard

import (
	"context"
	"testing"

	"wms-backend/internal/inventory"
	"wms-backend/internal/receiving"
)

// fakeReceivingSummarizer records every tenantID it was called with, so
// tests can assert Summary never substitutes a hardcoded tenant for the
// one it was actually given — the exact regression this file guards
// against (Summary used to always pass inventory.UnverifiedCallerTenantID
// regardless of the caller's real, re-verified tenant).
type fakeReceivingSummarizer struct {
	countOpenTenantIDs []int64
	listTenantIDs      []int64
	countOpenResult    int64
	listResult         []receiving.ListResponse
}

func (f *fakeReceivingSummarizer) CountOpen(_ context.Context, tenantID int64) (int64, error) {
	f.countOpenTenantIDs = append(f.countOpenTenantIDs, tenantID)
	return f.countOpenResult, nil
}

func (f *fakeReceivingSummarizer) List(_ context.Context, tenantID int64, _ receiving.ListParams) ([]receiving.ListResponse, int64, error) {
	f.listTenantIDs = append(f.listTenantIDs, tenantID)
	return f.listResult, int64(len(f.listResult)), nil
}

type fakePickingSummarizer struct {
	tenantIDs []int64
	result    int64
}

func (f *fakePickingSummarizer) CountOpen(_ context.Context, tenantID int64) (int64, error) {
	f.tenantIDs = append(f.tenantIDs, tenantID)
	return f.result, nil
}

type fakeInventorySummarizer struct {
	tenantIDs []int64
	result    inventory.SummaryResponse
}

func (f *fakeInventorySummarizer) Summary(_ context.Context, tenantID int64) (inventory.SummaryResponse, error) {
	f.tenantIDs = append(f.tenantIDs, tenantID)
	return f.result, nil
}

// TestSummary_UsesCallerTenantID_NotHardcodedDefault is the direct
// regression test for the tenant-isolation bug this phase fixes: calling
// Summary with a non-default tenant (42, deliberately not 1 — the old
// inventory.UnverifiedCallerTenantID value) must thread that exact ID
// into every downstream call, never silently substitute tenant 1.
func TestSummary_UsesCallerTenantID_NotHardcodedDefault(t *testing.T) {
	recv := &fakeReceivingSummarizer{countOpenResult: 2, listResult: []receiving.ListResponse{{ID: 1}}}
	pick := &fakePickingSummarizer{result: 1}
	inv := &fakeInventorySummarizer{result: inventory.SummaryResponse{AvailableQty: 428, LowStockCount: 2}}
	svc := NewService(recv, pick, inv)

	const callerTenantID int64 = 42
	resp, err := svc.Summary(context.Background(), callerTenantID)
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}

	for _, got := range recv.countOpenTenantIDs {
		if got != callerTenantID {
			t.Errorf("receiving.CountOpen called with tenantID %d, want %d", got, callerTenantID)
		}
	}
	for _, got := range recv.listTenantIDs {
		if got != callerTenantID {
			t.Errorf("receiving.List called with tenantID %d, want %d", got, callerTenantID)
		}
	}
	for _, got := range pick.tenantIDs {
		if got != callerTenantID {
			t.Errorf("picking.CountOpen called with tenantID %d, want %d", got, callerTenantID)
		}
	}
	for _, got := range inv.tenantIDs {
		if got != callerTenantID {
			t.Errorf("inventory.Summary called with tenantID %d, want %d", got, callerTenantID)
		}
	}

	if resp.PendingReceivingCount != 2 || resp.PendingPickingCount != 1 || resp.AvailableStockQty != 428 || resp.LowStockCount != 2 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

// TestSummary_DifferentTenants_AreIsolated proves two different callers
// never share a result: each tenant's Summary call must only ever see
// its own tenantID propagated downstream, regardless of call order.
func TestSummary_DifferentTenants_AreIsolated(t *testing.T) {
	recv := &fakeReceivingSummarizer{}
	pick := &fakePickingSummarizer{}
	inv := &fakeInventorySummarizer{}
	svc := NewService(recv, pick, inv)

	if _, err := svc.Summary(context.Background(), 1); err != nil {
		t.Fatalf("Summary(tenant 1) returned error: %v", err)
	}
	if _, err := svc.Summary(context.Background(), 2); err != nil {
		t.Fatalf("Summary(tenant 2) returned error: %v", err)
	}

	wantSeq := []int64{1, 2}
	for i, want := range wantSeq {
		if pick.tenantIDs[i] != want {
			t.Errorf("picking.CountOpen call %d used tenantID %d, want %d", i, pick.tenantIDs[i], want)
		}
		if inv.tenantIDs[i] != want {
			t.Errorf("inventory.Summary call %d used tenantID %d, want %d", i, inv.tenantIDs[i], want)
		}
	}
}

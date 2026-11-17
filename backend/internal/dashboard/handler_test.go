package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"wms-backend/internal/platform/middleware"
)

// fakeMembershipChecker always allows — this test is about which
// tenantID reaches Service.Summary, not about membership denial, which
// middleware.RequireTenantContext's own test suite already covers.
type fakeMembershipChecker struct{}

func (fakeMembershipChecker) Valid(context.Context, int64, int64) (bool, error) { return true, nil }

func newDashboardTestRouter(secret string, service *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(service).Register(r.Group("/api/v1"), secret, fakeMembershipChecker{}, time.Minute)
	return r
}

func signTestToken(t *testing.T, secret string, tenantID int64) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":              "11",
		"exp":              time.Now().Add(time.Hour).Unix(),
		"active_tenant_id": tenantID,
		"permissions":      []any{"dashboard.view"},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// TestHandlerSummary_ScopesToCallersOwnTenant is the end-to-end
// regression test: two requests carrying different active_tenant_id
// claims must reach Service.Summary with their own distinct tenantID,
// proving the full RequireJWT -> RequireTenantContext -> handler chain
// (not just the Service method in isolation) no longer collapses every
// caller onto tenant 1.
func TestHandlerSummary_ScopesToCallersOwnTenant(t *testing.T) {
	recv := &fakeReceivingSummarizer{}
	pick := &fakePickingSummarizer{}
	inv := &fakeInventorySummarizer{}
	svc := NewService(recv, pick, inv)

	const secret = "test-secret"
	router := newDashboardTestRouter(secret, svc)

	for _, tenantID := range []int64{2, 7} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
		req.Header.Set("Authorization", "Bearer "+signTestToken(t, secret, tenantID))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("tenant %d: status = %d, want %d, body = %s", tenantID, rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	wantSeq := []int64{2, 7}
	if len(pick.tenantIDs) != len(wantSeq) {
		t.Fatalf("picking.CountOpen called %d times, want %d", len(pick.tenantIDs), len(wantSeq))
	}
	for i, want := range wantSeq {
		if pick.tenantIDs[i] != want {
			t.Errorf("request %d: picking.CountOpen used tenantID %d, want %d", i, pick.tenantIDs[i], want)
		}
		if inv.tenantIDs[i] != want {
			t.Errorf("request %d: inventory.Summary used tenantID %d, want %d", i, inv.tenantIDs[i], want)
		}
	}
}

// TestHandlerSummary_MissingTenantContext_Rejected covers the fail-closed
// path: RequireJWT alone (a token with no active_tenant_id claim) must
// never reach Service.Summary at all.
func TestHandlerSummary_MissingTenantContext_Rejected(t *testing.T) {
	recv := &fakeReceivingSummarizer{}
	pick := &fakePickingSummarizer{}
	inv := &fakeInventorySummarizer{}
	svc := NewService(recv, pick, inv)

	const secret = "test-secret"
	router := newDashboardTestRouter(secret, svc)

	claims := jwt.MapClaims{"sub": "11", "exp": time.Now().Add(time.Hour).Unix()}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if len(pick.tenantIDs) != 0 {
		t.Fatalf("picking.CountOpen was called %d times, want 0 — request should have been rejected before reaching the service", len(pick.tenantIDs))
	}
	if !strings.Contains(rec.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("body = %s, want an UNAUTHORIZED error code", rec.Body.String())
	}
}

var _ middleware.MembershipChecker = fakeMembershipChecker{}

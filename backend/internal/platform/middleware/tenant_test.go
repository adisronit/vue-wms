package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// fakeChecker is a MembershipChecker test double that records every call
// so cache-hit/cache-expiry behavior can be asserted, not just the final
// HTTP outcome.
type fakeChecker struct {
	valid bool
	err   error
	calls int
}

func (f *fakeChecker) Valid(_ context.Context, _, _ int64) (bool, error) {
	f.calls++
	return f.valid, f.err
}

func newTenantTestRouter(checker MembershipChecker, ttl time.Duration, secret string) *gin.Engine {
	r := gin.New()
	r.GET("/protected", RequireJWT(secret), RequireTenantContext(checker, ttl), func(c *gin.Context) {
		tenantID, _ := EnforcedTenantID(c)
		c.JSON(http.StatusOK, gin.H{"ok": true, "tenant_id": tenantID})
	})
	return r
}

func signTokenWithTenant(t *testing.T, secret string, sub string, tenantID int64) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":              sub,
		"exp":              time.Now().Add(1 * time.Hour).Unix(),
		"active_tenant_id": tenantID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func doProtectedRequest(router *gin.Engine, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRequireTenantContext_ValidMembership_Allowed(t *testing.T) {
	checker := &fakeChecker{valid: true}
	router := newTenantTestRouter(checker, time.Minute, "test-secret")
	token := signTokenWithTenant(t, "test-secret", "1", 7)

	rec := doProtectedRequest(router, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
}

func TestRequireTenantContext_NoMembership_Denied(t *testing.T) {
	checker := &fakeChecker{valid: false}
	router := newTenantTestRouter(checker, time.Minute, "test-secret")
	token := signTokenWithTenant(t, "test-secret", "1", 7)

	rec := doProtectedRequest(router, token)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// TestRequireTenantContext_CheckerError_FailsClosed covers the "could not
// tell" case from the middleware's own doc comment: a checker error must
// deny the request exactly like an explicit false, never let it through.
func TestRequireTenantContext_CheckerError_FailsClosed(t *testing.T) {
	checker := &fakeChecker{valid: true, err: errors.New("database unreachable")}
	router := newTenantTestRouter(checker, time.Minute, "test-secret")
	token := signTokenWithTenant(t, "test-secret", "1", 7)

	rec := doProtectedRequest(router, token)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRequireTenantContext_MissingTenantClaim_Denied(t *testing.T) {
	checker := &fakeChecker{valid: true}
	router := newTenantTestRouter(checker, time.Minute, "test-secret")
	// signToken (jwt_test.go) carries no active_tenant_id claim at all.
	token := signToken(t, "test-secret", time.Now().Add(1*time.Hour))

	rec := doProtectedRequest(router, token)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0 — should fail before ever reaching the database", checker.calls)
	}
}

func TestRequireTenantContext_SetsEnforcedTenantID(t *testing.T) {
	checker := &fakeChecker{valid: true}
	router := newTenantTestRouter(checker, time.Minute, "test-secret")
	token := signTokenWithTenant(t, "test-secret", "1", 42)

	rec := doProtectedRequest(router, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"tenant_id":42`) {
		t.Fatalf("body = %s, want tenant_id 42 set by EnforcedTenantID", rec.Body.String())
	}
}

func TestRequireTenantContext_CachesPositiveResultWithinTTL(t *testing.T) {
	checker := &fakeChecker{valid: true}
	router := newTenantTestRouter(checker, time.Hour, "test-secret")
	token := signTokenWithTenant(t, "test-secret", "1", 7)

	doProtectedRequest(router, token)
	doProtectedRequest(router, token)

	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1 — second request within ttl should hit the cache", checker.calls)
	}
}

func TestRequireTenantContext_ReChecksAfterTTLExpires(t *testing.T) {
	checker := &fakeChecker{valid: true}
	router := newTenantTestRouter(checker, time.Nanosecond, "test-secret")
	token := signTokenWithTenant(t, "test-secret", "1", 7)

	doProtectedRequest(router, token)
	time.Sleep(time.Millisecond)
	doProtectedRequest(router, token)

	if checker.calls != 2 {
		t.Fatalf("checker calls = %d, want 2 — cache entry should have expired", checker.calls)
	}
}

// TestRequireTenantContext_DoesNotCacheNegativeResults covers the
// membershipCache doc comment's claim directly: a denial must never be
// remembered, so a membership that starts passing mid-session (e.g. an
// INVITED row that just became ACTIVE) is allowed on the very next
// request, not stuck behind a stale cached rejection.
func TestRequireTenantContext_DoesNotCacheNegativeResults(t *testing.T) {
	checker := &fakeChecker{valid: false}
	router := newTenantTestRouter(checker, time.Hour, "test-secret")
	token := signTokenWithTenant(t, "test-secret", "1", 7)

	doProtectedRequest(router, token)
	checker.valid = true
	rec := doProtectedRequest(router, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d — denial must not have been cached", rec.Code, http.StatusOK)
	}
	if checker.calls != 2 {
		t.Fatalf("checker calls = %d, want 2 — a denial should never be cached", checker.calls)
	}
}

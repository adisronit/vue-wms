package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	apihttp "wms-backend/internal/platform/http"
)

type fakeStatusChecker struct {
	admins map[int64]bool
	err    error
}

func (f *fakeStatusChecker) IsPlatformAdmin(_ context.Context, userID int64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.admins[userID], nil
}

func newMeTestRouter(secret string, checker statusChecker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rg := r.Group("/api/v1")
	NewHandler(checker).Register(rg, secret)
	return r
}

func signTokenForUser(t *testing.T, secret string, userID int64) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": strconv.FormatInt(userID, 10),
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// TestMe_PlatformAdmin proves a user holding a non-revoked
// platform_admins row gets is_platform_admin=true, with a plain 200 (not
// a privileged-only response) — any authenticated caller may ask this
// question.
func TestMe_PlatformAdmin(t *testing.T) {
	checker := &fakeStatusChecker{admins: map[int64]bool{42: true}}
	router := newMeTestRouter("test-secret", checker)
	token := signTokenForUser(t, "test-secret", 42)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var env apihttp.SuccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %#v", env.Data)
	}
	if data["is_platform_admin"] != true {
		t.Fatalf("is_platform_admin = %v, want true", data["is_platform_admin"])
	}
}

// TestMe_NotPlatformAdmin proves an ordinary, fully-authenticated tenant
// user — the common case for every login in the system — gets a plain
// 200 with is_platform_admin=false, never a 403. This is the entire
// reason this handler exists separately from the Phase 29C tenant
// routes, which would 403 this exact caller.
func TestMe_NotPlatformAdmin(t *testing.T) {
	checker := &fakeStatusChecker{admins: map[int64]bool{}}
	router := newMeTestRouter("test-secret", checker)
	token := signTokenForUser(t, "test-secret", 99)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var env apihttp.SuccessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := env.Data.(map[string]any)
	if data["is_platform_admin"] != false {
		t.Fatalf("is_platform_admin = %v, want false", data["is_platform_admin"])
	}
}

// TestMe_NoToken proves the endpoint fails closed with 401 before the
// checker is even consulted when there is no valid JWT — RequireJWT's
// own responsibility, re-asserted here as this route's first link.
func TestMe_NoToken(t *testing.T) {
	checker := &fakeStatusChecker{admins: map[int64]bool{1: true}}
	router := newMeTestRouter("test-secret", checker)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// TestMe_CheckerError proves a checker failure (e.g. the database is
// unreachable) surfaces as a 500, not a silently-false capability —
// "could not tell" must be visibly distinct from "verified not an
// admin".
func TestMe_CheckerError(t *testing.T) {
	checker := &fakeStatusChecker{err: errors.New("database unreachable")}
	router := newMeTestRouter("test-secret", checker)
	token := signTokenForUser(t, "test-secret", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

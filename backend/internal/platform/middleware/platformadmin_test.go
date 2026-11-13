package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type fakePlatformAdminChecker struct {
	admins map[int64]bool
	err    error
}

func (f *fakePlatformAdminChecker) IsPlatformAdmin(_ context.Context, userID int64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.admins[userID], nil
}

func newPlatformAdminTestRouter(secret string, checker PlatformAdminChecker) *gin.Engine {
	r := gin.New()
	r.GET("/protected", RequireJWT(secret), RequirePlatformAdmin(checker, slog.Default()), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
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

// TestRequirePlatformAdmin_Allowed proves a user with a non-revoked
// platform_admins row reaches the handler — independent of any
// permission/role claim embedded in the token (there is none on this
// test's token at all), per the middleware's own "never tenant RBAC
// alone" contract.
func TestRequirePlatformAdmin_Allowed(t *testing.T) {
	checker := &fakePlatformAdminChecker{admins: map[int64]bool{42: true}}
	router := newPlatformAdminTestRouter("test-secret", checker)
	token := signTokenForUser(t, "test-secret", 42)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestRequirePlatformAdmin_Denied_NotAnAdmin proves a perfectly valid
// JWT for a real, authenticated user is rejected when that user holds
// no platform_admins row — the ordinary case for every tenant user in
// the system.
func TestRequirePlatformAdmin_Denied_NotAnAdmin(t *testing.T) {
	checker := &fakePlatformAdminChecker{admins: map[int64]bool{}}
	router := newPlatformAdminTestRouter("test-secret", checker)
	token := signTokenForUser(t, "test-secret", 99)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// TestRequirePlatformAdmin_Denied_NoToken proves the chain fails closed
// before the checker is even consulted when there is no valid JWT at
// all — RequireJWT's own responsibility, re-asserted here as the first
// link of this specific chain.
func TestRequirePlatformAdmin_Denied_NoToken(t *testing.T) {
	checker := &fakePlatformAdminChecker{admins: map[int64]bool{1: true}}
	router := newPlatformAdminTestRouter("test-secret", checker)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

// TestRequirePlatformAdmin_Denied_CheckerError proves a checker failure
// (e.g. the database is unreachable) fails closed as 403, same as an
// explicit "not an admin" answer — "could not tell" must never be
// treated as "allowed".
func TestRequirePlatformAdmin_Denied_CheckerError(t *testing.T) {
	checker := &fakePlatformAdminChecker{err: errors.New("database unreachable")}
	router := newPlatformAdminTestRouter("test-secret", checker)
	token := signTokenForUser(t, "test-secret", 1)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

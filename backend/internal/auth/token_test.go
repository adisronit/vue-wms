package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIssueToken_RoundTrip(t *testing.T) {
	signed, expiresAt, err := IssueToken("test-secret", time.Hour, 42, "admin", "ADMIN", []string{"users.view"})
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("expiresAt should be in the future")
	}

	var claims Claims
	token, err := jwt.ParseWithClaims(signed, &claims, func(*jwt.Token) (any, error) {
		return []byte("test-secret"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("failed to parse issued token: %v", err)
	}

	if claims.Subject != "42" {
		t.Fatalf("subject = %q, want 42", claims.Subject)
	}
	if claims.Username != "admin" || claims.RoleCode != "ADMIN" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "users.view" {
		t.Fatalf("permissions = %v", claims.Permissions)
	}
	if claims.Issuer != Issuer {
		t.Fatalf("issuer = %q, want %q", claims.Issuer, Issuer)
	}
}

func TestIssueToken_WrongSecretFailsValidation(t *testing.T) {
	signed, _, err := IssueToken("secret-a", time.Hour, 1, "admin", "ADMIN", nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	var claims Claims
	_, err = jwt.ParseWithClaims(signed, &claims, func(*jwt.Token) (any, error) {
		return []byte("secret-b"), nil
	})
	if err == nil {
		t.Fatal("expected validation to fail with the wrong secret")
	}
}

func TestIssueToken_ExpiredTokenFailsValidation(t *testing.T) {
	signed, _, err := IssueToken("test-secret", -time.Hour, 1, "admin", "ADMIN", nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	var claims Claims
	_, err = jwt.ParseWithClaims(signed, &claims, func(*jwt.Token) (any, error) {
		return []byte("test-secret"), nil
	})
	if err == nil {
		t.Fatal("expected validation to fail for an already-expired token")
	}
}

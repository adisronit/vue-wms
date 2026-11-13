package auth

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer identifies this application as the JWT's issuer.
const Issuer = "smart-wms"

// SessionTypeTenant marks a token issued against one active tenant
// membership (the only session type this phase issues — see
// docs/proposals/wms-saas/tenant-isolation-security.md §2's "session_type:
// TENANT | PLATFORM_ADMIN" claim; the PLATFORM_ADMIN flow is a separate,
// step-up-authenticated phase of its own, out of this phase's scope).
const SessionTypeTenant = "TENANT"

// Claims is the JWT payload issued at login or tenant switch. It
// intentionally excludes password, email, and any other sensitive field —
// only what's needed to identify the caller and authorize a request
// without a database round trip for permissions (see
// middleware.RequirePermission; tenant membership itself is re-verified
// against the database by middleware.RequireTenantContext, not trusted
// from these claims alone).
//
// active_tenant_id/tenant_role_code/session_type replace the single global
// "role" claim this struct carried before tenant membership existed — a
// token never carries more than one tenant's context (tenant-isolation-
// security.md §2); a user who belongs to multiple tenants picks one active
// tenant per session via POST /auth/switch-tenant.
type Claims struct {
	Username       string   `json:"username"`
	TenantID       int64    `json:"active_tenant_id"`
	TenantRoleCode string   `json:"tenant_role_code"`
	SessionType    string   `json:"session_type"`
	Permissions    []string `json:"permissions"`
	jwt.RegisteredClaims
}

// IssueToken signs an HS256 JWT for userID, valid for expiresIn from now,
// scoped to exactly one tenant (tenantID) and the role/permissions that
// membership carries.
func IssueToken(secret string, expiresIn time.Duration, userID int64, username string, tenantID int64, tenantRoleCode string, permissions []string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)

	claims := Claims{
		Username:       username,
		TenantID:       tenantID,
		TenantRoleCode: tenantRoleCode,
		SessionType:    SessionTypeTenant,
		Permissions:    permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	return signed, expiresAt, err
}

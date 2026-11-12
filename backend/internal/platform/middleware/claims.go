package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// CurrentUserID reads the "sub" claim RequireJWT stored under
// ClaimsContextKey and parses it back into the numeric user ID. Moved
// here in Phase 6 (originally a private helper inside internal/auth)
// once internal/receiving became a second caller needing the same
// lookup — a shared platform-level helper instead of two copies of the
// same claim-parsing logic.
func CurrentUserID(c *gin.Context) (int64, bool) {
	raw, ok := c.Get(ClaimsContextKey)
	if !ok {
		return 0, false
	}

	claims, ok := raw.(jwt.MapClaims)
	if !ok {
		return 0, false
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return 0, false
	}

	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, false
	}

	return id, true
}

// TenantContextKey is where RequireTenantContext stores the re-verified
// tenant_id for downstream handlers to read, once that middleware has
// actually run. It is deliberately distinct from CurrentTenantID below —
// which reads the *raw, not-yet-re-verified* active_tenant_id JWT claim —
// so a handler can tell "the token claims tenant X" apart from "tenant X
// was just confirmed, server-side, to still be an active membership".
const TenantContextKey = "tenant_id"

// CurrentTenantID reads the "active_tenant_id" claim RequireJWT stored
// under ClaimsContextKey. Mirrors CurrentUserID's shape, except the raw
// value survives jwt.MapClaims' generic JSON decoding as float64 (every
// JSON number does, absent a custom Claims struct), not int64, so it is
// converted explicitly here rather than left for every caller to get
// right on their own.
//
// This is the claim as issued, not re-verified against the database — it
// is what /auth/me and /auth/switch-tenant use (they re-verify membership
// themselves, inside Service, since requiring RequireTenantContext on
// those routes would be circular). Any other handler wanting an
// enforced, re-verified tenant_id should use EnforcedTenantID instead,
// once RequireTenantContext has run.
func CurrentTenantID(c *gin.Context) (int64, bool) {
	raw, ok := c.Get(ClaimsContextKey)
	if !ok {
		return 0, false
	}

	claims, ok := raw.(jwt.MapClaims)
	if !ok {
		return 0, false
	}

	v, ok := claims["active_tenant_id"]
	if !ok {
		return 0, false
	}

	f, ok := v.(float64)
	if !ok {
		return 0, false
	}

	return int64(f), true
}

// EnforcedTenantID reads the tenant_id RequireTenantContext set under
// TenantContextKey after re-verifying membership against the database.
// Unlike CurrentTenantID, this is never the raw, unverified JWT claim —
// it only exists once RequireTenantContext has actually run for the
// current request.
func EnforcedTenantID(c *gin.Context) (int64, bool) {
	raw, ok := c.Get(TenantContextKey)
	if !ok {
		return 0, false
	}

	id, ok := raw.(int64)
	if !ok {
		return 0, false
	}

	return id, true
}

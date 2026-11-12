package middleware

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
)

// MembershipChecker is the narrow capability RequireTenantContext needs —
// satisfied by *tenant.Service's Valid method — kept as a local interface
// so this package does not depend on internal/tenant, the same
// depend-on-behavior-not-packages pattern internal/auth's own
// userFinder/roleReader interfaces already use.
type MembershipChecker interface {
	// Valid reports whether userID currently holds an ACTIVE membership in
	// an operational tenant with id tenantID. A non-nil error means the
	// check itself could not be completed (e.g. the database is
	// unreachable) — RequireTenantContext treats that the same as "not
	// valid", failing closed on both "definitely not a member" and
	// "could not tell".
	Valid(ctx context.Context, userID, tenantID int64) (bool, error)
}

// membershipCache remembers only POSITIVE results, for ttl, per
// docs/proposals/wms-saas/tenant-isolation-security.md §3 step 3: bounding
// the added per-request database round trip while accepting, as an
// explicit tradeoff rather than a silent gap, that a mid-session
// revocation becomes visible only once a cache entry expires, not
// instantly. Negative and error results are never cached, so a revoked or
// never-valid membership is re-checked — and can start succeeding again —
// on every single request.
type membershipCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]time.Time // key -> expiresAt
}

func newMembershipCache(ttl time.Duration) *membershipCache {
	return &membershipCache{ttl: ttl, entries: map[string]time.Time{}}
}

func (c *membershipCache) valid(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiresAt, ok := c.entries[key]
	if !ok {
		return false
	}
	if time.Now().After(expiresAt) {
		delete(c.entries, key)
		return false
	}
	return true
}

func (c *membershipCache) remember(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = time.Now().Add(c.ttl)
}

func membershipCacheKey(userID, tenantID int64) string {
	return strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(tenantID, 10)
}

// RequireTenantContext is the second stage of the
// RequireJWT -> RequireTenantContext -> RequirePermission -> handler chain
// (tenant-isolation-security.md §3). It must run after RequireJWT, since
// it reads the claims that middleware already validated and stored under
// ClaimsContextKey.
//
// Unlike RequirePermission, which still trusts the JWT's embedded claims
// outright (an accepted, explicitly documented tradeoff — see that file's
// own doc comment), this middleware exists specifically because trusting
// active_tenant_id from the token alone would let a membership that is
// revoked or changed mid-session keep working until the token itself
// expires. It re-verifies against the database (through checker, cached
// for ttl to bound added latency) before letting any downstream handler
// run, and sets the re-verified tenant_id on the request context under
// TenantContextKey — downstream handlers/services should read it via
// EnforcedTenantID, never by re-parsing the raw JWT claim themselves (that
// claim was only ever an input to this check, not something to trust
// again afterward).
//
// Fail-closed: a missing/malformed active_tenant_id claim, or a checker
// call that fails or returns false, both stop the request here with no
// handler ever running.
func RequireTenantContext(checker MembershipChecker, ttl time.Duration) gin.HandlerFunc {
	cache := newMembershipCache(ttl)

	return func(c *gin.Context) {
		userID, ok := CurrentUserID(c)
		if !ok {
			apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
			c.Abort()
			return
		}

		tenantID, ok := CurrentTenantID(c)
		if !ok {
			apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
			c.Abort()
			return
		}

		key := membershipCacheKey(userID, tenantID)
		if !cache.valid(key) {
			valid, err := checker.Valid(c.Request.Context(), userID, tenantID)
			if err != nil || !valid {
				apihttp.Fail(c, http.StatusForbidden, "no active membership in the current tenant", "NO_TENANT_MEMBERSHIP")
				c.Abort()
				return
			}
			cache.remember(key)
		}

		c.Set(TenantContextKey, tenantID)
		c.Next()
	}
}

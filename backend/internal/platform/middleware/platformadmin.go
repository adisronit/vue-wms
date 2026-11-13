package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
)

// PlatformAdminChecker is the narrow capability RequirePlatformAdmin
// needs — satisfied by *platformadmin.Repository's IsPlatformAdmin —
// kept as a local interface so this package does not depend on
// internal/platformadmin, mirroring MembershipChecker's
// depend-on-behavior-not-packages pattern in tenant.go.
type PlatformAdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID int64) (bool, error)
}

// RequirePlatformAdmin gates the Platform Admin API (/api/v1/platform/*)
// and must run after RequireJWT, since it reads the identity
// (CurrentUserID) that middleware already validated.
//
// Deliberately independent of tenant-level RBAC: it does not read the
// JWT's tenant_role_code or permissions claims at all, and does not run
// behind RequireTenantContext — a platform admin acts across every
// tenant, not inside the one tenant their own ordinary login session
// happens to be scoped to (see internal/auth's Claims, which carries
// exactly one active_tenant_id). Authorization here comes from one
// source only: a live, non-revoked platform_admins row for the caller's
// user_id, re-checked on every request with no caching (see
// platformadmin.Repository.IsPlatformAdmin's own doc for why).
//
// This is a narrower control than tenant-isolation-security.md §2's
// full design (a separate, step-up-authenticated PLATFORM_ADMIN
// session_type token) — that flow does not exist yet (see
// saas-module-roadmap.md's Phase 33). Until it does, an attacker who
// steals a platform admin's ordinary TENANT-session JWT can also reach
// these routes; this is a known, documented gap (see
// backend/test/isolation and the Tenant Management API's own README),
// not a silent one. It is still strictly stronger than authorizing on
// tenant RBAC alone, since no role/permission claim of any kind, from
// any tenant, grants access here — only an explicit, individually
// revocable platform_admins row does.
func RequirePlatformAdmin(checker PlatformAdminChecker, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := CurrentUserID(c)
		if !ok {
			apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
			c.Abort()
			return
		}

		isAdmin, err := checker.IsPlatformAdmin(c.Request.Context(), userID)
		if err != nil {
			logger.Error("platform admin check failed",
				slog.String("request_id", GetRequestID(c)),
				slog.Int64("user_id", userID),
				slog.String("error", err.Error()),
			)
			apihttp.Fail(c, http.StatusForbidden, "platform admin access required", "FORBIDDEN")
			c.Abort()
			return
		}
		if !isAdmin {
			apihttp.Fail(c, http.StatusForbidden, "platform admin access required", "FORBIDDEN")
			c.Abort()
			return
		}

		c.Next()
	}
}

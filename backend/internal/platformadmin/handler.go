package platformadmin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// MeResponse is the one field GET /api/v1/platform/me ever returns —
// deliberately nothing else about the platform_admins row (granted_at,
// granted_by, ...) is exposed here, since the only consumer is the
// frontend deciding whether to show the Platform Administration nav
// section, not an audit or profile view.
type MeResponse struct {
	IsPlatformAdmin bool `json:"is_platform_admin" example:"false"`
} //	@name	PlatformAdminMeResponse

// statusChecker is the narrow capability Handler needs — satisfied by
// *Repository — kept as a local interface so tests can supply a fake
// instead of a real database, mirroring middleware.PlatformAdminChecker's
// own depend-on-behavior-not-a-concrete-type pattern one layer up.
type statusChecker interface {
	IsPlatformAdmin(ctx context.Context, userID int64) (bool, error)
}

// Handler exposes the one read-only capability check every authenticated
// user (platform admin or not) needs: "am I a platform admin". Phase
// 29C's own Tenant Management API (internal/tenant's Handler) cannot
// answer this for a non-admin, because its entire route group is itself
// gated by middleware.RequirePlatformAdmin — asking a Phase 29C endpoint
// would just 403 instead of returning a usable false. This handler is
// the smallest addition that lets the frontend ask the question safely:
// same authoritative source (Repository.IsPlatformAdmin, re-queried live,
// no caching), same fail-closed shape, but gated only by RequireJWT so
// every caller gets a real answer instead of being refused the chance to
// ask.
type Handler struct {
	checker statusChecker
}

func NewHandler(checker statusChecker) *Handler {
	return &Handler{checker: checker}
}

// Register mounts GET /api/v1/platform/me on rg, gated by RequireJWT
// only — deliberately never RequirePlatformAdmin, and never mounted
// under the /platform/tenants group those routes already use, since a
// non-admin asking this question must get {is_platform_admin:false},
// not a 403.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	rg.GET("/platform/me", middleware.RequireJWT(jwtSecret), h.Me)
}

// Me godoc
//
//	@Summary		Check the caller's own Platform Admin status
//	@Description	Any authenticated user may call this. Answers strictly from a live platform_admins lookup (never from username/email, and never cached) — the one server-verified signal the frontend may use to decide whether to show Platform Administration navigation.
//	@Tags			PlatformAdmin
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=MeResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Router			/platform/me [get]
func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	isAdmin, err := h.checker.IsPlatformAdmin(c.Request.Context(), userID)
	if err != nil {
		// Fails closed, mirroring RequirePlatformAdmin's own "could not
		// tell is never allowed" rule — but here there is no privileged
		// action to deny, only a status to report, so the honest answer
		// to "could I verify this" is a 500, not a false capability.
		apihttp.Fail(c, http.StatusInternalServerError, "failed to check platform admin status", "INTERNAL_ERROR")
		return
	}

	apihttp.OK(c, MeResponse{IsPlatformAdmin: isAdmin}, "")
}

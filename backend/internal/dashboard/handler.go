package dashboard

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the dashboard endpoint behind the same
// RequireJWT -> RequireTenantContext -> RequirePermission chain every
// other tenant-scoped module uses (see internal/location/handler.go's
// own doc) — closing the gap dashboard/service.go's previous doc
// comments flagged: Summary used to always read inventory.
// UnverifiedCallerTenantID (tenant_id=1) regardless of the caller's
// real tenant. dashboard.view remains the permission every role in the
// catalog is already seeded with, so no new grant is introduced.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("/dashboard",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("/summary", middleware.RequirePermission("dashboard.view"), h.Summary)
}

// Summary godoc
//
//	@Summary		Get the operational dashboard summary
//	@Description	Aggregated counts (pending receiving/picking, available stock, low stock) plus a small recent-inbound-activity list — matches DashboardView.vue's Overview page. Scoped to the caller's own re-verified active tenant. Every number is computed from indexed COUNT/aggregate queries, never a full-table load.
//	@Tags			Dashboard
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=SummaryResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/dashboard/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	resp, err := h.service.Summary(c.Request.Context(), tenantID)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to load dashboard summary", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

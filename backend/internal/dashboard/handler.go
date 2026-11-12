package dashboard

import (
	"net/http"

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

// Register mounts the dashboard endpoint under dashboard.view — every
// role in the catalog is seeded with it (see internal/rbac's own doc),
// so this reuses an existing, already-universal grant rather than
// inventing a new one.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	group := rg.Group("/dashboard", middleware.RequireJWT(jwtSecret), middleware.RequirePermission("dashboard.view"))
	group.GET("/summary", h.Summary)
}

// Summary godoc
//
//	@Summary		Get the operational dashboard summary
//	@Description	Aggregated counts (pending receiving/picking, available stock, low stock) plus a small recent-inbound-activity list — matches DashboardView.vue's Overview page. Every number is computed from indexed COUNT/aggregate queries, never a full-table load.
//	@Tags			Dashboard
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=SummaryResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/dashboard/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	resp, err := h.service.Summary(c.Request.Context())
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to load dashboard summary", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

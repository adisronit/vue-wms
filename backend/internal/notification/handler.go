package notification

import (
	"errors"
	"net/http"
	"strconv"

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

// Register mounts the notification endpoints on rg. Both are gated on
// RequireJWT alone, the same "any authenticated user, self-scoped"
// pattern GET /auth/me already uses — no stock_adjustment.* or other
// module permission applies here: a user's own notification inbox is
// identity-scoped, not module-scoped, and Service.List/MarkRead never
// accept a caller-supplied user_id, only the one resolved from the JWT.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	group := rg.Group("/notifications", middleware.RequireJWT(jwtSecret))
	group.GET("", h.List)
	group.POST("/:id/read", h.MarkRead)
}

// List godoc
//
//	@Summary		List my notifications
//	@Description	Returns the authenticated caller's own notifications, newest first. Phase 21's only writer is the overdue Stock Adjustment reminder consumer, so today every row has type=STOCK_ADJUSTMENT_REMINDER. The frontend badge/list uses is_read=false plus meta.total as its unread count, rather than a separate count endpoint.
//	@Tags			Notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page		query		int		false	"Page number"			default(1)
//	@Param			limit		query		int		false	"Page size (max 100)"	default(20)
//	@Param			is_read		query		bool	false	"Filter to read (true) or unread (false) only; omit for both"
//	@Success		200			{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Router			/notifications [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	var isRead *bool
	if raw := c.Query("is_read"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			isRead = &parsed
		}
	}

	userID, _ := middleware.CurrentUserID(c)

	responses, total, err := h.service.List(c.Request.Context(), ListParams{
		UserID: userID, IsRead: isRead, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list notifications", "INTERNAL_ERROR")
		return
	}

	effectivePage, effectiveLimit := page, limit
	if effectivePage < 1 {
		effectivePage = 1
	}
	if effectiveLimit < 1 || effectiveLimit > 100 {
		effectiveLimit = 20
	}
	totalPages := int((total + int64(effectiveLimit) - 1) / int64(effectiveLimit))
	if totalPages < 1 {
		totalPages = 1
	}

	apihttp.List(c, responses, apihttp.ListMeta{
		Page: effectivePage, Limit: effectiveLimit, Total: int(total), TotalPages: totalPages,
	})
}

// MarkRead godoc
//
//	@Summary		Mark one of my notifications read
//	@Description	Only ever affects a notification owned by the authenticated caller — one belonging to another user is reported as 404, the same as a genuinely nonexistent ID, so this endpoint never confirms another user's notification exists. Marking an already-read notification read again is a no-op success, not an error.
//	@Tags			Notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Notification ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/notifications/{id}/read [post]
func (h *Handler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid notification id", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)

	resp, err := h.service.MarkRead(c.Request.Context(), id, userID)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "notification not found", "NOT_FOUND")
		return
	}
	apihttp.Fail(c, http.StatusInternalServerError, "failed to update notification", "INTERNAL_ERROR")
}

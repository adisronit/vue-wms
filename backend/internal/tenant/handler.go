package tenant

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// Handler exposes Service's tenant-management methods, plus (WMS-29E)
// membership's Platform Admin membership-management methods, over HTTP
// under /api/v1/platform/tenants — the Platform Admin API. It never
// exposes the membership-resolution methods (ActiveMembership(s),
// DefaultMembership, Valid) auth already consumes directly.
type Handler struct {
	service    *Service
	membership *MembershipService
}

func NewHandler(service *Service, membership *MembershipService) *Handler {
	return &Handler{service: service, membership: membership}
}

// Register mounts the Platform Admin tenant-management AND
// tenant-membership-management endpoints on rg, gated by RequireJWT
// then RequirePlatformAdmin — deliberately never
// middleware.RequirePermission/RequireTenantContext: per
// tenant-isolation-security.md §3, the platform-admin chain is a
// separate parallel chain from the tenant chain, and per this phase's
// own instructions, platform operations are never authorized on
// tenant-level RBAC alone.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, platformAdminChecker middleware.PlatformAdminChecker, logger *slog.Logger) {
	tenants := rg.Group("/platform/tenants",
		middleware.RequireJWT(jwtSecret),
		middleware.RequirePlatformAdmin(platformAdminChecker, logger),
	)

	tenants.GET("", h.List)
	tenants.POST("", h.Create)
	tenants.GET("/:id", h.Get)
	tenants.PUT("/:id", h.Update)
	tenants.PATCH("/:id/status", h.UpdateStatus)

	h.RegisterPlatformMembers(tenants)
}

// List godoc
//
//	@Summary		List tenants
//	@Description	Platform Admin only. Supports a search box (matches slug or name) and a status filter, with pagination.
//	@Tags			PlatformAdmin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page	query		int		false	"Page number"			default(1)
//	@Param			limit	query		int		false	"Page size (max 100)"	default(20)
//	@Param			search	query		string	false	"Matches slug or name"
//	@Param			status	query		string	false	"TRIAL, ACTIVE, SUSPENDED, or CANCELLED"
//	@Success		200		{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Router			/platform/tenants [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	params := ListParams{
		Search: c.Query("search"),
		Status: c.Query("status"),
		Page:   page,
		Limit:  limit,
	}

	responses, total, err := h.service.List(c.Request.Context(), params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list tenants", "INTERNAL_ERROR")
		return
	}

	effectivePage, effectiveLimit := params.Page, params.Limit
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

// Get godoc
//
//	@Summary		Get a tenant by ID
//	@Description	Platform Admin only.
//	@Tags			PlatformAdmin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Tenant ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a tenant
//	@Description	Platform Admin only — onboards a new customer. Writes a tenant_audit_logs row (action=tenant.create).
//	@Tags			PlatformAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New tenant"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		409		{object}	apihttp.ErrorResponse	"slug already exists"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	resp, err := h.service.Create(c.Request.Context(), req, actorUserID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "tenant created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a tenant's slug/name
//	@Description	Platform Admin only. Status transitions go through PATCH .../status instead. Writes a tenant_audit_logs row (action=tenant.update).
//	@Tags			PlatformAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Tenant ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"slug already exists"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	resp, err := h.service.Update(c.Request.Context(), id, req, actorUserID)
	writeResult(c, resp, err)
}

// UpdateStatus godoc
//
//	@Summary		Change a tenant's status
//	@Description	Platform Admin only. Transitions to TRIAL, ACTIVE, SUSPENDED, or CANCELLED, stamping suspended_at/cancelled_at as appropriate. Writes a tenant_audit_logs row (action=tenant.activate/suspend/cancel/trial).
//	@Tags			PlatformAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Tenant ID"
//	@Param			request	body		UpdateStatusRequest		true	"New status"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants/{id}/status [patch]
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	resp, err := h.service.UpdateStatus(c.Request.Context(), id, req, actorUserID)
	writeResult(c, resp, err)
}

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func writeResult(c *gin.Context, resp Response, err error) {
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

func writeError(c *gin.Context, err error) {
	var validationErr ErrValidation

	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "tenant not found", "NOT_FOUND")
	case errors.Is(err, ErrDuplicate):
		apihttp.Fail(c, http.StatusConflict, "tenant slug already exists", "DUPLICATE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "tenant operation failed", "INTERNAL_ERROR")
	}
}

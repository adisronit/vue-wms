package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts /auth/login (public) and /auth/me, /auth/switch-tenant
// (JWT-protected) on rg, and wires RequireJWT with jwtSecret — the same
// middleware built and tested in Phase 1, reused as-is per CLAUDE.md.
// Neither protected route runs RequireTenantContext: /auth/me and
// /auth/switch-tenant are how a caller establishes or changes tenant
// context in the first place, so requiring it already would be circular
// (tenant-isolation-security.md §3: the chain applies "to every /api/v1
// route except auth/*"). Both re-verify membership against the database
// themselves instead, inside Service.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	authGroup := rg.Group("/auth")
	authGroup.POST("/login", h.Login)
	authGroup.GET("/me", middleware.RequireJWT(jwtSecret), h.Me)
	authGroup.POST("/switch-tenant", middleware.RequireJWT(jwtSecret), h.SwitchTenant)
}

// Login godoc
//
//	@Summary		Log in
//	@Description	Verifies username/password with bcrypt, resolves the caller's default active tenant membership, and issues an HS256 JWT valid for JWT_EXPIRES_IN.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest	true	"Credentials"
//	@Success		200		{object}	apihttp.SuccessResponse{data=LoginData}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse	"invalid username or password"
//	@Failure		403		{object}	apihttp.ErrorResponse	"account is inactive/locked, or no active tenant membership"
//	@Router			/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	data, err := h.service.Login(c.Request.Context(), req.Username, req.Password)

	var notActive ErrAccountNotActive
	switch {
	case err == nil:
		apihttp.OK(c, data, "")
	case errors.Is(err, ErrInvalidCredentials):
		apihttp.Fail(c, http.StatusUnauthorized, "invalid username or password", "INVALID_CREDENTIALS")
	case errors.As(err, &notActive):
		apihttp.Fail(c, http.StatusForbidden, "account is "+string(notActive.Status), "ACCOUNT_"+string(notActive.Status))
	case errors.Is(err, tenant.ErrMembershipNotFound):
		apihttp.Fail(c, http.StatusForbidden, "no active tenant membership", "NO_TENANT_MEMBERSHIP")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "login failed", "INTERNAL_ERROR")
	}
}

// Me godoc
//
//	@Summary		Get the current authenticated user
//	@Description	Returns the caller's profile, active tenant, role, and permission codes from the live database row.
//	@Tags			Auth
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=UserSummary}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse	"active tenant membership no longer valid"
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	tenantID, ok := middleware.CurrentTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	summary, err := h.service.Me(c.Request.Context(), userID, tenantID)
	switch {
	case err == nil:
		apihttp.OK(c, summary, "")
	case errors.Is(err, user.ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "user not found", "NOT_FOUND")
	case errors.Is(err, tenant.ErrMembershipNotFound):
		apihttp.Fail(c, http.StatusForbidden, "active tenant membership no longer valid", "NO_TENANT_MEMBERSHIP")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "failed to load current user", "INTERNAL_ERROR")
	}
}

// SwitchTenant godoc
//
//	@Summary		Switch active tenant
//	@Description	Re-verifies, server-side, that an ACTIVE tenant_users row exists for the caller and the requested tenant, then issues a new JWT scoped to it. The requested tenant_id is never trusted as the basis for the new token's claim by itself.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		SwitchTenantRequest	true	"Target tenant"
//	@Success		200		{object}	apihttp.SuccessResponse{data=LoginData}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"account inactive/locked, or no active membership in the requested tenant"
//	@Router			/auth/switch-tenant [post]
func (h *Handler) SwitchTenant(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	var req SwitchTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	data, err := h.service.SwitchTenant(c.Request.Context(), userID, req.TenantID)

	var notActive ErrAccountNotActive
	switch {
	case err == nil:
		apihttp.OK(c, data, "")
	case errors.Is(err, tenant.ErrMembershipNotFound):
		apihttp.Fail(c, http.StatusForbidden, "no active membership in the requested tenant", "NO_TENANT_MEMBERSHIP")
	case errors.As(err, &notActive):
		apihttp.Fail(c, http.StatusForbidden, "account is "+string(notActive.Status), "ACCOUNT_"+string(notActive.Status))
	case errors.Is(err, user.ErrNotFound):
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "failed to switch tenant", "INTERNAL_ERROR")
	}
}

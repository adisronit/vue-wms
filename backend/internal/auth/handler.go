package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/user"
)

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts /auth/login (public) and /auth/me (JWT-protected) on
// rg, and wires RequireJWT with jwtSecret — the same middleware built and
// tested in Phase 1, reused as-is per CLAUDE.md.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	authGroup := rg.Group("/auth")
	authGroup.POST("/login", h.Login)
	authGroup.GET("/me", middleware.RequireJWT(jwtSecret), h.Me)
}

// Login godoc
//
//	@Summary		Log in
//	@Description	Verifies username/password with bcrypt and issues an HS256 JWT valid for JWT_EXPIRES_IN.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest	true	"Credentials"
//	@Success		200		{object}	apihttp.SuccessResponse{data=LoginData}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse	"invalid username or password"
//	@Failure		403		{object}	apihttp.ErrorResponse	"account is inactive or locked"
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
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "login failed", "INTERNAL_ERROR")
	}
}

// Me godoc
//
//	@Summary		Get the current authenticated user
//	@Description	Returns the caller's profile, role, and permission codes from the live database row.
//	@Tags			Auth
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=UserSummary}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	summary, err := h.service.Me(c.Request.Context(), userID)
	switch {
	case err == nil:
		apihttp.OK(c, summary, "")
	case errors.Is(err, user.ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "user not found", "NOT_FOUND")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "failed to load current user", "INTERNAL_ERROR")
	}
}

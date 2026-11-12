package user

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

// Register mounts the User Management endpoints on rg, each gated by
// RequireJWT then the specific users.* permission — the JWT Middleware ->
// Permission Middleware -> Handler chain CLAUDE.md's RBAC section
// describes, applied uniformly instead of re-implemented per handler.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	users := rg.Group("/users", middleware.RequireJWT(jwtSecret))
	users.GET("", middleware.RequirePermission("users.view"), h.List)
	users.GET("/:id", middleware.RequirePermission("users.view"), h.Get)
	users.POST("", middleware.RequirePermission("users.create"), h.Create)
	users.PUT("/:id", middleware.RequirePermission("users.update"), h.Update)
	users.DELETE("/:id", middleware.RequirePermission("users.delete"), h.Delete)
}

// List godoc
//
//	@Summary		List users
//	@Description	Supports the User Management screen's search box and role/status filters, with pagination.
//	@Tags			Users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page	query		int		false	"Page number"			default(1)
//	@Param			limit	query		int		false	"Page size (max 100)"	default(20)
//	@Param			search	query		string	false	"Matches full name, username, or email"
//	@Param			role	query		string	false	"Role code, e.g. ADMIN"
//	@Param			status	query		string	false	"ACTIVE, INACTIVE, or LOCKED"
//	@Success		200		{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/users [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	params := ListParams{
		Search:   c.Query("search"),
		RoleCode: c.Query("role"),
		Status:   Status(c.Query("status")),
		Page:     page,
		Limit:    limit,
	}

	responses, total, err := h.service.List(c.Request.Context(), params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list users", "INTERNAL_ERROR")
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
		Page:       effectivePage,
		Limit:      effectiveLimit,
		Total:      int(total),
		TotalPages: totalPages,
	})
}

// Get godoc
//
//	@Summary		Get a user by ID
//	@Tags			Users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"User ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/users/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	writeUserResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a user
//	@Tags			Users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New user"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"username or email already exists"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/users [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Create(c.Request.Context(), req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "user created")
		return
	}
	writeUserError(c, err)
}

// Update godoc
//
//	@Summary		Update a user
//	@Tags			Users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"User ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/users/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Update(c.Request.Context(), id, req)
	writeUserResult(c, resp, err)
}

// Delete godoc
//
//	@Summary		Delete a user
//	@Tags			Users
//	@Security		BearerAuth
//	@Param			id	path	int	true	"User ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/users/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			apihttp.Fail(c, http.StatusNotFound, "user not found", "NOT_FOUND")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "failed to delete user", "INTERNAL_ERROR")
		return
	}

	c.Status(http.StatusNoContent)
}

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func writeUserResult(c *gin.Context, resp Response, err error) {
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeUserError(c, err)
}

func writeUserError(c *gin.Context, err error) {
	var validationErr ErrValidation

	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "user not found", "NOT_FOUND")
	case errors.Is(err, ErrDuplicate):
		apihttp.Fail(c, http.StatusConflict, "username or email already exists", "DUPLICATE")
	case errors.Is(err, ErrRoleNotFound):
		apihttp.Fail(c, http.StatusUnprocessableEntity, "role_code does not match any role", "INVALID_ROLE")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusUnprocessableEntity, "warehouse_id does not match any warehouse", "INVALID_WAREHOUSE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "user operation failed", "INTERNAL_ERROR")
	}
}

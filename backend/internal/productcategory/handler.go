package productcategory

import (
	"errors"
	"net/http"
	"strconv"
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

// Register mounts the Product Category endpoints on rg, gated by
// RequireJWT -> RequireTenantContext -> RequirePermission — the
// tenant-scoped chain CLAUDE.md's RBAC section describes and
// internal/tenant.TenantAdminHandler already follows, rather than the
// RequireJWT-only chain internal/product and internal/warehouse use
// today (a pre-existing, documented gap — see repository.go's doc).
// Every handler method below reads its tenant_id exclusively from
// middleware.EnforcedTenantID — the caller's own re-verified active
// tenant — never from a path, query, or body parameter.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	categories := rg.Group("/product-categories",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	categories.GET("", middleware.RequirePermission("product_category.view"), h.List)
	categories.GET("/:id", middleware.RequirePermission("product_category.view"), h.Get)
	categories.POST("", middleware.RequirePermission("product_category.create"), h.Create)
	categories.PUT("/:id", middleware.RequirePermission("product_category.update"), h.Update)
	categories.DELETE("/:id", middleware.RequirePermission("product_category.delete"), h.Delete)
}

// List godoc
//
//	@Summary		List product categories
//	@Description	Supports the Product Category screen's search box, parent, and status filters, with pagination. Scoped to the caller's own active tenant.
//	@Tags			ProductCategories
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page		query		int		false	"Page number"			default(1)
//	@Param			limit		query		int		false	"Page size (max 100)"	default(20)
//	@Param			search		query		string	false	"Matches code or name"
//	@Param			parent_id	query		int		false	"Filter by parent category ID"
//	@Param			status		query		string	false	"ACTIVE or INACTIVE"
//	@Success		200			{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/product-categories [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	params := ListParams{
		Search: c.Query("search"),
		Status: Status(c.Query("status")),
		Page:   page,
		Limit:  limit,
	}
	if raw := c.Query("parent_id"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			params.ParentID = &v
		}
	}

	responses, total, err := h.service.List(c.Request.Context(), tenantID, params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list product categories", "INTERNAL_ERROR")
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
//	@Summary		Get a product category by ID
//	@Tags			ProductCategories
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Category ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/product-categories/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid category id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a product category
//	@Tags			ProductCategories
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New category"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"code already exists"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/product-categories [post]
func (h *Handler) Create(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Create(c.Request.Context(), tenantID, req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "product category created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a product category
//	@Tags			ProductCategories
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Category ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/product-categories/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid category id", "INVALID_REQUEST")
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Update(c.Request.Context(), tenantID, id, req)
	writeResult(c, resp, err)
}

// Delete godoc
//
//	@Summary		Delete a product category
//	@Description	Blocked (409) while child categories or products still reference this category.
//	@Tags			ProductCategories
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Category ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse
//	@Router			/product-categories/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid category id", "INVALID_REQUEST")
		return
	}

	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
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
	var invalidParentErr ErrInvalidParent

	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product category not found", "NOT_FOUND")
	case errors.Is(err, ErrDuplicateCode):
		apihttp.Fail(c, http.StatusConflict, "code already exists", "DUPLICATE_CODE")
	case errors.Is(err, ErrHasChildren):
		apihttp.Fail(c, http.StatusConflict, "cannot delete — child categories still exist", "HAS_CHILDREN")
	case errors.Is(err, ErrHasProducts):
		apihttp.Fail(c, http.StatusConflict, "cannot delete — products still reference this category", "HAS_PRODUCTS")
	case errors.As(err, &invalidParentErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, invalidParentErr.Message, "INVALID_PARENT")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "product category operation failed", "INTERNAL_ERROR")
	}
}

package supplier

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

// Register mounts the Supplier endpoints on rg, gated by
// RequireJWT -> RequireTenantContext -> RequirePermission — see
// internal/productcategory/handler.go's doc for why this chain, rather
// than the RequireJWT-only chain internal/product/internal/warehouse
// use today.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	suppliers := rg.Group("/suppliers",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	suppliers.GET("", middleware.RequirePermission("supplier.view"), h.List)
	suppliers.GET("/:id", middleware.RequirePermission("supplier.view"), h.Get)
	suppliers.POST("", middleware.RequirePermission("supplier.create"), h.Create)
	suppliers.PUT("/:id", middleware.RequirePermission("supplier.update"), h.Update)
	suppliers.DELETE("/:id", middleware.RequirePermission("supplier.delete"), h.Delete)
}

// List godoc
//
//	@Summary		List suppliers
//	@Description	Supports the Suppliers screen's search box and status filter, with pagination. Scoped to the caller's own active tenant.
//	@Tags			Suppliers
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page	query		int		false	"Page number"			default(1)
//	@Param			limit	query		int		false	"Page size (max 100)"	default(20)
//	@Param			search	query		string	false	"Matches code or company name"
//	@Param			status	query		string	false	"ACTIVE or INACTIVE"
//	@Success		200		{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/suppliers [get]
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

	responses, total, err := h.service.List(c.Request.Context(), tenantID, params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list suppliers", "INTERNAL_ERROR")
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
//	@Summary		Get a supplier by ID
//	@Tags			Suppliers
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Supplier ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/suppliers/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid supplier id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a supplier
//	@Tags			Suppliers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New supplier"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"code already exists"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/suppliers [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "supplier created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a supplier
//	@Tags			Suppliers
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Supplier ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/suppliers/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid supplier id", "INVALID_REQUEST")
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
//	@Summary		Delete a supplier
//	@Tags			Suppliers
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Supplier ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/suppliers/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid supplier id", "INVALID_REQUEST")
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

	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "supplier not found", "NOT_FOUND")
	case errors.Is(err, ErrDuplicateCode):
		apihttp.Fail(c, http.StatusConflict, "code already exists", "DUPLICATE_CODE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "supplier operation failed", "INTERNAL_ERROR")
	}
}

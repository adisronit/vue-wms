package warehouse

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

// Register mounts the Warehouse Setup endpoints on rg, each gated by
// RequireJWT then the specific warehouse.* permission.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	warehouses := rg.Group("/warehouses", middleware.RequireJWT(jwtSecret))
	warehouses.GET("", middleware.RequirePermission("warehouse.view"), h.List)
	warehouses.GET("/:id", middleware.RequirePermission("warehouse.view"), h.Get)
	warehouses.POST("", middleware.RequirePermission("warehouse.create"), h.Create)
	warehouses.PUT("/:id", middleware.RequirePermission("warehouse.update"), h.Update)
	warehouses.DELETE("/:id", middleware.RequirePermission("warehouse.delete"), h.Delete)
}

// List godoc
//
//	@Summary		List warehouses
//	@Description	Supports the Warehouse Setup screen's search box and type/status filters, with pagination.
//	@Tags			Warehouses
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page		query		int		false	"Page number"			default(1)
//	@Param			limit		query		int		false	"Page size (max 100)"	default(25)
//	@Param			search		query		string	false	"Matches code, name, or address"
//	@Param			type		query		string	false	"Distribution Center, Factory Warehouse, or Transit Warehouse"
//	@Param			status		query		string	false	"ACTIVE or INACTIVE"
//	@Param			sort_by		query		string	false	"code, name, type, or status"	default(code)
//	@Param			sort_dir	query		string	false	"asc or desc"					default(asc)
//	@Success		200			{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		400			{object}	apihttp.ErrorResponse	"invalid sort_by/sort_dir"
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/warehouses [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	params := ListParams{
		Search:  c.Query("search"),
		Type:    c.Query("type"),
		Status:  Status(c.Query("status")),
		Page:    page,
		Limit:   limit,
		SortBy:  c.Query("sort_by"),
		SortDir: c.Query("sort_dir"),
	}

	responses, total, err := h.service.List(c.Request.Context(), params)
	if err != nil {
		if errors.Is(err, ErrInvalidSort) {
			apihttp.Fail(c, http.StatusBadRequest, "sort_by must be code, name, type, or status; sort_dir must be asc or desc", "INVALID_SORT")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list warehouses", "INTERNAL_ERROR")
		return
	}

	effectivePage, effectiveLimit := params.Page, params.Limit
	if effectivePage < 1 {
		effectivePage = 1
	}
	if effectiveLimit < 1 || effectiveLimit > 100 {
		effectiveLimit = defaultLimit
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
//	@Summary		Get a warehouse by ID
//	@Tags			Warehouses
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Warehouse ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid warehouse id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a warehouse
//	@Tags			Warehouses
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New warehouse"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"warehouse code already exists"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/warehouses [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Create(c.Request.Context(), req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "warehouse created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a warehouse
//	@Tags			Warehouses
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Warehouse ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid warehouse id", "INVALID_REQUEST")
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Update(c.Request.Context(), id, req)
	writeResult(c, resp, err)
}

// Delete godoc
//
//	@Summary		Delete a warehouse
//	@Description	Refuses to delete a warehouse that still has zones (409), matching the project's referential-safety rule.
//	@Tags			Warehouses
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Warehouse ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse	"warehouse still has zones"
//	@Router			/warehouses/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid warehouse id", "INVALID_REQUEST")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
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
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "NOT_FOUND")
	case errors.Is(err, ErrDuplicate):
		apihttp.Fail(c, http.StatusConflict, "warehouse code already exists", "DUPLICATE")
	case errors.Is(err, ErrHasZones):
		apihttp.Fail(c, http.StatusConflict, "cannot delete warehouse: zones still exist", "HAS_ZONES")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "warehouse operation failed", "INTERNAL_ERROR")
	}
}

package product

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

// Register mounts the Product Master endpoints on rg, each gated by
// RequireJWT then the specific product.* permission.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	products := rg.Group("/products", middleware.RequireJWT(jwtSecret))
	products.GET("", middleware.RequirePermission("product.view"), h.List)
	products.GET("/:id", middleware.RequirePermission("product.view"), h.Get)
	products.POST("", middleware.RequirePermission("product.create"), h.Create)
	products.PUT("/:id", middleware.RequirePermission("product.update"), h.Update)
	products.DELETE("/:id", middleware.RequirePermission("product.delete"), h.Delete)
}

// List godoc
//
//	@Summary		List products
//	@Description	Supports the Product Master screen's search box and category/status/lot-rule filters, with pagination.
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches SKU, name, or barcode"
//	@Param			category		query		string	false	"PACKAGING, LABEL, RFID, or RAW_MATERIAL"
//	@Param			status			query		string	false	"ACTIVE or INACTIVE"
//	@Param			lot_required	query		bool	false	"Filter by whether lot tracking is required"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/products [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	params := ListParams{
		Search:   c.Query("search"),
		Category: Category(c.Query("category")),
		Status:   Status(c.Query("status")),
		Page:     page,
		Limit:    limit,
	}
	if raw := c.Query("lot_required"); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			params.LotRequired = &v
		}
	}

	responses, total, err := h.service.List(c.Request.Context(), params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list products", "INTERNAL_ERROR")
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
//	@Summary		Get a product by ID
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Product ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/products/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a product
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New product"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"sku or barcode already exists"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/products [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Create(c.Request.Context(), req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "product created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a product
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Product ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/products/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
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
//	@Summary		Delete a product
//	@Description	Hard delete. No UI button calls this today — Product Master only ever toggles status — but the endpoint exists for API/admin use. Once Inventory exists, this will need the same referential-safety check warehouse/zone delete already have.
//	@Tags			Products
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Product ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/products/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
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
		apihttp.Fail(c, http.StatusNotFound, "product not found", "NOT_FOUND")
	case errors.Is(err, ErrDuplicateSKU):
		apihttp.Fail(c, http.StatusConflict, "sku already exists", "DUPLICATE_SKU")
	case errors.Is(err, ErrDuplicateBarcode):
		apihttp.Fail(c, http.StatusConflict, "barcode already exists", "DUPLICATE_BARCODE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "product operation failed", "INTERNAL_ERROR")
	}
}

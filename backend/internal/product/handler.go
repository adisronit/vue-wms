package product

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// maxUploadMemory bounds how much of a multipart upload gin buffers in
// memory before spilling to a temp file — set well above
// maxImageUploadBytes so a 5MB image is never spilled to disk twice
// (once by gin's own multipart parsing, once by this package's own
// os.WriteFile), while still bounding worst-case memory for a request
// that lies about its Content-Length.
const maxUploadMemory = 8 << 20 // 8MB

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Product Master endpoints on rg, gated by
// RequireJWT -> RequireTenantContext -> RequirePermission — the EWTA
// enhancement closes this package's own, previously RequireJWT-only
// chain (a pre-existing, documented gap — see repository.go's doc) to
// match internal/productcategory/internal/supplier/internal/brand.
// Image mutation reuses product.update/product.view rather than
// minting a new product.image.* permission — there is no existing
// precedent in this codebase for a sub-resource permission, and an
// image is conceptually just another field on the product.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	products := rg.Group("/products",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	products.GET("", middleware.RequirePermission("product.view"), h.List)
	products.GET("/:id", middleware.RequirePermission("product.view"), h.Get)
	products.POST("", middleware.RequirePermission("product.create"), h.Create)
	products.PUT("/:id", middleware.RequirePermission("product.update"), h.Update)
	products.DELETE("/:id", middleware.RequirePermission("product.delete"), h.Delete)

	products.GET("/:id/image", middleware.RequirePermission("product.view"), h.ServeImage)
	products.POST("/:id/image", middleware.RequirePermission("product.update"), h.UploadImage)
	products.DELETE("/:id/image", middleware.RequirePermission("product.update"), h.RemoveImage)

	products.GET("/:id/barcodes", middleware.RequirePermission("product.view"), h.ListBarcodes)
	products.POST("/:id/barcodes", middleware.RequirePermission("product.update"), h.CreateBarcode)
	products.PUT("/:id/barcodes/:barcodeId", middleware.RequirePermission("product.update"), h.UpdateBarcode)
	products.DELETE("/:id/barcodes/:barcodeId", middleware.RequirePermission("product.update"), h.DeleteBarcode)
}

// List godoc
//
//	@Summary		List products
//	@Description	Supports the Product Master screen's search box and category/brand/status/lot-rule filters, with pagination. Scoped to the caller's own active tenant.
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(25)
//	@Param			search			query		string	false	"Matches SKU, name, or barcode"
//	@Param			category		query		string	false	"PACKAGING, LABEL, RFID, or RAW_MATERIAL"
//	@Param			category_id		query		int		false	"Filter by hierarchical product category ID"
//	@Param			brand_id		query		int		false	"Filter by brand ID"
//	@Param			status			query		string	false	"ACTIVE or INACTIVE"
//	@Param			lot_required	query		bool	false	"Filter by whether lot tracking is required"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/products [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	params := ListParams{
		Search:   c.Query("search"),
		Category: Category(c.Query("category")),
		Status:   Status(c.Query("status")),
		Page:     page,
		Limit:    limit,
	}
	if raw := c.Query("category_id"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			params.CategoryID = &v
		}
	}
	if raw := c.Query("brand_id"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			params.BrandID = &v
		}
	}
	if raw := c.Query("lot_required"); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			params.LotRequired = &v
		}
	}

	responses, total, err := h.service.List(c.Request.Context(), tenantID, params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list products", "INTERNAL_ERROR")
		return
	}

	effectivePage, effectiveLimit := params.Page, params.Limit
	if effectivePage < 1 {
		effectivePage = 1
	}
	if effectiveLimit < 1 || effectiveLimit > 100 {
		effectiveLimit = 25
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

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

	resp, err := h.service.Update(c.Request.Context(), tenantID, id, req)
	writeResult(c, resp, err)
}

// Delete godoc
//
//	@Summary		Delete a product
//	@Description	Hard delete. No UI button calls this today — Product Master only ever toggles status.
//	@Tags			Products
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Product ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/products/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// UploadImage godoc
//
//	@Summary		Upload or replace a product's image
//	@Description	Multipart form upload, field name "image". JPG/PNG/WebP only, max 5MB, content-sniffed (not trusted from Content-Type or filename). Replaces any existing image.
//	@Tags			Products
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int		true	"Product ID"
//	@Param			image	formData	file	true	"Image file"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse	"not a valid JPG/PNG/WebP, or over 5MB"
//	@Router			/products/{id}/image [post]
func (h *Handler) UploadImage(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	if err := c.Request.ParseMultipartForm(maxUploadMemory); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid multipart form", "INVALID_REQUEST")
		return
	}

	file, header, err := c.Request.FormFile("image")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "missing image file (field name: image)", "INVALID_REQUEST")
		return
	}
	defer file.Close()

	// Read at most maxImageUploadBytes+1 — enough to detect an
	// over-the-limit upload without reading an attacker-supplied
	// Content-Length's worth of data into memory first.
	data, err := io.ReadAll(io.LimitReader(file, maxImageUploadBytes+1))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to read upload", "INTERNAL_ERROR")
		return
	}
	if len(data) > maxImageUploadBytes {
		apihttp.Fail(c, http.StatusUnprocessableEntity, "image exceeds the 5MB size limit", "VALIDATION_ERROR")
		return
	}

	resp, err := h.service.UploadImage(c.Request.Context(), tenantID, id, data, header.Filename)
	writeResult(c, resp, err)
}

// ServeImage godoc
//
//	@Summary		Get a product's image file
//	@Description	Serves the raw image bytes. Returns 404 if the product has no image. Not a static mount — every request re-verifies the caller's tenant owns this product.
//	@Tags			Products
//	@Produce		image/jpeg,image/png,image/webp
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Product ID"
//	@Success		200	{file}	binary
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/products/{id}/image [get]
func (h *Handler) ServeImage(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	absPath, mimeType, err := h.service.ImageFile(c.Request.Context(), tenantID, id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.Header("Content-Type", mimeType)
	c.Header("Cache-Control", "private, max-age=3600")
	c.File(absPath)
}

// RemoveImage godoc
//
//	@Summary		Remove a product's image
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Product ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/products/{id}/image [delete]
func (h *Handler) RemoveImage(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.RemoveImage(c.Request.Context(), tenantID, id)
	writeResult(c, resp, err)
}

// ListBarcodes godoc
//
//	@Summary		List a product's additional barcode mappings
//	@Tags			Products
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Product ID"
//	@Success		200	{object}	apihttp.ListResponse{data=[]BarcodeResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/products/{id}/barcodes [get]
func (h *Handler) ListBarcodes(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	if _, err := h.service.Get(c.Request.Context(), tenantID, id); err != nil {
		writeError(c, err)
		return
	}

	barcodes, err := h.service.ListBarcodes(c.Request.Context(), tenantID, id)
	if err != nil {
		writeError(c, err)
		return
	}

	apihttp.List(c, barcodes, apihttp.ListMeta{Page: 1, Limit: len(barcodes), Total: len(barcodes), TotalPages: 1})
}

// CreateBarcode godoc
//
//	@Summary		Add an additional barcode mapping to a product
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Product ID"
//	@Param			request	body		CreateBarcodeRequest	true	"New barcode mapping"
//	@Success		201		{object}	apihttp.SuccessResponse{data=BarcodeResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"already mapped to this product"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/products/{id}/barcodes [post]
func (h *Handler) CreateBarcode(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}

	var req CreateBarcodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.CreateBarcode(c.Request.Context(), tenantID, id, req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "barcode added")
		return
	}
	writeError(c, err)
}

// UpdateBarcode godoc
//
//	@Summary		Update a product's additional barcode mapping
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		int						true	"Product ID"
//	@Param			barcodeId	path		int						true	"Barcode mapping ID"
//	@Param			request		body		UpdateBarcodeRequest	true	"Updated fields"
//	@Success		200			{object}	apihttp.SuccessResponse{data=BarcodeResponse}
//	@Failure		400			{object}	apihttp.ErrorResponse
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Failure		409			{object}	apihttp.ErrorResponse
//	@Failure		422			{object}	apihttp.ErrorResponse
//	@Router			/products/{id}/barcodes/{barcodeId} [put]
func (h *Handler) UpdateBarcode(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}
	barcodeID, err := strconv.ParseInt(c.Param("barcodeId"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid barcode id", "INVALID_REQUEST")
		return
	}

	var req UpdateBarcodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.UpdateBarcode(c.Request.Context(), tenantID, id, barcodeID, req)
	writeResult(c, resp, err)
}

// DeleteBarcode godoc
//
//	@Summary		Remove a product's additional barcode mapping
//	@Tags			Products
//	@Security		BearerAuth
//	@Param			id			path	int	true	"Product ID"
//	@Param			barcodeId	path	int	true	"Barcode mapping ID"
//	@Success		204			"deleted"
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/products/{id}/barcodes/{barcodeId} [delete]
func (h *Handler) DeleteBarcode(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid product id", "INVALID_REQUEST")
		return
	}
	barcodeID, err := strconv.ParseInt(c.Param("barcodeId"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid barcode id", "INVALID_REQUEST")
		return
	}

	if err := h.service.DeleteBarcode(c.Request.Context(), tenantID, id, barcodeID); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

func writeResult(c *gin.Context, resp any, err error) {
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
	case errors.Is(err, ErrDuplicateBarcodeMapping):
		apihttp.Fail(c, http.StatusConflict, "this barcode is already mapped to this product", "DUPLICATE_BARCODE_MAPPING")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "product operation failed", "INTERNAL_ERROR")
	}
}

package warehouselayout

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// maxUploadMemory bounds how much of a multipart upload gin buffers
// in memory before spilling to a temp file — mirrors internal/
// product's own handler.go exactly.
const maxUploadMemory = 8 << 20 // 8MB

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

func writeUnauthenticated(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED")
}

func writeError(c *gin.Context, err error) {
	switch e := err.(type) {
	case ErrValidation:
		apihttp.Fail(c, http.StatusUnprocessableEntity, e.Message, "VALIDATION_ERROR")
	default:
		if err == ErrNotFound {
			apihttp.Fail(c, http.StatusNotFound, "warehouse layout not found", "NOT_FOUND")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "internal error", "INTERNAL_ERROR")
	}
}

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Interactive 2D Warehouse Layout endpoints on
// rg, gated on this package's own "layout" RBAC module (view/update —
// see internal/rbac/catalog.go) rather than report.view/export or
// warehouse.*: this is an editor with real write access, independent
// of both "can view KPI reports" and "can manage warehouse master
// data."
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	g := rg.Group("/warehouses/:warehouseId/layout",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	g.GET("", middleware.RequirePermission("layout.view"), h.GetLayout)
	g.PUT("", middleware.RequirePermission("layout.update"), h.SaveLayout)
	g.GET("/versions", middleware.RequirePermission("layout.view"), h.ListVersions)
	g.GET("/versions/:versionId", middleware.RequirePermission("layout.view"), h.GetVersion)
	g.POST("/images", middleware.RequirePermission("layout.update"), h.UploadImage)
	g.GET("/images/:imageId", middleware.RequirePermission("layout.view"), h.ServeImage)
	g.GET("/pdf", middleware.RequirePermission("layout.view"), h.PDF)
}

func pathWarehouseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("warehouseId"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid warehouse id", "INVALID_REQUEST")
		return 0, false
	}
	return id, true
}

// GetLayout godoc
//
//	@Summary		Get a warehouse's current layout
//	@Description	Returns the highest-version_no layout saved for this warehouse, with every shape and its resolved zone/location reference label (if any). data is null if no layout has ever been saved yet — that is not an error. Gated on layout.view.
//	@Tags			Warehouse Layout
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouseId	path		int	true	"Warehouse ID"
//	@Success		200			{object}	apihttp.SuccessResponse{data=VersionResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{warehouseId}/layout [get]
func (h *Handler) GetLayout(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	warehouseID, ok := pathWarehouseID(c)
	if !ok {
		return
	}

	resp, err := h.service.GetLayout(c.Request.Context(), tenantID, warehouseID)
	if err != nil {
		writeError(c, err)
		return
	}
	apihttp.OK(c, resp, "")
}

// SaveLayout godoc
//
//	@Summary		Save a warehouse's layout (creates a new version)
//	@Description	Objects is the complete object graph, not a diff. Always creates a brand-new version — never updates a previous one in place. Gated on layout.update.
//	@Tags			Warehouse Layout
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouseId	path		int					true	"Warehouse ID"
//	@Param			body		body		SaveLayoutRequest	true	"Full layout object graph"
//	@Success		200			{object}	apihttp.SuccessResponse{data=VersionResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Failure		422			{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{warehouseId}/layout [put]
func (h *Handler) SaveLayout(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		writeUnauthenticated(c)
		return
	}
	warehouseID, ok := pathWarehouseID(c)
	if !ok {
		return
	}

	var req SaveLayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.SaveLayout(c.Request.Context(), tenantID, warehouseID, req, userID)
	if err != nil {
		writeError(c, err)
		return
	}
	apihttp.OK(c, resp, "layout saved")
}

// ListVersions godoc
//
//	@Summary		List a warehouse layout's version history
//	@Tags			Warehouse Layout
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouseId	path		int	true	"Warehouse ID"
//	@Success		200			{object}	apihttp.SuccessResponse{data=[]VersionSummaryResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{warehouseId}/layout/versions [get]
func (h *Handler) ListVersions(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	warehouseID, ok := pathWarehouseID(c)
	if !ok {
		return
	}

	versions, err := h.service.ListVersions(c.Request.Context(), tenantID, warehouseID)
	if err != nil {
		writeError(c, err)
		return
	}
	apihttp.OK(c, versions, "")
}

// GetVersion godoc
//
//	@Summary		Get one specific historical layout version, read-only
//	@Tags			Warehouse Layout
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouseId	path		int	true	"Warehouse ID"
//	@Param			versionId	path		int	true	"Layout version ID"
//	@Success		200			{object}	apihttp.SuccessResponse{data=VersionResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{warehouseId}/layout/versions/{versionId} [get]
func (h *Handler) GetVersion(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	warehouseID, ok := pathWarehouseID(c)
	if !ok {
		return
	}
	versionID, err := strconv.ParseInt(c.Param("versionId"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid version id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.GetVersion(c.Request.Context(), tenantID, warehouseID, versionID)
	if err != nil {
		writeError(c, err)
		return
	}
	apihttp.OK(c, resp, "")
}

// UploadImage godoc
//
//	@Summary		Upload a background image for layout calibration
//	@Description	Multipart form upload, field name "image". JPG/PNG only, max 5MB, content-sniffed. Returns an image_id to pass as background_image_id in a subsequent Save. Gated on layout.update.
//	@Tags			Warehouse Layout
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouseId	path		int		true	"Warehouse ID"
//	@Param			image		formData	file	true	"Image file"
//	@Success		200			{object}	apihttp.SuccessResponse{data=ImageResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		422			{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{warehouseId}/layout/images [post]
func (h *Handler) UploadImage(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		writeUnauthenticated(c)
		return
	}
	warehouseID, ok := pathWarehouseID(c)
	if !ok {
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

	data, err := io.ReadAll(io.LimitReader(file, maxImageUploadBytes+1))
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "failed to read uploaded file", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.UploadBackgroundImage(c.Request.Context(), tenantID, warehouseID, data, header.Filename, userID)
	if err != nil {
		writeError(c, err)
		return
	}
	apihttp.OK(c, resp, "image uploaded")
}

// ServeImage godoc
//
//	@Summary		Get a layout background image's file
//	@Description	Serves the raw image bytes. Not a static mount — every request re-verifies the caller's tenant owns this warehouse and image.
//	@Tags			Warehouse Layout
//	@Produce		image/jpeg,image/png
//	@Security		BearerAuth
//	@Param			warehouseId	path	int	true	"Warehouse ID"
//	@Param			imageId		path	int	true	"Image ID"
//	@Success		200			{file}	binary
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{warehouseId}/layout/images/{imageId} [get]
func (h *Handler) ServeImage(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	warehouseID, ok := pathWarehouseID(c)
	if !ok {
		return
	}
	imageID, err := strconv.ParseInt(c.Param("imageId"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid image id", "INVALID_REQUEST")
		return
	}

	absPath, mimeType, err := h.service.ImageFile(c.Request.Context(), tenantID, warehouseID, imageID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Content-Type", mimeType)
	c.File(absPath)
}

// PDF godoc
//
//	@Summary		Warehouse layout PDF (preview or download)
//	@Tags			Warehouse Layout
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			warehouseId	path	int		true	"Warehouse ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/warehouses/{warehouseId}/layout/pdf [get]
func (h *Handler) PDF(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	warehouseID, ok := pathWarehouseID(c)
	if !ok {
		return
	}

	data, err := h.service.RenderPDF(c.Request.Context(), tenantID, warehouseID)
	if err != nil {
		writeError(c, err)
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\"warehouse_layout.pdf\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

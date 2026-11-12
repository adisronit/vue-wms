package zone

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

// Register mounts the Zone Setup endpoints on rg, gated by the same
// warehouse.* permissions as the parent Warehouse Setup screen — zones
// have no separate nav entry of their own; they're a tab within the
// warehouse drawer.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	zones := rg.Group("/zones", middleware.RequireJWT(jwtSecret))
	zones.GET("", middleware.RequirePermission("warehouse.view"), h.List)
	zones.GET("/:id", middleware.RequirePermission("warehouse.view"), h.Get)
	zones.POST("", middleware.RequirePermission("warehouse.update"), h.Create)
	zones.PUT("/:id", middleware.RequirePermission("warehouse.update"), h.Update)
	zones.DELETE("/:id", middleware.RequirePermission("warehouse.update"), h.Delete)
}

// List godoc
//
//	@Summary		List zones for a warehouse
//	@Description	warehouse_id is required — the Zone Setup tab only ever lists zones for one open warehouse.
//	@Tags			Zones
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouse_id	query		int		true	"Warehouse ID"
//	@Param			status			query		string	false	"ACTIVE or INACTIVE"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		400				{object}	apihttp.ErrorResponse
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/zones [get]
func (h *Handler) List(c *gin.Context) {
	warehouseID, err := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "warehouse_id is required", "INVALID_REQUEST")
		return
	}

	responses, total, err := h.service.List(c.Request.Context(), ListParams{
		WarehouseID: warehouseID, Status: Status(c.Query("status")),
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list zones", "INTERNAL_ERROR")
		return
	}

	apihttp.List(c, responses, apihttp.ListMeta{Page: 1, Limit: len(responses), Total: int(total), TotalPages: 1})
}

// Get godoc
//
//	@Summary		Get a zone by ID
//	@Tags			Zones
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Zone ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/zones/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid zone id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a zone
//	@Tags			Zones
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New zone"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse_id does not exist"
//	@Failure		409		{object}	apihttp.ErrorResponse	"zone code already exists in this warehouse"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/zones [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Create(c.Request.Context(), req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "zone created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a zone
//	@Tags			Zones
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Zone ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/zones/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid zone id", "INVALID_REQUEST")
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
//	@Summary		Delete a zone
//	@Description	Refuses to delete a zone that still has locations (409).
//	@Tags			Zones
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Zone ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse	"zone still has locations"
//	@Router			/zones/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid zone id", "INVALID_REQUEST")
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
		apihttp.Fail(c, http.StatusNotFound, "zone not found", "NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, ErrDuplicate):
		apihttp.Fail(c, http.StatusConflict, "zone code already exists in this warehouse", "DUPLICATE")
	case errors.Is(err, ErrHasLocations):
		apihttp.Fail(c, http.StatusConflict, "cannot delete zone: locations still exist", "HAS_LOCATIONS")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "zone operation failed", "INTERNAL_ERROR")
	}
}

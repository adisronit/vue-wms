package location

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

// Register mounts the Location endpoints on rg, gated by the same
// warehouse.* permissions as the rest of the Warehouse Setup screen.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	locations := rg.Group("/locations", middleware.RequireJWT(jwtSecret))
	locations.GET("", middleware.RequirePermission("warehouse.view"), h.List)
	locations.GET("/:id", middleware.RequirePermission("warehouse.view"), h.Get)
	locations.POST("", middleware.RequirePermission("warehouse.update"), h.Create)
	locations.PUT("/:id", middleware.RequirePermission("warehouse.update"), h.Update)
	locations.DELETE("/:id", middleware.RequirePermission("warehouse.update"), h.Delete)
}

// List godoc
//
//	@Summary		List locations for a zone
//	@Description	zone_id is required — the Location Structure tab renders bins grouped by zone.
//	@Tags			Locations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			zone_id	query		int		true	"Zone ID"
//	@Param			status	query		string	false	"ACTIVE or INACTIVE"
//	@Success		200		{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/locations [get]
func (h *Handler) List(c *gin.Context) {
	zoneID, err := strconv.ParseInt(c.Query("zone_id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "zone_id is required", "INVALID_REQUEST")
		return
	}

	responses, total, err := h.service.List(c.Request.Context(), ListParams{
		ZoneID: zoneID, Status: Status(c.Query("status")),
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list locations", "INTERNAL_ERROR")
		return
	}

	apihttp.List(c, responses, apihttp.ListMeta{Page: 1, Limit: len(responses), Total: int(total), TotalPages: 1})
}

// Get godoc
//
//	@Summary		Get a location by ID
//	@Tags			Locations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Location ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/locations/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid location id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a location
//	@Tags			Locations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New location"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"zone_id does not exist"
//	@Failure		409		{object}	apihttp.ErrorResponse	"location code already exists in this zone"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/locations [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Create(c.Request.Context(), req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "location created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a location
//	@Tags			Locations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Location ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/locations/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid location id", "INVALID_REQUEST")
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
//	@Summary		Delete a location
//	@Tags			Locations
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Location ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/locations/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid location id", "INVALID_REQUEST")
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
		apihttp.Fail(c, http.StatusNotFound, "location not found", "NOT_FOUND")
	case errors.Is(err, ErrZoneNotFound):
		apihttp.Fail(c, http.StatusNotFound, "zone not found", "ZONE_NOT_FOUND")
	case errors.Is(err, ErrDuplicate):
		apihttp.Fail(c, http.StatusConflict, "location code already exists in this zone", "DUPLICATE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "location operation failed", "INTERNAL_ERROR")
	}
}

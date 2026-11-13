package location

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

// Register mounts the Location endpoints on rg, gated by the same
// warehouse.* permissions as the rest of the Warehouse Setup screen —
// unchanged by Location Master Management, so the existing Warehouse
// Setup drawer's Location Structure tab keeps working exactly as
// before. What IS new: the RequireJWT -> RequireTenantContext ->
// RequirePermission chain (internal/productcategory/handler.go's doc
// explains why this chain, rather than the RequireJWT-only chain this
// package itself used before this phase). Every handler method below
// reads its tenant_id exclusively from middleware.EnforcedTenantID —
// the caller's own re-verified active tenant — never from a path,
// query, or body parameter.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	locations := rg.Group("/locations",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	locations.GET("", middleware.RequirePermission("warehouse.view"), h.List)
	locations.GET("/:id", middleware.RequirePermission("warehouse.view"), h.Get)
	locations.POST("", middleware.RequirePermission("warehouse.update"), h.Create)
	locations.PUT("/:id", middleware.RequirePermission("warehouse.update"), h.Update)
	locations.DELETE("/:id", middleware.RequirePermission("warehouse.update"), h.Delete)
}

// List godoc
//
//	@Summary		List locations for a zone
//	@Description	zone_id is required — the Location Structure tab and the standalone Location Management page both render bins grouped by zone. Scoped to the caller's own active tenant.
//	@Tags			Locations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			zone_id	query		int		true	"Zone ID"
//	@Param			search	query		string	false	"Matches code or name"
//	@Param			status	query		string	false	"ACTIVE or INACTIVE"
//	@Param			page	query		int		false	"Page number"			default(1)
//	@Param			limit	query		int		false	"Page size (max 500)"	default(500)
//	@Success		200		{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/locations [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	zoneID, err := strconv.ParseInt(c.Query("zone_id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "zone_id is required", "INVALID_REQUEST")
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	responses, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		ZoneID: zoneID, Search: c.Query("search"), Status: Status(c.Query("status")), Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list locations", "INTERNAL_ERROR")
		return
	}

	effectivePage, effectiveLimit := page, limit
	if effectivePage < 1 {
		effectivePage = 1
	}
	if effectiveLimit < 1 || effectiveLimit > 500 {
		effectiveLimit = 500
	}
	totalPages := int((total + int64(effectiveLimit) - 1) / int64(effectiveLimit))
	if totalPages < 1 {
		totalPages = 1
	}

	apihttp.List(c, responses, apihttp.ListMeta{Page: effectivePage, Limit: effectiveLimit, Total: int(total), TotalPages: totalPages})
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid location id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

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

	resp, err := h.service.Update(c.Request.Context(), tenantID, id, req)
	writeResult(c, resp, err)
}

// Delete godoc
//
//	@Summary		Delete a location
//	@Description	Blocked (409) while on-hand inventory still exists at this location.
//	@Tags			Locations
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Location ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse
//	@Router			/locations/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid location id", "INVALID_REQUEST")
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
		apihttp.Fail(c, http.StatusNotFound, "location not found", "NOT_FOUND")
	case errors.Is(err, ErrZoneNotFound):
		apihttp.Fail(c, http.StatusNotFound, "zone not found", "ZONE_NOT_FOUND")
	case errors.Is(err, ErrDuplicate):
		apihttp.Fail(c, http.StatusConflict, "location code already exists in this zone", "DUPLICATE")
	case errors.Is(err, ErrHasStock):
		apihttp.Fail(c, http.StatusConflict, "cannot delete — stock still exists in this location", "HAS_STOCK")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "location operation failed", "INTERNAL_ERROR")
	}
}

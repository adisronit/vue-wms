package customerdeliverylocation

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

// Register mounts the Customer Delivery Location endpoints as their
// own top-level group — mirrors internal/zone's own relationship to
// internal/warehouse (a required customer_id query param on List,
// rather than a nested /customers/:id/... path) — gated by the same
// customer.* permissions as the parent Customer screen: delivery
// locations have no separate nav entry of their own, only a tab within
// the Customer drawer, the same "reuse the parent's permissions"
// precedent internal/product's image endpoints and internal/zone's own
// Register doc both already establish.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	locations := rg.Group("/customer-delivery-locations",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	locations.GET("", middleware.RequirePermission("customer.view"), h.List)
	locations.GET("/:id", middleware.RequirePermission("customer.view"), h.Get)
	locations.POST("", middleware.RequirePermission("customer.create"), h.Create)
	locations.PUT("/:id", middleware.RequirePermission("customer.update"), h.Update)
	locations.DELETE("/:id", middleware.RequirePermission("customer.delete"), h.Delete)
}

// List godoc
//
//	@Summary		List a customer's delivery locations
//	@Description	customer_id is required — the Delivery Locations tab only ever lists one open customer's own locations. Scoped to the caller's own active tenant.
//	@Tags			Customer Delivery Locations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			customer_id	query		int		true	"Customer ID"
//	@Param			search		query		string	false	"Matches code or name"
//	@Param			status		query		string	false	"ACTIVE or INACTIVE"
//	@Success		200			{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		400			{object}	apihttp.ErrorResponse
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse	"customer not found"
//	@Router			/customer-delivery-locations [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	customerID, err := strconv.ParseInt(c.Query("customer_id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "customer_id is required", "INVALID_REQUEST")
		return
	}

	responses, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		CustomerID: customerID, Search: c.Query("search"), Status: Status(c.Query("status")),
	})
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			apihttp.Fail(c, http.StatusNotFound, "customer not found", "CUSTOMER_NOT_FOUND")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list delivery locations", "INTERNAL_ERROR")
		return
	}

	apihttp.List(c, responses, apihttp.ListMeta{Page: 1, Limit: len(responses), Total: len(responses), TotalPages: 1})
}

// Get godoc
//
//	@Summary		Get a delivery location by ID
//	@Tags			Customer Delivery Locations
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Delivery Location ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/customer-delivery-locations/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid delivery location id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	writeResult(c, resp, err)
}

// Create godoc
//
//	@Summary		Create a delivery location
//	@Tags			Customer Delivery Locations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New delivery location"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"customer not found"
//	@Failure		409		{object}	apihttp.ErrorResponse	"code already exists for this customer"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/customer-delivery-locations [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "delivery location created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a delivery location
//	@Tags			Customer Delivery Locations
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Delivery Location ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse	"would leave the customer without a default location"
//	@Router			/customer-delivery-locations/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid delivery location id", "INVALID_REQUEST")
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
//	@Summary		Delete a delivery location
//	@Description	Refuses (422) to delete the customer's current default location while other ACTIVE locations still exist.
//	@Tags			Customer Delivery Locations
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Delivery Location ID"
//	@Success		204	"deleted"
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		422	{object}	apihttp.ErrorResponse
//	@Router			/customer-delivery-locations/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid delivery location id", "INVALID_REQUEST")
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
		apihttp.Fail(c, http.StatusNotFound, "delivery location not found", "NOT_FOUND")
	case errors.Is(err, ErrCustomerNotFound):
		apihttp.Fail(c, http.StatusNotFound, "customer not found", "CUSTOMER_NOT_FOUND")
	case errors.Is(err, ErrDuplicateCode):
		apihttp.Fail(c, http.StatusConflict, "code already exists for this customer", "DUPLICATE_CODE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "delivery location operation failed", "INTERNAL_ERROR")
	}
}

package loading

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"wms-backend/internal/platform/dbctx"
	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// Handler exposes Service (and, for the Loading Manifest, PDFService)
// over HTTP.
type Handler struct {
	service *Service
	pdf     *PDFService
}

func NewHandler(service *Service, pdf *PDFService) *Handler {
	return &Handler{service: service, pdf: pdf}
}

// servePDF writes data as a PDF response — matches every other module's
// own servePDF helper exactly (Phase 33D.3).
func servePDF(c *gin.Context, data []byte, filename string) {
	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

// Register mounts the Loading endpoints on rg, using the same
// RequireJWT -> RequireTenantContext -> RequirePermission chain every
// tenant-scoped inbound/outbound module uses.
//
// Permission mapping (loading.view/create/update/confirm/approve, the
// same five-action shape picking/relocation/packing already use):
//   - view: List/Sources/Get — read-only.
//   - create: CreateTrip — opens a new trip header.
//   - update: AssignShipment/ScanCarton/ShortCarton — the ordinary
//     loading-dock execution actions.
//   - confirm: ConfirmTrip — the action that transitions the trip to
//     COMPLETED and calls shipping.Service.MarkLoaded for every
//     shipment on it — the real dispatch-gate payoff, gated at the
//     same elevated authority level receiving.confirm/picking.confirm/
//     packing.confirm/shipping.confirm already established for their
//     own module's primary significant action.
//   - approve: CancelTrip — the more consequential, planning-level
//     action, matching picking.approve/packing.approve's own Cancel
//     gate.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("/loading-trips",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("", middleware.RequirePermission("loading.view"), h.List)
	group.GET("/sources", middleware.RequirePermission("loading.view"), h.Sources)
	group.GET("/:id", middleware.RequirePermission("loading.view"), h.Get)
	// Phase 33D.3 — the Loading Manifest PDF rides on the same
	// loading.view permission as Get, matching every other document-
	// owning module's own "view == can print" precedent.
	group.GET("/:id/manifest", middleware.RequirePermission("loading.view"), h.Manifest)
	group.POST("", middleware.RequirePermission("loading.create"), h.CreateTrip)
	group.POST("/:id/shipments", middleware.RequirePermission("loading.update"), h.AssignShipment)
	group.POST("/:id/scan", middleware.RequirePermission("loading.update"), h.ScanCarton)
	group.POST("/:id/short", middleware.RequirePermission("loading.update"), h.ShortCarton)
	group.POST("/:id/confirm", middleware.RequirePermission("loading.confirm"), h.ConfirmTrip)
	group.POST("/:id/cancel", middleware.RequirePermission("loading.approve"), h.CancelTrip)
}

// List godoc
//
//	@Summary		List loading trips
//	@Description	Trip-level rows with shipment/carton progress counts — supports search plus a status filter, with pagination.
//	@Tags			Loading
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page	query		int		false	"Page number"			default(1)
//	@Param			limit	query		int		false	"Page size (max 100)"	default(20)
//	@Param			search	query		string	false	"Matches trip no, vehicle no, or driver name"
//	@Param			status	query		string	false	"PLANNED, LOADING, COMPLETED, or CANCELLED"
//	@Success		200		{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/loading-trips [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	responses, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")), Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list loading trips", "INTERNAL_ERROR")
		return
	}

	effectivePage, effectiveLimit := page, limit
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

// Sources godoc
//
//	@Summary		List READY shipments not yet assigned to an active trip
//	@Description	Powers the "assign shipment to trip" picker — read-only.
//	@Tags			Loading
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=[]SourceResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/loading-trips/sources [get]
func (h *Handler) Sources(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	sources, err := h.service.Sources(c.Request.Context(), tenantID)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list assignable shipments", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, sources, "")
}

// Get godoc
//
//	@Summary		Get a loading trip with its assigned shipments and carton worklist
//	@Tags			Loading
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Loading trip ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/loading-trips/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid loading trip id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "loading trip not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get loading trip", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Manifest godoc
//
//	@Summary		Download the Loading Manifest PDF for a trip
//	@Description	Header (trip no./vehicle/driver/carrier/dock/route/scheduled vs actual loading time/vehicle seal/status), one section per assigned shipment each with its own carton worklist (LPN, planned vs loaded/short, short reason), a trip-wide planned/loaded/short summary, and a driver/loader signature block. Marked DRAFT until the trip is actually COMPLETED. Read-only — never mutates the trip, its shipments, or its cartons. ?download=true for an attachment, omitted for inline viewing.
//	@Tags			Loading
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Loading trip ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/loading-trips/{id}/manifest [get]
func (h *Handler) Manifest(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid loading trip id", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	data, filename, err := h.pdf.RenderLoadingManifest(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	servePDF(c, data, filename)
}

// CreateTrip godoc
//
//	@Summary		Create a new loading trip header
//	@Tags			Loading
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New loading trip"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/loading-trips [post]
func (h *Handler) CreateTrip(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	var req CreateRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.CreateTrip(ctx, tenantID, req, userID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "loading trip created")
		return
	}
	writeError(c, err)
}

// AssignShipment godoc
//
//	@Summary		Assign a READY shipment to a loading trip
//	@Description	Validates the shipment exists, belongs to the caller's own tenant, is READY, and is not already assigned to another active trip. Auto-populates the trip's own scan-to-load worklist from the shipment's own packing job's own cartons.
//	@Tags			Loading
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Loading trip ID"
//	@Param			request	body		AssignShipmentRequest	true	"Shipment to assign"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"shipment or packing job not found"
//	@Failure		409		{object}	apihttp.ErrorResponse	"trip not in an assignable status, or shipment already assigned to another active trip"
//	@Failure		422		{object}	apihttp.ErrorResponse	"shipment is not READY, or its packing job has no cartons"
//	@Router			/loading-trips/{id}/shipments [post]
func (h *Handler) AssignShipment(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid loading trip id", "INVALID_REQUEST")
		return
	}
	var req AssignShipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.AssignShipment(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "shipment assigned to trip")
		return
	}
	writeError(c, err)
}

// ScanCarton godoc
//
//	@Summary		Scan a carton as loaded onto the vehicle
//	@Description	Resolves the scanned LPN against this trip's own planned worklist — rejects a carton that belongs to a different trip or was never planned at all, and rejects (rather than silently no-op'ing) a carton already scanned loaded.
//	@Tags			Loading
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Loading trip ID"
//	@Param			request	body		ScanCartonRequest	true	"Carton LPN"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"carton not found, or not planned for this trip"
//	@Failure		409		{object}	apihttp.ErrorResponse	"trip not scannable, or carton already loaded/resolved"
//	@Router			/loading-trips/{id}/scan [post]
func (h *Handler) ScanCarton(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid loading trip id", "INVALID_REQUEST")
		return
	}
	var req ScanCartonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.ScanCarton(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "carton scanned loaded")
		return
	}
	writeError(c, err)
}

// ShortCarton godoc
//
//	@Summary		Flag a planned carton as unable to be loaded (damaged/missing)
//	@Tags			Loading
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Loading trip ID"
//	@Param			request	body		ShortCartonRequest	true	"Carton LPN, reason, optional remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"carton not found, or not planned for this trip"
//	@Failure		409		{object}	apihttp.ErrorResponse	"trip not scannable, or carton already loaded/resolved"
//	@Failure		422		{object}	apihttp.ErrorResponse	"invalid reason"
//	@Router			/loading-trips/{id}/short [post]
func (h *Handler) ShortCarton(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid loading trip id", "INVALID_REQUEST")
		return
	}
	var req ShortCartonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.ShortCarton(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "carton flagged short")
		return
	}
	writeError(c, err)
}

// ConfirmTrip godoc
//
//	@Summary		Confirm a loading trip — transitions it to COMPLETED
//	@Description	Requires the trip to already be LOADING and every one of its own planned cartons to be LOADED or flagged SHORT. On success, calls shipping.Service.MarkLoaded for every shipment on this trip — the real dispatch-gate payoff: internal/shipping's own Confirm action will now allow DISPATCH for each of them.
//	@Tags			Loading
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Loading trip ID"
//	@Param			request	body		ConfirmRequest	false	"Optional remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"trip not LOADING, or not every carton is resolved yet"
//	@Router			/loading-trips/{id}/confirm [post]
func (h *Handler) ConfirmTrip(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid loading trip id", "INVALID_REQUEST")
		return
	}
	var req ConfirmRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.ConfirmTrip(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "loading trip completed")
		return
	}
	writeError(c, err)
}

// CancelTrip godoc
//
//	@Summary		Cancel a loading trip
//	@Description	Allowed from PLANNED or LOADING. Never calls shipping.Service.MarkLoaded — this trip's own shipments remain not-loaded (still blocked from dispatch) and are free to be assigned to a brand new trip afterward.
//	@Tags			Loading
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Loading trip ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"trip already completed or cancelled"
//	@Router			/loading-trips/{id}/cancel [post]
func (h *Handler) CancelTrip(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid loading trip id", "INVALID_REQUEST")
		return
	}
	var req CancelRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.CancelTrip(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "loading trip cancelled")
		return
	}
	writeError(c, err)
}

func parseID(c *gin.Context, param string) (int64, error) {
	return strconv.ParseInt(c.Param(param), 10, 64)
}

func writeError(c *gin.Context, err error) {
	var validationErr ErrValidation

	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "loading trip not found", "NOT_FOUND")
	case errors.Is(err, ErrCartonNotFound):
		apihttp.Fail(c, http.StatusNotFound, "carton not found", "CARTON_NOT_FOUND")
	case errors.Is(err, ErrCartonNotPlannedForTrip):
		apihttp.Fail(c, http.StatusNotFound, err.Error(), "CARTON_NOT_PLANNED")
	case errors.Is(err, ErrShipmentNotFound):
		apihttp.Fail(c, http.StatusNotFound, "shipment not found", "SHIPMENT_NOT_FOUND")
	case errors.Is(err, ErrPackingJobNotFound):
		apihttp.Fail(c, http.StatusNotFound, "packing job not found", "PACKING_JOB_NOT_FOUND")
	case errors.Is(err, ErrShipmentAlreadyAssigned):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "SHIPMENT_ALREADY_ASSIGNED")
	case errors.Is(err, ErrCartonAlreadyLoaded):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "CARTON_ALREADY_LOADED")
	case errors.Is(err, ErrCartonAlreadyResolved):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "CARTON_ALREADY_RESOLVED")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "loading trip is not in a state that allows this action", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotReadyToConfirm):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "NOT_READY_TO_CONFIRM")
	case errors.Is(err, ErrShipmentNotReady):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "SHIPMENT_NOT_READY")
	case errors.Is(err, ErrNoCartons):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_CARTONS")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "loading operation failed", "INTERNAL_ERROR")
	}
}

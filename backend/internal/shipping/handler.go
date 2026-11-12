package shipping

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/platform/dbctx"
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

// Register mounts the Shipping endpoints on rg. Permissions reuse the
// existing "shipping" module from Phase 2's catalog exactly as-is — no
// new codes. Unlike Picking (whose PICKING role has an unusual
// view+update-only grant shape), the SHIPPING role's own Phase 2/3 seed
// grants (internal/rbac/catalog.go) mirror RECEIVING/PUTAWAY exactly:
// view/create/update, no approve. So Confirm (the action that actually
// transitions a shipment — dispatch/deliver/issue) is gated on
// shipping.approve, the same significant-action permission Receiving/
// Putaway's own Confirm already uses — under current seed data only
// ADMIN/WAREHOUSE_MANAGER have it, the same segregation-of-duties
// consequence already established for those two modules.
// Phase 15's Cancel is gated on shipping.update, matching Receiving/
// Putaway's own Cancel reasoning: it only ever runs pre-dispatch
// (Status.CanCancel is READY-only, the same boundary Update's own
// CanEdit already uses), so it sits at editing's authority level, not
// approve's dispatch-confirming level.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	shippingGroup := rg.Group("/shipping", middleware.RequireJWT(jwtSecret))
	shippingGroup.GET("", middleware.RequirePermission("shipping.view"), h.List)
	shippingGroup.GET("/export", middleware.RequirePermission("shipping.view"), h.Export)
	shippingGroup.GET("/sources", middleware.RequirePermission("shipping.view"), h.Sources)
	shippingGroup.GET("/:id", middleware.RequirePermission("shipping.view"), h.Get)
	shippingGroup.POST("", middleware.RequirePermission("shipping.create"), h.Create)
	shippingGroup.PUT("/:id", middleware.RequirePermission("shipping.update"), h.Update)
	shippingGroup.POST("/:id/confirm", middleware.RequirePermission("shipping.approve"), h.Confirm)
	shippingGroup.POST("/:id/cancel", middleware.RequirePermission("shipping.update"), h.Cancel)
}

// List godoc
//
//	@Summary		List shipments
//	@Description	Supports the Shipping screen's search box (shipment no, order no, customer, or tracking no) plus status/carrier/date filters, with pagination.
//	@Tags			Shipping
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page		query		int		false	"Page number"			default(1)
//	@Param			limit		query		int		false	"Page size (max 100)"	default(20)
//	@Param			search		query		string	false	"Matches shipment no, order no, customer, or tracking no"
//	@Param			status		query		string	false	"READY, DISPATCHED, IN_TRANSIT, DELIVERED, or ISSUE"
//	@Param			carrier		query		string	false	"DHL, KERRY, FLASH, or INTERNAL"
//	@Param			date		query		string	false	"Filter by ship date (YYYY-MM-DD)"
//	@Success		200			{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/shipping [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	responses, total, err := h.service.List(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")), Carrier: c.Query("carrier"),
		Date: date, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list shipments", "INTERNAL_ERROR")
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

// Export godoc
//
//	@Summary		Export Shipping History as CSV
//	@Description	Synchronous CSV export (Phase 22) — same search/status/carrier/date filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on the same shipping.view permission as the list itself.
//	@Tags			Shipping
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search		query	string	false	"Matches shipment no, order no, or customer"
//	@Param			status		query	string	false	"READY, DISPATCHED, IN_TRANSIT, DELIVERED, ISSUE, or CANCELLED"
//	@Param			carrier		query	string	false	"Filter by carrier"
//	@Param			date		query	string	false	"Filter by ship date (YYYY-MM-DD)"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/shipping/export [get]
func (h *Handler) Export(c *gin.Context) {
	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	rows, err := h.service.ListForExport(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")), Carrier: c.Query("carrier"), Date: date,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export shipments", "INTERNAL_ERROR")
		return
	}

	headers := []string{"shipment_no", "order_no", "customer", "carrier", "tracking_no", "packages", "weight", "ship_date", "status"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		weight := ""
		if r.Weight != nil {
			weight = csvexport.FormatFloat(*r.Weight)
		}
		csvRows = append(csvRows, []string{
			r.ShipmentNo, r.OrderNo, csvexport.FormatString(r.Customer), csvexport.FormatString(r.Carrier),
			csvexport.FormatString(r.TrackingNo), strconv.Itoa(r.Packages), weight,
			csvexport.FormatTime(r.ShipDate), string(r.Status),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "shipping_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Sources godoc
//
//	@Summary		List completed picking tasks not yet shipped
//	@Description	Powers ShippingView.vue's "+ New Shipment" flow with a real source picker — the mock itself has no way to choose which order to ship.
//	@Tags			Shipping
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=[]SourceResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/shipping/sources [get]
func (h *Handler) Sources(c *gin.Context) {
	sources, err := h.service.Sources(c.Request.Context())
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list shippable picking tasks", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, sources, "")
}

// Get godoc
//
//	@Summary		Get a shipment with its lines
//	@Tags			Shipping
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Shipment ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/shipping/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid shipment id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "shipment not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get shipment", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a shipment from a completed picking task
//	@Description	Copies the linked picking task's already-picked lines verbatim (no item quantities are ever accepted from the client) — a new shipment always starts READY; use POST /shipping/{id}/confirm to dispatch/deliver/flag it. One shipment per picking task is enforced (409 if the task is already shipped).
//	@Tags			Shipping
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New shipment"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"picking task does not exist"
//	@Failure		409		{object}	apihttp.ErrorResponse	"picking task already has a shipment"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/shipping [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Create(ctx, req, userID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "shipment created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a shipment header
//	@Description	Only allowed while the shipment is still READY (see Status.CanEdit).
//	@Tags			Shipping
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Shipment ID"
//	@Param			request	body		UpdateRequest	true	"Updated header fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"shipment can no longer be edited"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/shipping/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid shipment id", "INVALID_REQUEST")
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Update(c.Request.Context(), id, req)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Dispatch, deliver, or report an issue on a shipment
//	@Description	Action DISPATCH requires carrier and tracking_no to already be set (via Create/Update) and moves READY -> DISPATCHED. Action DELIVER moves DISPATCHED -> DELIVERED. Action ISSUE requires a non-empty remark — either already set via Create/Update, or supplied directly in this request's remark field (needed because header edits are only allowed while READY, but an issue may legitimately be reported after DISPATCHED) — and moves READY or DISPATCHED -> ISSUE. No inventory mutation occurs here — Picking (Phase 9) already removed the stock; this only transitions the shipment's own status. idempotency_key must be a fresh UUID per click; resending the same key returns the shipment's current state instead of applying the transition twice.
//	@Tags			Shipping
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Shipment ID"
//	@Param			request	body		ConfirmRequest	true	"Action to apply"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"shipment is not in a state that allows this action"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/shipping/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid shipment id", "INVALID_REQUEST")
		return
	}

	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Confirm(ctx, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "shipment updated")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a shipment
//	@Description	Marks the shipment CANCELLED. Only allowed while it is still READY — before dispatch. Never mutates inventory: Picking already removed the stock for these lines when it picked them, and cancelling here does not restore it (this is not an un-pick action). Rejected with 409 once dispatched, delivered, flagged ISSUE, or already cancelled. Phase 18: on success, records cancelled_at (server UTC time) and cancelled_by (the authenticated caller) on the shipment, plus the optional request remark in cancel_remark (a dedicated column — the shipment's own pre-existing remark is left untouched). These audit fields are returned in the detail response (cancelled_at/cancelled_by/cancelled_by_name/cancel_remark) and are only ever set once: a rejected duplicate/invalid cancel attempt (409) never reaches the write and cannot overwrite them.
//	@Tags			Shipping
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Shipment ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"shipment is not in a state that allows this action (already dispatched, delivered, issue-flagged, or cancelled)"
//	@Router			/shipping/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid shipment id", "INVALID_REQUEST")
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

	resp, err := h.service.Cancel(ctx, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "shipment cancelled")
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
		apihttp.Fail(c, http.StatusNotFound, "shipment not found", "NOT_FOUND")
	case errors.Is(err, ErrPickingTaskNotFound):
		apihttp.Fail(c, http.StatusNotFound, "picking task not found", "PICKING_TASK_NOT_FOUND")
	case errors.Is(err, ErrNotShippable):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NOT_SHIPPABLE")
	case errors.Is(err, ErrNoShippableItems):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_SHIPPABLE_ITEMS")
	case errors.Is(err, ErrAlreadyShipped):
		apihttp.Fail(c, http.StatusConflict, "picking task already has a shipment", "ALREADY_SHIPPED")
	case errors.Is(err, ErrNotEditable):
		apihttp.Fail(c, http.StatusConflict, "shipment can no longer be edited", "NOT_EDITABLE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "shipment is not in a state that allows this action", "INVALID_TRANSITION")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "shipping operation failed", "INTERNAL_ERROR")
	}
}

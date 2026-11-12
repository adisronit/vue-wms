package putaway

import (
	"errors"
	"net/http"
	"strconv"

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

// Register mounts the Putaway endpoints on rg. Permissions reuse the
// existing "putaway" module from Phase 2/3's catalog exactly as-is — no
// new codes. Confirm (the action that actually moves stock) is gated on
// putaway.approve, the same significant-action permission Receiving's
// Confirm already used for its own module: per the current seed data
// only ADMIN/WAREHOUSE_MANAGER have it — PUTAWAY staff can create tasks
// but not confirm them, the same segregation-of-duties consequence of
// grants already established in Phase 2/3 (see PROJECT_STATUS.md).
//
// Phase 15's Cancel is gated on putaway.update instead, matching
// Receiving's own Cancel reasoning: it only ever runs before any line
// has moved stock (Service.Cancel's own MovedQty check backs this up
// regardless of header status), so it sits at the same authority level
// as managing a task PUTAWAY staff already have — not at approve's
// stock-moving authority level.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	putawayGroup := rg.Group("/putaway", middleware.RequireJWT(jwtSecret))
	putawayGroup.GET("", middleware.RequirePermission("putaway.view"), h.List)
	putawayGroup.GET("/export", middleware.RequirePermission("putaway.view"), h.Export)
	putawayGroup.GET("/sources/:receiptId", middleware.RequirePermission("putaway.view"), h.Sources)
	putawayGroup.GET("/:id", middleware.RequirePermission("putaway.view"), h.Get)
	putawayGroup.POST("", middleware.RequirePermission("putaway.create"), h.Create)
	putawayGroup.POST("/:id/confirm", middleware.RequirePermission("putaway.approve"), h.Confirm)
	putawayGroup.POST("/:id/cancel", middleware.RequirePermission("putaway.update"), h.Cancel)
}

// List godoc
//
//	@Summary		List putaway tasks
//	@Description	Item-level rows matching the Putaway screen's table — supports its search box plus status/warehouse/zone filters, with pagination.
//	@Tags			Putaway
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches putaway no, receipt no, SKU, product name, or destination location"
//	@Param			status			query		string	false	"PENDING, PARTIALLY_COMPLETED, COMPLETED, or ISSUE (item status)"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			zone_id			query		int		false	"Filter by destination zone"
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/putaway [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	zoneID, _ := strconv.ParseInt(c.Query("zone_id"), 10, 64)

	responses, total, err := h.service.List(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")),
		WarehouseID: warehouseID, ZoneID: zoneID, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list putaway tasks", "INTERNAL_ERROR")
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
//	@Summary		Export Putaway History as CSV
//	@Description	Synchronous CSV export (Phase 22) — same search/status/warehouse_id/zone_id filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on the same putaway.view permission as the list itself.
//	@Tags			Putaway
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches putaway no or SKU"
//	@Param			status			query	string	false	"PENDING, PARTIALLY_COMPLETED, COMPLETED, or ISSUE"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			zone_id			query	int		false	"Filter by destination zone"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/putaway/export [get]
func (h *Handler) Export(c *gin.Context) {
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	zoneID, _ := strconv.ParseInt(c.Query("zone_id"), 10, 64)

	rows, err := h.service.ListForExport(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")),
		WarehouseID: warehouseID, ZoneID: zoneID,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export putaway tasks", "INTERNAL_ERROR")
		return
	}

	headers := []string{"putaway_no", "receipt_no", "sku", "product_name", "requested_qty", "moved_qty", "source_location", "destination_location", "status"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			r.PutawayNo, csvexport.FormatString(r.ReceiptNo), r.SKU, r.ProductName,
			csvexport.FormatFloat(r.RequestedQty), csvexport.FormatFloat(r.MovedQty),
			r.SourceLocationCode, r.DestinationLocationCode, string(r.Status),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "putaway_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Sources godoc
//
//	@Summary		List a receipt's lines still eligible for putaway
//	@Description	Powers NewPutawayView.vue's "Source Document" selector — only lines with received_qty greater than what has already been put away are returned.
//	@Tags			Putaway
//	@Produce		json
//	@Security		BearerAuth
//	@Param			receiptId	path		int	true	"Receipt ID"
//	@Success		200			{object}	apihttp.SuccessResponse{data=[]SourceLineResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/putaway/sources/{receiptId} [get]
func (h *Handler) Sources(c *gin.Context) {
	receiptID, err := parseID(c, "receiptId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}

	lines, err := h.service.Sources(c.Request.Context(), receiptID)
	if errors.Is(err, ErrReceiptNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "receipt not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to load source lines", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, lines, "")
}

// Get godoc
//
//	@Summary		Get a putaway task with its lines
//	@Tags			Putaway
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Putaway task ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/putaway/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid putaway task id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "putaway task not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get putaway task", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a putaway task
//	@Description	Embeds lines in one call — the "Save Draft"/"Confirm Putaway" buttons both submit the whole task (header + lines) at once. A new task always starts PENDING; use POST /putaway/{id}/confirm to actually move stock.
//	@Tags			Putaway
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New putaway task"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse, receipt, product, or location does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/putaway [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "putaway task created")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm (or report an issue on) one or more putaway lines
//	@Description	Atomically moves each CONFIRMED line's quantity via Inventory Core's MoveStock (same transaction) — never AddStock, so total system-wide quantity for the moved product/lot is unchanged — and recomputes the task's status; all committed together or all rolled back. ISSUE lines record a remark and move nothing. idempotency_key must be a fresh UUID per click; resending the same key returns the task's current state instead of moving stock twice. Can be called more than once on the same task for partial putaway.
//	@Tags			Putaway
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Putaway task ID"
//	@Param			request	body		ConfirmRequest	true	"Lines to confirm or flag, with this batch's quantity for each"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a confirmable status, or a line is already resolved"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/putaway/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid putaway task id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "putaway confirmed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a putaway task
//	@Description	Marks the task CANCELLED. Only allowed while no line has actually moved stock yet — rejected with 409 both when the header itself is already terminal (COMPLETED/ISSUE/CANCELLED) and when the header is still IN_PROGRESS solely because a line was flagged ISSUE while another line has already moved stock. Never calls MoveStock to compensate for an already-completed move. Phase 18: on success, records cancelled_at (server UTC time) and cancelled_by (the authenticated caller) on the task, plus the optional request remark in cancel_remark (a dedicated column — the task's own pre-existing remark is left untouched). These audit fields are returned in the detail response (cancelled_at/cancelled_by/cancelled_by_name/cancel_remark) and are only ever set once: a rejected duplicate/invalid cancel attempt (409) never reaches the write and cannot overwrite them.
//	@Tags			Putaway
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Putaway task ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a cancellable status, or stock has already been moved for one or more lines"
//	@Router			/putaway/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid putaway task id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "putaway task cancelled")
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
		apihttp.Fail(c, http.StatusNotFound, "putaway task not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "putaway task item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrReceiptNotFound):
		apihttp.Fail(c, http.StatusNotFound, "receipt not found", "RECEIPT_NOT_FOUND")
	case errors.Is(err, ErrNoSourceStock):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_SOURCE_STOCK")
	case errors.Is(err, ErrCrossWarehouse):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "CROSS_WAREHOUSE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "putaway task is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "putaway task is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.Is(err, ErrCancelHasMovedStock):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "CANCEL_HAS_MOVED_STOCK")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "putaway operation failed", "INTERNAL_ERROR")
	}
}

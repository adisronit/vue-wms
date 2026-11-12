package picking

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

// Register mounts the Picking endpoints on rg. Permissions reuse the
// existing "picking" module from Phase 2's catalog exactly as-is — no
// new codes. Unlike Receiving/Putaway, Confirm is gated on
// picking.update, not picking.approve: per the existing Phase 2 seed
// (internal/rbac/catalog.go), the PICKING role itself was only ever
// granted picking.view/picking.update — not picking.create and not
// picking.approve. That shape only makes sense if picking staff execute
// already-created tasks (view + update) rather than build or approve
// them, so Create is gated on picking.create (ADMIN/WAREHOUSE_MANAGER
// only, under current grants) while the per-line pick/short-pick
// execution actions — which PickingView.vue never modeled as a separate
// supervisor approval step the way Receiving/Putaway's Confirm is — are
// gated on picking.update, the permission PICKING staff actually have.
//
// Phase 13's Cancel is gated on picking.approve instead — the one
// existing picking.* code no endpoint had used until now (the comment
// above already flagged it as "reserved for a future approval
// workflow"). Cancelling a task is a more consequential, planning-level
// decision than confirming one line (it releases a reservation and ends
// the whole task, which a picker executing it in the field has no
// business doing unilaterally), so it belongs at the same authority
// level as Create rather than at picking.update's execution level.
// Under current seed data this again means ADMIN/WAREHOUSE_MANAGER
// only, the same as Create.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	pickingGroup := rg.Group("/picking", middleware.RequireJWT(jwtSecret))
	pickingGroup.GET("", middleware.RequirePermission("picking.view"), h.List)
	pickingGroup.GET("/export", middleware.RequirePermission("picking.view"), h.Export)
	pickingGroup.GET("/:id", middleware.RequirePermission("picking.view"), h.Get)
	pickingGroup.POST("", middleware.RequirePermission("picking.create"), h.Create)
	pickingGroup.POST("/:id/confirm", middleware.RequirePermission("picking.update"), h.Confirm)
	pickingGroup.POST("/:id/cancel", middleware.RequirePermission("picking.approve"), h.Cancel)
}

// List godoc
//
//	@Summary		List picking tasks
//	@Description	Task-level rows matching the Picking screen's table — supports its search box (pick no, order no, customer, or a line's SKU/product name) plus status/priority/warehouse/date filters, with pagination.
//	@Tags			Picking
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches pick no, order no, customer, or a line's SKU/product name"
//	@Param			status			query		string	false	"PENDING, IN_PROGRESS, COMPLETED, SHORT_PICK, or CANCELLED"
//	@Param			priority		query		string	false	"LOW, NORMAL, or HIGH"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			date			query		string	false	"Filter by due date (YYYY-MM-DD)"
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/picking [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	responses, total, err := h.service.List(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")), Priority: Priority(c.Query("priority")),
		WarehouseID: warehouseID, Date: date, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list picking tasks", "INTERNAL_ERROR")
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
//	@Summary		Export Picking History as CSV
//	@Description	Synchronous CSV export (Phase 22) — same search/status/priority/warehouse_id/date filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on the same picking.view permission as the list itself.
//	@Tags			Picking
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches pick no, order no, or customer"
//	@Param			status			query	string	false	"PENDING, IN_PROGRESS, COMPLETED, SHORT_PICK, or CANCELLED"
//	@Param			priority		query	string	false	"LOW, NORMAL, or HIGH"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			date			query	string	false	"Filter by due date (YYYY-MM-DD)"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/picking/export [get]
func (h *Handler) Export(c *gin.Context) {
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	rows, err := h.service.ListForExport(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")), Priority: Priority(c.Query("priority")),
		WarehouseID: warehouseID, Date: date,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export picking tasks", "INTERNAL_ERROR")
		return
	}

	headers := []string{"pick_no", "order_no", "customer", "warehouse_code", "priority", "item_count", "progress_pct", "due_date", "status"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			r.PickNo, r.OrderNo, csvexport.FormatString(r.Customer), r.WarehouseCode, string(r.Priority),
			strconv.Itoa(r.ItemCount), csvexport.FormatFloat(r.ProgressPct), csvexport.FormatTime(r.DueDate), string(r.Status),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "picking_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Get godoc
//
//	@Summary		Get a picking task with its lines
//	@Tags			Picking
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Picking task ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/picking/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "picking task not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get picking task", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a picking task
//	@Description	Embeds lines in one call — the drawer's "+ New Picking Task" flow submits the whole task (header + lines) at once. A new task always starts PENDING; use POST /picking/{id}/confirm to actually remove stock. There is no source-order lookup (no Sales Order module exists yet) — order_no/customer are free text, matching PickingView.vue's own mock fields.
//	@Tags			Picking
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New picking task"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse, product, or location does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/picking [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "picking task created")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm (or report a short pick on) one or more picking lines
//	@Description	Atomically removes each PICKED line's quantity via Inventory Core's RemoveStock (same transaction) and recomputes the task's status; all committed together or all rolled back. SHORT lines record a remark and remove nothing. idempotency_key must be a fresh UUID per click; resending the same key returns the task's current state instead of removing stock twice. Can be called more than once on the same task for partial picking.
//	@Tags			Picking
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Picking task ID"
//	@Param			request	body		ConfirmRequest	true	"Lines to confirm or flag, with this batch's quantity for each"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a confirmable status, or a line is already resolved"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/picking/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "picking confirmed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a picking task
//	@Description	Releases every still-open line's remaining reservation (via Inventory Core's ReleaseReservationTx) and marks the task CANCELLED, atomically. Never removes stock and never writes an inventory ledger row — cancellation only ever undoes a reservation, never a real movement. Lines already COMPLETED or SHORT keep their already-picked stock deductions untouched; only their outstanding (never-picked) quantity, if any, is what gets released elsewhere on the task. Only allowed while the task is PENDING or IN_PROGRESS — rejected once COMPLETED, SHORT_PICK, or already CANCELLED, which also prevents double cancellation. Phase 18: on success, records cancelled_at (server UTC time) and cancelled_by (the authenticated caller) on the task, plus the optional request remark in cancel_remark (a dedicated column — the task's own pre-existing remark is left untouched). These audit fields are returned in the detail response (cancelled_at/cancelled_by/cancelled_by_name/cancel_remark) and are only ever set once: a rejected duplicate/invalid cancel attempt (409) never reaches the write and cannot overwrite them.
//	@Tags			Picking
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Picking task ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a cancellable status (already completed, short-picked, or already cancelled)"
//	@Router			/picking/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "picking task cancelled")
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
		apihttp.Fail(c, http.StatusNotFound, "picking task not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "picking task item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrNoSourceStock):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_SOURCE_STOCK")
	case errors.Is(err, ErrCrossWarehouse):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "CROSS_WAREHOUSE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "picking task is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "picking task is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "picking operation failed", "INTERNAL_ERROR")
	}
}

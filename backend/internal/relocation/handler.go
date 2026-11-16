package relocation

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

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// Handler exposes Service (and, for the Transfer Note, PDFService) over
// HTTP.
type Handler struct {
	service *Service
	pdf     *PDFService
}

func NewHandler(service *Service, pdf *PDFService) *Handler {
	return &Handler{service: service, pdf: pdf}
}

// Register mounts the Relocation endpoints on rg, using the same
// RequireJWT -> RequireTenantContext -> RequirePermission chain every
// tenant-scoped inbound/outbound module uses. "approve" gates the
// Draft -> Ready transition (the supervisor-level "lock this task for
// execution" action), the same confirm/approve split receiving.confirm/
// quality_check.confirm/putaway.confirm established — ordinary staff
// confirm their own tasks via relocation.confirm; relocation.approve is
// reserved for marking a task Ready. The PDF route is gated on the same
// relocation.view permission as Get — if a role can see the task, it
// can print/reprint its Transfer Note (read-only, never mutates
// inventory).
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("/relocations",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("", middleware.RequirePermission("relocation.view"), h.List)
	group.GET("/export", middleware.RequirePermission("relocation.view"), h.Export)
	group.GET("/:id", middleware.RequirePermission("relocation.view"), h.Get)
	group.GET("/:id/pdf", middleware.RequirePermission("relocation.view"), h.TransferNote)
	group.POST("", middleware.RequirePermission("relocation.create"), h.Create)
	group.POST("/:id/ready", middleware.RequirePermission("relocation.approve"), h.Ready)
	group.POST("/:id/confirm", middleware.RequirePermission("relocation.confirm"), h.Confirm)
	group.POST("/:id/cancel", middleware.RequirePermission("relocation.update"), h.Cancel)
}

// servePDF writes data as a PDF response — inline by default (the
// frontend's "View PDF" button can open it in a new tab without
// forcing a save dialog), attachment when ?download=true, matching
// internal/inbounddoc's own servePDF helper exactly.
func servePDF(c *gin.Context, data []byte, filename string) {
	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

// TransferNote godoc
//
//	@Summary		Download the Relocation Transfer Note PDF for a task
//	@Description	Task header plus its lines (source -> destination, lot/expiry/LPN, requested vs moved) and ledger-derived movement history. Synchronous, read-only — never mutates inventory, so printing/reprinting is always safe. Gated on relocation.view.
//	@Tags			Relocation
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Relocation task ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/relocations/{id}/pdf [get]
func (h *Handler) TransferNote(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid relocation task id", "INVALID_REQUEST")
		return
	}

	data, filename, err := h.pdf.RenderTransferNote(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apihttp.Fail(c, http.StatusNotFound, "relocation task not found", "NOT_FOUND")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render transfer note", "INTERNAL_ERROR")
		return
	}
	servePDF(c, data, filename)
}

// List godoc
//
//	@Summary		List relocation tasks
//	@Description	Item-level rows — supports search plus item-status/task-status/warehouse filters, with pagination.
//	@Tags			Relocation
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches relocation no, SKU, product name, or destination location"
//	@Param			status			query		string	false	"PENDING, PARTIALLY_COMPLETED, or COMPLETED (item status)"
//	@Param			task_status		query		string	false	"DRAFT, READY, IN_PROGRESS, COMPLETED, or CANCELLED (task status)"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/relocations [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	responses, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		Search: c.Query("search"), Status: ItemStatus(c.Query("status")), TaskStatus: Status(c.Query("task_status")),
		WarehouseID: warehouseID, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list relocation tasks", "INTERNAL_ERROR")
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
//	@Summary		Export Relocation History as CSV
//	@Description	Synchronous CSV export — same search/status/task_status/warehouse_id filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on the same relocation.view permission as the list itself.
//	@Tags			Relocation
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches relocation no or SKU"
//	@Param			status			query	string	false	"PENDING, PARTIALLY_COMPLETED, or COMPLETED"
//	@Param			task_status		query	string	false	"DRAFT, READY, IN_PROGRESS, COMPLETED, or CANCELLED"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/relocations/export [get]
func (h *Handler) Export(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, ListParams{
		Search: c.Query("search"), Status: ItemStatus(c.Query("status")), TaskStatus: Status(c.Query("task_status")),
		WarehouseID: warehouseID,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export relocation tasks", "INTERNAL_ERROR")
		return
	}

	headers := []string{"relocation_no", "sku", "product_name", "lot_no", "requested_qty", "moved_qty", "source_location", "destination_location", "status", "task_status"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			r.RelocationNo, r.SKU, r.ProductName, r.LotNo,
			csvexport.FormatFloat(r.RequestedQty), csvexport.FormatFloat(r.MovedQty),
			r.SourceLocationCode, r.DestinationLocationCode, string(r.Status), string(r.TaskStatus),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "relocation_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Get godoc
//
//	@Summary		Get a relocation task with its lines
//	@Tags			Relocation
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Relocation task ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/relocations/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid relocation task id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "relocation task not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get relocation task", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a relocation task
//	@Description	Embeds lines in one call. A new task always starts DRAFT; use POST /relocations/{id}/ready to lock it, then POST /relocations/{id}/confirm to actually move stock.
//	@Tags			Relocation
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New relocation task"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse, product, or location does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/relocations [post]
func (h *Handler) Create(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Create(ctx, tenantID, req, userID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "relocation task created")
		return
	}
	writeError(c, err)
}

// Ready godoc
//
//	@Summary		Mark a relocation task Ready
//	@Description	Transitions DRAFT -> READY, locking its lines from further edits. Requires at least one item. Only once READY (or IN_PROGRESS) may Confirm be called.
//	@Tags			Relocation
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Relocation task ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse	"task is not DRAFT, or has no items"
//	@Router			/relocations/{id}/ready [post]
func (h *Handler) Ready(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid relocation task id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Ready(c.Request.Context(), tenantID, id)
	if err == nil {
		apihttp.OK(c, resp, "relocation task marked ready")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm one or more relocation lines
//	@Description	Atomically moves each line's quantity via Inventory Core's MoveAvailableStockTx (same transaction) — never AddStock, and never a reserved or QC-held unit, so total system-wide quantity for the moved product/lot is unchanged and held/reserved stock can never be relocated this way — and recomputes the task's status; all committed together or all rolled back. idempotency_key must be a fresh UUID per click; resending the same key returns the task's current state instead of moving stock twice. Can be called more than once on the same task for partial relocation across multiple destinations.
//	@Tags			Relocation
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Relocation task ID"
//	@Param			request	body		ConfirmRequest	true	"Lines to confirm, with this batch's quantity for each"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a confirmable status, or a line is already completed"
//	@Failure		422		{object}	apihttp.ErrorResponse	"e.g. insufficient AVAILABLE source stock — the remainder is reserved or QC-held"
//	@Router			/relocations/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid relocation task id", "INVALID_REQUEST")
		return
	}

	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Confirm(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "relocation confirmed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a relocation task
//	@Description	Marks the task CANCELLED. Allowed from DRAFT, READY, or IN_PROGRESS — including a task with lines already partially or fully moved, which are left exactly as they are; only the header moves to CANCELLED, abandoning whatever remains unconfirmed. Records cancelled_at/cancelled_by on success, plus the optional request remark in cancel_remark (a dedicated column — the task's own pre-existing remark is left untouched).
//	@Tags			Relocation
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Relocation task ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a cancellable status"
//	@Router			/relocations/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid relocation task id", "INVALID_REQUEST")
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

	resp, err := h.service.Cancel(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "relocation task cancelled")
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
		apihttp.Fail(c, http.StatusNotFound, "relocation task not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "relocation task item not found", "ITEM_NOT_FOUND")
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
	case errors.Is(err, ErrCapacityExceeded):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "CAPACITY_EXCEEDED")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "relocation task is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotReadyable):
		apihttp.Fail(c, http.StatusConflict, "relocation task is not in a status that can be marked ready", "NOT_READYABLE")
	case errors.Is(err, ErrEmptyTask):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "EMPTY_TASK")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "relocation task is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "relocation operation failed", "INTERNAL_ERROR")
	}
}

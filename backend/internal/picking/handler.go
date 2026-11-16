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

// writeTenantUnauthorized is shared by every handler method's
// EnforcedTenantID guard below.
func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// Handler exposes Service (and, for the Pick List, PDFService) over
// HTTP.
type Handler struct {
	service *Service
	pdf     *PDFService
}

func NewHandler(service *Service, pdf *PDFService) *Handler {
	return &Handler{service: service, pdf: pdf}
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
//
// Phase 30 Batch 2 upgrades the chain from RequireJWT-only to
// RequireJWT -> RequireTenantContext -> RequirePermission, matching
// internal/inventory/internal/customer/internal/supplier/internal/location's
// own chain — every handler method below now extracts tenantID via
// middleware.EnforcedTenantID and threads it through to Service.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	pickingGroup := rg.Group("/picking",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	pickingGroup.GET("", middleware.RequirePermission("picking.view"), h.List)
	pickingGroup.GET("/export", middleware.RequirePermission("picking.view"), h.Export)
	pickingGroup.GET("/lots/suggest", middleware.RequirePermission("picking.view"), h.SuggestLots)
	pickingGroup.GET("/:id", middleware.RequirePermission("picking.view"), h.Get)
	pickingGroup.GET("/:id/pdf", middleware.RequirePermission("picking.view"), h.PickList)
	pickingGroup.POST("", middleware.RequirePermission("picking.create"), h.Create)
	// Phase 33B: Confirm moves from picking.update to the new
	// picking.confirm — see catalog's own doc on the "picking" module.
	pickingGroup.POST("/:id/confirm", middleware.RequirePermission("picking.confirm"), h.Confirm)
	pickingGroup.POST("/:id/cancel", middleware.RequirePermission("picking.approve"), h.Cancel)
	// Assign/reassign and the scan-validation step are both non-stock-
	// mutating, execution-level actions — gated on picking.update, the
	// same level PICKING staff already operate at (and Confirm itself
	// used to sit at, before this phase's split above).
	pickingGroup.PUT("/:id/assign", middleware.RequirePermission("picking.update"), h.AssignPicker)
	pickingGroup.POST("/:id/items/:itemId/scan", middleware.RequirePermission("picking.update"), h.ScanConfirm)
	// Repick re-reserves stock (inventory.ReserveStockTx) — the same
	// authority level as Confirm, not picking.update's plain execution
	// level.
	pickingGroup.POST("/:id/items/:itemId/repick", middleware.RequirePermission("picking.confirm"), h.Repick)
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	responses, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, ListParams{
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

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

	resp, err := h.service.Confirm(ctx, tenantID, id, req, userID)
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
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

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

	resp, err := h.service.Cancel(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "picking task cancelled")
		return
	}
	writeError(c, err)
}

// AssignPicker godoc
//
//	@Summary		Assign (or reassign) a picker to a task
//	@Description	Claims the task for pickerUserID — writes assigned_picker_id/assigned_at. Never touches reservation or stock state. Only allowed while the task is PENDING or IN_PROGRESS.
//	@Tags			Picking
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Picking task ID"
//	@Param			request	body		AssignRequest	true	"Picker to assign"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in an assignable status"
//	@Router			/picking/{id}/assign [put]
func (h *Handler) AssignPicker(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
		return
	}
	var req AssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.AssignPicker(ctx, tenantID, id, req.PickerID, userID)
	if err == nil {
		apihttp.OK(c, resp, "picker assigned")
		return
	}
	writeError(c, err)
}

// ScanConfirm godoc
//
//	@Summary		Validate a location->product->lot scan against a picking line
//	@Description	Compares the scanned location code/SKU/lot against the line's own assigned ones and logs the attempt (MATCH or MISMATCH) to the scan audit trail. Read-only against inventory — never reserves, consumes, or releases stock. Intended to run right before Confirm for this line.
//	@Tags			Picking
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Picking task ID"
//	@Param			itemId	path		int				true	"Picking task item ID"
//	@Param			request	body		ScanRequest		true	"Scanned location/product/lot"
//	@Success		200		{object}	apihttp.SuccessResponse{data=ScanResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Router			/picking/{id}/items/{itemId}/scan [post]
func (h *Handler) ScanConfirm(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
		return
	}
	itemID, err := parseID(c, "itemId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task item id", "INVALID_REQUEST")
		return
	}
	var req ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.ScanConfirm(ctx, tenantID, id, itemID, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// Repick godoc
//
//	@Summary		Reopen a short-picked line for another pick attempt
//	@Description	Re-reserves (inventory.ReserveStockTx) the line's still-outstanding quantity and moves it back off SHORT — fails with 422 if the location still has no available stock to reserve. See internal/picking's own Repick doc for why this always re-reserves rather than reusing the original (fully-released) reservation.
//	@Tags			Picking
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int	true	"Picking task ID"
//	@Param			itemId	path		int	true	"Picking task item ID"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"line is not short-picked, or its task cannot be re-picked"
//	@Failure		422		{object}	apihttp.ErrorResponse	"insufficient stock to re-reserve"
//	@Router			/picking/{id}/items/{itemId}/repick [post]
func (h *Handler) Repick(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
		return
	}
	itemID, err := parseID(c, "itemId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task item id", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Repick(ctx, tenantID, id, itemID, userID)
	if err == nil {
		apihttp.OK(c, resp, "line re-opened for picking")
		return
	}
	writeError(c, err)
}

// SuggestLots godoc
//
//	@Summary		Suggest FEFO/FIFO-ordered available lots for a product at a warehouse
//	@Description	Read-only lot lookup (no reservation) ordered First-Expired-First-Out, tie-broken First-In-First-Out — for the UI to suggest/validate which lot a line should draw from.
//	@Tags			Picking
//	@Produce		json
//	@Security		BearerAuth
//	@Param			product_id		query		int	true	"Product ID"
//	@Param			warehouse_id	query		int	true	"Warehouse ID"
//	@Success		200				{object}	apihttp.SuccessResponse{data=[]LotSuggestionResponse}
//	@Failure		400				{object}	apihttp.ErrorResponse
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/picking/lots/suggest [get]
func (h *Handler) SuggestLots(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	productID, err := strconv.ParseInt(c.Query("product_id"), 10, 64)
	if err != nil || productID == 0 {
		apihttp.Fail(c, http.StatusBadRequest, "product_id is required", "INVALID_REQUEST")
		return
	}
	warehouseID, err := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	if err != nil || warehouseID == 0 {
		apihttp.Fail(c, http.StatusBadRequest, "warehouse_id is required", "INVALID_REQUEST")
		return
	}

	lots, err := h.service.SuggestLots(c.Request.Context(), tenantID, productID, warehouseID)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to suggest lots", "INTERNAL_ERROR")
		return
	}
	resp := make([]LotSuggestionResponse, 0, len(lots))
	for _, lot := range lots {
		resp = append(resp, LotSuggestionResponse{
			LocationID: lot.LocationID, LocationCode: lot.LocationCode, LotNo: lot.LotNo,
			ExpiryDate: lot.ExpiryDate, AvailableQty: lot.AvailableQty, OnHandQty: lot.OnHandQty,
		})
	}
	apihttp.OK(c, resp, "")
}

// servePDF writes data as an inline PDF response — matching
// internal/relocation and internal/order's own servePDF helper exactly.
func servePDF(c *gin.Context, data []byte, filename string) {
	c.Header("Content-Disposition", "inline; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

// PickList godoc
//
//	@Summary		Download the Pick List / Picking Work Order PDF for a task
//	@Description	Work order no., source order/customer/destination, warehouse, priority/due date, assigned picker, and each line's SKU/product/source location/lot/expiry/requested/allocated/picked qty — with a Code128 barcode encoding the pick number and a QR code per line encoding location|product|lot for scan confirmation in the warehouse. Read-only: printing/reprinting is always safe.
//	@Tags			Picking
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Picking task ID"
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/picking/{id}/pdf [get]
func (h *Handler) PickList(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid picking task id", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)

	data, filename, err := h.pdf.RenderPickList(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	servePDF(c, data, filename)
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
	case errors.Is(err, ErrPickerNotFound):
		apihttp.Fail(c, http.StatusNotFound, "picker user not found", "PICKER_NOT_FOUND")
	case errors.Is(err, ErrNoSourceStock):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_SOURCE_STOCK")
	case errors.Is(err, ErrCrossWarehouse):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "CROSS_WAREHOUSE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "picking task is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "picking task is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.Is(err, ErrNotAssignable):
		apihttp.Fail(c, http.StatusConflict, "picking task is not in an assignable status", "NOT_ASSIGNABLE")
	case errors.Is(err, ErrNotRepickable):
		apihttp.Fail(c, http.StatusConflict, "line is not short-picked, or its task cannot be re-picked", "NOT_REPICKABLE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "picking operation failed", "INTERNAL_ERROR")
	}
}

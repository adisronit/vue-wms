package receiving

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
// EnforcedTenantID guard below — same shape as internal/picking's own
// (Batch 2), internal/order's own (Batch 3), and internal/shipping's
// own (Batch 4).
func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Receiving endpoints on rg.
//
// Phase 31A upgrades the chain from RequireJWT-only to RequireJWT ->
// RequireTenantContext -> RequirePermission, matching internal/picking's
// own Batch 2 chain, internal/order's own Batch 3 chain, and
// internal/shipping's own Batch 4 chain — every handler method below
// now extracts tenantID via middleware.EnforcedTenantID and threads it
// through to Service. Receiving was the last inbound/outbound workflow
// module still on the old RequireJWT-only chain.
//
// Confirm (the action that actually moves stock into Available or Hold)
// is now gated on receiving.confirm, a new permission split out of the
// pre-existing receiving.approve — the approved Phase 31A architecture
// decision to separate ordinary Receiving-staff confirmation from
// supervisor-level exception approval and QC release. receiving.approve
// itself is kept in the catalog (granted to ADMIN/WAREHOUSE_MANAGER
// exactly as before) for a future exception-handling action; no such
// action exists yet in this phase. See internal/rbac/catalog.go's own
// doc for the permission-catalog side of this split.
//
// Phase 15's Cancel is still gated on receiving.update: it only ever
// runs pre-confirm (Status.CanCancel mirrors CanEdit exactly), so it
// sits at editing's authority level, not confirm's stock-moving level.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	receipts := rg.Group("/receipts",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	receipts.GET("", middleware.RequirePermission("receiving.view"), h.List)
	receipts.GET("/export", middleware.RequirePermission("receiving.view"), h.Export)
	receipts.GET("/:id", middleware.RequirePermission("receiving.view"), h.Get)
	receipts.POST("", middleware.RequirePermission("receiving.create"), h.Create)
	receipts.PUT("/:id", middleware.RequirePermission("receiving.update"), h.Update)
	receipts.POST("/:id/items", middleware.RequirePermission("receiving.update"), h.AddItem)
	receipts.PUT("/:id/items/:itemId", middleware.RequirePermission("receiving.update"), h.UpdateItem)
	receipts.POST("/:id/confirm", middleware.RequirePermission("receiving.confirm"), h.Confirm)
	receipts.POST("/:id/cancel", middleware.RequirePermission("receiving.update"), h.Cancel)
}

// List godoc
//
//	@Summary		List receiving documents
//	@Description	Supports the Goods Receiving screen's search box plus status/warehouse/date filters, with pagination.
//	@Tags			Receiving
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches receipt no, reference no, or vendor"
//	@Param			status			query		string	false	"DRAFT, WAITING, PARTIALLY_RECEIVED, COMPLETED, or CANCELLED"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			date			query		string	false	"Filter by receiving date (YYYY-MM-DD)"
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/receipts [get]
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
		Search: c.Query("search"), Status: Status(c.Query("status")),
		WarehouseID: warehouseID, Date: date, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list receipts", "INTERNAL_ERROR")
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
//	@Summary		Export Receiving History as CSV
//	@Description	Synchronous CSV export (Phase 22) — honors the exact same search/status/warehouse_id/date filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on the same receiving.view permission as the list itself: if a role can see the list, it can export it.
//	@Tags			Receiving
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches receipt no, reference no, or vendor"
//	@Param			status			query	string	false	"DRAFT, WAITING, PARTIALLY_RECEIVED, COMPLETED, or CANCELLED"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			date			query	string	false	"Filter by receiving date (YYYY-MM-DD)"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/receipts/export [get]
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
		Search: c.Query("search"), Status: Status(c.Query("status")),
		WarehouseID: warehouseID, Date: date,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export receipts", "INTERNAL_ERROR")
		return
	}

	headers := []string{"receipt_no", "reference_no", "vendor", "warehouse_code", "receiving_date", "item_count", "progress_pct", "status"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			r.ReceiptNo, csvexport.FormatString(r.ReferenceNo), csvexport.FormatString(r.Vendor),
			r.WarehouseCode, csvexport.FormatTime(r.ReceivingDate),
			strconv.Itoa(r.ItemCount), csvexport.FormatFloat(r.ProgressPct), string(r.Status),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "receiving_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Get godoc
//
//	@Summary		Get a receipt with its lines
//	@Tags			Receiving
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Receipt ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/receipts/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "receipt not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get receipt", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a receiving document
//	@Description	Optionally embeds lines in one call — the "Save Draft"/"Confirm Receiving" buttons both submit the whole form (header + items) at once. status defaults to DRAFT (Save Draft); pass WAITING to finalize without confirming yet.
//	@Tags			Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New receipt"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse, location, supplier, or product does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/receipts [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "receipt created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a receipt header
//	@Description	Only allowed while the receipt is still DRAFT or WAITING (see Status.CanEdit).
//	@Tags			Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Receipt ID"
//	@Param			request	body		UpdateRequest	true	"Updated header fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"receipt can no longer be edited"
//	@Router			/receipts/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Update(c.Request.Context(), tenantID, id, req)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// AddItem godoc
//
//	@Summary		Add a receiving line
//	@Description	Used by both the "+ Add Item" button and the barcode-scan flow.
//	@Tags			Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Receipt ID"
//	@Param			request	body		CreateItemRequest	true	"New line"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse	"e.g. product requires a lot number/expiry/serial tracking"
//	@Router			/receipts/{id}/items [post]
func (h *Handler) AddItem(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}

	var req CreateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))
	resp, err := h.service.AddItem(ctx, tenantID, id, req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "item added")
		return
	}
	writeError(c, err)
}

// UpdateItem godoc
//
//	@Summary		Update a receiving line
//	@Description	Only allowed while the line hasn't started receiving (status PENDING).
//	@Tags			Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Receipt ID"
//	@Param			itemId	path		int					true	"Item ID"
//	@Param			request	body		UpdateItemRequest	true	"Updated line"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/receipts/{id}/items/{itemId} [put]
func (h *Handler) UpdateItem(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}
	itemID, err := parseID(c, "itemId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid item id", "INVALID_REQUEST")
		return
	}

	var req UpdateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.UpdateItem(c.Request.Context(), tenantID, id, itemID, req)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm receiving for one or more lines
//	@Description	Atomically updates each line's received quantity, calls Inventory Core's AddStock (same transaction) for every lot/expiry/pallet split, records each split, and recomputes the receipt's status — all committed together or all rolled back. idempotency_key must be a fresh UUID per click; resending the same key (double-click, retry) returns the receipt's current state instead of adding stock twice. Can be called more than once on the same receipt for partial receiving. A QC-required product's confirmed quantity is posted into inventory's hold (not immediately available) — see ReceiptItemResponse.Splits.
//	@Tags			Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Receipt ID"
//	@Param			request	body		ConfirmRequest	true	"Lines to confirm, with this batch's quantity for each"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"receipt is not in a confirmable status, or a line is already fully received"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/receipts/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "receiving confirmed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a receipt
//	@Description	Marks the receipt CANCELLED. Only allowed while it is DRAFT or WAITING — i.e. before any line has actually received stock (AddStock has never been called for this receipt). Never mutates inventory and never reverses an already-received quantity; rejected with 409 once any line has received_qty > 0. Phase 18: on success, records cancelled_at (server UTC time) and cancelled_by (the authenticated caller) on the receipt, plus the optional request remark in cancel_remark (a dedicated column — the receipt's own pre-existing remark is left untouched). These audit fields are returned in the detail response (cancelled_at/cancelled_by/cancelled_by_name/cancel_remark) and are only ever set once: a rejected duplicate/invalid cancel attempt (409) never reaches the write and cannot overwrite them.
//	@Tags			Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Receipt ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"receipt is not in a cancellable status (stock already received, already cancelled, or already completed)"
//	@Router			/receipts/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "receipt cancelled")
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
		apihttp.Fail(c, http.StatusNotFound, "receipt not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "receipt item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrSupplierNotFound):
		apihttp.Fail(c, http.StatusNotFound, "supplier not found", "SUPPLIER_NOT_FOUND")
	case errors.Is(err, ErrNotEditable):
		apihttp.Fail(c, http.StatusConflict, "receipt can no longer be edited", "NOT_EDITABLE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "receipt is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "receipt is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "receiving operation failed", "INTERNAL_ERROR")
	}
}

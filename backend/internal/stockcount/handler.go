package stockcount

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

// Register mounts the Stock Count endpoints on rg. Per the task's own
// "reuse existing stock adjustment / stock count permissions... do not
// create duplicate concepts" instruction, and since no separate
// "stock_count" module exists anywhere in the permission catalog or
// RolePermissionView.vue's own module list, this package reuses the
// existing "stock_adjustment" codes exactly as Stock Adjustment itself
// does — the two screens share one RBAC domain in this codebase, even
// though they are two distinct workflows. Confirm (the action that
// actually applies variance through Inventory Core) is gated on
// stock_adjustment.approve, the same significant-action permission
// Stock Adjustment's own Confirm uses.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	group := rg.Group("/stock-counts", middleware.RequireJWT(jwtSecret))
	group.GET("", middleware.RequirePermission("stock_adjustment.view"), h.List)
	group.GET("/export", middleware.RequirePermission("stock_adjustment.view"), h.Export)
	group.GET("/:id", middleware.RequirePermission("stock_adjustment.view"), h.Get)
	group.POST("", middleware.RequirePermission("stock_adjustment.create"), h.Create)
	group.POST("/:id/items", middleware.RequirePermission("stock_adjustment.update"), h.AddItem)
	group.PUT("/:id/items/:itemId", middleware.RequirePermission("stock_adjustment.update"), h.RecordCount)
	group.POST("/:id/confirm", middleware.RequirePermission("stock_adjustment.approve"), h.Confirm)
	group.POST("/:id/create-adjustments", middleware.RequirePermission("stock_adjustment.create"), h.CreateAdjustments)
}

// List godoc
//
//	@Summary		List stock counts (Stock Count History)
//	@Description	Task-level rows matching the Stock Count History screen's table (Phase 14) — supports its search box (matches count_no) plus warehouse/status/date filters, with pagination.
//	@Tags			StockCount
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches count_no"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			status			query		string	false	"DRAFT or CONFIRMED"
//	@Param			date			query		string	false	"Filter by count date (YYYY-MM-DD)"
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/stock-counts [get]
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
		Search: c.Query("search"), WarehouseID: warehouseID, Status: Status(c.Query("status")),
		Date: date, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list stock counts", "INTERNAL_ERROR")
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
//	@Summary		Export Stock Count History as CSV
//	@Description	Synchronous CSV export (Phase 22) — same search/warehouse_id/status/date filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on stock_adjustment.view, the same permission the list itself reuses (Phase 11's own decision — Stock Count has no separate RBAC module).
//	@Tags			StockCount
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches count no"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			status			query	string	false	"DRAFT, IN_PROGRESS, CONFIRMED, or ADJUSTMENT_CREATED"
//	@Param			date			query	string	false	"Filter by count date (YYYY-MM-DD)"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/stock-counts/export [get]
func (h *Handler) Export(c *gin.Context) {
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	rows, err := h.service.ListForExport(c.Request.Context(), ListParams{
		Search: c.Query("search"), WarehouseID: warehouseID, Status: Status(c.Query("status")), Date: date,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export stock counts", "INTERNAL_ERROR")
		return
	}

	headers := []string{"count_no", "count_type", "warehouse_code", "zone_code", "count_date", "item_count", "variance_lines", "status", "created_by_name", "created_at"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		zoneCode := ""
		if r.ZoneCode != nil {
			zoneCode = *r.ZoneCode
		}
		csvRows = append(csvRows, []string{
			r.CountNo, string(r.CountType), r.WarehouseCode, zoneCode, csvexport.FormatTime(r.CountDate),
			strconv.Itoa(r.ItemCount), strconv.Itoa(r.VarianceLines), string(r.Status),
			csvexport.FormatString(r.CreatedByName), csvexport.FormatTimeValue(r.CreatedAt),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "stock_count_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Get godoc
//
//	@Summary		Get a stock count with its lines
//	@Description	Includes generated_adjustments (Phase 17) — every internal/stockadjustment document this count has produced via Create Adjustment, resolved fresh from that package's own table on every call (never stored or duplicated here). Empty for a count that was Confirmed directly, or hasn't been acted on yet — Confirm never creates an adjustment. Gated on the same stock_adjustment.view permission that gates Stock Adjustment's own endpoints, so this can never expose adjustment data to a role that couldn't already see it directly.
//	@Tags			StockCount
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Stock count ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/stock-counts/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid stock count id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "stock count not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get stock count", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a stock count task
//	@Description	Makes "+ New Count Task" real. Items may be included up front or added afterward via POST /stock-counts/{id}/items. Each line's system_qty is snapshotted server-side from Inventory Core's live balance at the moment it is added, and never recomputed afterward.
//	@Tags			StockCount
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New stock count"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse, zone, product, or location does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/stock-counts [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "stock count created")
		return
	}
	writeError(c, err)
}

// AddItem godoc
//
//	@Summary		Add a line to a stock count
//	@Description	The real backing for "+ Add Item". Only allowed while the count is still DRAFT.
//	@Tags			StockCount
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Stock count ID"
//	@Param			request	body		CreateItemRequest	true	"New line"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Router			/stock-counts/{id}/items [post]
func (h *Handler) AddItem(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid stock count id", "INVALID_REQUEST")
		return
	}

	var req CreateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))
	resp, err := h.service.AddItem(ctx, id, req)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "item added")
		return
	}
	writeError(c, err)
}

// RecordCount godoc
//
//	@Summary		Record a counted quantity for one line
//	@Description	Used by both the scan bar's "Confirm Count" action and the items table's own editable Count Qty/Condition/Remark cells. Computes and stores variance_qty immediately from the line's frozen system_qty. Only allowed while the count is still DRAFT.
//	@Tags			StockCount
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Stock count ID"
//	@Param			itemId	path		int					true	"Item ID"
//	@Param			request	body		RecordCountRequest	true	"Counted quantity and condition"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/stock-counts/{id}/items/{itemId} [put]
func (h *Handler) RecordCount(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid stock count id", "INVALID_REQUEST")
		return
	}
	itemID, err := parseID(c, "itemId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid item id", "INVALID_REQUEST")
		return
	}

	var req RecordCountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.RecordCount(c.Request.Context(), id, itemID, req)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm a stock count (the sole authoritative variance-applying action)
//	@Description	"Confirm Count" — the one path that ever applies a count's variance to inventory directly. Validates every line has been counted, re-checks each line's live inventory balance against the quantity frozen when it was added (returning 409 if it has drifted — see internal/stockcount's ErrStaleCount doc), then atomically applies each line's variance through Inventory Core (AddStock if positive, RemoveStock if negative, skipped if zero) and marks the count CONFIRMED. No inventory mutation is ever a direct overwrite. idempotency_key must be a fresh UUID per click; resending the same key returns the count's current state instead of applying variance twice. Mutually exclusive with POST /stock-counts/{id}/create-adjustments (Phase 16): whichever of the two runs first for a given count permanently claims it, and the other then returns 409 — the same variance can never be applied through both.
//	@Tags			StockCount
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Stock count ID"
//	@Param			request	body		ConfirmRequest	true	"Idempotency key"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"count is not in a confirmable status (including: already confirmed, already routed through Create Adjustment, or a concurrent request already claimed it), some items still pending, or a line's inventory has changed since it was counted"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/stock-counts/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid stock count id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "stock count confirmed")
		return
	}
	writeError(c, err)
}

// CreateAdjustments godoc
//
//	@Summary		Create stock adjustments from a count's variance lines (secondary, mutually exclusive with Confirm)
//	@Description	"Create Adjustment" — the alternate, non-immediate path to Confirm Count: generates one internal/stockadjustment PENDING document per non-zero-variance line, atomically, instead of applying variance immediately. Neither this call nor the created documents mutate inventory by themselves; a separate approver must still confirm each one, exactly like a manually-created adjustment. Phase 16: permanently claims the count (DRAFT -> ADJUSTMENT_CREATED) before creating anything, the same guarded transition Confirm uses for DRAFT -> CONFIRMED — calling this after Confirm, calling Confirm after this, or calling this a second time on the same count all now return 409 rather than silently generating a duplicate mutation path for the same variance. Phase 17: every generated document now carries a real source_stock_count_id FK back to this count (previously only a free-text remark) — visible via GET /stock-counts/{id}'s own generated_adjustments and GET /stock-adjustments/{id}'s own source_stock_count_id/source_stock_count_no.
//	@Tags			StockCount
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Stock count ID"
//	@Success		201	{object}	apihttp.SuccessResponse{data=CreateAdjustmentsResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse	"this count's variance was already applied via Confirm or a prior Create Adjustment call"
//	@Failure		422	{object}	apihttp.ErrorResponse	"no variance found"
//	@Router			/stock-counts/{id}/create-adjustments [post]
func (h *Handler) CreateAdjustments(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid stock count id", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.CreateAdjustments(ctx, id, userID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "stock adjustments created")
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
		apihttp.Fail(c, http.StatusNotFound, "stock count not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "stock count item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, ErrZoneNotFound):
		apihttp.Fail(c, http.StatusNotFound, "zone not found", "ZONE_NOT_FOUND")
	case errors.Is(err, ErrNotEditable):
		apihttp.Fail(c, http.StatusConflict, "stock count can no longer be edited", "NOT_EDITABLE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "stock count is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrStaleCount):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "STALE_COUNT_CONFLICT")
	case errors.Is(err, ErrVarianceAlreadyApplied):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "VARIANCE_ALREADY_APPLIED")
	case errors.Is(err, ErrInsufficientStock):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "INSUFFICIENT_STOCK")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "stock count operation failed", "INTERNAL_ERROR")
	}
}

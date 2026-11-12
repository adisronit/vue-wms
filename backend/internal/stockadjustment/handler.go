package stockadjustment

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

// Register mounts the Stock Adjustment endpoints on rg. Permissions
// reuse the existing "stock_adjustment" module from Phase 2's catalog
// exactly as-is — no new codes. Confirm (Approve/Reject — the action
// that actually moves stock) is gated on stock_adjustment.approve, the
// same significant-action permission Receiving/Putaway/Shipping's own
// Confirm uses. Under current seed data, no operational role
// (RECEIVING/QC/PUTAWAY/PICKING/SHIPPING) has ANY stock_adjustment.*
// grant at all — only ADMIN/WAREHOUSE_MANAGER do, via their generic
// all/all-except-delete computation. This is not a gap introduced here:
// RolePermissionView.vue's own createDefaultPermissions() never had a
// branch granting stock_adjustment to any operational role either, so
// nothing was silently narrowed — see PROJECT_STATUS.md.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	group := rg.Group("/stock-adjustments", middleware.RequireJWT(jwtSecret))
	group.GET("", middleware.RequirePermission("stock_adjustment.view"), h.List)
	group.GET("/export", middleware.RequirePermission("stock_adjustment.view"), h.Export)
	group.GET("/:id", middleware.RequirePermission("stock_adjustment.view"), h.Get)
	group.POST("", middleware.RequirePermission("stock_adjustment.create"), h.Create)
	group.PUT("/:id", middleware.RequirePermission("stock_adjustment.update"), h.Update)
	group.POST("/:id/confirm", middleware.RequirePermission("stock_adjustment.approve"), h.Confirm)
}

// List godoc
//
//	@Summary		List stock adjustments
//	@Description	Supports the Stock Adjustment screen's search box (adjustment no, SKU, or reason) plus type/status/date filters, with pagination. Phase 20 adds reason/created_by/older_than_days/overdue for PENDING follow-up — each independent and combinable with the others. Every row's own pending_since/age_hours/age_days/is_overdue/pending_warning_hours (Phase 20) are populated only while that row's status is PENDING.
//	@Tags			StockAdjustment
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches adjustment no, SKU, or reason"
//	@Param			type			query		string	false	"INCREASE or DECREASE"
//	@Param			status			query		string	false	"DRAFT, PENDING, APPROVED, or REJECTED"
//	@Param			date			query		string	false	"Filter by creation date (YYYY-MM-DD)"
//	@Param			reason			query		string	false	"Exact reason match, e.g. \"Found Goods\""
//	@Param			created_by		query		int		false	"Filter by the creating user's ID"
//	@Param			older_than_days	query		int		false	"Only rows not touched (updated_at) in at least this many days"
//	@Param			overdue			query		bool	false	"true: only PENDING rows whose age has reached STOCK_ADJUSTMENT_PENDING_WARNING_HOURS. Implies status=PENDING regardless of the status param."
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/stock-adjustments [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}

	createdBy, _ := strconv.ParseInt(c.Query("created_by"), 10, 64)
	olderThanDays, _ := strconv.Atoi(c.Query("older_than_days"))
	overdue, _ := strconv.ParseBool(c.Query("overdue"))

	responses, total, err := h.service.List(c.Request.Context(), ListParams{
		Search: c.Query("search"), Type: AdjustType(c.Query("type")), Status: Status(c.Query("status")),
		Date: date, Reason: Reason(c.Query("reason")), CreatedBy: createdBy,
		OlderThanDays: olderThanDays, Overdue: overdue, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list stock adjustments", "INTERNAL_ERROR")
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
//	@Summary		Export Stock Adjustment History as CSV
//	@Description	Synchronous CSV export (Phase 22) — same search/type/status/date/reason/created_by/older_than_days/overdue filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on the same stock_adjustment.view permission as the list itself.
//	@Tags			StockAdjustment
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches adjustment no, SKU, or reason"
//	@Param			type			query	string	false	"INCREASE or DECREASE"
//	@Param			status			query	string	false	"DRAFT, PENDING, APPROVED, or REJECTED"
//	@Param			date			query	string	false	"Filter by creation date (YYYY-MM-DD)"
//	@Param			reason			query	string	false	"Exact reason match"
//	@Param			created_by		query	int		false	"Filter by creating user's ID"
//	@Param			older_than_days	query	int		false	"Only rows not touched in at least this many days"
//	@Param			overdue			query	bool	false	"true: only overdue PENDING rows"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/stock-adjustments/export [get]
func (h *Handler) Export(c *gin.Context) {
	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			date = &parsed
		}
	}
	createdBy, _ := strconv.ParseInt(c.Query("created_by"), 10, 64)
	olderThanDays, _ := strconv.Atoi(c.Query("older_than_days"))
	overdue, _ := strconv.ParseBool(c.Query("overdue"))

	rows, err := h.service.ListForExport(c.Request.Context(), ListParams{
		Search: c.Query("search"), Type: AdjustType(c.Query("type")), Status: Status(c.Query("status")),
		Date: date, Reason: Reason(c.Query("reason")), CreatedBy: createdBy,
		OlderThanDays: olderThanDays, Overdue: overdue,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export stock adjustments", "INTERNAL_ERROR")
		return
	}

	headers := []string{"adjustment_no", "date", "sku", "product_name", "location_code", "before_qty", "adjust_qty", "adjust_type", "after_qty", "reason", "status", "created_by_name", "is_overdue"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		beforeQty, afterQty := "", ""
		if r.BeforeQty != nil {
			beforeQty = csvexport.FormatFloat(*r.BeforeQty)
		}
		if r.AfterQty != nil {
			afterQty = csvexport.FormatFloat(*r.AfterQty)
		}
		csvRows = append(csvRows, []string{
			r.AdjustmentNo, csvexport.FormatTimeValue(r.Date), r.SKU, r.ProductName, r.LocationCode,
			beforeQty, csvexport.FormatFloat(r.AdjustQty), string(r.AdjustType), afterQty,
			string(r.Reason), string(r.Status), csvexport.FormatString(r.CreatedByName), strconv.FormatBool(r.IsOverdue),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "stock_adjustment_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Get godoc
//
//	@Summary		Get a stock adjustment
//	@Description	source_stock_count_id/source_stock_count_no (Phase 17) are populated only when this document was generated by internal/stockcount's Create Adjustment; nil for a manually-created adjustment. created_by_name (Phase 20) resolves created_by to a display name. pending_since/age_hours/age_days/is_overdue/pending_warning_hours (Phase 20) are populated only while status is PENDING.
//	@Tags			StockAdjustment
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Adjustment ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/stock-adjustments/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid adjustment id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "stock adjustment not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get stock adjustment", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a stock adjustment
//	@Description	The drawer's single "Save Draft"/"Submit for Approval" click submits the whole (header + one line) document at once. status defaults to DRAFT (Save Draft); pass PENDING to submit directly. No inventory mutation happens at create time regardless of status — only POST /stock-adjustments/{id}/confirm's APPROVE action ever changes a balance.
//	@Tags			StockAdjustment
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New stock adjustment"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"product or location does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/stock-adjustments [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "stock adjustment created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update a stock adjustment
//	@Description	Only allowed while the adjustment is still DRAFT (see Status.CanEdit). Also doubles as "Submit for Approval": pass status PENDING to transition an existing draft.
//	@Tags			StockAdjustment
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Adjustment ID"
//	@Param			request	body		UpdateRequest	true	"Updated fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"adjustment can no longer be edited"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/stock-adjustments/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid adjustment id", "INVALID_REQUEST")
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
//	@Summary		Approve or reject a stock adjustment
//	@Description	Action APPROVE atomically calls Inventory Core's AddStock (INCREASE) or RemoveStock (DECREASE) in the same transaction as the status update — the only point in this workflow that ever changes inventory.quantity, and always through Inventory Core's own atomic guarded UPDATE, never a direct overwrite. Action REJECT performs no inventory mutation and requires a non-empty remark. idempotency_key must be a fresh UUID per click; resending the same key returns the adjustment's current state instead of applying the mutation twice.
//	@Tags			StockAdjustment
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Adjustment ID"
//	@Param			request	body		ConfirmRequest	true	"Action to apply"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"adjustment is not in a confirmable status"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/stock-adjustments/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid adjustment id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "stock adjustment updated")
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
		apihttp.Fail(c, http.StatusNotFound, "stock adjustment not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "stock adjustment item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrNotEditable):
		apihttp.Fail(c, http.StatusConflict, "stock adjustment can no longer be edited", "NOT_EDITABLE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "stock adjustment is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrInsufficientStock):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "INSUFFICIENT_STOCK")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "stock adjustment operation failed", "INTERNAL_ERROR")
	}
}

package pickingreport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/platform/xlsxexport"

	"wms-backend/internal/platform/csvexport"
)

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

func writeUnauthenticated(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED")
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Picking & Stock Issue Report endpoints on rg,
// gated on the Phase 34A report module.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	reports := rg.Group("/reports/picking",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	reports.GET("", middleware.RequirePermission("report.view"), h.List)
	reports.GET("/summary", middleware.RequirePermission("report.view"), h.Summary)
	reports.GET("/export", middleware.RequirePermission("report.export"), h.ExportCSV)
	reports.GET("/export/xlsx", middleware.RequirePermission("report.export"), h.ExportXLSX)
	reports.GET("/pdf", middleware.RequirePermission("report.export"), h.PDF)
	reports.GET("/batch-allocations", middleware.RequirePermission("report.view"), h.ListBatchAllocations)
	reports.GET("/batch-allocations/export", middleware.RequirePermission("report.export"), h.ExportBatchAllocationsCSV)
}

func parseFilters(c *gin.Context) Filters {
	var f Filters

	if raw := c.Query("from"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			f.From = &parsed
		}
	}
	if raw := c.Query("to"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			f.To = &parsed
		}
	}
	if raw := c.Query("warehouse_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.WarehouseID = &id
		}
	}
	if raw := c.Query("picker_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.PickerID = &id
		}
	}
	f.Status = c.Query("status")
	f.ItemStatus = c.Query("item_status")
	f.ShortOnly = c.Query("short_only") == "true"
	f.Search = c.Query("search")
	f.LotNo = c.Query("lot_no")

	return f
}

func parseBatchAllocationFilters(c *gin.Context) BatchAllocationFilters {
	var f BatchAllocationFilters
	if raw := c.Query("from"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			f.From = &parsed
		}
	}
	if raw := c.Query("to"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			f.To = &parsed
		}
	}
	if raw := c.Query("warehouse_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.WarehouseID = &id
		}
	}
	f.Search = c.Query("search")
	return f
}

// List godoc
//
//	@Summary		List Picking & Stock Issue Report rows
//	@Description	Line-level report, one row per picking_task_item (the real physical pick — never exploded per order for a consolidated batch line; see /batch-allocations for per-order attribution). Covers Detail/Summary/Short-Pick/Stock-Issue views via filters over the same data. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query	int		false	"Page number"			default(1)
//	@Param			limit			query	int		false	"Page size (max 100)"	default(20)
//	@Param			from			query	string	false	"Inclusive start, RFC3339 UTC, on the task's created_at"
//	@Param			to				query	string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			picker_id		query	int		false	"Filter by assigned picker"
//	@Param			status			query	string	false	"Task status"
//	@Param			item_status		query	string	false	"Line status: PENDING, PARTIALLY_PICKED, COMPLETED, SHORT"
//	@Param			short_only		query	bool	false	"Shortcut for item_status=SHORT"
//	@Param			search			query	string	false	"Matches pick no., order no., SKU, or product name"
//	@Param			lot_no			query	string	false	"Matches lot/batch number"
//	@Param			sort_by			query	string	false	"created_at (default), pick_no, warehouse_name, status"
//	@Param			sort_dir		query	string	false	"asc or desc (default desc)"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Row}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/picking [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	rows, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		Filters: parseFilters(c),
		Page:    page, Limit: limit,
		SortBy: c.Query("sort_by"), SortDir: c.Query("sort_dir"),
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list picking report", "INTERNAL_ERROR")
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

	apihttp.List(c, rows, apihttp.ListMeta{
		Page: effectivePage, Limit: effectiveLimit, Total: int(total), TotalPages: totalPages,
	})
}

// Summary godoc
//
//	@Summary		Picking & Stock Issue Report KPIs
//	@Description	Totals and short-pick rate over the exact same filter scope as the list/export endpoints. The full Picking KPI Dashboard (productivity, cycle time, on-time rate, ...) is Phase 34F's own scope. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=Summary}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/picking/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	summary, err := h.service.Summary(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to summarize picking report", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

func (h *Handler) rowsForExport(c *gin.Context, tenantID int64) ([]Row, bool) {
	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export picking report", "INTERNAL_ERROR")
		return nil, false
	}
	return rows, true
}

func csvHeadersAndRows(rows []Row) ([]string, [][]string) {
	headers := []string{
		"pick_no", "order_no", "picking_mode", "batch_no", "customer_name", "ship_to_name",
		"warehouse_code", "warehouse_name", "sku", "product_name", "brand_name", "category_name",
		"location_code", "location_name", "lot_no", "expiry_date", "lpn", "uom",
		"requested_qty", "allocated_qty", "picked_qty", "short_qty", "short_reason", "dispatched_qty",
		"item_status", "task_status", "assigned_picker", "start_date_time", "complete_date_time",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		shortQty, dispatchedQty := "", ""
		if r.ShortQty != nil {
			shortQty = csvexport.FormatFloat(*r.ShortQty)
		}
		if r.DispatchedQty != nil {
			dispatchedQty = csvexport.FormatFloat(*r.DispatchedQty)
		}
		csvRows = append(csvRows, []string{
			r.PickNo, r.OrderNo, r.PickingMode, csvexport.FormatString(r.BatchNo),
			csvexport.FormatString(r.CustomerName), csvexport.FormatString(r.ShipToName),
			r.WarehouseCode, r.WarehouseName, r.SKU, r.ProductName, csvexport.FormatString(r.BrandName), csvexport.FormatString(r.CategoryName),
			r.LocationCode, r.LocationName, r.LotNo, csvexport.FormatTime(r.ExpiryDate), csvexport.FormatString(r.LPN), r.UOM,
			csvexport.FormatFloat(r.RequestedQty), csvexport.FormatFloat(r.AllocatedQty), csvexport.FormatFloat(r.PickedQty),
			shortQty, csvexport.FormatString(r.ShortReason), dispatchedQty,
			r.ItemStatus, r.TaskStatus, csvexport.FormatString(r.AssignedPicker),
			csvexport.FormatTime(r.StartDateTime), csvexport.FormatTime(r.CompleteDateTime),
		})
	}
	return headers, csvRows
}

// ExportCSV godoc
//
//	@Summary		Export the Picking & Stock Issue Report as CSV
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/picking/export [get]
func (h *Handler) ExportCSV(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	rows, ok := h.rowsForExport(c, tenantID)
	if !ok {
		return
	}
	headers, csvRows := csvHeadersAndRows(rows)
	if err := csvexport.ServeAttachment(c.Writer, "picking_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ExportXLSX godoc
//
//	@Summary		Export the Picking & Stock Issue Report as Excel
//	@Tags			Reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/picking/export/xlsx [get]
func (h *Handler) ExportXLSX(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	rows, ok := h.rowsForExport(c, tenantID)
	if !ok {
		return
	}
	headers, dataRows := csvHeadersAndRows(rows)
	if err := xlsxexport.ServeAttachment(c.Writer, "picking_report", "Picking Report", headers, dataRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// PDF godoc
//
//	@Summary		Picking & Stock Issue Report PDF (preview or download)
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/reports/picking/pdf [get]
func (h *Handler) PDF(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		writeUnauthenticated(c)
		return
	}

	data, err := h.service.RenderPDF(c.Request.Context(), tenantID, userID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render picking report pdf", "INTERNAL_ERROR")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := csvexport.SafeFilename("picking_report_" + time.Now().UTC().Format("20060102_150405") + ".pdf")
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

// ListBatchAllocations godoc
//
//	@Summary		Batch/Wave Picking Report — per-order allocation detail
//	@Description	One row per order's own share of a batch-consolidated picking line (picking_batch_allocations grain) — distinct from the main list's picking_task_items grain. allocated_qty here is this order's own exclusive share; shared_task_picked_qty is cross-reference context only (the shared physical line's total pick) and must never be summed across rows of the same task item. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query	int		false	"Page number"			default(1)
//	@Param			limit			query	int		false	"Page size (max 100)"	default(20)
//	@Param			from			query	string	false	"Inclusive start, RFC3339 UTC, on the batch's created_at"
//	@Param			to				query	string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			search			query	string	false	"Matches batch no. or order no."
//	@Success		200				{object}	apihttp.ListResponse{data=[]BatchAllocationRow}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/picking/batch-allocations [get]
func (h *Handler) ListBatchAllocations(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	rows, total, err := h.service.ListBatchAllocations(c.Request.Context(), tenantID, BatchAllocationListParams{
		BatchAllocationFilters: parseBatchAllocationFilters(c),
		Page:                   page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list batch allocations report", "INTERNAL_ERROR")
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

	apihttp.List(c, rows, apihttp.ListMeta{
		Page: effectivePage, Limit: effectiveLimit, Total: int(total), TotalPages: totalPages,
	})
}

// ExportBatchAllocationsCSV godoc
//
//	@Summary		Export the Batch/Wave Picking Report as CSV
//	@Description	CSV only for this secondary view (not XLSX/PDF) — a deliberate scope decision, not an oversight; the main Picking Report already covers full PDF/Excel/CSV export.
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/picking/batch-allocations/export [get]
func (h *Handler) ExportBatchAllocationsCSV(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	rows, err := h.service.ListBatchAllocationsForExport(c.Request.Context(), tenantID, parseBatchAllocationFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export batch allocations report", "INTERNAL_ERROR")
		return
	}

	headers := []string{
		"batch_no", "task_pick_no", "order_no", "customer_name", "warehouse_code", "warehouse_name",
		"sku", "product_name", "location_code", "lot_no", "uom",
		"allocated_qty", "sorted_qty", "shortage_qty", "exception_reason", "shared_task_picked_qty",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		shortageQty, sharedPicked := "", ""
		if r.ShortageQty != nil {
			shortageQty = csvexport.FormatFloat(*r.ShortageQty)
		}
		if r.SharedTaskPickedQty != nil {
			sharedPicked = csvexport.FormatFloat(*r.SharedTaskPickedQty)
		}
		csvRows = append(csvRows, []string{
			r.BatchNo, csvexport.FormatString(r.TaskPickNo), r.OrderNo, csvexport.FormatString(r.CustomerName),
			r.WarehouseCode, r.WarehouseName, r.SKU, r.ProductName, r.LocationCode, r.LotNo, r.UOM,
			csvexport.FormatFloat(r.AllocatedQty), csvexport.FormatFloat(r.SortedQty), shortageQty,
			csvexport.FormatString(r.ExceptionReason), sharedPicked,
		})
	}
	if err := csvexport.ServeAttachment(c.Writer, "picking_batch_allocations", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

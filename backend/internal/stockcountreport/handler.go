package stockcountreport

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

// Register mounts the Stock Count Report endpoints on rg, gated on the
// Phase 34A report module — independent of stock_adjustment.*/
// stockcount's own reused permission codes (internal/stockcount has no
// RBAC module of its own; see internal/stockcount/handler.go's doc).
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	reports := rg.Group("/reports/stock-count",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	reports.GET("", middleware.RequirePermission("report.view"), h.List)
	reports.GET("/summary", middleware.RequirePermission("report.view"), h.Summary)
	reports.GET("/export", middleware.RequirePermission("report.export"), h.ExportCSV)
	reports.GET("/export/xlsx", middleware.RequirePermission("report.export"), h.ExportXLSX)
	reports.GET("/pdf", middleware.RequirePermission("report.export"), h.PDF)
}

func parseBoolPtr(raw string) *bool {
	if raw == "" {
		return nil
	}
	v := raw == "true"
	return &v
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
	if raw := c.Query("zone_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.ZoneID = &id
		}
	}
	if raw := c.Query("location_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.LocationID = &id
		}
	}
	if raw := c.Query("counter_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.CounterID = &id
		}
	}
	f.Status = c.Query("status")
	f.ItemStatus = c.Query("item_status")
	f.Search = c.Query("search")
	f.VarianceOnly = c.Query("variance_only") == "true"
	f.HasAdjustment = parseBoolPtr(c.Query("has_adjustment"))

	return f
}

// List godoc
//
//	@Summary		List Stock Count Report rows
//	@Description	Line-level Stock Count Report, one row per stock_count_item. Covers Detail/Variance/Recount/Adjustment-Reference views via the variance_only/item_status/has_adjustment filters over the same underlying data. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query	int		false	"Page number"			default(1)
//	@Param			limit			query	int		false	"Page size (max 100)"	default(20)
//	@Param			from			query	string	false	"Inclusive start, RFC3339 UTC"
//	@Param			to				query	string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			zone_id			query	int		false	"Filter by zone"
//	@Param			location_id		query	int		false	"Filter by location"
//	@Param			counter_id		query	int		false	"Filter by the count session's creator"
//	@Param			status			query	string	false	"Count session status: DRAFT, CONFIRMED, ADJUSTMENT_CREATED"
//	@Param			item_status		query	string	false	"Line status: PENDING, COUNTED, RECOUNT"
//	@Param			search			query	string	false	"Matches count no., SKU, or product name"
//	@Param			variance_only	query	bool	false	"Only lines with a non-zero variance"
//	@Param			has_adjustment	query	bool	false	"true: only lines with a generated adjustment; false: only lines without one"
//	@Param			sort_by			query	string	false	"count_date (default), count_no, warehouse_name, status"
//	@Param			sort_dir		query	string	false	"asc or desc (default desc)"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Row}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/stock-count [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list stock count report", "INTERNAL_ERROR")
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
//	@Summary		Stock Count Report KPIs
//	@Description	KPIs computed over the exact same filter scope as the list/export endpoints. stock_count_accuracy and recount_rate are null when there is nothing in scope to compute them from. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=Summary}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/stock-count/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	summary, err := h.service.Summary(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to summarize stock count report", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

func (h *Handler) rowsForExport(c *gin.Context, tenantID int64) ([]Row, bool) {
	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export stock count report", "INTERNAL_ERROR")
		return nil, false
	}
	return rows, true
}

func csvHeadersAndRows(rows []Row) ([]string, [][]string) {
	headers := []string{
		"count_no", "count_type", "count_date_time", "warehouse_code", "warehouse_name",
		"zone_code", "zone_name", "location_code", "location_name",
		"sku", "product_name", "brand_name", "category_name",
		"lot_no", "lpn", "uom", "system_qty", "counted_qty", "variance_qty", "variance_pct",
		"item_status", "count_status", "counted_by", "reviewed_by", "approved_by",
		"adjustment_no", "adjustment_status",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		countedQty, varianceQty, variancePct := "", "", ""
		if r.CountedQty != nil {
			countedQty = csvexport.FormatFloat(*r.CountedQty)
		}
		if r.VarianceQty != nil {
			varianceQty = csvexport.FormatFloat(*r.VarianceQty)
		}
		if r.VariancePct != nil {
			variancePct = csvexport.FormatFloat(*r.VariancePct)
		}
		csvRows = append(csvRows, []string{
			r.CountNo, r.CountType, csvexport.FormatTime(r.CountDateTime), r.WarehouseCode, r.WarehouseName,
			csvexport.FormatString(r.ZoneCode), csvexport.FormatString(r.ZoneName), r.LocationCode, r.LocationName,
			r.SKU, r.ProductName, csvexport.FormatString(r.BrandName), csvexport.FormatString(r.CategoryName),
			r.LotNo, csvexport.FormatString(r.LPN), r.UOM, csvexport.FormatFloat(r.SystemQty), countedQty, varianceQty, variancePct,
			r.ItemStatus, r.CountStatus, csvexport.FormatString(r.CountedBy), csvexport.FormatString(r.ReviewedBy), csvexport.FormatString(r.ApprovedBy),
			csvexport.FormatString(r.AdjustmentNo), csvexport.FormatString(r.AdjustmentStatus),
		})
	}
	return headers, csvRows
}

// ExportCSV godoc
//
//	@Summary		Export the Stock Count Report as CSV
//	@Description	Synchronous CSV export honoring the exact same filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on report.export.
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/stock-count/export [get]
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
	if err := csvexport.ServeAttachment(c.Writer, "stock_count_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ExportXLSX godoc
//
//	@Summary		Export the Stock Count Report as Excel
//	@Description	Synchronous XLSX export honoring the exact same filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on report.export.
//	@Tags			Reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/stock-count/export/xlsx [get]
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
	if err := xlsxexport.ServeAttachment(c.Writer, "stock_count_report", "Stock Count Report", headers, dataRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// PDF godoc
//
//	@Summary		Stock Count Report PDF (preview or download)
//	@Description	Renders the same filtered KPIs/rows as a printable PDF — inline by default (Preview), attachment when ?download=true (Download). Capped at csvexport.MaxExportRows rows. Gated on report.export.
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			download	query	boolean	false	"true for a download (Content-Disposition: attachment), omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/reports/stock-count/pdf [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render stock count report pdf", "INTERNAL_ERROR")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := csvexport.SafeFilename("stock_count_report_" + time.Now().UTC().Format("20060102_150405") + ".pdf")
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

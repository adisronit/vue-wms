package receivingreport

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

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Receiving Report endpoints on rg, gated on the
// Phase 34A report module rather than receiving.* — a report viewer
// role need not be granted the underlying operational receiving.view
// permission, and vice versa (see internal/rbac/catalog.go's own
// "report" module doc).
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	reports := rg.Group("/reports/receiving",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	reports.GET("", middleware.RequirePermission("report.view"), h.List)
	reports.GET("/summary", middleware.RequirePermission("report.view"), h.Summary)
	reports.GET("/export", middleware.RequirePermission("report.export"), h.ExportCSV)
	reports.GET("/export/xlsx", middleware.RequirePermission("report.export"), h.ExportXLSX)
	reports.GET("/pdf", middleware.RequirePermission("report.export"), h.PDF)
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
	if raw := c.Query("supplier_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.SupplierID = &id
		}
	}
	if raw := c.Query("operator_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.OperatorID = &id
		}
	}
	f.Status = c.Query("status")
	f.Search = c.Query("search")
	f.LotNo = c.Query("lot_no")

	return f
}

// List godoc
//
//	@Summary		List Receiving Report rows
//	@Description	Line-level Receiving Report, one row per receipt item split (undetailed items not yet received produce one row with null lot/pallet/temperature fields). Supports the shared Reports date range (inclusive from, exclusive to, RFC3339) plus warehouse/supplier/status/operator/search/lot filters. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			from			query		string	false	"Inclusive start, RFC3339 UTC"
//	@Param			to				query		string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			supplier_id		query		int		false	"Filter by supplier"
//	@Param			operator_id		query		int		false	"Filter by receiving operator (created_by)"
//	@Param			status			query		string	false	"Receipt status"
//	@Param			search			query		string	false	"Matches receipt no, reference no, SKU, or product name"
//	@Param			lot_no			query		string	false	"Matches lot/batch number"
//	@Param			sort_by			query		string	false	"receiving_date (default), receipt_no, warehouse_name, status"
//	@Param			sort_dir		query		string	false	"asc or desc (default desc)"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Row}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/receiving [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list receiving report", "INTERNAL_ERROR")
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
//	@Summary		Receiving Report KPIs and chart data
//	@Description	KPIs and chart series (trend, by supplier, by warehouse, by operator) computed over the exact same filter scope as the list/export endpoints. completion_rate and avg_receiving_lead_time_hours are null when there are no receipts in scope to compute them from; supplier_delivery_variance is always null (no expected-delivery-date field exists to compare against). Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			from			query		string	false	"Inclusive start, RFC3339 UTC"
//	@Param			to				query		string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			supplier_id		query		int		false	"Filter by supplier"
//	@Param			operator_id		query		int		false	"Filter by receiving operator (created_by)"
//	@Param			status			query		string	false	"Receipt status"
//	@Param			search			query		string	false	"Matches receipt no, reference no, SKU, or product name"
//	@Param			lot_no			query		string	false	"Matches lot/batch number"
//	@Success		200				{object}	apihttp.SuccessResponse{data=Summary}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/receiving/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	summary, err := h.service.Summary(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to summarize receiving report", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

func (h *Handler) rowsForExport(c *gin.Context, tenantID int64) ([]Row, bool) {
	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export receiving report", "INTERNAL_ERROR")
		return nil, false
	}
	return rows, true
}

func csvHeadersAndRows(rows []Row) ([]string, [][]string) {
	headers := []string{
		"receipt_no", "receiving_date", "receiving_type", "reference_no", "supplier_name",
		"invoice_no", "delivery_note_no", "warehouse_code", "warehouse_name",
		"sku", "product_name", "brand_name", "category_name",
		"expected_qty", "received_qty", "short_over_qty", "uom",
		"lot_no", "mfg_date", "expiry_date", "pallet_id", "temperature_c",
		"receiving_status", "qc_status", "putaway_status", "operator_name",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		temp := ""
		if r.TemperatureC != nil {
			temp = csvexport.FormatFloat(*r.TemperatureC)
		}
		csvRows = append(csvRows, []string{
			r.ReceiptNo, csvexport.FormatTime(r.ReceivingDate), r.ReceivingType,
			csvexport.FormatString(r.ReferenceNo), csvexport.FormatString(r.SupplierName),
			csvexport.FormatString(r.InvoiceNo), csvexport.FormatString(r.DeliveryNoteNo),
			r.WarehouseCode, r.WarehouseName,
			r.SKU, r.ProductName, csvexport.FormatString(r.BrandName), csvexport.FormatString(r.CategoryName),
			csvexport.FormatFloat(r.ExpectedQty), csvexport.FormatFloat(r.ReceivedQty), csvexport.FormatFloat(r.ShortOverQty), r.UOM,
			csvexport.FormatString(r.LotNo), csvexport.FormatTime(r.MfgDate), csvexport.FormatTime(r.ExpiryDate),
			csvexport.FormatString(r.PalletID), temp,
			r.ReceivingStatus, r.QCStatus, r.PutawayStatus, csvexport.FormatString(r.OperatorName),
		})
	}
	return headers, csvRows
}

// ExportCSV godoc
//
//	@Summary		Export the Receiving Report as CSV
//	@Description	Synchronous CSV export honoring the exact same filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on report.export.
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/receiving/export [get]
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
	if err := csvexport.ServeAttachment(c.Writer, "receiving_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ExportXLSX godoc
//
//	@Summary		Export the Receiving Report as Excel
//	@Description	Synchronous XLSX export honoring the exact same filters as the list endpoint, capped at csvexport.MaxExportRows. Gated on report.export.
//	@Tags			Reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/receiving/export/xlsx [get]
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
	if err := xlsxexport.ServeAttachment(c.Writer, "receiving_report", "Receiving Report", headers, dataRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// PDF godoc
//
//	@Summary		Receiving Report PDF (preview or download)
//	@Description	Renders the same filtered KPIs/rows as a printable PDF — inline by default (Preview), attachment when ?download=true (Download). Capped at csvexport.MaxExportRows rows. Gated on report.export.
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			download	query	boolean	false	"true for a download (Content-Disposition: attachment), omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/reports/receiving/pdf [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render receiving report pdf", "INTERNAL_ERROR")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := csvexport.SafeFilename("receiving_report_" + time.Now().UTC().Format("20060102_150405") + ".pdf")
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

package qcreport

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

// Register mounts the QC Report endpoints on rg, gated on the Phase
// 34A report module rather than qualitycheck.* — a report viewer role
// need not be granted the underlying operational qualitycheck.view
// permission, and vice versa.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	reports := rg.Group("/reports/qc",
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
	if raw := c.Query("inspector_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.InspectorID = &id
		}
	}
	f.QCType = c.Query("qc_type")
	f.Search = c.Query("search")
	f.Disposition = c.Query("resolution_status")
	f.DefectCode = c.Query("defect_code")

	return f
}

// List godoc
//
//	@Summary		List QC Report rows
//	@Description	Item-level QC Report, one row per qc_inspection_item (an inspection that has at least started). Resolution Status is one of PENDING, PARTIAL, MIXED, PASSED, REJECTED. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page				query		int		false	"Page number"			default(1)
//	@Param			limit				query		int		false	"Page size (max 100)"	default(20)
//	@Param			from				query		string	false	"Inclusive start, RFC3339 UTC, on the inspection's created_at"
//	@Param			to					query		string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id		query		int		false	"Filter by warehouse"
//	@Param			supplier_id			query		int		false	"Filter by supplier (via the receipt)"
//	@Param			inspector_id		query		int		false	"Filter by inspector"
//	@Param			qc_type				query		string	false	"NORMAL, FULL, RANDOM, or DAMAGE"
//	@Param			search				query		string	false	"Matches inspection no, receipt no, SKU, or product name"
//	@Param			resolution_status	query		string	false	"Filter to one Resolution Status (list-only, not applied to Summary KPIs)"
//	@Param			defect_code			query		string	false	"Filter by defect code (list-only, not applied to Summary KPIs)"
//	@Param			sort_by				query		string	false	"inspection_date (default), inspection_no, warehouse_name, resolution_status"
//	@Param			sort_dir			query		string	false	"asc or desc (default desc)"
//	@Success		200					{object}	apihttp.ListResponse{data=[]Row}
//	@Failure		401					{object}	apihttp.ErrorResponse
//	@Failure		403					{object}	apihttp.ErrorResponse
//	@Router			/reports/qc [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list qc report", "INTERNAL_ERROR")
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
//	@Summary		QC Report KPIs and chart data
//	@Description	Pass/Rejection/Hold rates, the two Pending QC sub-cases (not started vs awaiting decision), QC Turnaround Time, Hold Aging, Top Defect Types, and Supplier Rejection Trend — computed over the exact same filter scope as the list/export endpoints (excluding the list-only resolution_status/defect_code refinements). Every rate is null when its denominator is zero. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			from			query		string	false	"Inclusive start, RFC3339 UTC"
//	@Param			to				query		string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			supplier_id		query		int		false	"Filter by supplier"
//	@Param			inspector_id	query		int		false	"Filter by inspector"
//	@Param			qc_type			query		string	false	"NORMAL, FULL, RANDOM, or DAMAGE"
//	@Param			search			query		string	false	"Matches inspection no, receipt no, SKU, or product name"
//	@Success		200				{object}	apihttp.SuccessResponse{data=Summary}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/qc/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	summary, err := h.service.Summary(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to summarize qc report", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

func (h *Handler) rowsForExport(c *gin.Context, tenantID int64) ([]Row, bool) {
	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export qc report", "INTERNAL_ERROR")
		return nil, false
	}
	return rows, true
}

func csvHeadersAndRows(rows []Row) ([]string, [][]string) {
	headers := []string{
		"inspection_no", "receipt_no", "warehouse_code", "warehouse_name", "supplier_name",
		"inspector_name", "qc_type", "inspection_created_at",
		"sku", "product_name", "lot_no", "expiry_date", "location_code",
		"held_qty_snapshot", "sample_qty", "temperature_c", "packaging_ok", "label_ok",
		"pass_qty", "reject_qty", "remaining_hold_qty", "resolution_status",
		"defect_code", "evidence_reference", "remark",
		"qc_turnaround_hours", "hold_aging_hours",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		temp, pkgOK, lblOK := "", "", ""
		if r.TemperatureC != nil {
			temp = csvexport.FormatFloat(*r.TemperatureC)
		}
		if r.PackagingOK != nil {
			pkgOK = strconv.FormatBool(*r.PackagingOK)
		}
		if r.LabelOK != nil {
			lblOK = strconv.FormatBool(*r.LabelOK)
		}
		turnaround, holdAging := "", ""
		if r.QCTurnaroundHours != nil {
			turnaround = csvexport.FormatFloat(*r.QCTurnaroundHours)
		}
		if r.HoldAgingHours != nil {
			holdAging = csvexport.FormatFloat(*r.HoldAgingHours)
		}
		csvRows = append(csvRows, []string{
			r.InspectionNo, r.ReceiptNo, r.WarehouseCode, r.WarehouseName, csvexport.FormatString(r.SupplierName),
			csvexport.FormatString(r.InspectorName), r.QCType, csvexport.FormatTimeValue(r.InspectionCreatedAt),
			r.SKU, r.ProductName, r.LotNo, csvexport.FormatTime(r.ExpiryDate), csvexport.FormatString(r.LocationCode),
			csvexport.FormatFloat(r.HeldQtySnapshot), csvexport.FormatFloat(r.SampleQty), temp, pkgOK, lblOK,
			csvexport.FormatFloat(r.PassQty), csvexport.FormatFloat(r.RejectQty), csvexport.FormatFloat(r.RemainingHoldQty), r.ResolutionStatus,
			csvexport.FormatString(r.DefectCode), csvexport.FormatString(r.EvidenceReference), csvexport.FormatString(r.Remark),
			turnaround, holdAging,
		})
	}
	return headers, csvRows
}

// ExportCSV godoc
//
//	@Summary		Export the QC Report as CSV
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/qc/export [get]
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
	if err := csvexport.ServeAttachment(c.Writer, "qc_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ExportXLSX godoc
//
//	@Summary		Export the QC Report as Excel
//	@Tags			Reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/qc/export/xlsx [get]
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
	if err := xlsxexport.ServeAttachment(c.Writer, "qc_report", "QC Report", headers, dataRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// PDF godoc
//
//	@Summary		QC Report PDF (preview or download)
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/reports/qc/pdf [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render qc report pdf", "INTERNAL_ERROR")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := csvexport.SafeFilename("qc_report_" + time.Now().UTC().Format("20060102_150405") + ".pdf")
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

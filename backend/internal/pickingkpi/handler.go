package pickingkpi

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

// Register mounts the Picking KPI Dashboard endpoints on rg, gated on
// the Phase 34A report module.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	reports := rg.Group("/reports/picking-kpi",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
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
	if raw := c.Query("picker_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.PickerID = &id
		}
	}
	return f
}

// Summary godoc
//
//	@Summary		Picking KPI Dashboard
//	@Description	Lines/Units per hour, Picking Accuracy (scanned subset only), Completion Rate, On-Time Rate (tasks with a due date only), Average Cycle Time, Short Pick Rate, Re-pick Rate, and breakdowns by operator/warehouse/picking-mode — every nullable field is null (render "N/A") when its denominator is zero or the backing data doesn't exist; see the package doc for the exact formula and caveat behind each one. Shift Performance and Target vs Actual are not implemented — no shift/schedule or quota field exists anywhere in this schema. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			from			query	string	false	"Inclusive start, RFC3339 UTC, on the task's created_at"
//	@Param			to				query	string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			picker_id		query	int		false	"Filter by assigned picker"
//	@Success		200				{object}	apihttp.SuccessResponse{data=Summary}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/picking-kpi/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	summary, err := h.service.Summary(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to compute picking kpi dashboard", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

func (h *Handler) sourceRowsForExport(c *gin.Context, tenantID int64) ([]SourceRow, bool) {
	rows, err := h.service.ListSource(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export picking kpi source details", "INTERNAL_ERROR")
		return nil, false
	}
	return rows, true
}

func csvHeadersAndRows(rows []SourceRow) ([]string, [][]string) {
	headers := []string{
		"task_id", "pick_no", "warehouse_name", "picking_mode", "assigned_picker",
		"created_at", "assigned_at", "completed_at", "due_date",
		"sku", "ordered_qty", "picked_qty", "short_qty", "repick_count", "item_status", "task_status",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		shortQty := ""
		if r.ShortQty != nil {
			shortQty = csvexport.FormatFloat(*r.ShortQty)
		}
		csvRows = append(csvRows, []string{
			strconv.FormatInt(r.TaskID, 10), r.PickNo, r.WarehouseName, r.PickingMode, csvexport.FormatString(r.AssignedPicker),
			csvexport.FormatTimeValue(r.CreatedAt), csvexport.FormatTime(r.AssignedAt), csvexport.FormatTime(r.CompletedAt), csvexport.FormatTime(r.DueDate),
			r.SKU, csvexport.FormatFloat(r.OrderedQty), csvexport.FormatFloat(r.PickedQty), shortQty,
			strconv.Itoa(r.RepickCount), r.ItemStatus, r.TaskStatus,
		})
	}
	return headers, csvRows
}

// ExportCSV godoc
//
//	@Summary		Export the Picking KPI source rows as CSV
//	@Description	The picking_task_item rows that feed the dashboard's KPIs, for audit — not a report of its own.
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/picking-kpi/export [get]
func (h *Handler) ExportCSV(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	rows, ok := h.sourceRowsForExport(c, tenantID)
	if !ok {
		return
	}
	headers, csvRows := csvHeadersAndRows(rows)
	if err := csvexport.ServeAttachment(c.Writer, "picking_kpi_source", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ExportXLSX godoc
//
//	@Summary		Export the Picking KPI source rows as Excel
//	@Tags			Reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/picking-kpi/export/xlsx [get]
func (h *Handler) ExportXLSX(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	rows, ok := h.sourceRowsForExport(c, tenantID)
	if !ok {
		return
	}
	headers, dataRows := csvHeadersAndRows(rows)
	if err := xlsxexport.ServeAttachment(c.Writer, "picking_kpi_source", "Picking KPI Source", headers, dataRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// PDF godoc
//
//	@Summary		Picking KPI Dashboard PDF (preview or download)
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/reports/picking-kpi/pdf [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render picking kpi dashboard pdf", "INTERNAL_ERROR")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := csvexport.SafeFilename("picking_kpi_dashboard_" + time.Now().UTC().Format("20060102_150405") + ".pdf")
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

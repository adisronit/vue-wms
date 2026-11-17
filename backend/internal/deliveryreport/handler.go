package deliveryreport

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

// Register mounts the Delivery & Shipping Report endpoints on rg,
// gated on the Phase 34A report module rather than shipping.*/
// loading.*.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	reports := rg.Group("/reports/delivery",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	reports.GET("", middleware.RequirePermission("report.view"), h.List)
	reports.GET("/summary", middleware.RequirePermission("report.view"), h.Summary)
	reports.GET("/export", middleware.RequirePermission("report.export"), h.ExportCSV)
	reports.GET("/export/xlsx", middleware.RequirePermission("report.export"), h.ExportXLSX)
	reports.GET("/pdf", middleware.RequirePermission("report.export"), h.PDF)
	reports.GET("/trips/export", middleware.RequirePermission("report.export"), h.ExportTripsCSV)
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
	f.Carrier = c.Query("carrier")
	f.Status = c.Query("status")
	f.OrderType = c.Query("order_type")
	f.Search = c.Query("search")

	return f
}

// List godoc
//
//	@Summary		List Delivery & Shipping Report rows (Shipment Item Detail)
//	@Description	Line-level report, one row per shipment item. DeliveredAt reflects a manual "Mark Delivered" action with no proof-of-delivery evidence behind it — never inferred from DISPATCHED/IN_TRANSIT. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			from			query		string	false	"Inclusive start, RFC3339 UTC, on the shipment's created_at"
//	@Param			to				query		string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse (via the linked packing job)"
//	@Param			carrier			query		string	false	"DHL, KERRY, FLASH, or INTERNAL"
//	@Param			status			query		string	false	"READY, DISPATCHED, IN_TRANSIT, DELIVERED, ISSUE, or CANCELLED"
//	@Param			order_type		query		string	false	"SALES_ORDER or INTERNAL_TRANSFER"
//	@Param			search			query		string	false	"Matches shipment no, order no, tracking no, SKU, or product name"
//	@Param			sort_by			query		string	false	"ship_date (default), shipment_no, status"
//	@Param			sort_dir		query		string	false	"asc or desc (default desc)"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Row}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/delivery [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list delivery report", "INTERNAL_ERROR")
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
//	@Summary		Delivery & Shipping Report KPIs and chart data
//	@Description	Dispatch Rate, Delivery Confirmation Rate (self-reported "Mark Delivered", not carrier-verified proof of delivery), Exception Rate (shipments.status=ISSUE only — no closed reason taxonomy exists), Avg. Delivery Lead Time (dispatched_at to delivered_at, only when both are set), plus Loading & Dispatch trip counts/duration. Every rate is null when its denominator is zero. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			from			query		string	false	"Inclusive start, RFC3339 UTC"
//	@Param			to				query		string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			carrier			query		string	false	"DHL, KERRY, FLASH, or INTERNAL"
//	@Param			status			query		string	false	"Shipment status"
//	@Param			order_type		query		string	false	"SALES_ORDER or INTERNAL_TRANSFER"
//	@Param			search			query		string	false	"Matches shipment no, order no, tracking no, SKU, or product name"
//	@Success		200				{object}	apihttp.SuccessResponse{data=Summary}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/delivery/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	summary, err := h.service.Summary(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to summarize delivery report", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

func (h *Handler) rowsForExport(c *gin.Context, tenantID int64) ([]Row, bool) {
	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export delivery report", "INTERNAL_ERROR")
		return nil, false
	}
	return rows, true
}

func csvHeadersAndRows(rows []Row) ([]string, [][]string) {
	headers := []string{
		"shipment_no", "order_no", "order_type", "customer_name", "ship_to_name", "ship_to_address",
		"warehouse_code", "warehouse_name", "carrier", "tracking_no", "packages", "weight", "status",
		"ship_date", "dispatched_at", "delivered_at",
		"sku", "product_name", "lot_no", "location_code", "qty", "uom",
		"delivery_lead_time_hours", "is_exception", "exception_remark",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		weight, leadTime := "", ""
		if r.Weight != nil {
			weight = csvexport.FormatFloat(*r.Weight)
		}
		if r.DeliveryLeadTimeHours != nil {
			leadTime = csvexport.FormatFloat(*r.DeliveryLeadTimeHours)
		}
		csvRows = append(csvRows, []string{
			r.ShipmentNo, r.OrderNo, r.OrderType, csvexport.FormatString(r.CustomerName),
			csvexport.FormatString(r.ShipToName), csvexport.FormatString(r.ShipToAddress),
			r.WarehouseCode, r.WarehouseName, csvexport.FormatString(r.Carrier), csvexport.FormatString(r.TrackingNo),
			strconv.Itoa(r.Packages), weight, r.Status,
			csvexport.FormatTime(r.ShipDate), csvexport.FormatTime(r.DispatchedAt), csvexport.FormatTime(r.DeliveredAt),
			r.SKU, r.ProductName, r.LotNo, csvexport.FormatString(r.LocationCode), csvexport.FormatFloat(r.Qty), r.UOM,
			leadTime, strconv.FormatBool(r.IsException), csvexport.FormatString(r.ExceptionRemark),
		})
	}
	return headers, csvRows
}

// ExportCSV godoc
//
//	@Summary		Export the Delivery & Shipping Report as CSV
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/delivery/export [get]
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
	if err := csvexport.ServeAttachment(c.Writer, "delivery_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ExportXLSX godoc
//
//	@Summary		Export the Delivery & Shipping Report as Excel
//	@Tags			Reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/delivery/export/xlsx [get]
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
	if err := xlsxexport.ServeAttachment(c.Writer, "delivery_report", "Delivery Report", headers, dataRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// PDF godoc
//
//	@Summary		Delivery & Shipping Report PDF (preview or download)
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/reports/delivery/pdf [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render delivery report pdf", "INTERNAL_ERROR")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := csvexport.SafeFilename("delivery_report_" + time.Now().UTC().Format("20060102_150405") + ".pdf")
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

func tripCsvHeadersAndRows(rows []TripRow) ([]string, [][]string) {
	headers := []string{
		"trip_no", "vehicle_no", "driver_name", "carrier", "loading_dock", "route", "status",
		"scheduled_loading_time", "actual_loading_start_at", "actual_loading_end_at", "completed_at",
		"shipment_count", "carton_count", "short_carton_count", "loading_duration_hours",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		duration := ""
		if r.LoadingDurationHours != nil {
			duration = csvexport.FormatFloat(*r.LoadingDurationHours)
		}
		csvRows = append(csvRows, []string{
			r.TripNo, csvexport.FormatString(r.VehicleNo), csvexport.FormatString(r.DriverName),
			csvexport.FormatString(r.Carrier), csvexport.FormatString(r.LoadingDock), csvexport.FormatString(r.Route), r.Status,
			csvexport.FormatTime(r.ScheduledLoadingTime), csvexport.FormatTime(r.ActualLoadingStartAt), csvexport.FormatTime(r.ActualLoadingEndAt), csvexport.FormatTime(r.CompletedAt),
			strconv.FormatInt(r.ShipmentCount, 10), strconv.FormatInt(r.CartonCount, 10), strconv.FormatInt(r.ShortCartonCount, 10), duration,
		})
	}
	return headers, csvRows
}

// ExportTripsCSV godoc
//
//	@Summary		Export the Loading & Dispatch Report (trip grain) as CSV
//	@Description	CSV export only — a deliberate scope decision (no XLSX/PDF), matching every other Phase 34 secondary-grain view. Scoped by from/to/carrier/search on the trip itself — warehouse/status/order_type do not apply to a loading trip.
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			from	query	string	false	"Inclusive start, RFC3339 UTC, on the trip's created_at"
//	@Param			to		query	string	false	"Exclusive end, RFC3339 UTC"
//	@Param			carrier	query	string	false	"DHL, KERRY, FLASH, or INTERNAL"
//	@Param			search	query	string	false	"Matches trip no, vehicle no, or driver name"
//	@Success		200		{file}	file
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/reports/delivery/trips/export [get]
func (h *Handler) ExportTripsCSV(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	rows, err := h.service.ListTrips(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export loading trips", "INTERNAL_ERROR")
		return
	}
	headers, csvRows := tripCsvHeadersAndRows(rows)
	if err := csvexport.ServeAttachment(c.Writer, "loading_dispatch_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

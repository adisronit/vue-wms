package inventoryreport

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

func writeBadRequest(c *gin.Context, message string) {
	apihttp.Fail(c, http.StatusBadRequest, message, "INVALID_REQUEST")
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Inventory Balance Report endpoints on rg, gated
// on the Phase 34A report module.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	reports := rg.Group("/reports/inventory",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	reports.GET("", middleware.RequirePermission("report.view"), h.List)
	reports.GET("/summary", middleware.RequirePermission("report.view"), h.Summary)
	reports.GET("/export", middleware.RequirePermission("report.export"), h.ExportCSV)
	reports.GET("/export/xlsx", middleware.RequirePermission("report.export"), h.ExportXLSX)
	reports.GET("/pdf", middleware.RequirePermission("report.export"), h.PDF)
	reports.GET("/movements", middleware.RequirePermission("report.view"), h.ListMovements)
	reports.GET("/movements/export", middleware.RequirePermission("report.export"), h.ExportMovementsCSV)
	reports.GET("/as-of", middleware.RequirePermission("report.view"), h.ListAsOf)
}

func parseFilters(c *gin.Context) Filters {
	var f Filters

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
	f.StorageType = c.Query("storage_type")
	f.Search = c.Query("search")
	f.LotNo = c.Query("lot_no")

	if raw := c.Query("below_qty"); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			f.BelowQty = &v
		}
	}
	if raw := c.Query("expiring_within_days"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			f.ExpiringWithinDays = &v
		}
	}
	f.ExpiredOnly = c.Query("expired_only") == "true"
	if raw := c.Query("inactive_days"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			f.InactiveDays = &v
		}
	}

	return f
}

// List godoc
//
//	@Summary		List Inventory Balance Report rows
//	@Description	One row per inventory balance (product, location, lot). Low Stock/Near Expiry/Expired/Slow-Moving are lenses over this same data via below_qty/expiring_within_days/expired_only/inactive_days — not separate reports. Available is always computed (OnHand - Reserved - Hold), never stored. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page					query	int		false	"Page number"			default(1)
//	@Param			limit					query	int		false	"Page size (max 100)"	default(20)
//	@Param			warehouse_id			query	int		false	"Filter by warehouse"
//	@Param			zone_id					query	int		false	"Filter by zone"
//	@Param			location_id				query	int		false	"Filter by location"
//	@Param			storage_type			query	string	false	"Location type: RECEIVING, STORAGE, PICKING, PACKING, SHIPPING, QC, HOLD"
//	@Param			search					query	string	false	"Matches SKU, product name, or lot no."
//	@Param			lot_no					query	string	false	"Exact lot/batch number"
//	@Param			below_qty				query	number	false	"Low Stock lens: only rows with Available strictly below this value"
//	@Param			expiring_within_days	query	int		false	"Near Expiry lens: only rows expiring within N days"
//	@Param			expired_only			query	bool	false	"Only rows already past their expiry date"
//	@Param			inactive_days			query	int		false	"Slow/Non-Moving lens: only rows with no movement in N days (or never moved)"
//	@Param			sort_by					query	string	false	"on_hand (default), available, expiry_date, last_movement_date, warehouse_name, sku"
//	@Param			sort_dir				query	string	false	"asc or desc (default desc)"
//	@Success		200						{object}	apihttp.ListResponse{data=[]Row}
//	@Failure		401						{object}	apihttp.ErrorResponse
//	@Failure		403						{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list inventory report", "INTERNAL_ERROR")
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
//	@Summary		Inventory Balance Report KPIs
//	@Description	On-hand/available totals by UOM, plus Low Stock/Near Expiry/Expired/Non-Moving counts (null/N/A unless the matching lens filter is supplied — there is no stored threshold to default to). Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=Summary}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	summary, err := h.service.Summary(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to summarize inventory report", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

func (h *Handler) rowsForExport(c *gin.Context, tenantID int64) ([]Row, bool) {
	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, parseFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export inventory report", "INTERNAL_ERROR")
		return nil, false
	}
	return rows, true
}

func csvHeadersAndRows(rows []Row) ([]string, [][]string) {
	headers := []string{
		"warehouse_code", "warehouse_name", "zone_code", "zone_name", "location_code", "location_name",
		"storage_type", "sku", "product_name", "brand_name", "category_name",
		"lot_no", "expiry_date", "lpn", "uom", "on_hand", "reserved", "hold", "available",
		"last_movement_date", "inventory_status",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			r.WarehouseCode, r.WarehouseName, r.ZoneCode, r.ZoneName, r.LocationCode, r.LocationName,
			r.StorageType, r.SKU, r.ProductName, csvexport.FormatString(r.BrandName), csvexport.FormatString(r.CategoryName),
			r.LotNo, csvexport.FormatTime(r.ExpiryDate), csvexport.FormatString(r.LPN), r.UOM,
			csvexport.FormatFloat(r.OnHand), csvexport.FormatFloat(r.Reserved), csvexport.FormatFloat(r.Hold), csvexport.FormatFloat(r.Available),
			csvexport.FormatTime(r.LastMovementDate), r.InventoryStatus,
		})
	}
	return headers, csvRows
}

// ExportCSV godoc
//
//	@Summary		Export the Inventory Balance Report as CSV
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory/export [get]
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
	if err := csvexport.ServeAttachment(c.Writer, "inventory_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ExportXLSX godoc
//
//	@Summary		Export the Inventory Balance Report as Excel
//	@Tags			Reports
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory/export/xlsx [get]
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
	if err := xlsxexport.ServeAttachment(c.Writer, "inventory_report", "Inventory Report", headers, dataRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// PDF godoc
//
//	@Summary		Inventory Balance Report PDF (preview or download)
//	@Tags			Reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory/pdf [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to render inventory report pdf", "INTERNAL_ERROR")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := csvexport.SafeFilename("inventory_report_" + time.Now().UTC().Format("20060102_150405") + ".pdf")
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

func parseMovementFilters(c *gin.Context) MovementFilters {
	var f MovementFilters
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
	if raw := c.Query("location_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.LocationID = &id
		}
	}
	f.TransactionType = c.Query("transaction_type")
	f.Search = c.Query("search")
	return f
}

// ListMovements godoc
//
//	@Summary		Stock Movement report
//	@Description	One row per inventory_transactions row (a SEPARATE grain from the balance list above) — quantity is signed (negative = debit, positive = credit). Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page				query	int		false	"Page number"			default(1)
//	@Param			limit				query	int		false	"Page size (max 100)"	default(20)
//	@Param			from				query	string	false	"Inclusive start, RFC3339 UTC"
//	@Param			to					query	string	false	"Exclusive end, RFC3339 UTC"
//	@Param			warehouse_id		query	int		false	"Filter by warehouse"
//	@Param			location_id			query	int		false	"Filter by location"
//	@Param			transaction_type	query	string	false	"RECEIVE, PUTAWAY, MOVE, PICK, SHIP, ADJUST, RETURN, COUNT, REJECT, TRANSFER_RECEIVE"
//	@Param			search				query	string	false	"Matches SKU, product name, or lot no."
//	@Success		200					{object}	apihttp.ListResponse{data=[]MovementRow}
//	@Failure		401					{object}	apihttp.ErrorResponse
//	@Failure		403					{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory/movements [get]
func (h *Handler) ListMovements(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	rows, total, err := h.service.ListMovements(c.Request.Context(), tenantID, MovementListParams{
		MovementFilters: parseMovementFilters(c),
		Page:            page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list stock movements", "INTERNAL_ERROR")
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

// ExportMovementsCSV godoc
//
//	@Summary		Export the Stock Movement report as CSV
//	@Description	CSV only for this secondary view (not XLSX/PDF) — a deliberate scope decision; the main Inventory Report already covers full PDF/Excel/CSV export.
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory/movements/export [get]
func (h *Handler) ExportMovementsCSV(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	rows, err := h.service.ListMovementsForExport(c.Request.Context(), tenantID, parseMovementFilters(c))
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export stock movements", "INTERNAL_ERROR")
		return
	}

	headers := []string{
		"transaction_id", "created_at", "transaction_type", "warehouse_code", "warehouse_name",
		"location_code", "sku", "product_name", "lot_no", "quantity", "reference_type", "reference_id", "operator",
	}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			strconv.FormatInt(r.TransactionID, 10), csvexport.FormatTimeValue(r.CreatedAt), r.TransactionType,
			r.WarehouseCode, r.WarehouseName, r.LocationCode, r.SKU, r.ProductName, r.LotNo,
			csvexport.FormatFloat(r.Quantity), csvexport.FormatString(r.ReferenceType), csvexport.FormatString(r.ReferenceID),
			csvexport.FormatString(r.Operator),
		})
	}
	if err := csvexport.ServeAttachment(c.Writer, "stock_movements", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// ListAsOf godoc
//
//	@Summary		Historical As-of Stock Balance (reconstructed)
//	@Description	Reconstructs each (product, location, lot)'s balance by summing inventory_transactions up to as_of. Reliable only if every balance's full history was captured in the ledger from its first unit — not independently verified against every migration/seed script, so every response carries an explicit caveat string. Gated on report.view.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			as_of			query	string	true	"Cutoff timestamp, RFC3339 UTC"
//	@Param			page			query	int		false	"Page number"			default(1)
//	@Param			limit			query	int		false	"Page size (max 100)"	default(20)
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			location_id		query	int		false	"Filter by location"
//	@Param			search			query	string	false	"Matches SKU, product name, or lot no."
//	@Success		200				{object}	apihttp.ListResponse{data=[]AsOfRow}
//	@Failure		400				{object}	apihttp.ErrorResponse
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory/as-of [get]
func (h *Handler) ListAsOf(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	asOfRaw := c.Query("as_of")
	asOf, err := time.Parse(time.RFC3339, asOfRaw)
	if err != nil {
		writeBadRequest(c, "as_of is required and must be RFC3339 UTC")
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	params := AsOfListParams{
		AsOfFilters: AsOfFilters{AsOf: asOf, Search: c.Query("search")},
		Page:        page, Limit: limit,
	}
	if raw := c.Query("warehouse_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			params.WarehouseID = &id
		}
	}
	if raw := c.Query("location_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			params.LocationID = &id
		}
	}

	rows, total, err := h.service.ListAsOf(c.Request.Context(), tenantID, params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to compute as-of balance", "INTERNAL_ERROR")
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

	// A plain apihttp.List envelope has no room for the caveat every
	// As-of response must carry (see package doc) — built manually here
	// so "data" stays the real row array, consistent with every other
	// list endpoint, with "caveat" as one extra top-level sibling field.
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"meta": apihttp.ListMeta{
			Page: effectivePage, Limit: effectiveLimit, Total: int(total), TotalPages: totalPages,
		},
		"caveat": AsOfCaveat,
	})
}

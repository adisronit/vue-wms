package inventory

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"wms-backend/internal/platform/csvexport"
	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// Handler exposes the read side of Service over HTTP. The mutation
// methods (AddStock/RemoveStock/MoveStock) are deliberately not
// reachable from any route — see service.go's package doc — so this
// handler only ever calls List/Get/Summary/Transactions.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the read-only Inventory endpoints on rg, gated by the
// existing stock_inquiry.view permission (Phase 2) — reused as-is, no
// new permission module, per CLAUDE.md's "don't create duplicate
// concepts" guidance.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	inv := rg.Group("/inventory", middleware.RequireJWT(jwtSecret), middleware.RequirePermission("stock_inquiry.view"))
	inv.GET("/summary", h.Summary)
	inv.GET("", h.List)
	inv.GET("/:id", h.Get)
	inv.GET("/:id/transactions", h.Transactions)
	// Phase 22 export routes additionally require stock_inquiry.export
	// (already cataloged in Phase 2, never enforced until now) on top of
	// the group's own stock_inquiry.view — a role needs both to export,
	// matching the catalog's own reason for the two being separate
	// actions in the first place.
	inv.GET("/export", middleware.RequirePermission("stock_inquiry.export"), h.Export)
}

// List godoc
//
//	@Summary		List current inventory balances
//	@Description	Supports the Stock Inquiry screen's search box plus warehouse/zone/location/status/lot filters, with pagination. Filtering and pagination both happen in SQL Server, not in the browser.
//	@Tags			Inventory
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches SKU, product name, barcode, location code, or lot"
//	@Param			product_id		query		int		false	"Filter by product"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Param			zone_id			query		int		false	"Filter by zone"
//	@Param			location_id		query		int		false	"Filter by location"
//	@Param			status			query		string	false	"AVAILABLE, RESERVED, HOLD, or LOW_STOCK"
//	@Param			lot_no			query		string	false	"Exact lot number"
//	@Success		200				{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/inventory [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	productID, _ := strconv.ParseInt(c.Query("product_id"), 10, 64)
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	zoneID, _ := strconv.ParseInt(c.Query("zone_id"), 10, 64)
	locationID, _ := strconv.ParseInt(c.Query("location_id"), 10, 64)

	params := ListParams{
		Search: c.Query("search"), ProductID: productID,
		WarehouseID: warehouseID, ZoneID: zoneID, LocationID: locationID,
		Status: Status(c.Query("status")), LotNo: c.Query("lot_no"),
		Page: page, Limit: limit,
	}

	responses, total, err := h.service.List(c.Request.Context(), params)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list inventory", "INTERNAL_ERROR")
		return
	}

	effectivePage, effectiveLimit := params.Page, params.Limit
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

// Get godoc
//
//	@Summary		Get one inventory balance by ID
//	@Tags			Inventory
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Inventory ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/inventory/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid inventory id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "inventory record not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get inventory", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Summary godoc
//
//	@Summary		Get inventory summary totals
//	@Description	Matches the Stock Inquiry screen's four summary cards (Total SKUs, Available Qty, Reserved Qty, Low Stock).
//	@Tags			Inventory
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=SummaryResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/inventory/summary [get]
func (h *Handler) Summary(c *gin.Context) {
	summary, err := h.service.Summary(c.Request.Context())
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to load inventory summary", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, summary, "")
}

// Transactions godoc
//
//	@Summary		Get recent ledger entries for one inventory balance
//	@Description	Powers the Stock Inquiry detail drawer's "Recent Movement" panel. Likely empty this phase — no workflow writes to the ledger yet (see PROJECT_STATUS.md).
//	@Tags			Inventory
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Inventory ID"
//	@Success		200	{object}	apihttp.ListResponse{data=[]TransactionResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/inventory/{id}/transactions [get]
func (h *Handler) Transactions(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid inventory id", "INVALID_REQUEST")
		return
	}

	txns, err := h.service.Transactions(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "inventory record not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to load transactions", "INTERNAL_ERROR")
		return
	}

	apihttp.List(c, txns, apihttp.ListMeta{Page: 1, Limit: len(txns), Total: len(txns), TotalPages: 1})
}

// Export godoc
//
//	@Summary		Export the Inventory Stock Report as CSV
//	@Description	Synchronous CSV export (Phase 22) — same search/warehouse_id/zone_id/location_id/status/lot_no filters as the list endpoint, capped at csvexport.MaxExportRows. Requires stock_inquiry.export in addition to the group's own stock_inquiry.view.
//	@Tags			Inventory
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches SKU, product name, or barcode"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Param			zone_id			query	int		false	"Filter by zone"
//	@Param			location_id		query	int		false	"Filter by location"
//	@Param			status			query	string	false	"AVAILABLE, RESERVED, HOLD, or LOW_STOCK"
//	@Param			lot_no			query	string	false	"Filter by lot number"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/inventory/export [get]
func (h *Handler) Export(c *gin.Context) {
	productID, _ := strconv.ParseInt(c.Query("product_id"), 10, 64)
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	zoneID, _ := strconv.ParseInt(c.Query("zone_id"), 10, 64)
	locationID, _ := strconv.ParseInt(c.Query("location_id"), 10, 64)

	rows, err := h.service.ListForExport(c.Request.Context(), ListParams{
		Search: c.Query("search"), ProductID: productID,
		WarehouseID: warehouseID, ZoneID: zoneID, LocationID: locationID,
		Status: Status(c.Query("status")), LotNo: c.Query("lot_no"),
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export inventory", "INTERNAL_ERROR")
		return
	}

	headers := []string{"sku", "product_name", "barcode", "warehouse_code", "zone_code", "location_code", "lot_no", "expiry_date", "available_qty", "reserved_qty", "on_hand_qty", "uom", "status"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			r.SKU, r.ProductName, csvexport.FormatString(r.Barcode), r.WarehouseCode, r.ZoneCode, r.LocationCode,
			r.LotNo, csvexport.FormatTime(r.ExpiryDate), csvexport.FormatFloat(r.AvailableQty),
			csvexport.FormatFloat(r.ReservedQty), csvexport.FormatFloat(r.OnHandQty), r.UOM, string(r.Status),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "inventory_stock_report", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

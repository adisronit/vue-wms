package packing

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

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// Handler exposes Service (and, for the Packing List/Carton Label,
// PDFService) over HTTP.
type Handler struct {
	service *Service
	pdf     *PDFService
}

func NewHandler(service *Service, pdf *PDFService) *Handler {
	return &Handler{service: service, pdf: pdf}
}

// Register mounts the Packing endpoints on rg, using the same
// RequireJWT -> RequireTenantContext -> RequirePermission chain every
// tenant-scoped inbound/outbound module uses.
//
// Permission mapping (packing.view/create/update/confirm/approve, the
// same five-action shape picking/relocation already use):
//   - view: List/Export/Get/both PDFs — read-only.
//   - create: Create — opens a new job from a terminal picking task.
//   - update: AssignPacker/CreateCarton/UpdateCarton/FlagException/
//     PackItem — the ordinary packer's day-to-day execution actions,
//     the same authority level picking.update gates AssignPicker/
//     ScanConfirm at.
//   - confirm: CheckerConfirm — the second-person verification step
//     that marks a job COMPLETED, gated on this distinct code per this
//     phase's own explicit instruction (the same confirm/approve split
//     order.confirm/picking.confirm/relocation.confirm already
//     established, here marking the checker's elevated single action
//     rather than the packer's routine one).
//   - approve: Cancel — the more consequential, planning-level action,
//     matching picking.approve's own Cancel gate.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("/packing-jobs",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("", middleware.RequirePermission("packing.view"), h.List)
	group.GET("/export", middleware.RequirePermission("packing.view"), h.Export)
	group.GET("/sources", middleware.RequirePermission("packing.view"), h.Sources)
	group.GET("/:id", middleware.RequirePermission("packing.view"), h.Get)
	group.GET("/:id/pdf", middleware.RequirePermission("packing.view"), h.PackingList)
	group.GET("/:id/cartons/label", middleware.RequirePermission("packing.view"), h.CartonLabels)
	group.POST("", middleware.RequirePermission("packing.create"), h.Create)
	group.PUT("/:id/assign", middleware.RequirePermission("packing.update"), h.AssignPacker)
	group.POST("/:id/pack", middleware.RequirePermission("packing.update"), h.PackItem)
	group.POST("/:id/items/:itemId/exception", middleware.RequirePermission("packing.update"), h.FlagException)
	group.POST("/:id/cartons", middleware.RequirePermission("packing.update"), h.CreateCarton)
	group.PUT("/:id/cartons/:cartonId", middleware.RequirePermission("packing.update"), h.UpdateCarton)
	group.POST("/:id/checker-confirm", middleware.RequirePermission("packing.confirm"), h.CheckerConfirm)
	group.POST("/:id/cancel", middleware.RequirePermission("packing.approve"), h.Cancel)
}

// servePDF writes data as a PDF response — matches every other
// module's own servePDF helper exactly.
func servePDF(c *gin.Context, data []byte, filename string) {
	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

// List godoc
//
//	@Summary		List packing jobs
//	@Description	Job-level rows — supports search plus status/warehouse filters, with pagination.
//	@Tags			Packing
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches job no, order no, customer, or a line's SKU/product name"
//	@Param			status			query		string	false	"DRAFT, IN_PROGRESS, COMPLETED, or CANCELLED"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/packing-jobs [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	responses, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")), WarehouseID: warehouseID,
		Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list packing jobs", "INTERNAL_ERROR")
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
//	@Summary		Export Packing History as CSV
//	@Description	Synchronous CSV export — same search/status/warehouse_id filters as the list endpoint, capped at csvexport.MaxExportRows.
//	@Tags			Packing
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			search			query	string	false	"Matches job no, order no, or customer"
//	@Param			status			query	string	false	"DRAFT, IN_PROGRESS, COMPLETED, or CANCELLED"
//	@Param			warehouse_id	query	int		false	"Filter by warehouse"
//	@Success		200				{file}	file
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/packing-jobs/export [get]
func (h *Handler) Export(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	rows, err := h.service.ListForExport(c.Request.Context(), tenantID, ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")), WarehouseID: warehouseID,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to export packing jobs", "INTERNAL_ERROR")
		return
	}

	headers := []string{"job_no", "picking_task_id", "order_no", "customer", "warehouse_code", "status", "item_count", "progress_pct", "assigned_packer_name"}
	csvRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		csvRows = append(csvRows, []string{
			r.JobNo, strconv.FormatInt(r.PickingTaskID, 10), r.OrderNo, csvexport.FormatString(r.Customer),
			r.WarehouseCode, string(r.Status), strconv.Itoa(r.ItemCount), csvexport.FormatFloat(r.ProgressPct),
			csvexport.FormatString(r.AssignedPackerName),
		})
	}

	if err := csvexport.ServeAttachment(c.Writer, "packing_history", headers, csvRows); err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to write export", "INTERNAL_ERROR")
	}
}

// Sources godoc
//
//	@Summary		List completed/short-pick picking tasks not yet packed
//	@Description	Powers the "create packing job from picking task" picker — read-only.
//	@Tags			Packing
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.SuccessResponse{data=[]SourceResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/packing-jobs/sources [get]
func (h *Handler) Sources(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	sources, err := h.service.Sources(c.Request.Context(), tenantID)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list packable picking tasks", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, sources, "")
}

// Get godoc
//
//	@Summary		Get a packing job with its lines and cartons
//	@Tags			Packing
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Packing job ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/packing-jobs/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "packing job not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get packing job", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a packing job from a terminal picking task
//	@Description	Copies the picking task's already-picked lines verbatim (never re-derives or inflates PickedQty). The source picking task must be COMPLETED or SHORT_PICK and must not already have a packing job (one-to-one). Never calls any inventory function — stock already left the building when Picking confirmed it.
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New packing job"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"picking task does not exist"
//	@Failure		409		{object}	apihttp.ErrorResponse	"picking task already has a packing job"
//	@Failure		422		{object}	apihttp.ErrorResponse	"picking task is not yet terminal, or has no picked lines"
//	@Router			/packing-jobs [post]
func (h *Handler) Create(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Create(ctx, tenantID, req, userID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "packing job created")
		return
	}
	writeError(c, err)
}

// AssignPacker godoc
//
//	@Summary		Assign (or reassign) a packer to a job
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Packing job ID"
//	@Param			request	body		AssignRequest	true	"Packer to assign"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"job is not in an assignable status"
//	@Router			/packing-jobs/{id}/assign [put]
func (h *Handler) AssignPacker(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	var req AssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.AssignPacker(ctx, tenantID, id, req.PackerID, userID)
	if err == nil {
		apihttp.OK(c, resp, "packer assigned")
		return
	}
	writeError(c, err)
}

// PackItem godoc
//
//	@Summary		Scan a picked item into a carton
//	@Description	Validates the scanned SKU/lot against the line's own expected product/lot, then atomically increments the line's packed_qty (bounded by its own picked_qty — overpacking is rejected, never clamped) and records the carton's own contents. idempotency_key must be a fresh UUID per scan; resending the same key returns the job's current state instead of double-packing. Never calls any inventory function.
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Packing job ID"
//	@Param			request	body		PackItemRequest	true	"Line, carton, scan, and quantity to pack"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"job not packable, line already resolved, or carton not open"
//	@Failure		422		{object}	apihttp.ErrorResponse	"scan mismatch, or packing this quantity would exceed the line's picked quantity"
//	@Router			/packing-jobs/{id}/pack [post]
func (h *Handler) PackItem(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	var req PackItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.PackItem(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "item packed")
		return
	}
	writeError(c, err)
}

// FlagException godoc
//
//	@Summary		Flag a damage/shortage/wrong-item exception on a packing line
//	@Description	Terminates the line (EXCEPTION) regardless of its current packed_qty — a packing-stage discovery, deliberately separate from picking's own short-pick reason.
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Packing job ID"
//	@Param			itemId	path		int						true	"Packing job item ID"
//	@Param			request	body		FlagExceptionRequest	true	"Exception reason/quantity/remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"job not packable, or line already resolved"
//	@Router			/packing-jobs/{id}/items/{itemId}/exception [post]
func (h *Handler) FlagException(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	itemID, err := parseID(c, "itemId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job item id", "INVALID_REQUEST")
		return
	}
	var req FlagExceptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.FlagException(ctx, tenantID, id, itemID, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "exception flagged")
		return
	}
	writeError(c, err)
}

// CreateCarton godoc
//
//	@Summary		Add a new carton to a packing job
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Packing job ID"
//	@Param			request	body		CreateCartonRequest	true	"New carton"
//	@Success		201		{object}	apihttp.SuccessResponse{data=CartonResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"job is completed or cancelled"
//	@Router			/packing-jobs/{id}/cartons [post]
func (h *Handler) CreateCarton(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	var req CreateCartonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.CreateCarton(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "carton created")
		return
	}
	writeError(c, err)
}

// UpdateCarton godoc
//
//	@Summary		Update a carton's packaging/weight/dimensions, or close it
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		int					true	"Packing job ID"
//	@Param			cartonId	path		int					true	"Carton ID"
//	@Param			request		body		UpdateCartonRequest	true	"Updated carton fields"
//	@Success		200			{object}	apihttp.SuccessResponse{data=CartonResponse}
//	@Failure		400			{object}	apihttp.ErrorResponse
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/packing-jobs/{id}/cartons/{cartonId} [put]
func (h *Handler) UpdateCarton(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	cartonID, err := parseID(c, "cartonId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid carton id", "INVALID_REQUEST")
		return
	}
	var req UpdateCartonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.UpdateCarton(ctx, tenantID, id, cartonID, req)
	if err == nil {
		apihttp.OK(c, resp, "carton updated")
		return
	}
	writeError(c, err)
}

// CheckerConfirm godoc
//
//	@Summary		Second-person checker verification — marks a packing job COMPLETED
//	@Description	Requires the job to be IN_PROGRESS and every line to already be PACKED or EXCEPTION. Never mutates quantity.
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Packing job ID"
//	@Param			request	body		CheckerConfirmRequest	false	"Optional checker remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"job not in IN_PROGRESS, or a line is still unresolved"
//	@Router			/packing-jobs/{id}/checker-confirm [post]
func (h *Handler) CheckerConfirm(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	var req CheckerConfirmRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.CheckerConfirm(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "packing job completed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a packing job
//	@Description	Allowed from DRAFT or IN_PROGRESS. Never calls any inventory function; whatever has already been packed into a carton is left exactly as it is.
//	@Tags			Packing
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Packing job ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"job is not in a cancellable status"
//	@Router			/packing-jobs/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	var req CancelRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Cancel(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "packing job cancelled")
		return
	}
	writeError(c, err)
}

// PackingList godoc
//
//	@Summary		Download the Packing List PDF for a job
//	@Description	Job header plus every line (SKU/product/lot, ordered/picked/packed qty, which carton(s) it went into). Read-only: printing/reprinting is always safe.
//	@Tags			Packing
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Packing job ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/packing-jobs/{id}/pdf [get]
func (h *Handler) PackingList(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)

	data, filename, err := h.pdf.RenderPackingList(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	servePDF(c, data, filename)
}

// CartonLabels godoc
//
//	@Summary		Download Carton/Pallet Label PDF(s) for a job's cartons
//	@Description	One compact 4x6" label per carton — LPN barcode, destination/customer, contents summary, gross/net weight, dimensions. Omit carton_id to print every carton in the job (one per page); pass it to print just one.
//	@Tags			Packing
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Packing job ID"
//	@Param			carton_id	query	int		false	"Print only this carton"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/packing-jobs/{id}/cartons/label [get]
func (h *Handler) CartonLabels(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid packing job id", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)

	var cartonID int64
	if raw := c.Query("carton_id"); raw != "" {
		cartonID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			apihttp.Fail(c, http.StatusBadRequest, "invalid carton_id", "INVALID_REQUEST")
			return
		}
	}

	data, filename, err := h.pdf.RenderCartonLabels(c.Request.Context(), tenantID, userID, id, cartonID)
	if err != nil {
		writeError(c, err)
		return
	}
	servePDF(c, data, filename)
}

func parseID(c *gin.Context, param string) (int64, error) {
	return strconv.ParseInt(c.Param(param), 10, 64)
}

func writeError(c *gin.Context, err error) {
	var validationErr ErrValidation

	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "packing job not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "packing job item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrCartonNotFound):
		apihttp.Fail(c, http.StatusNotFound, "carton not found", "CARTON_NOT_FOUND")
	case errors.Is(err, ErrPickingTaskNotFound):
		apihttp.Fail(c, http.StatusNotFound, "picking task not found", "PICKING_TASK_NOT_FOUND")
	case errors.Is(err, ErrPackerNotFound):
		apihttp.Fail(c, http.StatusNotFound, "packer user not found", "PACKER_NOT_FOUND")
	case errors.Is(err, ErrAlreadyPacked):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "ALREADY_PACKED")
	case errors.Is(err, ErrNotPackable):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NOT_PACKABLE")
	case errors.Is(err, ErrNoPackableItems):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_PACKABLE_ITEMS")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "packing job is not in a state that allows this action", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotAssignable):
		apihttp.Fail(c, http.StatusConflict, "packing job is not in an assignable status", "NOT_ASSIGNABLE")
	case errors.Is(err, ErrNotReadyToComplete):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "NOT_READY_TO_COMPLETE")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "packing job is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.Is(err, ErrCartonNotOpen):
		apihttp.Fail(c, http.StatusConflict, "carton is not open for packing", "CARTON_NOT_OPEN")
	case errors.Is(err, ErrItemNotInJob):
		apihttp.Fail(c, http.StatusNotFound, "item does not belong to this job", "ITEM_NOT_IN_JOB")
	case errors.Is(err, ErrCartonNotInJob):
		apihttp.Fail(c, http.StatusNotFound, "carton does not belong to this job", "CARTON_NOT_IN_JOB")
	case errors.Is(err, ErrScanMismatch):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "SCAN_MISMATCH")
	case errors.Is(err, ErrOverpack):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "OVERPACK")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "packing operation failed", "INTERNAL_ERROR")
	}
}

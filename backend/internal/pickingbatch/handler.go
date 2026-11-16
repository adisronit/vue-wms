package pickingbatch

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"wms-backend/internal/picking"
	"wms-backend/internal/platform/dbctx"
	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Batch Picking endpoints on rg, using a new
// "picking_batch" RBAC catalog module (view/create/confirm/cancel —
// internal/rbac/catalog.go) kept separate from the existing "picking"
// module: a batch is its own header/workflow object (consolidation,
// multi-order claim) distinct from executing a single already-created
// task, the same reasoning relocation/packing each got their own
// module rather than folding into an existing one. No default grant to
// any operational staff role yet (matches relocation/packing's own
// conservative precedent) — only ADMIN (every code) and
// WAREHOUSE_MANAGER (every non-delete code) get it automatically.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("/picking-batches",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("", middleware.RequirePermission("picking_batch.view"), h.List)
	group.GET("/available-orders", middleware.RequirePermission("picking_batch.view"), h.AvailableOrders)
	group.GET("/:id", middleware.RequirePermission("picking_batch.view"), h.Get)
	group.POST("", middleware.RequirePermission("picking_batch.create"), h.Create)
	group.POST("/:id/confirm", middleware.RequirePermission("picking_batch.confirm"), h.Confirm)
	group.POST("/:id/cancel", middleware.RequirePermission("picking_batch.cancel"), h.Cancel)

	// Phase 33C.2 -- Sortation. The worklist read stays on the existing
	// "view" code; the three mutating actions share the new
	// "sortation" code (see internal/rbac/catalog.go's own doc on why
	// this is an addition to the existing picking_batch module, not a
	// new one).
	group.GET("/:id/sortation", middleware.RequirePermission("picking_batch.view"), h.GetSortation)
	group.POST("/:id/sortation/scan", middleware.RequirePermission("picking_batch.sortation"), h.ScanSortation)
	group.POST("/:id/sortation/exception", middleware.RequirePermission("picking_batch.sortation"), h.FlagSortationException)
	group.PUT("/:id/sortation/assign", middleware.RequirePermission("picking_batch.sortation"), h.AssignSortation)
}

// List godoc
//
//	@Summary		List picking batches
//	@Tags			Picking Batches
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			status			query		string	false	"DRAFT, CONFIRMED, COMPLETED, or CANCELLED"
//	@Param			warehouse_id	query		int		false	"Warehouse ID"
//	@Success		200				{object}	apihttp.SuccessResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/picking-batches [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	rows, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		Status: Status(c.Query("status")), WarehouseID: warehouseID, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list picking batches", "INTERNAL_ERROR")
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
	apihttp.List(c, rows, apihttp.ListMeta{Page: effectivePage, Limit: effectiveLimit, Total: int(total), TotalPages: totalPages})
}

// AvailableOrders godoc
//
//	@Summary		List RELEASED, PICK_BY_PRODUCT orders eligible for a new picking batch
//	@Description	Excludes any order already a member of another active (DRAFT/CONFIRMED) batch.
//	@Tags			Picking Batches
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouse_id	query		int	true	"Warehouse ID"
//	@Success		200				{object}	apihttp.SuccessResponse{data=[]AvailableOrderResponse}
//	@Failure		400				{object}	apihttp.ErrorResponse
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/picking-batches/available-orders [get]
func (h *Handler) AvailableOrders(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	warehouseID, err := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	if err != nil || warehouseID <= 0 {
		apihttp.Fail(c, http.StatusBadRequest, "warehouse_id is required", "INVALID_REQUEST")
		return
	}
	rows, err := h.service.ListAvailableOrders(c.Request.Context(), tenantID, warehouseID)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list available orders", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, rows, "")
}

// Get godoc
//
//	@Summary		Get a picking batch
//	@Description	Includes the consolidated lines (product/lot/location) and each line's own per-order allocation breakdown.
//	@Tags			Picking Batches
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Batch ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/picking-batches/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid batch id", "INVALID_REQUEST")
		return
	}
	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// Create godoc
//
//	@Summary		Create a picking batch (DRAFT) by consolidating multiple orders
//	@Description	Validates every order_id is RELEASED, PICK_BY_PRODUCT, in warehouse_id, and not already in another active batch — rejecting (not dropping) any order that fails. Consolidates demand by product, suggests FEFO lot/location allocation via the same primitive internal/picking itself uses, and persists the DRAFT preview with full per-order allocation traceability. Entirely read-only against inventory — no stock is reserved and no picking_tasks row exists until POST /picking-batches/{id}/confirm.
//	@Tags			Picking Batches
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"Warehouse + order ids to consolidate"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse not found"
//	@Failure		422		{object}	apihttp.ErrorResponse	"one or more orders are not eligible, or insufficient available stock"
//	@Router			/picking-batches [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "picking batch created")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm a picking batch (DRAFT -> CONFIRMED)
//	@Description	Atomically claims every included order and creates exactly one real picking task from the consolidated lines, via internal/picking's own unmodified Service.Create logic (full reservation included) — if any order can no longer be claimed, the whole operation rolls back and nothing is created or reserved. From this point on, the created task is executed entirely through the normal /picking/{id}/... endpoints.
//	@Tags			Picking Batches
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Batch ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse	"batch is not DRAFT, or one of its orders could not be claimed"
//	@Router			/picking-batches/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid batch id", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Confirm(ctx, tenantID, id, userID)
	if err == nil {
		apihttp.OK(c, resp, "picking batch confirmed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a picking batch (DRAFT only)
//	@Description	Only valid while still DRAFT. A CONFIRMED batch's real picking task must be cancelled via POST /picking/{id}/cancel instead — this batch's own status then mirrors it automatically on next read.
//	@Tags			Picking Batches
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Batch ID"
//	@Param			request	body		CancelRequest	false	"Optional remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"batch is not in a status that can be cancelled directly"
//	@Router			/picking-batches/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid batch id", "INVALID_REQUEST")
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

	resp, err := h.service.Cancel(ctx, tenantID, id, userID, req)
	if err == nil {
		apihttp.OK(c, resp, "picking batch cancelled")
		return
	}
	writeError(c, err)
}

// ---------------------------------------------------------------------
// Phase 33C.2 -- Sortation
// ---------------------------------------------------------------------

// GetSortation godoc
//
//	@Summary		Get the Sortation worklist for a batch
//	@Description	Requires the batch to be CONFIRMED and its underlying picking task to have reached COMPLETED or SHORT_PICK. One line per order-share slice (picking_batch_allocations row), with cumulative sorted_qty/shortage progress.
//	@Tags			Picking Batches
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Batch ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=SortationResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse	"sortation not yet available"
//	@Router			/picking-batches/{id}/sortation [get]
func (h *Handler) GetSortation(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid batch id", "INVALID_REQUEST")
		return
	}
	resp, err := h.service.GetSortation(c.Request.Context(), tenantID, id)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// ScanSortation godoc
//
//	@Summary		Scan one order/SKU/lot confirmation into the Sortation worklist
//	@Description	Resolves order_barcode against the batch's own member orders and sku/lot_no against that order's own allocation rows, then atomically increments sorted_qty bounded by allocated_qty. Rejects (never clamps) a quantity that would exceed the remaining allocation.
//	@Tags			Picking Batches
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Batch ID"
//	@Param			request	body		SortationScanRequest	true	"Order/SKU/lot/quantity scan"
//	@Success		200		{object}	apihttp.SuccessResponse{data=SortationResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"sortation not yet available"
//	@Failure		422		{object}	apihttp.ErrorResponse	"scan mismatch, or sorting this quantity would exceed the remaining allocation"
//	@Router			/picking-batches/{id}/sortation/scan [post]
func (h *Handler) ScanSortation(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid batch id", "INVALID_REQUEST")
		return
	}
	var req SortationScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.ScanSortation(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "sortation scan recorded")
		return
	}
	writeError(c, err)
}

// FlagSortationException godoc
//
//	@Summary		Flag a sortation-stage shortage/damage exception on one allocation line
//	@Tags			Picking Batches
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Batch ID"
//	@Param			request	body		SortationExceptionRequest	true	"Allocation, shortage quantity, and reason"
//	@Success		200		{object}	apihttp.SuccessResponse{data=SortationResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"sortation not yet available, or line already flagged"
//	@Failure		422		{object}	apihttp.ErrorResponse	"shortage_qty exceeds this allocation's own remaining quantity"
//	@Router			/picking-batches/{id}/sortation/exception [post]
func (h *Handler) FlagSortationException(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid batch id", "INVALID_REQUEST")
		return
	}
	var req SortationExceptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.FlagSortationException(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "sortation exception flagged")
		return
	}
	writeError(c, err)
}

// AssignSortation godoc
//
//	@Summary		Assign a sortation station and/or operator to a batch
//	@Tags			Picking Batches
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Batch ID"
//	@Param			request	body		SortationAssignRequest	true	"Station and/or user to assign"
//	@Success		200		{object}	apihttp.SuccessResponse{data=SortationResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"sortation not yet available"
//	@Router			/picking-batches/{id}/sortation/assign [put]
func (h *Handler) AssignSortation(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid batch id", "INVALID_REQUEST")
		return
	}
	var req SortationAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.AssignSortation(ctx, tenantID, id, req)
	if err == nil {
		apihttp.OK(c, resp, "sortation assignment updated")
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
		apihttp.Fail(c, http.StatusNotFound, "picking batch not found", "NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, ErrNotConfirmable):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "NOT_CONFIRMABLE")
	case errors.Is(err, ErrConfirmConflict):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "CONFIRM_CONFLICT")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "NOT_CANCELLABLE")
	case errors.Is(err, ErrSortationNotReady):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "SORTATION_NOT_READY")
	case errors.Is(err, ErrSortationScanMismatch):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "SORTATION_SCAN_MISMATCH")
	case errors.Is(err, ErrSortationOverAllocation):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "SORTATION_OVER_ALLOCATION")
	case errors.Is(err, ErrAllocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, err.Error(), "ALLOCATION_NOT_FOUND")
	case errors.Is(err, ErrSortationAlreadyFlagged):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "SORTATION_ALREADY_FLAGGED")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		var pickingValidationErr picking.ErrValidation
		if errors.As(err, &pickingValidationErr) {
			apihttp.Fail(c, http.StatusUnprocessableEntity, pickingValidationErr.Message, "VALIDATION_ERROR")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "picking batch operation failed", "INTERNAL_ERROR")
	}
}

package order

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"wms-backend/internal/picking"
	"wms-backend/internal/platform/dbctx"
	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the Order Management endpoints on rg, using Phase 25's
// new "order" catalog module (view/create/update/approve — added to
// internal/rbac/catalog.go). CreatePickingTask sits on order.update (the
// same "commits the document to its next real workflow step" authority
// level Picking's own Confirm uses), Cancel sits on order.approve (the
// same "ends the document without completing it" authority level every
// other workflow package's own Cancel uses).
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	group := rg.Group("/orders", middleware.RequireJWT(jwtSecret))
	group.GET("", middleware.RequirePermission("order.view"), h.List)
	group.GET("/:id", middleware.RequirePermission("order.view"), h.Get)
	group.POST("", middleware.RequirePermission("order.create"), h.Create)
	group.PUT("/:id", middleware.RequirePermission("order.update"), h.Update)
	group.POST("/:id/create-picking-task", middleware.RequirePermission("order.update"), h.CreatePickingTask)
	group.POST("/:id/cancel", middleware.RequirePermission("order.approve"), h.Cancel)
}

// List godoc
//
//	@Summary		List orders
//	@Description	Matches the (search, status, warehouse) filters every other workflow list endpoint supports, with pagination. fulfillment_status cannot be filtered on — it is derived per row from the linked picking task/shipment's own live status, not a stored column.
//	@Tags			Orders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"			default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches order no, external reference, or customer"
//	@Param			status			query		string	false	"PENDING, PICKING, or CANCELLED"
//	@Param			warehouse_id	query		int		false	"Warehouse ID"
//	@Success		200				{object}	apihttp.SuccessResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/orders [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)

	rows, total, err := h.service.List(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")),
		WarehouseID: warehouseID, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list orders", "INTERNAL_ERROR")
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

// Get godoc
//
//	@Summary		Get an order
//	@Description	Includes a live-derived fulfillment_status resolved from the linked picking task/shipment, if any.
//	@Tags			Orders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Order ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/orders/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid order id", "INVALID_REQUEST")
		return
	}
	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "order not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get order", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create an order (header + lines)
//	@Description	Never touches inventory — pure header + lines persistence. Use POST /orders/{id}/create-picking-task to orchestrate a real picking task once locations/lots are chosen.
//	@Tags			Orders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New order"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse or product does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"external_reference already used by another order"
//	@Router			/orders [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Create(ctx, req, userID)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "order created")
		return
	}
	writeError(c, err)
}

// Update godoc
//
//	@Summary		Update an order (header + lines, full replace)
//	@Description	Only while the order is PENDING — once a picking task has been created for it (POST /orders/{id}/create-picking-task), header/lines are frozen.
//	@Tags			Orders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Order ID"
//	@Param			request	body		UpdateRequest	true	"Updated order"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"order can no longer be edited, or external_reference already used by another order"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/orders/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid order id", "INVALID_REQUEST")
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Update(ctx, id, req)
	if err == nil {
		apihttp.OK(c, resp, "order updated")
		return
	}
	writeError(c, err)
}

// CreatePickingTask godoc
//
//	@Summary		Create a picking task from this order
//	@Description	Orchestration action — the one and only point Order Management calls into internal/picking. One location_id/lot_no assignment per order item is required (matched by order_item_id); everything else passes straight through to picking.Service.Create, which performs its own reservation exactly as a task created directly through PickingView.vue would. Order itself never reserves or mutates inventory. Only valid once, from PENDING — a second call is rejected with 409, never a duplicate picking task.
//	@Tags			Orders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Order ID"
//	@Param			request	body		CreatePickingTaskRequest	true	"Location/lot assignment per order item"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"order is not in a status that can create a picking task"
//	@Failure		422		{object}	apihttp.ErrorResponse	"assignment mismatch, or insufficient available stock to reserve"
//	@Router			/orders/{id}/create-picking-task [post]
func (h *Handler) CreatePickingTask(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid order id", "INVALID_REQUEST")
		return
	}
	var req CreatePickingTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.CreatePickingTask(ctx, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "picking task created")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel an order
//	@Description	Never mutates inventory — Order itself never reserved anything. Only PENDING orders (no picking task created yet) can be cancelled here; once a picking task exists, cancel it directly via POST /picking/{id}/cancel instead.
//	@Tags			Orders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Order ID"
//	@Param			request	body		CancelRequest	false	"Optional remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"order is not in a cancellable status"
//	@Router			/orders/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid order id", "INVALID_REQUEST")
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

	resp, err := h.service.Cancel(ctx, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "order cancelled")
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
		apihttp.Fail(c, http.StatusNotFound, "order not found", "NOT_FOUND")
	case errors.Is(err, ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, picking.ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrDuplicateExternalReference):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "DUPLICATE_EXTERNAL_REFERENCE")
	case errors.Is(err, ErrNotEditable):
		apihttp.Fail(c, http.StatusConflict, "order can no longer be edited", "NOT_EDITABLE")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "order is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.Is(err, ErrNotReleasable):
		apihttp.Fail(c, http.StatusConflict, "order is not in a status that can create a picking task", "NOT_RELEASABLE")
	case errors.Is(err, picking.ErrCrossWarehouse):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "CROSS_WAREHOUSE")
	case errors.Is(err, picking.ErrNoSourceStock):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_SOURCE_STOCK")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		var pickingValidationErr picking.ErrValidation
		if errors.As(err, &pickingValidationErr) {
			apihttp.Fail(c, http.StatusUnprocessableEntity, pickingValidationErr.Message, "VALIDATION_ERROR")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "order operation failed", "INTERNAL_ERROR")
	}
}

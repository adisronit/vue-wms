package transferreceipt

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"wms-backend/internal/inventory"
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

// Register mounts the Destination Receiving endpoints on rg, using the
// same RequireJWT -> RequireTenantContext -> RequirePermission chain
// every tenant-scoped inbound/outbound module uses.
// "transfer_receipt.confirm" gates BOTH mutating actions on a receipt
// — ReceiveItem (the AddStockTx-calling action) and Confirm (DRAFT ->
// COMPLETED) — the same elevated authority level receiving.confirm/
// relocation.confirm/packing.confirm already established for their own
// module's primary stock-mutating action. "approve" gates Cancel —
// see internal/rbac's own catalog doc for why this module chose that
// split over relocation's own "update gates Cancel" precedent.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("/transfer-receipts",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("", middleware.RequirePermission("transfer_receipt.view"), h.List)
	group.GET("/eligible-shipments", middleware.RequirePermission("transfer_receipt.view"), h.EligibleShipments)
	group.GET("/:id", middleware.RequirePermission("transfer_receipt.view"), h.Get)
	group.POST("", middleware.RequirePermission("transfer_receipt.create"), h.Create)
	group.POST("/:id/items/:itemId/receive", middleware.RequirePermission("transfer_receipt.confirm"), h.ReceiveItem)
	group.POST("/:id/confirm", middleware.RequirePermission("transfer_receipt.confirm"), h.Confirm)
	group.POST("/:id/cancel", middleware.RequirePermission("transfer_receipt.approve"), h.Cancel)
}

// List godoc
//
//	@Summary		List transfer receipts
//	@Description	Header-level rows — supports search plus status/destination-warehouse filters, with pagination.
//	@Tags			Transfer Receiving
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page						query		int		false	"Page number"			default(1)
//	@Param			limit						query		int		false	"Page size (max 100)"	default(20)
//	@Param			search						query		string	false	"Matches receipt no, shipment no, or order no"
//	@Param			status						query		string	false	"DRAFT, COMPLETED, or CANCELLED"
//	@Param			destination_warehouse_id	query		int		false	"Filter by destination warehouse"
//	@Success		200							{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401							{object}	apihttp.ErrorResponse
//	@Failure		403							{object}	apihttp.ErrorResponse
//	@Router			/transfer-receipts [get]
func (h *Handler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	destWarehouseID, _ := strconv.ParseInt(c.Query("destination_warehouse_id"), 10, 64)

	responses, total, err := h.service.List(c.Request.Context(), tenantID, ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")),
		DestinationWarehouseID: destWarehouseID, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list transfer receipts", "INTERNAL_ERROR")
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

// EligibleShipments godoc
//
//	@Summary		List transfer shipments awaiting receipt
//	@Description	DISPATCHED, IN_TRANSIT, or DELIVERED INTERNAL_TRANSFER shipments bound for destination_warehouse_id that have no transfer receipt yet. Powers the "+ New Receipt" shipment picker.
//	@Tags			Transfer Receiving
//	@Produce		json
//	@Security		BearerAuth
//	@Param			destination_warehouse_id	query		int		true	"Destination warehouse"
//	@Param			search						query		string	false	"Matches shipment no or order no"
//	@Success		200							{object}	apihttp.SuccessResponse{data=[]EligibleShipmentResponse}
//	@Failure		401							{object}	apihttp.ErrorResponse
//	@Failure		403							{object}	apihttp.ErrorResponse
//	@Router			/transfer-receipts/eligible-shipments [get]
func (h *Handler) EligibleShipments(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	destWarehouseID, _ := strconv.ParseInt(c.Query("destination_warehouse_id"), 10, 64)

	rows, err := h.service.EligibleShipments(c.Request.Context(), tenantID, EligibleShipmentParams{
		DestinationWarehouseID: destWarehouseID, Search: c.Query("search"),
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list eligible shipments", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, rows, "")
}

// Get godoc
//
//	@Summary		Get a transfer receipt with its lines
//	@Tags			Transfer Receiving
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Transfer receipt ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/transfer-receipts/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid transfer receipt id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "transfer receipt not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get transfer receipt", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a transfer receipt from a dispatched transfer shipment
//	@Description	Validates the shipment is DISPATCHED/IN_TRANSIT/DELIVERED, its own order is an INTERNAL_TRANSFER order, and no receipt already exists for it, then copies one line per shipment item with dispatched_qty frozen from that line's own shipped quantity. No stock mutation happens here — a new receipt always starts DRAFT; use POST .../items/{itemId}/receive to actually credit the destination warehouse, then POST .../confirm to finalize.
//	@Tags			Transfer Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"Source shipment"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"shipment or its order does not exist"
//	@Failure		409		{object}	apihttp.ErrorResponse	"a receipt already exists for this shipment"
//	@Failure		422		{object}	apihttp.ErrorResponse	"shipment not dispatched, or its order is not an INTERNAL_TRANSFER order"
//	@Router			/transfer-receipts [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "transfer receipt created")
		return
	}
	writeError(c, err)
}

// ReceiveItem godoc
//
//	@Summary		Receive a quantity against one transfer receipt line
//	@Description	Atomically credits the destination warehouse via Inventory Core's AddStockTx (same transaction, one TRANSFER_RECEIVE ledger row) and increments the line's received_qty, bounded by its own dispatched_qty — rejected outright, never clamped, on over-receipt. idempotency_key must be a fresh UUID per call; resending the same key returns the receipt's current state instead of adding stock twice. Can be called more than once on the same line for partial/incremental receiving. Set close_short=true with a discrepancy_reason to permanently stop expecting more of this line even though received_qty remains below dispatched_qty.
//	@Tags			Transfer Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Transfer receipt ID"
//	@Param			itemId	path		int					true	"Transfer receipt item ID"
//	@Param			request	body		ReceiveItemRequest	true	"Quantity and staging location for this call"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"receipt is not DRAFT, or line is already fully received/closed"
//	@Failure		422		{object}	apihttp.ErrorResponse	"e.g. over-receipt, or staging location invalid"
//	@Router			/transfer-receipts/{id}/items/{itemId}/receive [post]
func (h *Handler) ReceiveItem(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid transfer receipt id", "INVALID_REQUEST")
		return
	}
	itemID, err := parseID(c, "itemId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid transfer receipt item id", "INVALID_REQUEST")
		return
	}

	var req ReceiveItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.ReceiveItem(ctx, tenantID, id, itemID, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "item received")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm a transfer receipt
//	@Description	Transitions DRAFT -> COMPLETED once every line is either fully received or has a recorded discrepancy. Performs no stock mutation of its own — ReceiveItem already did that. idempotency_key must be a fresh UUID per click; resending the same key returns the receipt's current state instead of confirming twice.
//	@Tags			Transfer Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Transfer receipt ID"
//	@Param			request	body		ConfirmRequest	true	"Idempotency key"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"receipt is not DRAFT, or a line is still open"
//	@Router			/transfer-receipts/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid transfer receipt id", "INVALID_REQUEST")
		return
	}

	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Confirm(ctx, tenantID, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "transfer receipt confirmed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a transfer receipt
//	@Description	Marks the receipt CANCELLED. DRAFT only.
//	@Tags			Transfer Receiving
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Transfer receipt ID"
//	@Param			request	body		CancelRequest	false	"Optional cancellation remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"receipt is not in a cancellable status"
//	@Router			/transfer-receipts/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid transfer receipt id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "transfer receipt cancelled")
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
		apihttp.Fail(c, http.StatusNotFound, "transfer receipt not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound), errors.Is(err, ErrItemNotInReceipt):
		apihttp.Fail(c, http.StatusNotFound, "transfer receipt item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrShipmentNotFound):
		apihttp.Fail(c, http.StatusNotFound, "shipment not found", "SHIPMENT_NOT_FOUND")
	case errors.Is(err, ErrOrderNotFound):
		apihttp.Fail(c, http.StatusNotFound, "order not found", "ORDER_NOT_FOUND")
	case errors.Is(err, ErrLocationNotFound), errors.Is(err, inventory.ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, inventory.ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrReceiptAlreadyExists):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "RECEIPT_ALREADY_EXISTS")
	case errors.Is(err, ErrNotTransferOrder):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NOT_TRANSFER_ORDER")
	case errors.Is(err, ErrShipmentNotDispatched):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "SHIPMENT_NOT_DISPATCHED")
	case errors.Is(err, ErrCrossWarehouse):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "CROSS_WAREHOUSE")
	case errors.Is(err, ErrOverReceipt):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "OVER_RECEIPT")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "transfer receipt is not in a receivable/confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotReadyToConfirm):
		apihttp.Fail(c, http.StatusConflict, err.Error(), "NOT_READY_TO_CONFIRM")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "transfer receipt is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "transfer receipt operation failed", "INTERNAL_ERROR")
	}
}

package qualitycheck

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

// Register mounts the Quality Check endpoints on rg, using the
// RequireJWT -> RequireTenantContext -> RequirePermission chain every
// Phase 30/31A inbound/outbound module uses. Confirm (the action that
// actually releases or rejects held stock) is gated on
// quality_check.confirm, split out of quality_check.approve the same
// way internal/receiving's own Confirm was in Phase 31A — ordinary QC
// staff record their own PASS/HOLD/REJECT decisions; approve stays
// reserved for a future supervisor-level exception action.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	qc := rg.Group("/qc",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	qc.GET("/eligible-splits", middleware.RequirePermission("quality_check.view"), h.EligibleSplits)
	qc.GET("/inspections", middleware.RequirePermission("quality_check.view"), h.List)
	qc.GET("/inspections/:id", middleware.RequirePermission("quality_check.view"), h.Get)
	qc.POST("/inspections", middleware.RequirePermission("quality_check.create"), h.Create)
	qc.PUT("/inspections/:id/items/:itemId", middleware.RequirePermission("quality_check.update"), h.UpdateItem)
	qc.POST("/inspections/:id/confirm", middleware.RequirePermission("quality_check.confirm"), h.Confirm)
}

// EligibleSplits godoc
//
//	@Summary		List receipt lines eligible for QC inspection
//	@Description	Lines whose product requires QC and still carry inventory hold — powers the "Receiving No." picker.
//	@Tags			QualityCheck
//	@Produce		json
//	@Security		BearerAuth
//	@Param			receipt_id	query		int	true	"Receipt ID"
//	@Success		200			{object}	apihttp.SuccessResponse{data=[]EligibleSplitResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/qc/eligible-splits [get]
func (h *Handler) EligibleSplits(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	receiptID, err := strconv.ParseInt(c.Query("receipt_id"), 10, 64)
	if err != nil || receiptID == 0 {
		apihttp.Fail(c, http.StatusBadRequest, "receipt_id is required", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.EligibleSplits(c.Request.Context(), tenantID, receiptID)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// List godoc
//
//	@Summary		List QC inspections
//	@Tags			QualityCheck
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page			query		int		false	"Page number"	default(1)
//	@Param			limit			query		int		false	"Page size (max 100)"	default(20)
//	@Param			search			query		string	false	"Matches inspection no or receipt no"
//	@Param			status			query		string	false	"PENDING, IN_PROGRESS, or COMPLETED"
//	@Param			warehouse_id	query		int		false	"Filter by warehouse"
//	@Success		200				{object}	apihttp.ListResponse{data=[]ListResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Router			/qc/inspections [get]
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
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list inspections", "INTERNAL_ERROR")
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

// Get godoc
//
//	@Summary		Get an inspection with its lines and disposition history
//	@Tags			QualityCheck
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Inspection ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/qc/inspections/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid inspection id", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.Get(c.Request.Context(), tenantID, id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "inspection not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get inspection", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a QC inspection
//	@Description	Auto-populates one line per eligible receipt_item_splits row on the given receipt (see EligibleSplitResponse).
//	@Tags			QualityCheck
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New inspection"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse or receipt does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/qc/inspections [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "inspection created")
		return
	}
	writeError(c, err)
}

// UpdateItem godoc
//
//	@Summary		Update an inspection line's checklist/sample fields
//	@Description	Never touches inventory.
//	@Tags			QualityCheck
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int					true	"Inspection ID"
//	@Param			itemId	path		int					true	"Item ID"
//	@Param			request	body		UpdateItemRequest	true	"Updated checklist fields"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Router			/qc/inspections/{id}/items/{itemId} [put]
func (h *Handler) UpdateItem(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid inspection id", "INVALID_REQUEST")
		return
	}
	itemID, err := parseID(c, "itemId")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid item id", "INVALID_REQUEST")
		return
	}

	var req UpdateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.service.UpdateItem(c.Request.Context(), tenantID, id, itemID, req)
	if err == nil {
		apihttp.OK(c, resp, "")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Record PASS/HOLD/REJECT dispositions for one or more inspection lines
//	@Description	Atomically releases hold (PASS), deducts quantity and hold (REJECT), or records a pure annotation (HOLD) for each entry, all committed together or all rolled back. idempotency_key must be a fresh UUID per click. Can be called more than once on the same inspection for partial/re- inspection of whatever quantity remains held.
//	@Tags			QualityCheck
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Inspection ID"
//	@Param			request	body		ConfirmRequest	true	"Dispositions to record"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"inspection already completed, or a disposition exceeds what's currently held"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/qc/inspections/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid inspection id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "dispositions confirmed")
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
		apihttp.Fail(c, http.StatusNotFound, "inspection not found", "NOT_FOUND")
	case errors.Is(err, ErrItemNotFound):
		apihttp.Fail(c, http.StatusNotFound, "inspection item not found", "ITEM_NOT_FOUND")
	case errors.Is(err, ErrReceiptNotFound):
		apihttp.Fail(c, http.StatusNotFound, "receipt not found", "RECEIPT_NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, inventory.ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, inventory.ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrWarehouseMismatch):
		apihttp.Fail(c, http.StatusConflict, "warehouse_id does not match the receipt's own warehouse", "WAREHOUSE_MISMATCH")
	case errors.Is(err, ErrNoEligibleLines):
		apihttp.Fail(c, http.StatusConflict, "receipt has no QC-required lines with held stock", "NO_ELIGIBLE_LINES")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "inspection is already completed", "INVALID_TRANSITION")
	case errors.Is(err, inventory.ErrHoldUnderflow):
		apihttp.Fail(c, http.StatusConflict, "disposition quantity exceeds what's currently held", "HOLD_UNDERFLOW")
	case errors.Is(err, inventory.ErrNoStock):
		apihttp.Fail(c, http.StatusConflict, "no held stock at this location", "NO_STOCK")
	case errors.Is(err, inventory.ErrInsufficientStock):
		apihttp.Fail(c, http.StatusConflict, "insufficient stock", "INSUFFICIENT_STOCK")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		var invValidationErr inventory.ErrValidation
		if errors.As(err, &invValidationErr) {
			apihttp.Fail(c, http.StatusUnprocessableEntity, invValidationErr.Message, "VALIDATION_ERROR")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "quality check operation failed", "INTERNAL_ERROR")
	}
}

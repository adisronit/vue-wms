package replenishment

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

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

// Register mounts the Replenishment endpoints on rg, using Phase 24's
// new "replenishment" catalog module (view/create/update/approve —
// added to internal/rbac/catalog.go, seeded to PUTAWAY alongside its
// own putaway.* grants, since replenishment is the same "move stock
// into place" duty). Start/Confirm sit on replenishment.update (staff
// executing an already-created task); Cancel/ReportIssue sit on
// replenishment.approve, the same authority-level split Picking's own
// Phase 13 Cancel established for "ends the task without completing
// it" actions.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	group := rg.Group("/replenishment", middleware.RequireJWT(jwtSecret))
	group.GET("", middleware.RequirePermission("replenishment.view"), h.List)
	group.GET("/recommendations", middleware.RequirePermission("replenishment.view"), h.Recommend)
	group.GET("/:id", middleware.RequirePermission("replenishment.view"), h.Get)
	group.POST("", middleware.RequirePermission("replenishment.create"), h.Create)
	group.POST("/:id/start", middleware.RequirePermission("replenishment.update"), h.Start)
	group.POST("/:id/confirm", middleware.RequirePermission("replenishment.update"), h.Confirm)
	group.POST("/:id/cancel", middleware.RequirePermission("replenishment.approve"), h.Cancel)
	group.POST("/:id/issue", middleware.RequirePermission("replenishment.approve"), h.ReportIssue)
}

// List godoc
//
//	@Summary		List replenishment tasks
//	@Description	Matches the (search, status, warehouse, product) filters every other workflow list endpoint supports, with pagination.
//	@Tags			Replenishment
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page		query		int		false	"Page number"			default(1)
//	@Param			limit		query		int		false	"Page size (max 100)"	default(20)
//	@Param			search		query		string	false	"Matches task no, SKU, or product name"
//	@Param			status		query		string	false	"PENDING, IN_PROGRESS, COMPLETED, CANCELLED, or ISSUE"
//	@Param			warehouse_id	query	int		false	"Warehouse ID"
//	@Param			product_id	query		int		false	"Product ID"
//	@Success		200			{object}	apihttp.SuccessResponse{data=[]ListResponse}
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/replenishment [get]
func (h *Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	warehouseID, _ := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	productID, _ := strconv.ParseInt(c.Query("product_id"), 10, 64)

	rows, total, err := h.service.List(c.Request.Context(), ListParams{
		Search: c.Query("search"), Status: Status(c.Query("status")),
		WarehouseID: warehouseID, ProductID: productID, Page: page, Limit: limit,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list replenishment tasks", "INTERNAL_ERROR")
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

// Recommend godoc
//
//	@Summary		Generate simple replenishment recommendations for a warehouse
//	@Description	Advisory only — every low-stock (product, location) pair (available_qty <= the product's own min_stock, the same threshold Inventory's own LOW_STOCK status uses) paired with that product's own highest-surplus other location in the same warehouse. Not an optimizer: two recommendations may name the same source location: the real availability check only happens when a recommendation is turned into a real POST /replenishment call.
//	@Tags			Replenishment
//	@Produce		json
//	@Security		BearerAuth
//	@Param			warehouse_id	query		int	true	"Warehouse ID"
//	@Success		200				{object}	apihttp.SuccessResponse{data=[]RecommendationResponse}
//	@Failure		401				{object}	apihttp.ErrorResponse
//	@Failure		403				{object}	apihttp.ErrorResponse
//	@Failure		404				{object}	apihttp.ErrorResponse
//	@Router			/replenishment/recommendations [get]
func (h *Handler) Recommend(c *gin.Context) {
	warehouseID, err := strconv.ParseInt(c.Query("warehouse_id"), 10, 64)
	if err != nil || warehouseID == 0 {
		apihttp.Fail(c, http.StatusBadRequest, "warehouse_id is required", "INVALID_REQUEST")
		return
	}

	recs, err := h.service.Recommend(c.Request.Context(), warehouseID)
	if errors.Is(err, ErrWarehouseNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to generate recommendations", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, recs, "")
}

// Get godoc
//
//	@Summary		Get a replenishment task
//	@Tags			Replenishment
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Replenishment task ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/replenishment/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid replenishment task id", "INVALID_REQUEST")
		return
	}
	resp, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "replenishment task not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get replenishment task", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create a replenishment task (manual)
//	@Description	Reserves quantity against (product_id, from_location_id, lot_no) immediately, the same Phase 12 reservation pattern Picking uses — a second task cannot oversubscribe the same available stock. Nothing physically moves until POST /replenishment/{id}/confirm.
//	@Tags			Replenishment
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateRequest	true	"New replenishment task"
//	@Success		201		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse	"warehouse, product, or location does not exist"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/replenishment [post]
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
		apihttp.Status(c, http.StatusCreated, resp, "replenishment task created")
		return
	}
	writeError(c, err)
}

// Start godoc
//
//	@Summary		Mark a replenishment task as being worked on
//	@Description	PENDING -> IN_PROGRESS only, no inventory effect. Confirm accepts either status equally, so calling Start is optional.
//	@Tags			Replenishment
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Replenishment task ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse
//	@Router			/replenishment/{id}/start [post]
func (h *Handler) Start(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid replenishment task id", "INVALID_REQUEST")
		return
	}
	resp, err := h.service.Start(c.Request.Context(), id)
	if err == nil {
		apihttp.OK(c, resp, "replenishment task started")
		return
	}
	writeError(c, err)
}

// Confirm godoc
//
//	@Summary		Confirm a replenishment task
//	@Description	Atomically releases the task's own reservation and moves its quantity from from_location_id to to_location_id via Inventory Core's MoveStock (two ledger rows, same transaction). idempotency_key must be a fresh UUID per click; resending the same key returns the task's current state instead of moving stock twice.
//	@Tags			Replenishment
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Replenishment task ID"
//	@Param			request	body		ConfirmRequest	true	"Idempotency key"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a confirmable status"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/replenishment/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid replenishment task id", "INVALID_REQUEST")
		return
	}
	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))

	resp, err := h.service.Confirm(ctx, id, req, userID)
	if err == nil {
		apihttp.OK(c, resp, "replenishment task confirmed")
		return
	}
	writeError(c, err)
}

// Cancel godoc
//
//	@Summary		Cancel a replenishment task
//	@Description	Never mutates inventory — only releases the reservation Create placed. Only PENDING/IN_PROGRESS tasks can be cancelled.
//	@Tags			Replenishment
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Replenishment task ID"
//	@Param			request	body		CancelRequest	false	"Optional remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"task is not in a cancellable status"
//	@Router			/replenishment/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid replenishment task id", "INVALID_REQUEST")
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
		apihttp.OK(c, resp, "replenishment task cancelled")
		return
	}
	writeError(c, err)
}

// ReportIssue godoc
//
//	@Summary		Report an issue on a replenishment task
//	@Description	Never mutates inventory — only releases the reservation Create placed, the same as Cancel, but lands on ISSUE (not CANCELLED) and requires a remark explaining what went wrong.
//	@Tags			Replenishment
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int				true	"Replenishment task ID"
//	@Param			request	body		IssueRequest	true	"Required remark"
//	@Success		200		{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Router			/replenishment/{id}/issue [post]
func (h *Handler) ReportIssue(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid replenishment task id", "INVALID_REQUEST")
		return
	}
	var req IssueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}
	resp, err := h.service.ReportIssue(c.Request.Context(), id, req)
	if err == nil {
		apihttp.OK(c, resp, "replenishment issue reported")
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
		apihttp.Fail(c, http.StatusNotFound, "replenishment task not found", "NOT_FOUND")
	case errors.Is(err, ErrProductNotFound):
		apihttp.Fail(c, http.StatusNotFound, "product not found", "PRODUCT_NOT_FOUND")
	case errors.Is(err, ErrWarehouseNotFound):
		apihttp.Fail(c, http.StatusNotFound, "warehouse not found", "WAREHOUSE_NOT_FOUND")
	case errors.Is(err, ErrLocationNotFound):
		apihttp.Fail(c, http.StatusNotFound, "location not found", "LOCATION_NOT_FOUND")
	case errors.Is(err, ErrNoSourceStock):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "NO_SOURCE_STOCK")
	case errors.Is(err, ErrCrossWarehouse):
		apihttp.Fail(c, http.StatusUnprocessableEntity, err.Error(), "CROSS_WAREHOUSE")
	case errors.Is(err, ErrInvalidTransition):
		apihttp.Fail(c, http.StatusConflict, "replenishment task is not in a confirmable status", "INVALID_TRANSITION")
	case errors.Is(err, ErrNotStartable):
		apihttp.Fail(c, http.StatusConflict, "replenishment task is not in a startable status", "NOT_STARTABLE")
	case errors.Is(err, ErrNotCancellable):
		apihttp.Fail(c, http.StatusConflict, "replenishment task is not in a cancellable status", "NOT_CANCELLABLE")
	case errors.Is(err, ErrCannotReportIssue):
		apihttp.Fail(c, http.StatusConflict, "replenishment task cannot report an issue in its current status", "CANNOT_REPORT_ISSUE")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "replenishment operation failed", "INTERNAL_ERROR")
	}
}

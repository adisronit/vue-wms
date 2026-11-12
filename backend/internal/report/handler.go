package report

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"wms-backend/internal/inventory"
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

// Register mounts the async export-job endpoints. Gated on
// stock_inquiry.export — the same permission Phase 22's synchronous
// Inventory Stock Report export uses, since this is the same report
// category (inventory data), just the one variant expected to
// genuinely outgrow a synchronous request.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	group := rg.Group("/reports", middleware.RequireJWT(jwtSecret), middleware.RequirePermission("stock_inquiry.export"))
	group.POST("/inventory-transactions/export", h.CreateInventoryTransactionsExport)
	group.GET("/export-jobs/:id", h.GetJob)
	group.GET("/export-jobs/:id/download", h.Download)
}

// CreateInventoryTransactionsExport godoc
//
//	@Summary		Queue an async Inventory Transaction History export
//	@Description	Phase 22's one async report (the ledger is append-only and expected to outgrow a synchronous request). Creates a PENDING export_jobs row and publishes a job to wms.report.export; poll GET /reports/export-jobs/{id} for status, then GET .../download once COMPLETED.
//	@Tags			Reports
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		InventoryTransactionsExportRequest	false	"Optional filters"
//	@Success		202		{object}	apihttp.SuccessResponse{data=CreateJobResponse}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/reports/inventory-transactions/export [post]
func (h *Handler) CreateInventoryTransactionsExport(c *gin.Context) {
	var req InventoryTransactionsExportRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}
	}

	userID, _ := middleware.CurrentUserID(c)

	job, err := h.service.CreateInventoryTransactionsJob(c.Request.Context(), userID, inventory.TransactionExportFilters{
		ProductID: req.ProductID, WarehouseID: req.WarehouseID,
		TransactionType: inventory.TransactionType(req.TransactionType),
		DateFrom:        req.DateFrom, DateTo: req.DateTo,
	})
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to queue export", "INTERNAL_ERROR")
		return
	}

	apihttp.Status(c, http.StatusAccepted, CreateJobResponse{JobID: job.ID}, "export job accepted")
}

// GetJob godoc
//
//	@Summary		Get an export job's status
//	@Description	Only ever returns a job owned by the authenticated caller — another user's job is reported as 404, not a permission error.
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Export job ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/reports/export-jobs/{id} [get]
func (h *Handler) GetJob(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid export job id", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)

	resp, err := h.service.GetJob(c.Request.Context(), id, userID)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "export job not found", "NOT_FOUND")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to get export job", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, resp, "")
}

// Download godoc
//
//	@Summary		Download a completed export job's file
//	@Description	Only ever serves a file for a job owned by the authenticated caller (404 for another user's job or a nonexistent one) and only once status=COMPLETED (409 otherwise) — the file itself lives on server-side storage (the export_files volume), never a client-supplied path.
//	@Tags			Reports
//	@Produce		text/csv
//	@Security		BearerAuth
//	@Param			id	path	int	true	"Export job ID"
//	@Success		200	{file}	file
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Failure		409	{object}	apihttp.ErrorResponse
//	@Router			/reports/export-jobs/{id}/download [get]
func (h *Handler) Download(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid export job id", "INVALID_REQUEST")
		return
	}
	userID, _ := middleware.CurrentUserID(c)

	filePath, fileName, err := h.service.ResolveDownload(c.Request.Context(), id, userID)
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "export job not found", "NOT_FOUND")
		return
	}
	if errors.Is(err, ErrNotReady) {
		apihttp.Fail(c, http.StatusConflict, "export job is not ready for download", "NOT_READY")
		return
	}
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to resolve export file", "INTERNAL_ERROR")
		return
	}

	c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	c.File(filePath)
}

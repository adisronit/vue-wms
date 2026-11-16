package inbounddoc

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// Handler exposes Service over HTTP. Every route is read-only (GET),
// synchronous (CLAUDE.md's own "use RabbitMQ only for genuinely
// long-running/background work" rule — rendering one document from
// already-loaded rows is neither), and gated on the same "can you see
// this record" permission as the owning domain's own Get endpoint: if a
// role can view a receipt/inspection/task, it can download that
// record's own PDF.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the inbound-document endpoints on rg. Routes nest one
// level deeper than each owning domain's own GET /{id} (e.g.
// /receipts/:id/pdf/grn, not /receipts/:id), so there is no path
// collision with receiving/qualitycheck/putaway's own Handler.Register
// calls sharing the same /receipts, /qc/inspections, and /putaway
// prefixes.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("/receipts/:id/pdf/grn", middleware.RequirePermission("receiving.view"), h.GRN)
	group.GET("/receipts/:id/pdf/completion", middleware.RequirePermission("receiving.view"), h.CompletionReport)
	group.GET("/receipts/:id/pdf/discrepancy", middleware.RequirePermission("receiving.view"), h.DiscrepancyReport)
	group.GET("/qc/inspections/:id/pdf", middleware.RequirePermission("quality_check.view"), h.QCReport)
	group.GET("/putaway/:id/pdf", middleware.RequirePermission("putaway.view"), h.PutawayTaskSheet)
}

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

// servePDF writes data as a PDF response — inline by default (so the
// frontend's "View PDF" button can open it in a new tab/iframe without
// forcing a save dialog), attachment when ?download=true (the
// "Download PDF" button).
func servePDF(c *gin.Context, data []byte, filename string) {
	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", data)
}

func writeRenderError(c *gin.Context, err error) {
	if errors.Is(err, ErrNotFound) {
		apihttp.Fail(c, http.StatusNotFound, "document source not found", "NOT_FOUND")
		return
	}
	apihttp.Fail(c, http.StatusInternalServerError, "failed to generate document", "INTERNAL_ERROR")
}

// GRN godoc
//
//	@Summary		Download the Goods Receiving Note (GRN) PDF for a receipt
//	@Description	Synchronous, read-only — re-downloading never changes the receipt's status or stock. A4, Thai/English text, multi-page with repeated headers and page numbers. Gated on receiving.view: if a role can view the receipt, it can download its GRN.
//	@Tags			Inbound Documents
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Receipt ID"
//	@Param			download	query	boolean	false	"true for a download (Content-Disposition: attachment), omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/receipts/{id}/pdf/grn [get]
func (h *Handler) GRN(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}

	data, filename, err := h.service.RenderGRN(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeRenderError(c, err)
		return
	}
	servePDF(c, data, filename)
}

// CompletionReport godoc
//
//	@Summary		Download the Inbound Completion Report PDF for a receipt
//	@Description	Composes Receiving, every QC inspection, and every Putaway task for this receipt, plus a live Final Stock Position section (current Quantity/Reserved/Hold/Available per product-location-lot). Synchronous, read-only. Gated on receiving.view.
//	@Tags			Inbound Documents
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Receipt ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/receipts/{id}/pdf/completion [get]
func (h *Handler) CompletionReport(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}

	data, filename, err := h.service.RenderCompletionReport(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeRenderError(c, err)
		return
	}
	servePDF(c, data, filename)
}

// DiscrepancyReport godoc
//
//	@Summary		Download the Receiving Discrepancy / Damage Report PDF for a receipt
//	@Description	Expected-vs-received quantity variances on the receipt's own lines, plus every QC REJECT disposition recorded against it. Synchronous, read-only. Gated on receiving.view.
//	@Tags			Inbound Documents
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Receipt ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/receipts/{id}/pdf/discrepancy [get]
func (h *Handler) DiscrepancyReport(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid receipt id", "INVALID_REQUEST")
		return
	}

	data, filename, err := h.service.RenderDiscrepancyReport(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeRenderError(c, err)
		return
	}
	servePDF(c, data, filename)
}

// QCReport godoc
//
//	@Summary		Download the QC Inspection Report PDF for an inspection
//	@Description	Inspection header, checklist/sample items, and the full PASS/HOLD/REJECT disposition history. Synchronous, read-only. Gated on quality_check.view.
//	@Tags			Inbound Documents
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Inspection ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/qc/inspections/{id}/pdf [get]
func (h *Handler) QCReport(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid inspection id", "INVALID_REQUEST")
		return
	}

	data, filename, err := h.service.RenderQCReport(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeRenderError(c, err)
		return
	}
	servePDF(c, data, filename)
}

// PutawayTaskSheet godoc
//
//	@Summary		Download the Putaway Task Sheet PDF for a task
//	@Description	Task header plus its lines (source -> destination, requested vs moved). Synchronous, read-only. Gated on putaway.view.
//	@Tags			Inbound Documents
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Putaway task ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/putaway/{id}/pdf [get]
func (h *Handler) PutawayTaskSheet(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	userID, _ := middleware.CurrentUserID(c)
	id, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid putaway task id", "INVALID_REQUEST")
		return
	}

	data, filename, err := h.service.RenderPutawayTaskSheet(c.Request.Context(), tenantID, userID, id)
	if err != nil {
		writeRenderError(c, err)
		return
	}
	servePDF(c, data, filename)
}

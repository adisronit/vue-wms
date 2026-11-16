package docrender

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

func writeTenantUnauthorized(c *gin.Context) {
	apihttp.Fail(c, http.StatusUnauthorized, "tenant context required", "UNAUTHORIZED")
}

// permissionByDocumentType maps each of the six document types this
// phase covers to the SAME permission code the owning module's own live
// PDF endpoint already requires — history/reprint of a document is
// exactly as privileged as viewing the live document it is a history
// of, never a separate "documents.view" blanket code. Kept here (not in
// internal/rbac) since it is purely a routing decision local to this
// package's own two endpoints, not a catalog entry.
var permissionByDocumentType = map[string]string{
	TypeOrder:               "order.view",
	TypePickList:            "picking.view",
	TypePickingConfirmation: "picking.view",
	TypePickingBatchList:    "picking_batch.view",
	TypePackingList:         "packing.view",
	TypeCartonLabel:         "packing.view",
	// Phase 33D.3.
	TypeDeliveryNote:    "shipping.view",
	TypeLoadingManifest: "loading.view",
	// Phase 33E.3.
	TypeTransferNote: "transfer_receipt.view",
}

// Handler exposes Service over HTTP.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the two Document History / Reprint endpoints on rg.
// Both require RequireJWT -> RequireTenantContext, matching every other
// tenant-scoped module's own chain, but NEITHER uses
// middleware.RequirePermission at route-registration time — the actual
// gating permission depends on the ":type" path param (History) or on
// the fetched row's own DocumentType (DownloadRender), both only known
// once the handler runs, so each checks middleware.HasPermission itself
// against permissionByDocumentType.
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	group := rg.Group("/documents",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)
	group.GET("/:type/:sourceId/history", h.History)
	group.GET("/renders/:id", h.DownloadRender)
}

// History godoc
//
//	@Summary		List past renders of one document
//	@Description	Returns every historical render recorded for one (document type, source row) — newest first. Gated on the SAME permission code the owning document type's own live PDF endpoint requires (order.view for ORDER, picking.view for PICK_LIST/PICKING_CONFIRMATION, picking_batch.view for PICKING_BATCH_LIST, packing.view for PACKING_LIST/CARTON_LABEL, shipping.view for DELIVERY_NOTE, loading.view for LOADING_MANIFEST).
//	@Tags			Document History
//	@Produce		json
//	@Security		BearerAuth
//	@Param			type		path		string	true	"Document type: ORDER, PICK_LIST, PICKING_BATCH_LIST, PICKING_CONFIRMATION, PACKING_LIST, CARTON_LABEL, DELIVERY_NOTE, or LOADING_MANIFEST"
//	@Param			sourceId	path		int		true	"The source row's own id (order id, picking task id, batch id, job id, or carton id)"
//	@Success		200			{object}	apihttp.SuccessResponse{data=[]RenderSummary}
//	@Failure		400			{object}	apihttp.ErrorResponse
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Router			/documents/{type}/{sourceId}/history [get]
func (h *Handler) History(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	docType := strings.ToUpper(c.Param("type"))
	perm, known := permissionByDocumentType[docType]
	if !known {
		apihttp.Fail(c, http.StatusBadRequest, "unknown document type", "INVALID_REQUEST")
		return
	}
	if !middleware.HasPermission(c, perm) {
		apihttp.Fail(c, http.StatusForbidden, "permission denied: "+perm, "FORBIDDEN")
		return
	}
	sourceID, err := strconv.ParseInt(c.Param("sourceId"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid sourceId", "INVALID_REQUEST")
		return
	}

	rows, err := h.service.History(c.Request.Context(), tenantID, docType, sourceID)
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list document history", "INTERNAL_ERROR")
		return
	}
	apihttp.OK(c, rows, "")
}

// DownloadRender godoc
//
//	@Summary		Re-download one specific historical render's exact original bytes
//	@Description	Returns the PDF bytes exactly as they were at the time this render was produced — never a fresh re-render against current data, even if the underlying order/task/job/batch has since changed. Gated on the SAME permission the render's own document type requires for its live PDF.
//	@Tags			Document History
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			id			path	int		true	"Render ID"
//	@Param			download	query	boolean	false	"true for a download, omitted for inline viewing"
//	@Success		200			{file}	file
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse
//	@Failure		404			{object}	apihttp.ErrorResponse
//	@Router			/documents/renders/{id} [get]
func (h *Handler) DownloadRender(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		writeTenantUnauthorized(c)
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid render id", "INVALID_REQUEST")
		return
	}

	rec, err := h.service.Fetch(c.Request.Context(), tenantID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			apihttp.Fail(c, http.StatusNotFound, "render not found", "NOT_FOUND")
			return
		}
		apihttp.Fail(c, http.StatusInternalServerError, "failed to fetch render", "INTERNAL_ERROR")
		return
	}

	perm, known := permissionByDocumentType[rec.DocumentType]
	if !known || !middleware.HasPermission(c, perm) {
		apihttp.Fail(c, http.StatusForbidden, "permission denied", "FORBIDDEN")
		return
	}

	disposition := "inline"
	if c.Query("download") == "true" {
		disposition = "attachment"
	}
	filename := rec.DocumentNo + ".pdf"
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", disposition+"; filename=\""+filename+"\"")
	c.Data(http.StatusOK, "application/pdf", rec.PDFData)
}

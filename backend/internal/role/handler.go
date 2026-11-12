package role

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

// Register mounts only what the Roles & Permissions screen actually
// calls: no role create/delete endpoint exists because the frontend
// itself never persists those actions today (see PROJECT_STATUS.md).
func (h *Handler) Register(rg *gin.RouterGroup, jwtSecret string) {
	authed := rg.Group("", middleware.RequireJWT(jwtSecret))

	authed.GET("/roles", middleware.RequirePermission("roles.view"), h.ListRoles)
	authed.GET("/permissions", middleware.RequirePermission("roles.view"), h.ListPermissions)
	authed.GET("/roles/:id/permissions", middleware.RequirePermission("roles.view"), h.GetRolePermissions)
	authed.PUT("/roles/:id/permissions", middleware.RequirePermission("roles.update"), h.SetRolePermissions)

	// Warehouse Access reuses the same roles.view/roles.update permissions
	// as the permission matrix above — it is a second panel on the same
	// Roles & Permissions screen, not a separate feature with its own
	// RBAC module.
	authed.GET("/roles/:id/warehouse-access", middleware.RequirePermission("roles.view"), h.GetRoleWarehouseAccess)
	authed.PUT("/roles/:id/warehouse-access", middleware.RequirePermission("roles.update"), h.SetRoleWarehouseAccess)
}

// ListRoles godoc
//
//	@Summary		List roles
//	@Tags			Roles
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.ListResponse{data=[]Response}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/roles [get]
func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.service.ListRoles(c.Request.Context())
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list roles", "INTERNAL_ERROR")
		return
	}

	responses := make([]Response, 0, len(roles))
	for _, r := range roles {
		responses = append(responses, toResponse(r))
	}

	apihttp.List(c, responses, apihttp.ListMeta{Page: 1, Limit: len(responses), Total: len(responses), TotalPages: 1})
}

// ListPermissions godoc
//
//	@Summary		List the permission catalog
//	@Description	Flat list of every module x action permission — the frontend groups these into its matrix by module.
//	@Tags			Roles
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	apihttp.ListResponse{data=[]PermissionResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Router			/permissions [get]
func (h *Handler) ListPermissions(c *gin.Context) {
	permissions, err := h.service.ListPermissions(c.Request.Context())
	if err != nil {
		apihttp.Fail(c, http.StatusInternalServerError, "failed to list permissions", "INTERNAL_ERROR")
		return
	}

	responses := make([]PermissionResponse, 0, len(permissions))
	for _, p := range permissions {
		responses = append(responses, toPermissionResponse(p))
	}

	apihttp.List(c, responses, apihttp.ListMeta{Page: 1, Limit: len(responses), Total: len(responses), TotalPages: 1})
}

// GetRolePermissions godoc
//
//	@Summary		Get a role's granted permissions
//	@Tags			Roles
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Role ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=RolePermissionsResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/roles/{id}/permissions [get]
func (h *Handler) GetRolePermissions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid role id", "INVALID_REQUEST")
		return
	}

	r, codes, err := h.service.GetRolePermissions(c.Request.Context(), id)
	if err != nil {
		writeRoleError(c, err)
		return
	}

	apihttp.OK(c, RolePermissionsResponse{Role: toResponse(*r), PermissionCodes: codes}, "")
}

// SetRolePermissions godoc
//
//	@Summary		Replace a role's granted permissions
//	@Description	Submits the complete desired permission set for the role — matches the Roles & Permissions screen's "Save Permissions" button, which always sends the full matrix state.
//	@Tags			Roles
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int						true	"Role ID"
//	@Param			request	body		SetPermissionsRequest	true	"Full permission code set to grant"
//	@Success		200		{object}	apihttp.SuccessResponse{data=RolePermissionsResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse	"one or more permission_codes are not in the catalog"
//	@Router			/roles/{id}/permissions [put]
func (h *Handler) SetRolePermissions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid role id", "INVALID_REQUEST")
		return
	}

	var req SetPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))
	codes, err := h.service.SetRolePermissions(ctx, id, req.PermissionCodes)
	if err != nil {
		writeRoleError(c, err)
		return
	}

	r, err := h.service.GetRole(c.Request.Context(), id)
	if err != nil {
		writeRoleError(c, err)
		return
	}

	apihttp.OK(c, RolePermissionsResponse{Role: toResponse(*r), PermissionCodes: codes}, "permissions updated")
}

// GetRoleWarehouseAccess godoc
//
//	@Summary		Get a role's warehouse access
//	@Description	The Roles & Permissions screen's "Warehouse Access" panel — which warehouses a role's users may operate against.
//	@Tags			Roles
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		int	true	"Role ID"
//	@Success		200	{object}	apihttp.SuccessResponse{data=WarehouseAccessResponse}
//	@Failure		401	{object}	apihttp.ErrorResponse
//	@Failure		403	{object}	apihttp.ErrorResponse
//	@Failure		404	{object}	apihttp.ErrorResponse
//	@Router			/roles/{id}/warehouse-access [get]
func (h *Handler) GetRoleWarehouseAccess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid role id", "INVALID_REQUEST")
		return
	}

	r, warehouses, err := h.service.GetRoleWarehouseAccess(c.Request.Context(), id)
	if err != nil {
		writeRoleError(c, err)
		return
	}

	apihttp.OK(c, WarehouseAccessResponse{RoleID: r.ID, WarehouseAccess: warehouses}, "")
}

// SetRoleWarehouseAccess godoc
//
//	@Summary		Replace a role's warehouse access
//	@Description	Submits the complete desired warehouse_id set for the role — matches the Warehouse Access panel's "Save Permissions" button, which always sends the full selection, not a delta.
//	@Tags			Roles
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Role ID"
//	@Param			request	body		SetWarehouseAccessRequest	true	"Full warehouse_id set to grant"
//	@Success		200		{object}	apihttp.SuccessResponse{data=WarehouseAccessResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse	"one or more warehouse_ids are not in the warehouse master"
//	@Router			/roles/{id}/warehouse-access [put]
func (h *Handler) SetRoleWarehouseAccess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid role id", "INVALID_REQUEST")
		return
	}

	var req SetWarehouseAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	userID, _ := middleware.CurrentUserID(c)

	ctx := dbctx.WithRequestID(c.Request.Context(), middleware.GetRequestID(c))
	warehouses, err := h.service.SetRoleWarehouseAccess(ctx, id, req.WarehouseIDs, userID)
	if err != nil {
		writeRoleError(c, err)
		return
	}

	r, err := h.service.GetRole(c.Request.Context(), id)
	if err != nil {
		writeRoleError(c, err)
		return
	}

	apihttp.OK(c, WarehouseAccessResponse{RoleID: r.ID, WarehouseAccess: warehouses}, "warehouse access updated")
}

func writeRoleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "role not found", "NOT_FOUND")
	case errors.Is(err, ErrInvalidPermissionCodes):
		apihttp.Fail(c, http.StatusUnprocessableEntity, "one or more permission_codes are not in the catalog", "INVALID_PERMISSION_CODES")
	case errors.Is(err, ErrInvalidWarehouseIDs):
		apihttp.Fail(c, http.StatusUnprocessableEntity, "one or more warehouse_ids are not in the warehouse master", "INVALID_WAREHOUSE_IDS")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "role operation failed", "INTERNAL_ERROR")
	}
}

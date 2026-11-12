package role

// Response is the role shape returned by GET /roles and embedded in the
// permissions endpoints.
type Response struct {
	ID          int64  `json:"id,string" example:"1"`
	Code        string `json:"code" example:"ADMIN"`
	Name        string `json:"name" example:"Administrator"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active" example:"true"`
} //	@name	RoleResponse

// PermissionResponse is one row of the permission catalog, as returned
// by GET /permissions.
type PermissionResponse struct {
	ID          int64  `json:"id,string" example:"1"`
	Code        string `json:"code" example:"users.view"`
	Module      string `json:"module" example:"users"`
	Action      string `json:"action" example:"view"`
	Description string `json:"description" example:"View User Management"`
} //	@name	PermissionResponse

// RolePermissionsResponse is returned by both GET and PUT
// .../roles/{id}/permissions — the role plus its currently granted codes.
type RolePermissionsResponse struct {
	Role            Response `json:"role"`
	PermissionCodes []string `json:"permission_codes"`
} //	@name	RolePermissionsResponse

// SetPermissionsRequest is the PUT .../roles/{id}/permissions body — the
// Roles & Permissions screen's "Save Permissions" button always submits
// the complete desired matrix state, not a delta.
type SetPermissionsRequest struct {
	PermissionCodes []string `json:"permission_codes"`
} //	@name	SetPermissionsRequest

// WarehouseSummary is the minimal warehouse info embedded in a role's
// warehouse-access response — deliberately duplicated from package
// warehouse's own Response shape (not imported), the same
// self-contained-DTO pattern internal/user's RoleSummary already uses
// for role data embedded in user responses.
type WarehouseSummary struct {
	ID   int64  `json:"id,string" example:"1"`
	Code string `json:"code" example:"WH-01"`
	Name string `json:"name" example:"Bangkok Warehouse"`
} //	@name	RoleWarehouseSummary

// WarehouseAccessResponse is returned by both GET and PUT
// .../roles/{id}/warehouse-access — the role's currently granted
// warehouses, resolved to code/name for display.
type WarehouseAccessResponse struct {
	RoleID          int64              `json:"role_id,string" example:"1"`
	WarehouseAccess []WarehouseSummary `json:"warehouse_access"`
} //	@name	RoleWarehouseAccessResponse

// SetWarehouseAccessRequest is the PUT .../roles/{id}/warehouse-access
// body — like SetPermissionsRequest, the Warehouse Access panel always
// submits the complete desired set of warehouse IDs, not a delta.
type SetWarehouseAccessRequest struct {
	WarehouseIDs []int64 `json:"warehouse_ids"`
} //	@name	SetRoleWarehouseAccessRequest

func toResponse(r Role) Response {
	return Response{
		ID:          r.ID,
		Code:        r.Code,
		Name:        r.Name,
		Description: r.Description,
		IsActive:    r.IsActive,
	}
}

func toPermissionResponse(p Permission) PermissionResponse {
	return PermissionResponse{
		ID:          p.ID,
		Code:        p.Code,
		Module:      p.Module,
		Action:      p.Action,
		Description: p.Description,
	}
}

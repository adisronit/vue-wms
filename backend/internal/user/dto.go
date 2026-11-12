package user

import "time"

// RoleSummary is the minimal role info embedded in a user response —
// deliberately duplicated from package role's shape (rather than
// importing package role's DTO type) to keep package user's public
// surface self-contained; see internal/role for the source of truth.
type RoleSummary struct {
	Code string `json:"code" example:"ADMIN"`
	Name string `json:"name" example:"Administrator"`
} //	@name	UserRoleSummary

// Response is what every user-returning endpoint sends back. It never
// includes password_hash.
//
// WarehouseID is the raw FK (nil when unassigned); WarehouseCode/
// WarehouseName are resolved display fields (see Service.toResponse),
// nil together whenever WarehouseID is nil — there is no case where one
// is set without the other.
type Response struct {
	ID            int64       `json:"id,string" example:"1"`
	Username      string      `json:"username" example:"receiving01"`
	FullName      string      `json:"full_name" example:"Receiving Staff"`
	Email         string      `json:"email" example:"receiving@smartwms.com"`
	Role          RoleSummary `json:"role"`
	WarehouseID   *int64      `json:"warehouse_id,string" example:"1"`
	WarehouseCode *string     `json:"warehouse_code" example:"WH-01"`
	WarehouseName *string     `json:"warehouse_name" example:"Bangkok Warehouse"`
	Status        Status      `json:"status" example:"ACTIVE"`
	LastLoginAt   *time.Time  `json:"last_login_at"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
} //	@name	UserResponse

// CreateRequest is the POST /api/v1/users body. Password is required
// here even though the current User Management screen has no password
// field yet — a minimal password input was added to the "New User" panel
// specifically to support this (see PROJECT_STATUS.md).
type CreateRequest struct {
	Username string `json:"username" binding:"required" example:"receiving02"`
	Password string `json:"password" binding:"required" example:"ChangeMe123!"`
	FullName string `json:"full_name" binding:"required" example:"New Receiving Staff"`
	Email    string `json:"email" binding:"required,email" example:"receiving02@smartwms.com"`
	RoleCode string `json:"role_code" binding:"required" example:"RECEIVING"`
	// WarehouseID is optional — omit or send null to leave the user
	// unassigned to a specific warehouse (see migrations/0039).
	WarehouseID *int64 `json:"warehouse_id,string" example:"1"`
	Status      Status `json:"status" example:"ACTIVE"`
} //	@name	CreateUserRequest

// UpdateRequest is the PUT /api/v1/users/:id body. Password is optional —
// omit it to leave the existing password unchanged.
type UpdateRequest struct {
	FullName string `json:"full_name" binding:"required" example:"Receiving Staff"`
	Email    string `json:"email" binding:"required,email" example:"receiving@smartwms.com"`
	RoleCode string `json:"role_code" binding:"required" example:"RECEIVING"`
	// WarehouseID is optional — omit or send null to clear the user's
	// warehouse assignment (see migrations/0039).
	WarehouseID *int64 `json:"warehouse_id,string" example:"1"`
	Status      Status `json:"status" binding:"required" example:"ACTIVE"`
	Password    string `json:"password,omitempty" example:""`
} //	@name	UpdateUserRequest

// ListParams narrows GET /api/v1/users to the User Management screen's
// actual search box plus role/status filters, with pagination.
type ListParams struct {
	Search   string
	RoleCode string
	Status   Status
	Page     int
	Limit    int
}

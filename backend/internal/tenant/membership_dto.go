package tenant

import "time"

// Membership create modes — CreateMembershipRequest.Mode. NEW
// provisions a brand-new users row (bcrypt-hashed password supplied by
// the admin creating it — see Phase 5's doc on membership_service.go)
// plus its tenant_users row; EXISTING links an already-registered user
// (resolved by exact username/email match only, never a broad search —
// see Service.createMembershipWithExistingUser) to one more tenant.
const (
	MembershipModeNew      = "NEW"
	MembershipModeExisting = "EXISTING"
)

// MembershipRoleSummary is the minimal role info embedded in a
// membership response.
type MembershipRoleSummary struct {
	Code string `json:"code" example:"RECEIVING"`
	Name string `json:"name" example:"Receiving Staff"`
} //	@name	TenantMembershipRoleSummary

// MembershipResponse is what every tenant-membership-returning endpoint
// sends back — never includes password_hash (it isn't even queried; see
// MembershipRow).
type MembershipResponse struct {
	ID        int64                 `json:"id,string" example:"1"`
	TenantID  int64                 `json:"tenant_id,string" example:"1"`
	UserID    int64                 `json:"user_id,string" example:"1"`
	Username  string                `json:"username" example:"receiving02"`
	FullName  string                `json:"full_name" example:"New Receiving Staff"`
	Email     string                `json:"email" example:"receiving02@smartwms.com"`
	Role      MembershipRoleSummary `json:"role"`
	Status    string                `json:"status" example:"ACTIVE"`
	InvitedBy *int64                `json:"invited_by,string"`
	InvitedAt *time.Time            `json:"invited_at"`
	JoinedAt  *time.Time            `json:"joined_at"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
} //	@name	TenantMembershipResponse

// CreateMembershipRequest is the POST .../users body for both the
// Platform Admin and Tenant Admin membership-create endpoints. Which
// fields are required depends on Mode (see the service's validation):
// NEW requires Username/Password/FullName/Email; EXISTING requires
// Identifier (an exact username or email) instead.
type CreateMembershipRequest struct {
	Mode     string `json:"mode" binding:"required" example:"NEW"`
	RoleCode string `json:"role_code" binding:"required" example:"RECEIVING"`

	// NEW mode only.
	Username string `json:"username" example:"receiving02"`
	Password string `json:"password" example:"ChangeMe123!"`
	FullName string `json:"full_name" example:"New Receiving Staff"`
	Email    string `json:"email" example:"receiving02@smartwms.com"`

	// EXISTING mode only — an exact, already-known username or email;
	// never a partial/fuzzy search term (see CLAUDE.md's "Do not expose
	// unrestricted global user search to Tenant Admins").
	Identifier string `json:"identifier" example:"existing.user@example.com"`
} //	@name	CreateTenantMembershipRequest

// ChangeMembershipRoleRequest is the PATCH .../role body.
type ChangeMembershipRoleRequest struct {
	RoleCode string `json:"role_code" binding:"required" example:"WAREHOUSE_MANAGER"`
} //	@name	ChangeTenantMembershipRoleRequest

// ChangeMembershipStatusRequest is the PATCH .../status body. Status
// must be ACTIVE (reactivate) or DISABLED (suspend) — INVITED is a
// system-set initial value only, never a target of this endpoint.
type ChangeMembershipStatusRequest struct {
	Status string  `json:"status" binding:"required" example:"DISABLED"`
	Reason *string `json:"reason" example:"Left the company"`
} //	@name	ChangeTenantMembershipStatusRequest

// MembershipListParams narrows GET .../users to the Tenant Users tab's
// search box and role/status filters, with pagination.
type MembershipListParams struct {
	Search   string
	RoleCode string
	Status   string
	Page     int
	Limit    int
	SortBy   string
	SortDir  string
}

func toMembershipResponse(row MembershipRow) MembershipResponse {
	return MembershipResponse{
		ID:       row.ID,
		TenantID: row.TenantID,
		UserID:   row.UserID,
		Username: row.Username,
		FullName: row.FullName,
		Email:    row.Email,
		Role:     MembershipRoleSummary{Code: row.RoleCode, Name: row.RoleName},
		Status:   row.Status,
		InvitedBy: row.InvitedBy,
		InvitedAt: row.InvitedAt,
		JoinedAt:  row.JoinedAt,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

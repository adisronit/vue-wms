package auth

// LoginRequest is the POST /api/v1/auth/login request body.
type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"changeme"`
} //	@name	LoginRequest

// RoleSummary is the minimal role info embedded in auth responses — just
// enough for the frontend to label the current user, not the full role
// record. Since the RBAC catalog (internal/rbac) stays global rather than
// per-tenant, this is the role the caller's *active tenant membership*
// (tenant_users.role_id) currently points at, not a role owned by the
// tenant itself.
type RoleSummary struct {
	Code string `json:"code" example:"ADMIN"`
	Name string `json:"name" example:"Administrator"`
} //	@name	RoleSummary

// TenantSummary is the minimal active-tenant info embedded in auth
// responses — enough for the frontend to label which tenant the current
// session is scoped to, not the full tenant record (billing, settings,
// ...).
type TenantSummary struct {
	ID   int64  `json:"id,string" example:"1"`
	Slug string `json:"slug" example:"acme"`
	Name string `json:"name" example:"Acme Co"`
} //	@name	TenantSummary

// UserSummary is the authenticated user's profile as returned by login,
// tenant switch, and /auth/me. Password and password hash are never
// included. Role and Permissions are always scoped to Tenant — the active
// tenant membership the caller is currently authenticated into, per
// docs/proposals/wms-saas/tenant-isolation-security.md §2 ("a token never
// carries more than one tenant's context").
type UserSummary struct {
	ID          int64         `json:"id,string" example:"1"`
	Username    string        `json:"username" example:"admin"`
	FullName    string        `json:"full_name" example:"Administrator"`
	Email       string        `json:"email" example:"admin@smartwms.local"`
	Tenant      TenantSummary `json:"tenant"`
	Role        RoleSummary   `json:"role"`
	Permissions []string      `json:"permissions"`
} //	@name	UserSummary

// SwitchTenantRequest is the POST /api/v1/auth/switch-tenant request body.
// tenant_id is only ever used to select which membership to re-verify
// server-side — it is never trusted as the basis for the new token's
// active_tenant_id claim by itself (tenant-isolation-security.md §2).
type SwitchTenantRequest struct {
	TenantID int64 `json:"tenant_id,string" binding:"required" example:"2"`
} //	@name	SwitchTenantRequest

// LoginData is the `data` payload of a successful login response.
type LoginData struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type" example:"Bearer"`
	ExpiresIn   int         `json:"expires_in" example:"3600"`
	User        UserSummary `json:"user"`
} //	@name	LoginData

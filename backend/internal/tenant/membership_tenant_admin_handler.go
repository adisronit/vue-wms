package tenant

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
)

// TenantAdminHandler exposes MembershipService to a tenant's own
// ADMIN-role users, under /api/v1/admin/tenant-users — deliberately a
// separate route prefix and a separate Go type from Handler
// (membership_handler.go), per CLAUDE.md Phase 3's "for Tenant Admin
// access, use separately scoped routes and authorization rather than
// silently reusing Platform Admin privileges." Every method here reads
// its tenant_id exclusively from middleware.EnforcedTenantID (the
// caller's own re-verified active tenant) — never from a path or body
// parameter — so there is no request shape that lets a tenant admin
// name another tenant at all, let alone manage it.
type TenantAdminHandler struct {
	membership *MembershipService
}

func NewTenantAdminHandler(membership *MembershipService) *TenantAdminHandler {
	return &TenantAdminHandler{membership: membership}
}

// Register mounts the Tenant Admin membership-management endpoints on
// rg, gated by RequireJWT -> RequireTenantContext -> RequirePermission —
// the ordinary tenant-scoped chain CLAUDE.md's RBAC section describes,
// never RequirePlatformAdmin. RequireTenantContext re-verifies the
// caller's membership against the database (not just the JWT claim) on
// every request per its own doc, which is what keeps a suspended
// membership from staying authorized through a stale token; the
// mutating handlers below additionally require the caller's OWN
// membership to currently hold the ADMIN role (EnsureActingTenantAdmin)
// and never target themselves (EnsureNotSelf) — see those methods' own
// docs for why a permission-claim check alone is not enough here.
func (h *TenantAdminHandler) Register(rg *gin.RouterGroup, jwtSecret string, membershipChecker middleware.MembershipChecker, membershipCacheTTL time.Duration) {
	admin := rg.Group("/admin/tenant-users",
		middleware.RequireJWT(jwtSecret),
		middleware.RequireTenantContext(membershipChecker, membershipCacheTTL),
	)

	admin.GET("", middleware.RequirePermission("tenant_users.view"), h.List)
	admin.GET("/:userId", middleware.RequirePermission("tenant_users.view"), h.Get)
	admin.POST("", middleware.RequirePermission("tenant_users.create"), h.Create)
	admin.PATCH("/:userId/role", middleware.RequirePermission("tenant_users.update"), h.ChangeRole)
	admin.PATCH("/:userId/status", middleware.RequirePermission("tenant_users.suspend"), h.ChangeStatus)
}

// List godoc
//
//	@Summary		List my tenant's users
//	@Description	Tenant Admin (or any role granted tenant_users.view). Scoped to the caller's own active tenant only. Supports a search box and role/status filters, with pagination.
//	@Tags			TenantAdmin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			page	query		int		false	"Page number"			default(1)
//	@Param			limit	query		int		false	"Page size (max 100)"	default(20)
//	@Param			search	query		string	false	"Matches full name, username, or email"
//	@Param			role	query		string	false	"Role code, e.g. ADMIN"
//	@Param			status	query		string	false	"INVITED, ACTIVE, or DISABLED"
//	@Success		200		{object}	apihttp.ListResponse{data=[]MembershipResponse}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Router			/admin/tenant-users [get]
func (h *TenantAdminHandler) List(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	params := MembershipListParams{
		Search: c.Query("search"), RoleCode: c.Query("role"), Status: c.Query("status"),
		Page: page, Limit: limit,
	}

	responses, total, err := h.membership.ListMemberships(c.Request.Context(), tenantID, params)
	if err != nil {
		writeMembershipError(c, err)
		return
	}

	effectivePage, effectiveLimit := params.Page, params.Limit
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
//	@Summary		Get one of my tenant's memberships
//	@Description	Tenant Admin (or any role granted tenant_users.view). Scoped to the caller's own active tenant only.
//	@Tags			TenantAdmin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userId	path		int	true	"User ID"
//	@Success		200		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Router			/admin/tenant-users/{userId} [get]
func (h *TenantAdminHandler) Get(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}
	userID, err := parseUserID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}

	resp, err := h.membership.GetMembership(c.Request.Context(), tenantID, userID)
	if err != nil {
		writeMembershipError(c, err)
		return
	}
	apihttp.OK(c, resp, "")
}

// Create godoc
//
//	@Summary		Create or link a user in my tenant
//	@Description	Only the tenant's own ADMIN-role holder may call this (EnsureActingTenantAdmin), regardless of what tenant_users.create might otherwise grant — see TenantAdminHandler's own doc. mode=NEW provisions a brand-new user account plus its membership; mode=EXISTING links an already-registered user (resolved by exact username/email match only) to this tenant. Writes a tenant_audit_logs row.
//	@Tags			TenantAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateMembershipRequest	true	"New or existing-user membership"
//	@Success		201		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"caller is not this tenant's administrator"
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/admin/tenant-users [post]
func (h *TenantAdminHandler) Create(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}
	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	if err := h.membership.EnsureActingTenantAdmin(c.Request.Context(), tenantID, actorUserID); err != nil {
		writeMembershipError(c, err)
		return
	}

	var req CreateMembershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.membership.CreateMembership(c.Request.Context(), tenantID, req, actorUserID, AuditActorTenantAdmin)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "tenant membership created")
		return
	}
	writeMembershipError(c, err)
}

// ChangeRole godoc
//
//	@Summary		Change a tenant member's role
//	@Description	Only the tenant's own ADMIN-role holder may call this, and never on their own membership (EnsureActingTenantAdmin, EnsureNotSelf — see TenantAdminHandler's own doc). role_code must belong to this tenant. Rejected if it would leave the tenant with no active administrator. Writes a tenant_audit_logs row.
//	@Tags			TenantAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userId	path		int							true	"User ID"
//	@Param			request	body		ChangeMembershipRoleRequest	true	"New role"
//	@Success		200		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/admin/tenant-users/{userId}/role [patch]
func (h *TenantAdminHandler) ChangeRole(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}
	userID, err := parseUserID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}
	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	if err := h.membership.EnsureActingTenantAdmin(c.Request.Context(), tenantID, actorUserID); err != nil {
		writeMembershipError(c, err)
		return
	}
	if err := EnsureNotSelf(actorUserID, userID); err != nil {
		writeMembershipError(c, err)
		return
	}

	var req ChangeMembershipRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.membership.ChangeRole(c.Request.Context(), tenantID, userID, req, actorUserID, AuditActorTenantAdmin)
	if err != nil {
		writeMembershipError(c, err)
		return
	}
	apihttp.OK(c, resp, "")
}

// ChangeStatus godoc
//
//	@Summary		Suspend or reactivate a tenant membership
//	@Description	Only the tenant's own ADMIN-role holder may call this, and never on their own membership (EnsureActingTenantAdmin, EnsureNotSelf — see TenantAdminHandler's own doc). status must be ACTIVE or DISABLED. Rejected if it would leave the tenant with no active administrator. Writes a tenant_audit_logs row.
//	@Tags			TenantAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userId	path		int								true	"User ID"
//	@Param			request	body		ChangeMembershipStatusRequest	true	"New status"
//	@Success		200		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/admin/tenant-users/{userId}/status [patch]
func (h *TenantAdminHandler) ChangeStatus(c *gin.Context) {
	tenantID, ok := middleware.EnforcedTenantID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing tenant context", "UNAUTHORIZED")
		return
	}
	userID, err := parseUserID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}
	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	if err := h.membership.EnsureActingTenantAdmin(c.Request.Context(), tenantID, actorUserID); err != nil {
		writeMembershipError(c, err)
		return
	}
	if err := EnsureNotSelf(actorUserID, userID); err != nil {
		writeMembershipError(c, err)
		return
	}

	var req ChangeMembershipStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	resp, err := h.membership.ChangeStatus(c.Request.Context(), tenantID, userID, req, actorUserID, AuditActorTenantAdmin)
	if err != nil {
		writeMembershipError(c, err)
		return
	}
	apihttp.OK(c, resp, "")
}

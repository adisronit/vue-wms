package tenant

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
	"wms-backend/internal/platform/middleware"
	"wms-backend/internal/user"
)

// RegisterPlatformMembers mounts the Platform Admin tenant-membership
// endpoints (WMS-29E) on the SAME route group Handler.Register already
// built for tenant CRUD — rg here must be the group returned by that
// call (already gated by RequireJWT -> RequirePlatformAdmin), not the
// bare /platform group, so a Platform Admin can manage any tenant's
// membership without any tenant-level RBAC check at all, exactly like
// every other Platform Admin capability.
func (h *Handler) RegisterPlatformMembers(tenants *gin.RouterGroup) {
	tenants.GET("/:id/users", h.ListMembers)
	tenants.POST("/:id/users", h.CreateMember)
	tenants.GET("/:id/users/:userId", h.GetMember)
	tenants.PATCH("/:id/users/:userId/role", h.ChangeMemberRole)
	tenants.PATCH("/:id/users/:userId/status", h.ChangeMemberStatus)
}

func parseUserID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("userId"), 10, 64)
}

// ListMembers godoc
//
//	@Summary		List a tenant's users
//	@Description	Platform Admin only. Supports a search box (matches full name, username, or email) and role/status filters, with pagination.
//	@Tags			PlatformAdmin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		int		true	"Tenant ID"
//	@Param			page		query		int		false	"Page number"			default(1)
//	@Param			limit		query		int		false	"Page size (max 100)"	default(25)
//	@Param			search		query		string	false	"Matches full name, username, or email"
//	@Param			role		query		string	false	"Role code, e.g. ADMIN"
//	@Param			status		query		string	false	"INVITED, ACTIVE, or DISABLED"
//	@Param			sort_by		query		string	false	"name, role, status, or joinedAt"	default(name)
//	@Param			sort_dir	query		string	false	"asc or desc"						default(asc)
//	@Success		200			{object}	apihttp.ListResponse{data=[]MembershipResponse}
//	@Failure		400			{object}	apihttp.ErrorResponse	"invalid sort_by/sort_dir"
//	@Failure		401			{object}	apihttp.ErrorResponse
//	@Failure		403			{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404			{object}	apihttp.ErrorResponse	"tenant not found"
//	@Router			/platform/tenants/{id}/users [get]
func (h *Handler) ListMembers(c *gin.Context) {
	tenantID, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
		return
	}

	page, _ := strconv.Atoi(c.Query("page"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	params := MembershipListParams{
		Search: c.Query("search"), RoleCode: c.Query("role"), Status: c.Query("status"),
		Page: page, Limit: limit,
		SortBy: c.Query("sort_by"), SortDir: c.Query("sort_dir"),
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

// GetMember godoc
//
//	@Summary		Get one tenant membership
//	@Description	Platform Admin only.
//	@Tags			PlatformAdmin
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int	true	"Tenant ID"
//	@Param			userId	path		int	true	"User ID"
//	@Success		200		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants/{id}/users/{userId} [get]
func (h *Handler) GetMember(c *gin.Context) {
	tenantID, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
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

// CreateMember godoc
//
//	@Summary		Create or link a tenant membership
//	@Description	Platform Admin only. mode=NEW provisions a brand-new user account plus its membership; mode=EXISTING links an already-registered user (resolved by exact username/email match) to this tenant. Writes a tenant_audit_logs row (action=tenant_user.create or tenant_user.link).
//	@Tags			PlatformAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Tenant ID"
//	@Param			request	body		CreateMembershipRequest	true	"New or existing-user membership"
//	@Success		201		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404		{object}	apihttp.ErrorResponse	"tenant or linked user not found"
//	@Failure		409		{object}	apihttp.ErrorResponse	"duplicate membership or username/email"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants/{id}/users [post]
func (h *Handler) CreateMember(c *gin.Context) {
	tenantID, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
		return
	}

	var req CreateMembershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	resp, err := h.membership.CreateMembership(c.Request.Context(), tenantID, req, actorUserID, AuditActorPlatformAdmin)
	if err == nil {
		apihttp.Status(c, http.StatusCreated, resp, "tenant membership created")
		return
	}
	writeMembershipError(c, err)
}

// ChangeMemberRole godoc
//
//	@Summary		Change a tenant member's role
//	@Description	Platform Admin only. role_code must belong to this tenant's own role catalog. Rejected if it would leave the tenant with no active administrator. Writes a tenant_audit_logs row (action=tenant_user.role_change).
//	@Tags			PlatformAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int							true	"Tenant ID"
//	@Param			userId	path		int							true	"User ID"
//	@Param			request	body		ChangeMembershipRoleRequest	true	"New role"
//	@Success		200		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"would remove the tenant's last active administrator"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants/{id}/users/{userId}/role [patch]
func (h *Handler) ChangeMemberRole(c *gin.Context) {
	tenantID, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
		return
	}
	userID, err := parseUserID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}

	var req ChangeMembershipRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	resp, err := h.membership.ChangeRole(c.Request.Context(), tenantID, userID, req, actorUserID, AuditActorPlatformAdmin)
	if err != nil {
		writeMembershipError(c, err)
		return
	}
	apihttp.OK(c, resp, "")
}

// ChangeMemberStatus godoc
//
//	@Summary		Suspend or reactivate a tenant membership
//	@Description	Platform Admin only. status must be ACTIVE or DISABLED. Rejected if it would leave the tenant with no active administrator. Writes a tenant_audit_logs row (action=tenant_user.suspend or tenant_user.reactivate).
//	@Tags			PlatformAdmin
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		int								true	"Tenant ID"
//	@Param			userId	path		int								true	"User ID"
//	@Param			request	body		ChangeMembershipStatusRequest	true	"New status"
//	@Success		200		{object}	apihttp.SuccessResponse{data=MembershipResponse}
//	@Failure		400		{object}	apihttp.ErrorResponse
//	@Failure		401		{object}	apihttp.ErrorResponse
//	@Failure		403		{object}	apihttp.ErrorResponse	"not a platform admin"
//	@Failure		404		{object}	apihttp.ErrorResponse
//	@Failure		409		{object}	apihttp.ErrorResponse	"would remove the tenant's last active administrator"
//	@Failure		422		{object}	apihttp.ErrorResponse
//	@Router			/platform/tenants/{id}/users/{userId}/status [patch]
func (h *Handler) ChangeMemberStatus(c *gin.Context) {
	tenantID, err := parseID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid tenant id", "INVALID_REQUEST")
		return
	}
	userID, err := parseUserID(c)
	if err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid user id", "INVALID_REQUEST")
		return
	}

	var req ChangeMembershipStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apihttp.Fail(c, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	actorUserID, ok := middleware.CurrentUserID(c)
	if !ok {
		apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
		return
	}

	resp, err := h.membership.ChangeStatus(c.Request.Context(), tenantID, userID, req, actorUserID, AuditActorPlatformAdmin)
	if err != nil {
		writeMembershipError(c, err)
		return
	}
	apihttp.OK(c, resp, "")
}

// writeMembershipError maps every error the membership service can
// return to the standard error envelope — shared by both the Platform
// Admin and Tenant Admin handlers (membership_tenant_admin_handler.go),
// since the underlying errors are identical regardless of which persona
// triggered them.
func writeMembershipError(c *gin.Context, err error) {
	var validationErr ErrValidation

	switch {
	case errors.Is(err, ErrNotFound):
		apihttp.Fail(c, http.StatusNotFound, "tenant not found", "NOT_FOUND")
	case errors.Is(err, ErrMembershipRowNotFound):
		apihttp.Fail(c, http.StatusNotFound, "membership not found", "MEMBERSHIP_NOT_FOUND")
	case errors.Is(err, ErrLinkUserNotFound):
		apihttp.Fail(c, http.StatusNotFound, "no matching user found", "USER_NOT_FOUND")
	case errors.Is(err, ErrMembershipDuplicate):
		apihttp.Fail(c, http.StatusConflict, "user already has a membership in this tenant", "DUPLICATE_MEMBERSHIP")
	case errors.Is(err, user.ErrDuplicate):
		apihttp.Fail(c, http.StatusConflict, "username or email already exists", "DUPLICATE_USER")
	case errors.Is(err, ErrLastActiveAdmin):
		apihttp.Fail(c, http.StatusConflict, "cannot remove the tenant's last active administrator", "LAST_ADMIN_PROTECTED")
	case errors.Is(err, ErrNotTenantAdmin):
		apihttp.Fail(c, http.StatusForbidden, "only this tenant's administrator can manage memberships", "FORBIDDEN")
	case errors.Is(err, ErrCannotModifySelf):
		apihttp.Fail(c, http.StatusForbidden, "cannot change your own membership through this endpoint", "FORBIDDEN")
	case errors.As(err, &validationErr):
		apihttp.Fail(c, http.StatusUnprocessableEntity, validationErr.Message, "VALIDATION_ERROR")
	case errors.Is(err, ErrInvalidSort):
		apihttp.Fail(c, http.StatusBadRequest, "sort_by must be name, role, status, or joinedAt; sort_dir must be asc or desc", "INVALID_SORT")
	default:
		apihttp.Fail(c, http.StatusInternalServerError, "tenant membership operation failed", "INTERNAL_ERROR")
	}
}

package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"wms-backend/internal/platform/dbctx"
)

// membershipRepository is the narrow slice of *Repository Service actually
// needs — mirrors internal/auth's own userFinder/roleReader pattern of
// depending on behavior, not a concrete repository type, so tests can
// supply a fake.
type membershipRepository interface {
	ActiveMemberships(ctx context.Context, userID int64) ([]Membership, error)
	ActiveMembership(ctx context.Context, userID, tenantID int64) (*Membership, error)
}

// managementRepository is the second, separate slice of *Repository
// Service needs for the Platform Admin tenant-management API (WMS-29) —
// kept as its own interface, and its own constructor parameter, rather
// than folded into membershipRepository above, so every existing test
// double built against membershipRepository alone (see service_test.go's
// fakeRepo) keeps compiling unchanged. In production both parameters
// are the same *Repository value (main.go passes tenantRepo twice);
// they are only ever two different values in a test that wants to fake
// one capability without implementing the other.
type managementRepository interface {
	FindByID(ctx context.Context, id int64) (*Tenant, error)
	List(ctx context.Context, filter ListFilter) ([]Tenant, int64, error)
	CreateWithAudit(ctx context.Context, t *Tenant, buildLog func(tenantID int64) *TenantAuditLog) error
	UpdateWithAudit(ctx context.Context, t *Tenant, log *TenantAuditLog) error
	UpdateStatusWithAudit(ctx context.Context, id int64, status string, suspendedAt, cancelledAt *time.Time, log *TenantAuditLog) error
}

// ErrValidation carries a client-facing message for a 422 response —
// the same shape internal/warehouse, internal/zone, etc. already use.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// slugPattern enforces a URL/subdomain-safe tenant slug: lowercase
// letters, digits, and single hyphens between segments — never a
// leading/trailing/doubled hyphen. tenants.slug is VARCHAR(63)
// (migrations/0041); validating length here turns a would-be silent
// truncation or raw SQL error into a clear 422 instead.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// validStatuses are the only values tenants.status's CHECK constraint
// (CK_tenants_status, migrations/0041) accepts.
var validStatuses = map[string]bool{
	TenantStatusTrial:     true,
	TenantStatusActive:    true,
	TenantStatusSuspended: true,
	TenantStatusCancelled: true,
}

// Service resolves which tenant(s) a user may act as (for login-time
// auto-selection, explicit tenant switching, and
// middleware.RequireTenantContext's periodic re-check), and — as of
// WMS-29 — implements the Platform Admin tenant-management API's
// business rules: validation, unique-slug handling, and audit logging.
// Every management method here is called only from behind
// middleware.RequirePlatformAdmin; this Service does not itself check
// who is calling it.
type Service struct {
	repo       membershipRepository
	management managementRepository
}

func NewService(repo membershipRepository, management managementRepository) *Service {
	return &Service{repo: repo, management: management}
}

// ActiveMemberships lists every tenant userID currently holds an ACTIVE,
// operational-tenant membership in — e.g. for a future tenant-picker UI.
func (s *Service) ActiveMemberships(ctx context.Context, userID int64) ([]Membership, error) {
	return s.repo.ActiveMemberships(ctx, userID)
}

// DefaultMembership resolves which tenant a login with no explicit
// tenant_id should activate: the lowest tenant_id among the caller's
// ACTIVE, operational memberships. Deterministic because tenant_users
// carries no "default/primary" flag (migrations/0042) and the existing
// frontend has no tenant-picker UI to ask the caller instead (CLAUDE.md:
// don't add frontend features beyond what's requested). A user with
// exactly one membership — the only shape the seeded dev admin and most
// of this phase's tests exercise — gets that membership either way.
// Returns ErrMembershipNotFound (fail-closed) when the caller has no
// active, operational-tenant membership at all.
func (s *Service) DefaultMembership(ctx context.Context, userID int64) (*Membership, error) {
	memberships, err := s.repo.ActiveMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, ErrMembershipNotFound
	}
	return &memberships[0], nil
}

// ActiveMembership re-verifies a specific (userID, tenantID) pair against
// the database — used by tenant switching and by RequireTenantContext's
// mid-session re-check, never trusting a client-supplied tenant_id or a
// JWT's embedded claim on its own.
func (s *Service) ActiveMembership(ctx context.Context, userID, tenantID int64) (*Membership, error) {
	return s.repo.ActiveMembership(ctx, userID, tenantID)
}

// Valid adapts ActiveMembership to middleware.MembershipChecker's boolean
// shape: ErrMembershipNotFound becomes (false, nil) rather than an error,
// since "not a member" is an expected, fail-closed outcome, not a failure
// to even determine the answer. Any other error (e.g. the database is
// unreachable) is returned as-is so the caller can distinguish "denied"
// from "could not tell" if it ever needs to — RequireTenantContext itself
// currently treats both as a 403, per tenant-isolation-security.md's
// fail-closed rule.
func (s *Service) Valid(ctx context.Context, userID, tenantID int64) (bool, error) {
	_, err := s.ActiveMembership(ctx, userID, tenantID)
	if errors.Is(err, ErrMembershipNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// List returns the Platform Admin tenant list for GET
// /api/v1/platform/tenants, applying the same page/limit
// defaulting/clamping convention warehouse.Service.List and
// product.Service.List already use.
func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	rows, total, err := s.management.List(ctx, ListFilter{
		Search: params.Search, Status: params.Status, Page: page, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(rows))
	for _, t := range rows {
		responses = append(responses, toResponse(t))
	}
	return responses, total, nil
}

// Get returns one tenant for GET /api/v1/platform/tenants/:id.
func (s *Service) Get(ctx context.Context, id int64) (Response, error) {
	t, err := s.management.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*t), nil
}

// Create validates req and inserts a new tenant plus its
// tenant.create audit log row in one transaction, for POST
// /api/v1/platform/tenants. actorUserID is the authenticated platform
// admin — stamped onto the audit log as actor_user_id, never
// client-supplied.
func (s *Service) Create(ctx context.Context, req CreateRequest, actorUserID int64) (Response, error) {
	slug, err := normalizeSlug(req.Slug)
	if err != nil {
		return Response{}, err
	}
	name := req.Name
	if name == "" {
		return Response{}, ErrValidation{Message: "name is required"}
	}

	status := req.Status
	if status == "" {
		status = TenantStatusTrial
	}
	if !validStatuses[status] {
		return Response{}, ErrValidation{Message: "status must be one of TRIAL, ACTIVE, SUSPENDED, CANCELLED"}
	}

	t := &Tenant{Slug: slug, Name: name, Status: status}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()
	start := time.Now()
	err = s.management.CreateWithAudit(ctx, t, func(tenantID int64) *TenantAuditLog {
		return &TenantAuditLog{
			TenantID:    &tenantID,
			ActorUserID: actorUserID,
			ActorType:   AuditActorPlatformAdmin,
			Action:      "tenant.create",
			TargetType:  strPtr("tenant"),
			TargetID:    &tenantID,
			Metadata:    auditMetadata(map[string]any{"slug": slug, "name": name, "status": status}),
		}
	})
	dbctx.LogTransaction(ctx, "tenant.create", start, err)
	if err != nil {
		return Response{}, err
	}

	return toResponse(*t), nil
}

// Update validates req and renames/re-slugs an existing tenant plus its
// tenant.update audit log row in one transaction, for PUT
// /api/v1/platform/tenants/:id.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest, actorUserID int64) (Response, error) {
	before, err := s.management.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}

	slug, err := normalizeSlug(req.Slug)
	if err != nil {
		return Response{}, err
	}
	name := req.Name
	if name == "" {
		return Response{}, ErrValidation{Message: "name is required"}
	}

	t := &Tenant{ID: id, Slug: slug, Name: name}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()
	start := time.Now()
	err = s.management.UpdateWithAudit(ctx, t, &TenantAuditLog{
		TenantID:    &id,
		ActorUserID: actorUserID,
		ActorType:   AuditActorPlatformAdmin,
		Action:      "tenant.update",
		TargetType:  strPtr("tenant"),
		TargetID:    &id,
		Metadata: auditMetadata(map[string]any{
			"before": map[string]any{"slug": before.Slug, "name": before.Name},
			"after":  map[string]any{"slug": slug, "name": name},
		}),
	})
	dbctx.LogTransaction(ctx, "tenant.update", start, err)
	if err != nil {
		return Response{}, err
	}

	updated, err := s.management.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*updated), nil
}

// statusAuditAction maps a target status to the audit action code
// saas-module-roadmap.md's Platform Administration table names
// explicitly for Activate/Suspend; Trial/Cancel follow the same
// "tenant.<verb>" shape for consistency since the roadmap's table only
// enumerates the two it expected a dedicated endpoint for, and this API
// exposes all four transitions through one PATCH instead.
func statusAuditAction(status string) string {
	switch status {
	case TenantStatusActive:
		return "tenant.activate"
	case TenantStatusSuspended:
		return "tenant.suspend"
	case TenantStatusCancelled:
		return "tenant.cancel"
	default:
		return "tenant.trial"
	}
}

// UpdateStatus validates req and transitions tenantID's status plus its
// audit log row in one transaction, for PATCH
// /api/v1/platform/tenants/:id/status. Stamps/clears suspended_at and
// cancelled_at to match the new status (see
// Repository.UpdateStatusWithAudit's doc).
func (s *Service) UpdateStatus(ctx context.Context, id int64, req UpdateStatusRequest, actorUserID int64) (Response, error) {
	before, err := s.management.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}

	if !validStatuses[req.Status] {
		return Response{}, ErrValidation{Message: "status must be one of TRIAL, ACTIVE, SUSPENDED, CANCELLED"}
	}

	var suspendedAt, cancelledAt *time.Time
	now := time.Now().UTC()
	switch req.Status {
	case TenantStatusSuspended:
		suspendedAt = &now
	case TenantStatusCancelled:
		cancelledAt = &now
	}

	metadata := map[string]any{"from_status": before.Status, "to_status": req.Status}
	if req.Reason != nil {
		metadata["reason"] = *req.Reason
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()
	start := time.Now()
	err = s.management.UpdateStatusWithAudit(ctx, id, req.Status, suspendedAt, cancelledAt, &TenantAuditLog{
		TenantID:    &id,
		ActorUserID: actorUserID,
		ActorType:   AuditActorPlatformAdmin,
		Action:      statusAuditAction(req.Status),
		TargetType:  strPtr("tenant"),
		TargetID:    &id,
		Metadata:    auditMetadata(metadata),
	})
	dbctx.LogTransaction(ctx, "tenant.update_status", start, err)
	if err != nil {
		return Response{}, err
	}

	updated, err := s.management.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*updated), nil
}

// normalizeSlug validates req.Slug against slugPattern and its column
// length, returning ErrValidation on failure.
func normalizeSlug(slug string) (string, error) {
	if slug == "" {
		return "", ErrValidation{Message: "slug is required"}
	}
	if len(slug) > 63 {
		return "", ErrValidation{Message: "slug must be 63 characters or fewer"}
	}
	if !slugPattern.MatchString(slug) {
		return "", ErrValidation{Message: "slug must be lowercase letters, digits, and single hyphens only (e.g. acme-logistics)"}
	}
	return slug, nil
}

// auditMetadata marshals v to a *string for TenantAuditLog.Metadata,
// returning nil (not a pointer to the literal string "null") if
// marshaling ever fails — an audit log write must never fail, or be
// rejected by the NVARCHAR(MAX) column, over a metadata detail that is
// secondary to the fact of the action itself.
func auditMetadata(v map[string]any) *string {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

func strPtr(s string) *string { return &s }

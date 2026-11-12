package tenant

import (
	"context"
	"errors"
)

// membershipRepository is the narrow slice of *Repository Service actually
// needs — mirrors internal/auth's own userFinder/roleReader pattern of
// depending on behavior, not a concrete repository type, so tests can
// supply a fake.
type membershipRepository interface {
	ActiveMemberships(ctx context.Context, userID int64) ([]Membership, error)
	ActiveMembership(ctx context.Context, userID, tenantID int64) (*Membership, error)
}

// Service resolves which tenant(s) a user may act as, for login-time
// auto-selection, explicit tenant switching, and the periodic re-check
// RequireTenantContext performs.
type Service struct {
	repo membershipRepository
}

func NewService(repo membershipRepository) *Service { return &Service{repo: repo} }

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

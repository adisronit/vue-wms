package tenant

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is a membershipRepository double so Service's own logic
// (deterministic default selection, error passthrough, the Valid
// boolean adapter) can be tested without a real SQL Server connection —
// the repository's SQL itself is covered separately by
// model_integration_test.go's real-database tests.
type fakeRepo struct {
	memberships map[int64][]Membership
	err         error
}

func (f *fakeRepo) ActiveMemberships(_ context.Context, userID int64) ([]Membership, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.memberships[userID], nil
}

func (f *fakeRepo) ActiveMembership(_ context.Context, userID, tenantID int64) (*Membership, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, m := range f.memberships[userID] {
		if m.TenantID == tenantID {
			found := m
			return &found, nil
		}
	}
	return nil, ErrMembershipNotFound
}

func TestDefaultMembership_PicksFirstOfOrderedList(t *testing.T) {
	repo := &fakeRepo{memberships: map[int64][]Membership{
		1: {{TenantID: 100, TenantSlug: "a"}, {TenantID: 200, TenantSlug: "b"}},
	}}
	svc := NewService(repo, nil) // nil: these tests exercise membership methods only

	m, err := svc.DefaultMembership(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.TenantID != 100 {
		t.Fatalf("tenant id = %d, want 100 (the first entry)", m.TenantID)
	}
}

func TestDefaultMembership_NoMemberships_FailsClosed(t *testing.T) {
	repo := &fakeRepo{memberships: map[int64][]Membership{}}
	svc := NewService(repo, nil) // nil: these tests exercise membership methods only

	_, err := svc.DefaultMembership(context.Background(), 1)
	if !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("error = %v, want ErrMembershipNotFound", err)
	}
}

func TestDefaultMembership_PropagatesRepositoryError(t *testing.T) {
	wantErr := errors.New("database unreachable")
	repo := &fakeRepo{err: wantErr}
	svc := NewService(repo, nil) // nil: these tests exercise membership methods only

	_, err := svc.DefaultMembership(context.Background(), 1)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestActiveMembership_Found(t *testing.T) {
	repo := &fakeRepo{memberships: map[int64][]Membership{
		1: {{TenantID: 100, TenantSlug: "a"}},
	}}
	svc := NewService(repo, nil) // nil: these tests exercise membership methods only

	m, err := svc.ActiveMembership(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.TenantSlug != "a" {
		t.Fatalf("slug = %q, want a", m.TenantSlug)
	}
}

func TestValid_TrueForActiveMembership(t *testing.T) {
	repo := &fakeRepo{memberships: map[int64][]Membership{
		1: {{TenantID: 100}},
	}}
	svc := NewService(repo, nil) // nil: these tests exercise membership methods only

	ok, err := svc.Valid(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected Valid to report true for an active membership")
	}
}

// TestValid_FalseNotErrorForMissingMembership covers the adapter's own
// documented contract: "not a member" must come back as (false, nil), not
// as ErrMembershipNotFound bubbling up as a generic error — callers like
// middleware.RequireTenantContext key their fail-closed behavior off the
// bool, and must not need to know this package's specific error type.
func TestValid_FalseNotErrorForMissingMembership(t *testing.T) {
	repo := &fakeRepo{memberships: map[int64][]Membership{}}
	svc := NewService(repo, nil) // nil: these tests exercise membership methods only

	ok, err := svc.Valid(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v, want (false, nil) for a missing membership", err)
	}
	if ok {
		t.Fatal("expected Valid to report false for a missing membership")
	}
}

func TestValid_PropagatesNonMembershipErrors(t *testing.T) {
	wantErr := errors.New("database unreachable")
	repo := &fakeRepo{err: wantErr}
	svc := NewService(repo, nil) // nil: these tests exercise membership methods only

	ok, err := svc.Valid(context.Background(), 1, 100)
	if ok {
		t.Fatal("expected Valid to report false when the check itself failed")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

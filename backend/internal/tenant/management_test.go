package tenant

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeManagementRepo is a managementRepository double, mirroring
// fakeRepo's role for membershipRepository in service_test.go — lets
// Service's validation/defaulting/audit-construction logic be tested
// without a real SQL Server connection. The repository's own SQL
// (transactions, unique-constraint mapping) is covered separately by a
// real-database test.
type fakeManagementRepo struct {
	tenants map[int64]*Tenant
	nextID  int64

	createErr error
	updateErr error
	statusErr error

	lastCreateLog *TenantAuditLog
	lastUpdateLog *TenantAuditLog
	lastStatusLog *TenantAuditLog
}

func newFakeManagementRepo(seed ...*Tenant) *fakeManagementRepo {
	r := &fakeManagementRepo{tenants: map[int64]*Tenant{}, nextID: 1}
	for _, t := range seed {
		r.tenants[t.ID] = t
		if t.ID >= r.nextID {
			r.nextID = t.ID + 1
		}
	}
	return r
}

func (f *fakeManagementRepo) FindByID(_ context.Context, id int64) (*Tenant, error) {
	t, ok := f.tenants[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *t
	return &copy, nil
}

func (f *fakeManagementRepo) List(_ context.Context, _ ListFilter) ([]Tenant, int64, error) {
	var out []Tenant
	for _, t := range f.tenants {
		out = append(out, *t)
	}
	return out, int64(len(out)), nil
}

func (f *fakeManagementRepo) CreateWithAudit(_ context.Context, t *Tenant, buildLog func(int64) *TenantAuditLog) error {
	if f.createErr != nil {
		return f.createErr
	}
	t.ID = f.nextID
	f.nextID++
	stored := *t
	f.tenants[t.ID] = &stored
	f.lastCreateLog = buildLog(t.ID)
	return nil
}

func (f *fakeManagementRepo) UpdateWithAudit(_ context.Context, t *Tenant, log *TenantAuditLog) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	existing, ok := f.tenants[t.ID]
	if !ok {
		return ErrNotFound
	}
	existing.Slug = t.Slug
	existing.Name = t.Name
	f.lastUpdateLog = log
	return nil
}

func (f *fakeManagementRepo) UpdateStatusWithAudit(_ context.Context, id int64, status string, suspendedAt, cancelledAt *time.Time, log *TenantAuditLog) error {
	if f.statusErr != nil {
		return f.statusErr
	}
	existing, ok := f.tenants[id]
	if !ok {
		return ErrNotFound
	}
	existing.Status = status
	existing.SuspendedAt = suspendedAt
	existing.CancelledAt = cancelledAt
	f.lastStatusLog = log
	return nil
}

func TestCreate_DefaultsStatusToTrial(t *testing.T) {
	repo := newFakeManagementRepo()
	svc := NewService(nil, repo)

	resp, err := svc.Create(context.Background(), CreateRequest{Slug: "acme-logistics", Name: "Acme"}, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != TenantStatusTrial {
		t.Fatalf("status = %q, want %q", resp.Status, TenantStatusTrial)
	}
	if repo.lastCreateLog == nil || repo.lastCreateLog.Action != "tenant.create" || repo.lastCreateLog.ActorUserID != 7 {
		t.Fatalf("audit log not recorded correctly: %+v", repo.lastCreateLog)
	}
}

func TestCreate_RejectsInvalidSlug(t *testing.T) {
	repo := newFakeManagementRepo()
	svc := NewService(nil, repo)

	cases := []string{"Not Valid", "-leading-hyphen", "trailing-hyphen-", "double--hyphen", "", "UPPERCASE"}
	for _, slug := range cases {
		_, err := svc.Create(context.Background(), CreateRequest{Slug: slug, Name: "X"}, 1)
		var verr ErrValidation
		if !errors.As(err, &verr) {
			t.Errorf("slug %q: error = %v, want ErrValidation", slug, err)
		}
	}
}

func TestCreate_RejectsInvalidStatus(t *testing.T) {
	repo := newFakeManagementRepo()
	svc := NewService(nil, repo)

	_, err := svc.Create(context.Background(), CreateRequest{Slug: "acme", Name: "Acme", Status: "BOGUS"}, 1)
	var verr ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_PropagatesDuplicateSlug(t *testing.T) {
	repo := newFakeManagementRepo()
	repo.createErr = ErrDuplicate
	svc := NewService(nil, repo)

	_, err := svc.Create(context.Background(), CreateRequest{Slug: "acme", Name: "Acme"}, 1)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("error = %v, want ErrDuplicate", err)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := newFakeManagementRepo()
	svc := NewService(nil, repo)

	_, err := svc.Update(context.Background(), 999, UpdateRequest{Slug: "acme", Name: "Acme"}, 1)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateStatus_SuspendStampsTimestampAndClearsOnReactivate(t *testing.T) {
	repo := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme", Status: TenantStatusActive})
	svc := NewService(nil, repo)

	reason := "non-payment"
	resp, err := svc.UpdateStatus(context.Background(), 1, UpdateStatusRequest{Status: TenantStatusSuspended, Reason: &reason}, 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != TenantStatusSuspended || resp.SuspendedAt == nil {
		t.Fatalf("got status=%q suspendedAt=%v, want SUSPENDED with a stamped timestamp", resp.Status, resp.SuspendedAt)
	}
	if repo.lastStatusLog.Action != "tenant.suspend" {
		t.Fatalf("audit action = %q, want tenant.suspend", repo.lastStatusLog.Action)
	}

	resp, err = svc.UpdateStatus(context.Background(), 1, UpdateStatusRequest{Status: TenantStatusActive}, 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != TenantStatusActive || resp.SuspendedAt != nil {
		t.Fatalf("got status=%q suspendedAt=%v, want ACTIVE with suspended_at cleared", resp.Status, resp.SuspendedAt)
	}
	if repo.lastStatusLog.Action != "tenant.activate" {
		t.Fatalf("audit action = %q, want tenant.activate", repo.lastStatusLog.Action)
	}
}

func TestUpdateStatus_RejectsInvalidStatus(t *testing.T) {
	repo := newFakeManagementRepo(&Tenant{ID: 1, Slug: "acme", Name: "Acme", Status: TenantStatusActive})
	svc := NewService(nil, repo)

	_, err := svc.UpdateStatus(context.Background(), 1, UpdateStatusRequest{Status: "BOGUS"}, 9)
	var verr ErrValidation
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

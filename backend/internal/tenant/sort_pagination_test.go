package tenant

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
)

// seedTenant creates a tenant row directly (not via Service.CreateWithAudit,
// which also requires an audit log callback) — these tests only exercise
// List's sort/pagination/filter behavior, not creation. slug must be
// unique across the whole tenants table, so every seed call here uses a
// label-scoped prefix rather than a shared literal.
func seedTenant(t *testing.T, tx *gorm.DB, slug, name, status string) Tenant {
	t.Helper()
	tn := Tenant{Slug: slug, Name: name, Status: status}
	if err := tx.Create(&tn).Error; err != nil {
		t.Fatalf("seedTenant(%s): %v", slug, err)
	}
	return tn
}

func TestService_List_DefaultsToNameAscending(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	seedTenant(t, tx, "sort-default-charlie", "Charlie Co", TenantStatusActive)
	seedTenant(t, tx, "sort-default-alpha", "Alpha Co", TenantStatusActive)
	seedTenant(t, tx, "sort-default-bravo", "Bravo Co", TenantStatusActive)

	svc := NewService(nil, NewRepository(tx))
	responses, total, err := svc.List(context.Background(), ListParams{Search: "sort-default"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	got := []string{responses[0].Name, responses[1].Name, responses[2].Name}
	want := []string{"Alpha Co", "Bravo Co", "Charlie Co"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("default order = %v, want name ASC %v", got, want)
	}
}

func TestService_List_SortByColumn_AscAndDesc(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	seedTenant(t, tx, "sort-col-charlie", "Charlie Co", TenantStatusActive)
	seedTenant(t, tx, "sort-col-alpha", "Alpha Co", TenantStatusActive)
	seedTenant(t, tx, "sort-col-bravo", "Bravo Co", TenantStatusActive)

	svc := NewService(nil, NewRepository(tx))

	asc, _, err := svc.List(context.Background(), ListParams{Search: "sort-col", SortBy: "slug", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(slug asc): %v", err)
	}
	if asc[0].Slug != "sort-col-alpha" || asc[1].Slug != "sort-col-bravo" || asc[2].Slug != "sort-col-charlie" {
		t.Errorf("slug ASC order = [%s %s %s], want alpha/bravo/charlie", asc[0].Slug, asc[1].Slug, asc[2].Slug)
	}

	desc, _, err := svc.List(context.Background(), ListParams{Search: "sort-col", SortBy: "slug", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(slug desc): %v", err)
	}
	if desc[0].Slug != "sort-col-charlie" || desc[1].Slug != "sort-col-bravo" || desc[2].Slug != "sort-col-alpha" {
		t.Errorf("slug DESC order = [%s %s %s], want charlie/bravo/alpha", desc[0].Slug, desc[1].Slug, desc[2].Slug)
	}
}

// TestService_List_SecondarySortIsDeterministic proves that when every
// seeded row shares the same status (the primary sort column here), the
// result order is still fully determined by id ASC rather than left to
// whatever order SQL Server happens to return tied rows in.
func TestService_List_SecondarySortIsDeterministic(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	first := seedTenant(t, tx, "sort-secondary-z", "Zulu Co", TenantStatusActive)
	second := seedTenant(t, tx, "sort-secondary-y", "Yankee Co", TenantStatusActive)
	third := seedTenant(t, tx, "sort-secondary-x", "Xray Co", TenantStatusActive)

	svc := NewService(nil, NewRepository(tx))

	for i := 0; i < 3; i++ {
		responses, _, err := svc.List(context.Background(), ListParams{Search: "sort-secondary", SortBy: "status", SortDir: "asc"})
		if err != nil {
			t.Fatalf("List(status asc) run %d: %v", i, err)
		}
		if responses[0].ID != first.ID || responses[1].ID != second.ID || responses[2].ID != third.ID {
			t.Fatalf("run %d: order = [%d %d %d], want insertion/id order [%d %d %d] every time",
				i, responses[0].ID, responses[1].ID, responses[2].ID, first.ID, second.ID, third.ID)
		}
	}
}

func TestService_List_InvalidSortBy_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	svc := NewService(nil, NewRepository(tx))
	_, _, err := svc.List(context.Background(), ListParams{SortBy: "status; DROP TABLE tenants"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_by) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_InvalidSortDir_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	svc := NewService(nil, NewRepository(tx))
	_, _, err := svc.List(context.Background(), ListParams{SortBy: "name", SortDir: "sideways"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_dir) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_Pagination_PagesThroughNameOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	names := []string{"A Co", "B Co", "C Co", "D Co", "E Co"}
	for i, name := range names {
		seedTenant(t, tx, fmt.Sprintf("sort-page-%d", i), name, TenantStatusActive)
	}

	svc := NewService(nil, NewRepository(tx))

	page1, total, err := svc.List(context.Background(), ListParams{Search: "sort-page", Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 1): %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(page1) != 2 || page1[0].Name != "A Co" || page1[1].Name != "B Co" {
		t.Fatalf("page 1 = %+v, want [A Co, B Co]", page1)
	}

	page3, _, err := svc.List(context.Background(), ListParams{Search: "sort-page", Page: 3, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 3): %v", err)
	}
	if len(page3) != 1 || page3[0].Name != "E Co" {
		t.Fatalf("page 3 (final, partial) = %+v, want [E Co]", page3)
	}
}

func TestService_List_LimitClampedToDefault_WhenOutOfRange(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	for i := 0; i < defaultLimit+1; i++ {
		seedTenant(t, tx, fmt.Sprintf("sort-clamp-%03d", i), fmt.Sprintf("Clamp Co %03d", i), TenantStatusActive)
	}

	svc := NewService(nil, NewRepository(tx))

	zero, total, err := svc.List(context.Background(), ListParams{Search: "sort-clamp", Limit: 0})
	if err != nil {
		t.Fatalf("List(limit=0): %v", err)
	}
	if total != int64(defaultLimit+1) {
		t.Fatalf("total = %d, want %d", total, defaultLimit+1)
	}
	if len(zero) != defaultLimit {
		t.Errorf("List(limit=0) returned %d rows, want %d (clamped default)", len(zero), defaultLimit)
	}

	tooBig, _, err := svc.List(context.Background(), ListParams{Search: "sort-clamp", Limit: 1000})
	if err != nil {
		t.Fatalf("List(limit=1000): %v", err)
	}
	if len(tooBig) != defaultLimit {
		t.Errorf("List(limit=1000) returned %d rows, want %d (clamped default)", len(tooBig), defaultLimit)
	}
}

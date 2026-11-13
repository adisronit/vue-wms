package productcategory

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
)

// newSortTestTenant inserts a fresh, empty tenant on tx — unlike the
// shared bootstrap tenant model_tenant_integration_test.go's tests
// reuse, sort/pagination assertions need to know the *exact* row set
// and count for the tenant under test, which a tenant that might
// already carry seed/fixture categories from other tests cannot
// guarantee.
func newSortTestTenant(t *testing.T, tx *gorm.DB, label string) int64 {
	t.Helper()
	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("cat-sort-%s", label), fmt.Sprintf("Category Sort Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newSortTestTenant(%s): insert tenant: %v", label, err)
	}
	return tenantID
}

func seedCategory(t *testing.T, tx *gorm.DB, tenantID int64, code, name string, status Status) Category {
	t.Helper()
	c := Category{TenantID: tenantID, Code: code, Name: name, Status: status}
	if err := tx.Create(&c).Error; err != nil {
		t.Fatalf("seedCategory(%s): %v", code, err)
	}
	return c
}

func TestService_List_DefaultsToCodeAscending(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "default-sort")

	seedCategory(t, tx, tenantID, "CAT-C", "Charlie", StatusActive)
	seedCategory(t, tx, tenantID, "CAT-A", "Alpha", StatusActive)
	seedCategory(t, tx, tenantID, "CAT-B", "Bravo", StatusActive)

	svc := NewService(NewRepository(tx), nil)
	responses, total, err := svc.List(context.Background(), tenantID, ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	got := []string{responses[0].Code, responses[1].Code, responses[2].Code}
	want := []string{"CAT-A", "CAT-B", "CAT-C"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("default order = %v, want code ASC %v", got, want)
	}
}

func TestService_List_SortByColumn_AscAndDesc(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "sort-column")

	seedCategory(t, tx, tenantID, "CAT-1", "Charlie", StatusActive)
	seedCategory(t, tx, tenantID, "CAT-2", "Alpha", StatusActive)
	seedCategory(t, tx, tenantID, "CAT-3", "Bravo", StatusActive)

	svc := NewService(NewRepository(tx), nil)

	asc, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "name", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(name asc): %v", err)
	}
	if asc[0].Name != "Alpha" || asc[1].Name != "Bravo" || asc[2].Name != "Charlie" {
		t.Errorf("name ASC order = [%s %s %s], want [Alpha Bravo Charlie]", asc[0].Name, asc[1].Name, asc[2].Name)
	}

	desc, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "name", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(name desc): %v", err)
	}
	if desc[0].Name != "Charlie" || desc[1].Name != "Bravo" || desc[2].Name != "Alpha" {
		t.Errorf("name DESC order = [%s %s %s], want [Charlie Bravo Alpha]", desc[0].Name, desc[1].Name, desc[2].Name)
	}
}

// TestService_List_SecondarySortIsDeterministic proves that when the
// primary sort column ties (status only has two values, so this is the
// common case), the result order is still fully determined — by id
// ascending — rather than left to whatever order SQL Server happens to
// return tied rows in, which could otherwise change from call to call
// and make pagination flaky.
func TestService_List_SecondarySortIsDeterministic(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "secondary-sort")

	first := seedCategory(t, tx, tenantID, "CAT-Z", "Zulu", StatusActive)
	second := seedCategory(t, tx, tenantID, "CAT-Y", "Yankee", StatusActive)
	third := seedCategory(t, tx, tenantID, "CAT-X", "Xray", StatusActive)

	svc := NewService(NewRepository(tx), nil)

	for i := 0; i < 3; i++ {
		responses, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "status", SortDir: "asc"})
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
	tenantID := newSortTestTenant(t, tx, "invalid-sortby")

	svc := NewService(NewRepository(tx), nil)
	_, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "description; DROP TABLE product_categories"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_by) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_InvalidSortDir_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "invalid-sortdir")

	svc := NewService(NewRepository(tx), nil)
	_, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "code", SortDir: "sideways"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_dir) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_Pagination_PagesThroughCodeOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "pagination")

	codes := []string{"CAT-A", "CAT-B", "CAT-C", "CAT-D", "CAT-E"}
	for _, code := range codes {
		seedCategory(t, tx, tenantID, code, code, StatusActive)
	}

	svc := NewService(NewRepository(tx), nil)

	page1, total, err := svc.List(context.Background(), tenantID, ListParams{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 1): %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(page1) != 2 || page1[0].Code != "CAT-A" || page1[1].Code != "CAT-B" {
		t.Fatalf("page 1 = %+v, want [CAT-A CAT-B]", page1)
	}

	page3, _, err := svc.List(context.Background(), tenantID, ListParams{Page: 3, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 3): %v", err)
	}
	if len(page3) != 1 || page3[0].Code != "CAT-E" {
		t.Fatalf("page 3 (final, partial) = %+v, want [CAT-E]", page3)
	}
}

func TestService_List_LimitClampedToDefault_WhenOutOfRange(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "limit-clamp")

	// Seed one more row than the default page size (25) so an
	// unclamped/zero limit would wrongly return every row instead of
	// being clamped to defaultLimit.
	for i := 0; i < defaultLimit+1; i++ {
		seedCategory(t, tx, tenantID, fmt.Sprintf("CAT-%03d", i), fmt.Sprintf("Category %03d", i), StatusActive)
	}

	svc := NewService(NewRepository(tx), nil)

	zero, total, err := svc.List(context.Background(), tenantID, ListParams{Limit: 0})
	if err != nil {
		t.Fatalf("List(limit=0): %v", err)
	}
	if total != int64(defaultLimit+1) {
		t.Fatalf("total = %d, want %d", total, defaultLimit+1)
	}
	if len(zero) != defaultLimit {
		t.Errorf("List(limit=0) returned %d rows, want %d (clamped default)", len(zero), defaultLimit)
	}

	tooBig, _, err := svc.List(context.Background(), tenantID, ListParams{Limit: 1000})
	if err != nil {
		t.Fatalf("List(limit=1000): %v", err)
	}
	if len(tooBig) != defaultLimit {
		t.Errorf("List(limit=1000) returned %d rows, want %d (clamped default)", len(tooBig), defaultLimit)
	}
}

func TestService_List_SearchCombinesWithSort(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "search-sort")

	seedCategory(t, tx, tenantID, "PKG-BOX", "Boxes", StatusActive)
	seedCategory(t, tx, tenantID, "PKG-WRAP", "Wraps", StatusActive)
	seedCategory(t, tx, tenantID, "RAW-STEEL", "Steel", StatusActive)

	svc := NewService(NewRepository(tx), nil)
	responses, total, err := svc.List(context.Background(), tenantID, ListParams{
		Search: "PKG", SortBy: "name", SortDir: "desc",
	})
	if err != nil {
		t.Fatalf("List(search+sort): %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (only PKG-* rows)", total)
	}
	if responses[0].Name != "Wraps" || responses[1].Name != "Boxes" {
		t.Errorf("order = [%s %s], want [Wraps Boxes] (name DESC within the PKG search)", responses[0].Name, responses[1].Name)
	}
}

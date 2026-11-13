package customer

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
)

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusActive: true, StatusInactive: true,
		"NOT_REAL": false, "": false,
	}
	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

// newSortTestTenant inserts a fresh, empty tenant on tx — unlike the
// shared bootstrap tenant model_tenant_integration_test.go's test
// reuses, sort/pagination assertions need to know the *exact* row set
// and count for the tenant under test, which a tenant that might
// already carry seed/fixture customers from other tests cannot
// guarantee. Mirrors internal/productcategory/sort_pagination_test.go's
// own newSortTestTenant.
func newSortTestTenant(t *testing.T, tx *gorm.DB, label string) int64 {
	t.Helper()
	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("cust-sort-%s", label), fmt.Sprintf("Customer Sort Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newSortTestTenant(%s): insert tenant: %v", label, err)
	}
	return tenantID
}

func seedCustomer(t *testing.T, tx *gorm.DB, tenantID int64, code, companyName string, status Status) Customer {
	t.Helper()
	c := Customer{TenantID: tenantID, Code: code, CompanyName: companyName, Status: status}
	if err := tx.Create(&c).Error; err != nil {
		t.Fatalf("seedCustomer(%s): %v", code, err)
	}
	return c
}

func TestService_List_DefaultsToCodeAscending(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "default-sort")

	seedCustomer(t, tx, tenantID, "CUST-C", "Charlie Co.", StatusActive)
	seedCustomer(t, tx, tenantID, "CUST-A", "Alpha Co.", StatusActive)
	seedCustomer(t, tx, tenantID, "CUST-B", "Bravo Co.", StatusActive)

	svc := NewService(NewRepository(tx))
	responses, total, err := svc.List(context.Background(), tenantID, ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	got := []string{responses[0].Code, responses[1].Code, responses[2].Code}
	want := []string{"CUST-A", "CUST-B", "CUST-C"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("default order = %v, want code ASC %v", got, want)
	}
}

func TestService_List_SortByColumn_AscAndDesc(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "sort-column")

	seedCustomer(t, tx, tenantID, "CUST-1", "Charlie Co.", StatusActive)
	seedCustomer(t, tx, tenantID, "CUST-2", "Alpha Co.", StatusActive)
	seedCustomer(t, tx, tenantID, "CUST-3", "Bravo Co.", StatusActive)

	svc := NewService(NewRepository(tx))

	asc, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "companyName", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(companyName asc): %v", err)
	}
	if asc[0].CompanyName != "Alpha Co." || asc[1].CompanyName != "Bravo Co." || asc[2].CompanyName != "Charlie Co." {
		t.Errorf("companyName ASC order = [%s %s %s], want [Alpha Co. Bravo Co. Charlie Co.]",
			asc[0].CompanyName, asc[1].CompanyName, asc[2].CompanyName)
	}

	desc, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "companyName", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(companyName desc): %v", err)
	}
	if desc[0].CompanyName != "Charlie Co." || desc[1].CompanyName != "Bravo Co." || desc[2].CompanyName != "Alpha Co." {
		t.Errorf("companyName DESC order = [%s %s %s], want [Charlie Co. Bravo Co. Alpha Co.]",
			desc[0].CompanyName, desc[1].CompanyName, desc[2].CompanyName)
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

	first := seedCustomer(t, tx, tenantID, "CUST-Z", "Zulu Co.", StatusActive)
	second := seedCustomer(t, tx, tenantID, "CUST-Y", "Yankee Co.", StatusActive)
	third := seedCustomer(t, tx, tenantID, "CUST-X", "Xray Co.", StatusActive)

	svc := NewService(NewRepository(tx))

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

	svc := NewService(NewRepository(tx))
	_, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "taxId; DROP TABLE customers"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_by) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_InvalidSortDir_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "invalid-sortdir")

	svc := NewService(NewRepository(tx))
	_, _, err := svc.List(context.Background(), tenantID, ListParams{SortBy: "code", SortDir: "sideways"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_dir) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_Pagination_PagesThroughCodeOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newSortTestTenant(t, tx, "pagination")

	codes := []string{"CUST-A", "CUST-B", "CUST-C", "CUST-D", "CUST-E"}
	for _, code := range codes {
		seedCustomer(t, tx, tenantID, code, code, StatusActive)
	}

	svc := NewService(NewRepository(tx))

	page1, total, err := svc.List(context.Background(), tenantID, ListParams{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 1): %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(page1) != 2 || page1[0].Code != "CUST-A" || page1[1].Code != "CUST-B" {
		t.Fatalf("page 1 = %+v, want [CUST-A CUST-B]", page1)
	}

	page3, _, err := svc.List(context.Background(), tenantID, ListParams{Page: 3, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 3): %v", err)
	}
	if len(page3) != 1 || page3[0].Code != "CUST-E" {
		t.Fatalf("page 3 (final, partial) = %+v, want [CUST-E]", page3)
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
		seedCustomer(t, tx, tenantID, fmt.Sprintf("CUST-%03d", i), fmt.Sprintf("Customer %03d Co.", i), StatusActive)
	}

	svc := NewService(NewRepository(tx))

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

	seedCustomer(t, tx, tenantID, "PKG-BOX", "Boxes Co.", StatusActive)
	seedCustomer(t, tx, tenantID, "PKG-WRAP", "Wraps Co.", StatusActive)
	seedCustomer(t, tx, tenantID, "RAW-STEEL", "Steel Co.", StatusActive)

	svc := NewService(NewRepository(tx))
	responses, total, err := svc.List(context.Background(), tenantID, ListParams{
		Search: "PKG", SortBy: "companyName", SortDir: "desc",
	})
	if err != nil {
		t.Fatalf("List(search+sort): %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (only PKG-* rows)", total)
	}
	if responses[0].CompanyName != "Wraps Co." || responses[1].CompanyName != "Boxes Co." {
		t.Errorf("order = [%s %s], want [Wraps Co. Boxes Co.] (companyName DESC within the PKG search)",
			responses[0].CompanyName, responses[1].CompanyName)
	}
}

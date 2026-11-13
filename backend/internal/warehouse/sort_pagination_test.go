package warehouse

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
)

// seedWarehouse inserts a warehouse row on tx for sort/pagination
// assertions. Unlike internal/productcategory's equivalent helper,
// there is no tenant to scope by — warehouses are deliberately not
// tenant-owned rows (see repository.go/CLAUDE.md) — so every test here
// gives its rows a unique code/name prefix and narrows List with
// ListParams.Search to that prefix, keeping assertions about exact
// counts/order correct regardless of whatever other warehouse rows
// already exist in the test database.
func seedWarehouse(t *testing.T, tx *gorm.DB, code, name, typ string, status Status) Warehouse {
	t.Helper()
	w := Warehouse{Code: code, Name: name, Type: typ, Status: status}
	if err := tx.Create(&w).Error; err != nil {
		t.Fatalf("seedWarehouse(%s): %v", code, err)
	}
	return w
}

func TestService_List_DefaultsToCodeAscending(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	seedWarehouse(t, tx, "WHSORT1-C", "Charlie Warehouse", "Distribution Center", StatusActive)
	seedWarehouse(t, tx, "WHSORT1-A", "Alpha Warehouse", "Distribution Center", StatusActive)
	seedWarehouse(t, tx, "WHSORT1-B", "Bravo Warehouse", "Distribution Center", StatusActive)

	svc := NewService(NewRepository(tx), nil)
	responses, total, err := svc.List(context.Background(), ListParams{Search: "WHSORT1-"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	got := []string{responses[0].Code, responses[1].Code, responses[2].Code}
	want := []string{"WHSORT1-A", "WHSORT1-B", "WHSORT1-C"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("default order = %v, want code ASC %v", got, want)
	}
}

func TestService_List_SortByColumn_AscAndDesc(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	seedWarehouse(t, tx, "WHSORT2-1", "Charlie Warehouse", "Distribution Center", StatusActive)
	seedWarehouse(t, tx, "WHSORT2-2", "Alpha Warehouse", "Distribution Center", StatusActive)
	seedWarehouse(t, tx, "WHSORT2-3", "Bravo Warehouse", "Distribution Center", StatusActive)

	svc := NewService(NewRepository(tx), nil)

	asc, _, err := svc.List(context.Background(), ListParams{Search: "WHSORT2-", SortBy: "name", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(name asc): %v", err)
	}
	if asc[0].Name != "Alpha Warehouse" || asc[1].Name != "Bravo Warehouse" || asc[2].Name != "Charlie Warehouse" {
		t.Errorf("name ASC order = [%s %s %s], want [Alpha Bravo Charlie]", asc[0].Name, asc[1].Name, asc[2].Name)
	}

	desc, _, err := svc.List(context.Background(), ListParams{Search: "WHSORT2-", SortBy: "name", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(name desc): %v", err)
	}
	if desc[0].Name != "Charlie Warehouse" || desc[1].Name != "Bravo Warehouse" || desc[2].Name != "Alpha Warehouse" {
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

	first := seedWarehouse(t, tx, "WHSORT3-Z", "Zulu Warehouse", "Distribution Center", StatusActive)
	second := seedWarehouse(t, tx, "WHSORT3-Y", "Yankee Warehouse", "Distribution Center", StatusActive)
	third := seedWarehouse(t, tx, "WHSORT3-X", "Xray Warehouse", "Distribution Center", StatusActive)

	svc := NewService(NewRepository(tx), nil)

	for i := 0; i < 3; i++ {
		responses, _, err := svc.List(context.Background(), ListParams{Search: "WHSORT3-", SortBy: "status", SortDir: "asc"})
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

	svc := NewService(NewRepository(tx), nil)
	_, _, err := svc.List(context.Background(), ListParams{SortBy: "description; DROP TABLE warehouses"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_by) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_InvalidSortDir_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	svc := NewService(NewRepository(tx), nil)
	_, _, err := svc.List(context.Background(), ListParams{SortBy: "code", SortDir: "sideways"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_dir) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_Pagination_PagesThroughCodeOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	codes := []string{"WHSORT4-A", "WHSORT4-B", "WHSORT4-C", "WHSORT4-D", "WHSORT4-E"}
	for _, code := range codes {
		seedWarehouse(t, tx, code, code, "Distribution Center", StatusActive)
	}

	svc := NewService(NewRepository(tx), nil)

	page1, total, err := svc.List(context.Background(), ListParams{Search: "WHSORT4-", Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 1): %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(page1) != 2 || page1[0].Code != "WHSORT4-A" || page1[1].Code != "WHSORT4-B" {
		t.Fatalf("page 1 = %+v, want [WHSORT4-A WHSORT4-B]", page1)
	}

	page3, _, err := svc.List(context.Background(), ListParams{Search: "WHSORT4-", Page: 3, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 3): %v", err)
	}
	if len(page3) != 1 || page3[0].Code != "WHSORT4-E" {
		t.Fatalf("page 3 (final, partial) = %+v, want [WHSORT4-E]", page3)
	}
}

func TestService_List_LimitClampedToDefault_WhenOutOfRange(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	// Seed one more row than the default page size (25) so an
	// unclamped/zero limit would wrongly return every row instead of
	// being clamped to defaultLimit.
	for i := 0; i < defaultLimit+1; i++ {
		code := fmt.Sprintf("WHCLAMP-%03d", i)
		seedWarehouse(t, tx, code, code, "Distribution Center", StatusActive)
	}

	svc := NewService(NewRepository(tx), nil)

	zero, total, err := svc.List(context.Background(), ListParams{Search: "WHCLAMP-", Limit: 0})
	if err != nil {
		t.Fatalf("List(limit=0): %v", err)
	}
	if total != int64(defaultLimit+1) {
		t.Fatalf("total = %d, want %d", total, defaultLimit+1)
	}
	if len(zero) != defaultLimit {
		t.Errorf("List(limit=0) returned %d rows, want %d (clamped default)", len(zero), defaultLimit)
	}

	tooBig, _, err := svc.List(context.Background(), ListParams{Search: "WHCLAMP-", Limit: 1000})
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

	seedWarehouse(t, tx, "WHPKG-BOX", "Boxes Warehouse", "Distribution Center", StatusActive)
	seedWarehouse(t, tx, "WHPKG-WRAP", "Wraps Warehouse", "Distribution Center", StatusActive)
	seedWarehouse(t, tx, "WHRAW-STEEL", "Steel Warehouse", "Distribution Center", StatusActive)

	svc := NewService(NewRepository(tx), nil)
	responses, total, err := svc.List(context.Background(), ListParams{
		Search: "WHPKG", SortBy: "name", SortDir: "desc",
	})
	if err != nil {
		t.Fatalf("List(search+sort): %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (only WHPKG-* rows)", total)
	}
	if responses[0].Name != "Wraps Warehouse" || responses[1].Name != "Boxes Warehouse" {
		t.Errorf("order = [%s %s], want [Wraps Boxes] (name DESC within the WHPKG search)", responses[0].Name, responses[1].Name)
	}
}

package location

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/zone"
)

type fakeZones struct {
	found *zone.Zone
	err   error
}

func (f *fakeZones) FindByID(_ context.Context, _ int64) (*zone.Zone, error) {
	return f.found, f.err
}

const testTenantID int64 = 42

// Status/Type validation runs before any zone lookup or repository
// access, so it's testable against a zero-value Service.
func TestCreate_RejectsInvalidStatus(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 1, Code: "A-01-01-01", Status: "SUSPENDED",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsInvalidType(t *testing.T) {
	svc := &Service{}

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 1, Code: "A-01-01-01", Type: "NOT_REAL",
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsNegativeCapacity(t *testing.T) {
	svc := &Service{}
	negative := -1

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 1, Code: "A-01-01-01", Capacity: &negative,
	})

	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

// TestCreate_ZoneNotFound exercises the zone-existence check with a fake
// zoneChecker (an interface, so no real database is needed) — it fails
// before ever reaching the concrete repository.
func TestCreate_ZoneNotFound(t *testing.T) {
	svc := NewService(nil, &fakeZones{err: errors.New("not found")}, nil)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 999, Code: "A-01-01-01",
	})

	if !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("error = %v, want ErrZoneNotFound", err)
	}
}

// TestCreate_RejectsZoneBelongingToAnotherTenant proves
// validateZoneOwnership is an ownership check, not a bare existence
// check: a zone that exists but belongs to a different tenant must be
// rejected exactly like a nonexistent one (same error, same status —
// tenant-isolation-security.md §6).
func TestCreate_RejectsZoneBelongingToAnotherTenant(t *testing.T) {
	otherTenantZone := &zone.Zone{ID: 5, TenantID: testTenantID + 1}
	svc := NewService(nil, &fakeZones{found: otherTenantZone}, nil)

	_, err := svc.Create(context.Background(), testTenantID, CreateRequest{
		ZoneID: 5, Code: "A-01-01-01",
	})

	if !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("error = %v, want ErrZoneNotFound (cross-tenant zone must read as not-found)", err)
	}
}

func TestStatus_IsValid(t *testing.T) {
	cases := map[Status]bool{
		StatusActive:   true,
		StatusInactive: true,
		"SUSPENDED":    false,
		"":             false,
	}

	for status, want := range cases {
		if got := status.IsValid(); got != want {
			t.Errorf("Status(%q).IsValid() = %v, want %v", status, got, want)
		}
	}
}

func TestType_IsValid(t *testing.T) {
	cases := map[Type]bool{
		TypeReceiving: true, TypeStorage: true, TypePicking: true, TypePacking: true,
		TypeShipping: true, TypeQC: true, TypeHold: true,
		"":         true, // optional field — unset is valid
		"NOT_REAL": false,
	}

	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("Type(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

// --- Sort/pagination integration tests (Location Master Management —
// server-side sortable headers + pagination) ---
//
// These mirror internal/productcategory/sort_pagination_test.go's own
// shape exactly, adapted for location's own required zone_id scoping:
// every location.List call needs a zone, so newLocationSortTestTenant
// below creates a fresh, empty tenant + warehouse + zone on tx (rather
// than reusing the shared bootstrap tenant model_tenant_integration_test.go's
// tests rely on), so sort/pagination assertions can know the *exact*
// row set and count for the zone under test.

// newLocationSortTestTenant inserts a fresh tenant, warehouse, and zone
// on tx and returns (tenantID, zoneID) — the minimum hierarchy a
// location.List call needs. Raw SQL (not GORM Create via the
// warehouse/zone packages) to avoid importing internal/warehouse here
// purely for test setup, matching model_tenant_integration_test.go's
// own existing pattern in this same package.
func newLocationSortTestTenant(t *testing.T, tx *gorm.DB, label string) (tenantID, zoneID int64) {
	t.Helper()

	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("loc-sort-%s", label), fmt.Sprintf("Location Sort Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newLocationSortTestTenant(%s): insert tenant: %v", label, err)
	}

	// warehouses.code/zones.code are both VARCHAR(20) — "LSW-"/"LSZ-"
	// (4 chars) leaves room for the longest label used below
	// ("position-sortby", "invalid-sortdir", 15 chars) without
	// truncation, unlike the originally-longer "LOC-SORT-WH-"/
	// "LOC-SORT-Z-" prefixes this test file started with.
	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, fmt.Sprintf("LSW-%s", label), fmt.Sprintf("Location Sort Warehouse %s", label),
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newLocationSortTestTenant(%s): insert warehouse: %v", label, err)
	}

	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, fmt.Sprintf("LSZ-%s", label), fmt.Sprintf("Location Sort Zone %s", label),
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newLocationSortTestTenant(%s): insert zone: %v", label, err)
	}

	return tenantID, zoneID
}

func seedLocation(t *testing.T, tx *gorm.DB, tenantID, zoneID int64, code, name string, typ Type, capacity *int, status Status) Location {
	t.Helper()
	l := Location{
		TenantID: tenantID, ZoneID: zoneID, Code: code, Name: name,
		Type: typ, Capacity: capacity, Status: status,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("seedLocation(%s): %v", code, err)
	}
	return l
}

func TestService_List_DefaultsToCodeAscending(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, zoneID := newLocationSortTestTenant(t, tx, "default-sort")

	seedLocation(t, tx, tenantID, zoneID, "LOC-C", "Charlie", "", nil, StatusActive)
	seedLocation(t, tx, tenantID, zoneID, "LOC-A", "Alpha", "", nil, StatusActive)
	seedLocation(t, tx, tenantID, zoneID, "LOC-B", "Bravo", "", nil, StatusActive)

	svc := NewService(NewRepository(tx), nil, nil)
	responses, total, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	got := []string{responses[0].Code, responses[1].Code, responses[2].Code}
	want := []string{"LOC-A", "LOC-B", "LOC-C"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("default order = %v, want code ASC %v", got, want)
	}
}

func TestService_List_SortByColumn_AscAndDesc(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, zoneID := newLocationSortTestTenant(t, tx, "sort-column")

	seedLocation(t, tx, tenantID, zoneID, "LOC-1", "Charlie", "", nil, StatusActive)
	seedLocation(t, tx, tenantID, zoneID, "LOC-2", "Alpha", "", nil, StatusActive)
	seedLocation(t, tx, tenantID, zoneID, "LOC-3", "Bravo", "", nil, StatusActive)

	svc := NewService(NewRepository(tx), nil, nil)

	asc, _, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, SortBy: "name", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(name asc): %v", err)
	}
	if asc[0].Name != "Alpha" || asc[1].Name != "Bravo" || asc[2].Name != "Charlie" {
		t.Errorf("name ASC order = [%s %s %s], want [Alpha Bravo Charlie]", asc[0].Name, asc[1].Name, asc[2].Name)
	}

	desc, _, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, SortBy: "name", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(name desc): %v", err)
	}
	if desc[0].Name != "Charlie" || desc[1].Name != "Bravo" || desc[2].Name != "Alpha" {
		t.Errorf("name DESC order = [%s %s %s], want [Charlie Bravo Alpha]", desc[0].Name, desc[1].Name, desc[2].Name)
	}
}

// TestService_List_SortByCapacity_AscAndDesc exercises "capacity"
// specifically — a nullable numeric column, unlike productcategory's
// own sortable set, which this package's allowlist adds.
func TestService_List_SortByCapacity_AscAndDesc(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, zoneID := newLocationSortTestTenant(t, tx, "sort-capacity")

	ten, twenty, thirty := 10, 20, 30
	seedLocation(t, tx, tenantID, zoneID, "LOC-CAP-30", "", "", &thirty, StatusActive)
	seedLocation(t, tx, tenantID, zoneID, "LOC-CAP-10", "", "", &ten, StatusActive)
	seedLocation(t, tx, tenantID, zoneID, "LOC-CAP-20", "", "", &twenty, StatusActive)

	svc := NewService(NewRepository(tx), nil, nil)

	asc, _, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, SortBy: "capacity", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(capacity asc): %v", err)
	}
	if *asc[0].Capacity != 10 || *asc[1].Capacity != 20 || *asc[2].Capacity != 30 {
		t.Errorf("capacity ASC order = [%d %d %d], want [10 20 30]", *asc[0].Capacity, *asc[1].Capacity, *asc[2].Capacity)
	}

	desc, _, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, SortBy: "capacity", SortDir: "desc"})
	if err != nil {
		t.Fatalf("List(capacity desc): %v", err)
	}
	if *desc[0].Capacity != 30 || *desc[1].Capacity != 20 || *desc[2].Capacity != 10 {
		t.Errorf("capacity DESC order = [%d %d %d], want [30 20 10]", *desc[0].Capacity, *desc[1].Capacity, *desc[2].Capacity)
	}
}

func TestService_List_InvalidSortBy_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, zoneID := newLocationSortTestTenant(t, tx, "invalid-sortby")

	svc := NewService(NewRepository(tx), nil, nil)
	_, _, err := svc.List(context.Background(), tenantID, ListParams{
		ZoneID: zoneID, SortBy: "position; DROP TABLE locations",
	})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_by) = %v, want ErrInvalidSort", err)
	}
}

// TestService_List_InvalidSortBy_PositionRejected proves "position" —
// the aisle/rack/bin composite the Location Master table displays —
// is deliberately NOT in the sort allowlist, since it is not a single
// real column the database can ORDER BY.
func TestService_List_InvalidSortBy_PositionRejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, zoneID := newLocationSortTestTenant(t, tx, "position-sortby")

	svc := NewService(NewRepository(tx), nil, nil)
	_, _, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, SortBy: "position"})
	if err != ErrInvalidSort {
		t.Errorf("List(sort_by=position) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_InvalidSortDir_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, zoneID := newLocationSortTestTenant(t, tx, "invalid-sortdir")

	svc := NewService(NewRepository(tx), nil, nil)
	_, _, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, SortBy: "code", SortDir: "sideways"})
	if err != ErrInvalidSort {
		t.Errorf("List(bad sort_dir) = %v, want ErrInvalidSort", err)
	}
}

func TestService_List_Pagination_PagesThroughCodeOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID, zoneID := newLocationSortTestTenant(t, tx, "pagination")

	codes := []string{"LOC-A", "LOC-B", "LOC-C", "LOC-D", "LOC-E"}
	for _, code := range codes {
		seedLocation(t, tx, tenantID, zoneID, code, code, "", nil, StatusActive)
	}

	svc := NewService(NewRepository(tx), nil, nil)

	page1, total, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 1): %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(page1) != 2 || page1[0].Code != "LOC-A" || page1[1].Code != "LOC-B" {
		t.Fatalf("page 1 = %+v, want [LOC-A LOC-B]", page1)
	}

	page3, _, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, Page: 3, Limit: 2})
	if err != nil {
		t.Fatalf("List(page 3): %v", err)
	}
	if len(page3) != 1 || page3[0].Code != "LOC-E" {
		t.Fatalf("page 3 (final, partial) = %+v, want [LOC-E]", page3)
	}
}

// TestService_List_CrossTenantIsolation_IgnoresSortParams proves that a
// location belonging to another tenant's zone is never returned by
// List, regardless of which sort_by/sort_dir is requested — sorting
// must never become a way to leak rows past the tenant_id scoping
// repository.go's own doc describes.
func TestService_List_CrossTenantIsolation_IgnoresSortParams(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	tenantID, zoneID := newLocationSortTestTenant(t, tx, "isolation-a")
	otherTenantID, otherZoneID := newLocationSortTestTenant(t, tx, "isolation-b")

	seedLocation(t, tx, tenantID, zoneID, "LOC-MINE", "Mine", "", nil, StatusActive)
	otherLocation := seedLocation(t, tx, otherTenantID, otherZoneID, "LOC-THEIRS", "Theirs", "", nil, StatusActive)

	svc := NewService(NewRepository(tx), nil, nil)

	for _, tc := range []struct {
		sortBy, sortDir string
	}{
		{"", ""},
		{"code", "asc"},
		{"name", "desc"},
		{"status", "asc"},
	} {
		// Even scoped to the other tenant's own zone, tenantID itself
		// must still gate the result — a guessed/known otherZoneID must
		// not leak the other tenant's location into this tenant's list.
		responses, total, err := svc.List(context.Background(), tenantID, ListParams{
			ZoneID: otherZoneID, SortBy: tc.sortBy, SortDir: tc.sortDir,
		})
		if err != nil {
			t.Fatalf("List(sort_by=%q sort_dir=%q): %v", tc.sortBy, tc.sortDir, err)
		}
		if total != 0 {
			t.Fatalf("List(sort_by=%q sort_dir=%q) against another tenant's zone: total = %d, want 0", tc.sortBy, tc.sortDir, total)
		}
		for _, r := range responses {
			if r.ID == otherLocation.ID {
				t.Fatalf("List(sort_by=%q sort_dir=%q) leaked another tenant's location %d", tc.sortBy, tc.sortDir, r.ID)
			}
		}
	}

	// Sanity: tenantID's own zone/location is still returned normally.
	mine, total, err := svc.List(context.Background(), tenantID, ListParams{ZoneID: zoneID, SortBy: "code", SortDir: "asc"})
	if err != nil {
		t.Fatalf("List(own zone): %v", err)
	}
	if total != 1 || len(mine) != 1 || mine[0].Code != "LOC-MINE" {
		t.Fatalf("List(own zone) = %+v (total %d), want exactly LOC-MINE", mine, total)
	}
}

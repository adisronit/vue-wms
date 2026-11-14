package customerdeliverylocation

import (
	"context"
	"errors"
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

func TestType_IsValid(t *testing.T) {
	cases := map[Type]bool{
		TypeBranch: true, TypeDistributionCenter: true, TypeDeliveryAddress: true,
		"": true, "NOT_REAL": false,
	}
	for typ, want := range cases {
		if got := typ.IsValid(); got != want {
			t.Errorf("Type(%q).IsValid() = %v, want %v", typ, got, want)
		}
	}
}

// customerExistsStub is a customerChecker test double — the
// integration tests below seed a real tenant/customer row themselves
// and just need Exists to report true for that row, false otherwise,
// without pulling in internal/customer as a dependency.
type customerExistsStub struct {
	tenantID, customerID int64
}

func (s customerExistsStub) Exists(ctx context.Context, tenantID, id int64) (bool, error) {
	return tenantID == s.tenantID && id == s.customerID, nil
}

func newTestTenant(t *testing.T, tx *gorm.DB, label string) int64 {
	t.Helper()
	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		fmt.Sprintf("cdl-%s", label), fmt.Sprintf("Customer Delivery Location Test Tenant %s", label),
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newTestTenant(%s): insert tenant: %v", label, err)
	}
	return tenantID
}

func newTestCustomer(t *testing.T, tx *gorm.DB, tenantID int64, code string) int64 {
	t.Helper()
	var customerID int64
	if err := tx.Raw(
		"INSERT INTO customers (tenant_id, code, company_name, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'ACTIVE')",
		tenantID, code, code+" Co.",
	).Scan(&customerID).Error; err != nil {
		t.Fatalf("newTestCustomer(%s): %v", code, err)
	}
	return customerID
}

func TestService_Create_FirstLocationForcedDefault(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "first-default")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C1")

	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	resp, err := svc.Create(context.Background(), tenantID, CreateRequest{
		CustomerID: customerID, Code: "LOC-1", Name: "First Location", IsDefault: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !resp.IsDefault {
		t.Errorf("IsDefault = false, want true — a customer's first location must always be forced default")
	}
}

func TestService_Create_SecondLocationNotDefaultUnlessRequested(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "second-not-default")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C2")

	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	first, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-1", Name: "First"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-2", Name: "Second", IsDefault: false})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if !first.IsDefault {
		t.Errorf("first.IsDefault = false, want true")
	}
	if second.IsDefault {
		t.Errorf("second.IsDefault = true, want false — not explicitly requested")
	}
}

func TestService_Create_PromotingNewDefaultClearsPrior(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "promote-clears-prior")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C3")

	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	first, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-1", Name: "First"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-2", Name: "Second", IsDefault: true})
	if err != nil {
		t.Fatalf("Create second (explicit default): %v", err)
	}
	if !second.IsDefault {
		t.Fatalf("second.IsDefault = false, want true")
	}

	refreshedFirst, err := svc.Get(ctx, tenantID, first.ID)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	if refreshedFirst.IsDefault {
		t.Errorf("first location is still default after a second was explicitly promoted — exactly-one-default invariant violated")
	}
}

func TestService_Create_RejectsInvalidType(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "invalid-type")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C4")

	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	_, err := svc.Create(context.Background(), tenantID, CreateRequest{
		CustomerID: customerID, Code: "LOC-1", Name: "Bad Type", Type: "NOT_REAL",
	})
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(bad type) = %v, want ErrValidation", err)
	}
}

func TestService_Create_DuplicateCodePerCustomer_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "dup-code")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C5")

	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	if _, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-DUP", Name: "A"}); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	_, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-DUP", Name: "B"})
	if !errors.Is(err, ErrDuplicateCode) {
		t.Fatalf("Create(dup code) = %v, want ErrDuplicateCode", err)
	}
}

// TestService_Create_SameCodeDifferentCustomer_Allowed proves ship-to
// codes are scoped to their own customer, not the whole tenant — two
// different customers may both use "LOC-1" for their own head office.
func TestService_Create_SameCodeDifferentCustomer_Allowed(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "same-code-diff-customer")
	customerA := newTestCustomer(t, tx, tenantID, "CDL-C6A")
	customerB := newTestCustomer(t, tx, tenantID, "CDL-C6B")
	ctx := context.Background()

	svcA := NewService(NewRepository(tx), customerExistsStub{tenantID, customerA})
	if _, err := svcA.Create(ctx, tenantID, CreateRequest{CustomerID: customerA, Code: "LOC-1", Name: "A HQ"}); err != nil {
		t.Fatalf("Create for customer A: %v", err)
	}

	svcB := NewService(NewRepository(tx), customerExistsStub{tenantID, customerB})
	if _, err := svcB.Create(ctx, tenantID, CreateRequest{CustomerID: customerB, Code: "LOC-1", Name: "B HQ"}); err != nil {
		t.Fatalf("Create for customer B with the same code: %v", err)
	}
}

func TestService_Create_CrossTenantCustomer_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantA := newTestTenant(t, tx, "cross-tenant-a")
	tenantB := newTestTenant(t, tx, "cross-tenant-b")
	customerUnderB := newTestCustomer(t, tx, tenantB, "CDL-C7")

	// svc only ever reports customerUnderB as existing when asked about
	// tenantB — asking as tenantA must be refused.
	svc := NewService(NewRepository(tx), customerExistsStub{tenantB, customerUnderB})
	_, err := svc.Create(context.Background(), tenantA, CreateRequest{
		CustomerID: customerUnderB, Code: "LOC-1", Name: "Should Not Exist",
	})
	if !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("Create(cross-tenant customer_id) = %v, want ErrCustomerNotFound", err)
	}
}

func TestService_Update_RejectsRemovingDefaultWhileOthersActive(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "reject-remove-default")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C8")
	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	first, _ := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-1", Name: "First"})
	if _, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-2", Name: "Second"}); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	_, err := svc.Update(ctx, tenantID, first.ID, UpdateRequest{
		Code: "LOC-1", Name: "First", IsDefault: false, Status: StatusActive,
	})
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Update(un-default the sole default while another ACTIVE location exists) = %v, want ErrValidation", err)
	}
}

func TestService_Update_AllowsRemovingDefaultWhenNoOtherActiveExists(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "allow-remove-default-alone")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C9")
	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	first, _ := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-1", Name: "Only One"})

	resp, err := svc.Update(ctx, tenantID, first.ID, UpdateRequest{
		Code: "LOC-1", Name: "Only One", IsDefault: false, Status: StatusActive,
	})
	if err != nil {
		t.Fatalf("Update(un-default the sole location, no other actives) = %v, want success", err)
	}
	if resp.IsDefault {
		t.Errorf("IsDefault = true, want false")
	}
}

func TestService_Update_RejectsDeactivatingDefaultWhileOthersActive(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "reject-deactivate-default")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C10")
	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	first, _ := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-1", Name: "First"})
	if _, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-2", Name: "Second"}); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	_, err := svc.Update(ctx, tenantID, first.ID, UpdateRequest{
		Code: "LOC-1", Name: "First", IsDefault: true, Status: StatusInactive,
	})
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Update(deactivate the default while another ACTIVE location exists) = %v, want ErrValidation", err)
	}
}

func TestService_Delete_RejectsDeletingDefaultWhileOthersActive(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "reject-delete-default")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C11")
	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	first, _ := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-1", Name: "First"})
	if _, err := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-2", Name: "Second"}); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	err := svc.Delete(ctx, tenantID, first.ID)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Delete(the default while another ACTIVE location exists) = %v, want ErrValidation", err)
	}
}

func TestService_Delete_AllowsDeletingSoleLocation(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "allow-delete-sole")
	customerID := newTestCustomer(t, tx, tenantID, "CDL-C12")
	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, customerID})
	ctx := context.Background()

	first, _ := svc.Create(ctx, tenantID, CreateRequest{CustomerID: customerID, Code: "LOC-1", Name: "Only One"})

	if err := svc.Delete(ctx, tenantID, first.ID); err != nil {
		t.Fatalf("Delete(sole location): %v", err)
	}
	if _, err := svc.Get(ctx, tenantID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestService_List_ScopedToCustomerAndTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "list-scoped")
	customerA := newTestCustomer(t, tx, tenantID, "CDL-C13A")
	customerB := newTestCustomer(t, tx, tenantID, "CDL-C13B")
	ctx := context.Background()

	svcA := NewService(NewRepository(tx), customerExistsStub{tenantID, customerA})
	svcA.Create(ctx, tenantID, CreateRequest{CustomerID: customerA, Code: "LOC-A1", Name: "A1"})
	svcA.Create(ctx, tenantID, CreateRequest{CustomerID: customerA, Code: "LOC-A2", Name: "A2"})

	svcB := NewService(NewRepository(tx), customerExistsStub{tenantID, customerB})
	svcB.Create(ctx, tenantID, CreateRequest{CustomerID: customerB, Code: "LOC-B1", Name: "B1"})

	listForA, err := svcA.List(ctx, tenantID, ListParams{CustomerID: customerA})
	if err != nil {
		t.Fatalf("List(customer A): %v", err)
	}
	if len(listForA) != 2 {
		t.Fatalf("List(customer A) returned %d rows, want 2 (must not include customer B's locations)", len(listForA))
	}
}

func TestService_List_UnknownCustomer_Rejected(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)
	tenantID := newTestTenant(t, tx, "list-unknown-customer")

	svc := NewService(NewRepository(tx), customerExistsStub{tenantID, 999999999})
	_, err := svc.List(context.Background(), tenantID, ListParams{CustomerID: 1})
	if !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("List(unknown customer_id) = %v, want ErrCustomerNotFound", err)
	}
}

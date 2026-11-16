// Customer Ship-to -> Order/Shipping Integration. Mirrors
// tenant_isolation_test.go's own shape (fresh tenant per test, real
// seeded rows, testsupport.ConnectDB + BeginRollbackTx) but covers the
// surface unique to this feature: cross-tenant/cross-customer/inactive
// ship-to rejection, default-ship-to auto-selection with explicit
// override, snapshot immutability against a later master-data edit, and
// propagation of the snapshot all the way through to Shipping via
// picking_tasks' own denormalized columns.
package order

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/customer"
	"wms-backend/internal/customerdeliverylocation"
	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
)

// seedCustomer inserts one ACTIVE customer for tenantID.
func seedCustomer(t *testing.T, tx *gorm.DB, tenantID int64, label string) int64 {
	t.Helper()
	c := customer.Customer{
		TenantID: tenantID, Code: "CUST-" + label, CompanyName: "Customer " + label,
		Status: customer.StatusActive,
	}
	if err := tx.Create(&c).Error; err != nil {
		t.Fatalf("seedCustomer(%s): %v", label, err)
	}
	return c.ID
}

// seedShipTo inserts one delivery location for customerID — isDefault
// and status are caller-chosen so tests can exercise both the "has a
// default" and "has no default" cases, and the inactive-rejection case.
func seedShipTo(t *testing.T, tx *gorm.DB, tenantID, customerID int64, label string, isDefault bool, status customerdeliverylocation.Status) int64 {
	t.Helper()
	l := customerdeliverylocation.Location{
		TenantID: tenantID, CustomerID: customerID, Code: "SHIPTO-" + label, Name: "Ship To " + label,
		Address: "123 " + label + " Road", Province: "Bangkok", District: "Bang Na", PostalCode: "10260",
		ContactPerson: "Somchai " + label, ContactPhone: "+66-2-000-0000",
		IsDefault: isDefault, Status: status,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("seedShipTo(%s): %v", label, err)
	}
	return l.ID
}

// --- Create: customer/ship-to resolution ---

func TestCreate_AutoSelectsDefaultShipTo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "autoship")
	custID := seedCustomer(t, tx, a.tenantID, "autoship")
	shipToID := seedShipTo(t, tx, a.tenantID, custID, "autoship-default", true, customerdeliverylocation.StatusActive)

	svc, _ := newOrderServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create(customer, no explicit ship_to): %v", err)
	}
	if resp.ShipToID == nil || *resp.ShipToID != shipToID {
		t.Fatalf("ShipToID = %v, want auto-selected default %d", resp.ShipToID, shipToID)
	}
	if resp.ShipToAddress == nil || *resp.ShipToAddress != "123 autoship-default Road" {
		t.Errorf("ShipToAddress = %v, want the default location's own address", resp.ShipToAddress)
	}
	if resp.Customer == nil || *resp.Customer != "Customer autoship" {
		t.Errorf("Customer = %v, want resolved customer display name", resp.Customer)
	}
}

func TestCreate_ExplicitShipToOverridesDefault(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "override")
	custID := seedCustomer(t, tx, a.tenantID, "override")
	seedShipTo(t, tx, a.tenantID, custID, "override-default", true, customerdeliverylocation.StatusActive)
	altID := seedShipTo(t, tx, a.tenantID, custID, "override-alt", false, customerdeliverylocation.StatusActive)

	svc, _ := newOrderServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custID, ShipToID: &altID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create(explicit non-default ship_to): %v", err)
	}
	if resp.ShipToID == nil || *resp.ShipToID != altID {
		t.Fatalf("ShipToID = %v, want the explicitly chosen %d, not the default", resp.ShipToID, altID)
	}
}

func TestCreate_NoCustomer_PreservesFreeTextOrder(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "freetext")
	svc, _ := newOrderServiceAndRepo(tx)

	freeText := "Walk-in Customer"
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, Customer: &freeText,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create(no customer_id, free-text only): %v", err)
	}
	if resp.Customer == nil || *resp.Customer != freeText {
		t.Errorf("Customer = %v, want unchanged free text %q", resp.Customer, freeText)
	}
	if resp.CustomerID != nil || resp.ShipToID != nil {
		t.Errorf("CustomerID/ShipToID = %v/%v, want both nil (no customer link requested)", resp.CustomerID, resp.ShipToID)
	}
}

// --- Rejection cases: cross-tenant, cross-customer, inactive, invalid ---

func TestCreate_RejectsCrossTenantCustomer(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "xtenant-cust-a")
	b := newOrderFixture(t, tx, "xtenant-cust-b")
	otherCustID := seedCustomer(t, tx, b.tenantID, "xtenant-cust-b")

	svc, _ := newOrderServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &otherCustID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("Create(tenantA, tenantB's customer_id) = %v, want ErrCustomerNotFound", err)
	}
}

func TestCreate_RejectsCrossTenantShipTo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "xtenant-ship-a")
	b := newOrderFixture(t, tx, "xtenant-ship-b")
	custA := seedCustomer(t, tx, a.tenantID, "xtenant-ship-a")
	custB := seedCustomer(t, tx, b.tenantID, "xtenant-ship-b")
	shipToB := seedShipTo(t, tx, b.tenantID, custB, "xtenant-ship-b", true, customerdeliverylocation.StatusActive)

	svc, _ := newOrderServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custA, ShipToID: &shipToB,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrShipToNotFound) {
		t.Fatalf("Create(tenantA's own customer, tenantB's ship_to_id) = %v, want ErrShipToNotFound", err)
	}
}

func TestCreate_RejectsCrossCustomerShipTo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "xcust-ship")
	custOne := seedCustomer(t, tx, a.tenantID, "xcust-one")
	custTwo := seedCustomer(t, tx, a.tenantID, "xcust-two")
	shipToOfCustTwo := seedShipTo(t, tx, a.tenantID, custTwo, "xcust-two", true, customerdeliverylocation.StatusActive)

	svc, _ := newOrderServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custOne, ShipToID: &shipToOfCustTwo,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if !errors.Is(err, ErrShipToNotFound) {
		t.Fatalf("Create(customerOne, customerTwo's own ship_to_id) = %v, want ErrShipToNotFound (same tenant, wrong customer)", err)
	}
}

func TestCreate_RejectsInactiveShipTo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "inactive-ship")
	custID := seedCustomer(t, tx, a.tenantID, "inactive-ship")
	inactiveShipToID := seedShipTo(t, tx, a.tenantID, custID, "inactive", false, customerdeliverylocation.StatusInactive)

	svc, _ := newOrderServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custID, ShipToID: &inactiveShipToID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(inactive ship_to) = %v, want ErrValidation", err)
	}
}

func TestCreate_RejectsShipToWithoutCustomer(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "noship-nocust")
	custID := seedCustomer(t, tx, a.tenantID, "noship-nocust")
	shipToID := seedShipTo(t, tx, a.tenantID, custID, "noship-nocust", true, customerdeliverylocation.StatusActive)

	svc, _ := newOrderServiceAndRepo(tx)
	_, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, ShipToID: &shipToID, // CustomerID deliberately omitted
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	var validationErr ErrValidation
	if !errors.As(err, &validationErr) {
		t.Fatalf("Create(ship_to_id without customer_id) = %v, want ErrValidation", err)
	}
}

// --- Snapshot immutability ---

// TestSnapshot_SurvivesLaterShipToEdit proves the order's own ship_to_*
// columns are a one-time copy, never re-synced: editing the source
// CustomerDeliveryLocation's address after the order was created must
// not change what the already-placed order reports.
func TestSnapshot_SurvivesLaterShipToEdit(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "snapshot")
	custID := seedCustomer(t, tx, a.tenantID, "snapshot")
	shipToID := seedShipTo(t, tx, a.tenantID, custID, "snapshot-original", true, customerdeliverylocation.StatusActive)

	svc, _ := newOrderServiceAndRepo(tx)
	resp, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.ShipToAddress == nil || *resp.ShipToAddress != "123 snapshot-original Road" {
		t.Fatalf("initial ShipToAddress = %v, want the original address", resp.ShipToAddress)
	}

	// Edit the master data's address directly — the order must not see
	// this change.
	if err := tx.Model(&customerdeliverylocation.Location{}).Where("id = ?", shipToID).
		Update("address", "999 Changed Address").Error; err != nil {
		t.Fatalf("edit ship-to address: %v", err)
	}

	reloaded, err := svc.Get(context.Background(), a.tenantID, resp.ID)
	if err != nil {
		t.Fatalf("Get after later ship-to edit: %v", err)
	}
	if reloaded.ShipToAddress == nil || *reloaded.ShipToAddress != "123 snapshot-original Road" {
		t.Errorf("ShipToAddress after later master-data edit = %v, want unchanged original snapshot", reloaded.ShipToAddress)
	}
}

// TestUpdate_RefreshesSnapshotOnDeliberateEdit proves the OTHER half of
// immutability: a deliberate Update to the order itself (not an edit to
// the master data) DOES refresh the snapshot, since Update's own
// full-replace semantics re-resolve customer_id/ship_to_id exactly like
// Create does.
func TestUpdate_RefreshesSnapshotOnDeliberateEdit(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "refresh")
	custID := seedCustomer(t, tx, a.tenantID, "refresh")
	shipToOld := seedShipTo(t, tx, a.tenantID, custID, "refresh-old", true, customerdeliverylocation.StatusActive)
	shipToNew := seedShipTo(t, tx, a.tenantID, custID, "refresh-new", false, customerdeliverylocation.StatusActive)

	svc, _ := newOrderServiceAndRepo(tx)
	created, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ShipToID == nil || *created.ShipToID != shipToOld {
		t.Fatalf("initial ShipToID = %v, want auto-selected default %d", created.ShipToID, shipToOld)
	}

	updated, err := svc.Update(context.Background(), a.tenantID, created.ID, UpdateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custID, ShipToID: &shipToNew,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 5, UOM: "PCS"}},
	})
	if err != nil {
		t.Fatalf("Update(switch ship-to): %v", err)
	}
	if updated.ShipToID == nil || *updated.ShipToID != shipToNew {
		t.Errorf("ShipToID after Update = %v, want the newly chosen %d", updated.ShipToID, shipToNew)
	}
	if updated.ShipToAddress == nil || *updated.ShipToAddress != "123 refresh-new Road" {
		t.Errorf("ShipToAddress after Update = %v, want the new location's own address", updated.ShipToAddress)
	}
}

// --- Propagation: Order -> Picking -> Shipping ---

// TestCreatePickingTask_PropagatesShipToSnapshot proves the snapshot
// reaches Shipping's own detail view purely through picking_tasks'
// denormalized columns — the same existing pipeline order_no/customer
// already use, extended here for ship-to.
func TestCreatePickingTask_PropagatesShipToSnapshot(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newOrderFixture(t, tx, "propagate")
	custID := seedCustomer(t, tx, a.tenantID, "propagate")
	shipToID := seedShipTo(t, tx, a.tenantID, custID, "propagate", true, customerdeliverylocation.StatusActive)
	seedOrderBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newOrderServiceAndRepo(tx)
	order, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		WarehouseID: a.warehouseID, CustomerID: &custID,
		Items: []CreateItemRequest{{ProductID: a.productID, OrderedQty: 10, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	released, err := svc.CreatePickingTask(context.Background(), a.tenantID, order.ID, CreatePickingTaskRequest{
		Items: []LocationAssignment{{OrderItemID: order.Items[0].ID, LocationID: a.locationID, LotNo: "LOT-A"}},
	}, 1)
	if err != nil {
		t.Fatalf("CreatePickingTask: %v", err)
	}

	// Picking's own Get must show the same snapshot.
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)
	pickingRepo := picking.NewRepository(tx)
	pickingSvc := picking.NewService(pickingRepo, productRepo, warehouseRepo, invSvc)
	pickingResp, err := pickingSvc.Get(context.Background(), a.tenantID, *released.PickingTaskID)
	if err != nil {
		t.Fatalf("picking Get: %v", err)
	}
	if pickingResp.ShipToID == nil || *pickingResp.ShipToID != shipToID {
		t.Errorf("picking task ShipToID = %v, want %d (copied from the order)", pickingResp.ShipToID, shipToID)
	}
	if pickingResp.ShipToAddress == nil || *pickingResp.ShipToAddress != "123 propagate Road" {
		t.Errorf("picking task ShipToAddress = %v, want the order's own snapshot", pickingResp.ShipToAddress)
	}

	// Shipping's own Get, through the normal Create flow, must show the
	// identical snapshot — read purely via the picking_tasks JOIN,
	// confirming the pipeline Order -> Picking -> Shipping end to end.
	confirmed, err := pickingSvc.Confirm(context.Background(), a.tenantID, *released.PickingTaskID, picking.ConfirmRequest{
		IdempotencyKey: "propagate-pick-confirm",
		Items:          []picking.ConfirmItemRequest{{ItemID: pickingResp.Items[0].ID, Action: "PICKED", Quantity: 10}},
	}, 1)
	if err != nil {
		t.Fatalf("picking Confirm: %v", err)
	}
	if confirmed.Status != picking.StatusCompleted {
		t.Fatalf("picking task status = %v, want COMPLETED", confirmed.Status)
	}

	shippingRepo := shipping.NewRepository(tx)
	shippingSvc := shipping.NewService(shippingRepo, pickingSvc)
	shipmentResp, err := shippingSvc.Create(context.Background(), a.tenantID, shipping.CreateRequest{
		PickingTaskID: *released.PickingTaskID,
	}, 1)
	if err != nil {
		t.Fatalf("shipping Create: %v", err)
	}
	if shipmentResp.ShipToID == nil || *shipmentResp.ShipToID != shipToID {
		t.Errorf("shipment ShipToID = %v, want %d (propagated all the way from the order)", shipmentResp.ShipToID, shipToID)
	}
	if shipmentResp.ShipToAddress == nil || *shipmentResp.ShipToAddress != "123 propagate Road" {
		t.Errorf("shipment ShipToAddress = %v, want the order's own original snapshot", shipmentResp.ShipToAddress)
	}
	if shipmentResp.Customer == nil || *shipmentResp.Customer != "Customer propagate" {
		t.Errorf("shipment Customer = %v, want the resolved customer name, propagated via picking_tasks.customer", shipmentResp.Customer)
	}
}

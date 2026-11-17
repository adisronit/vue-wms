package deliveryreport

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/loading"
	"wms-backend/internal/location"
	"wms-backend/internal/packing"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/warehouse"
)

type fixture struct {
	tenantID      int64
	productID     int64
	warehouseID   int64
	warehouseName string
	locationID    int64
}

// newFixture mirrors internal/pickingreport's own newFixture — a fresh
// tenant/product/warehouse/zone/location per test.
func newFixture(t *testing.T, tx *gorm.DB, label string) fixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"dlv-"+label, "Delivery Report Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newFixture(%s): seed product: %v", label, err)
	}

	warehouseName := "Warehouse " + label
	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, warehouseName,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newFixture(%s): seed location: %v", label, err)
	}

	return fixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, warehouseName: warehouseName, locationID: l.ID}
}

func seedBalance(t *testing.T, tx *gorm.DB, f fixture, lotNo string, quantity float64) {
	t.Helper()
	inv := inventory.Inventory{TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo, Quantity: quantity}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedBalance: %v", err)
	}
}

type chainServices struct {
	picking  *picking.Service
	packing  *packing.Service
	shipping *shipping.Service
	loading  *loading.Service
	repo     *Repository
}

func newChainServices(tx *gorm.DB) chainServices {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	pickingSvc := picking.NewService(picking.NewRepository(tx), productRepo, warehouseRepo, invSvc)
	packingSvc := packing.NewService(packing.NewRepository(tx), pickingSvc)
	shippingSvc := shipping.NewService(shipping.NewRepository(tx), packingSvc)
	loadingSvc := loading.NewService(loading.NewRepository(tx), shippingSvc, packingSvc)

	return chainServices{
		picking: pickingSvc, packing: packingSvc, shipping: shippingSvc, loading: loadingSvc,
		repo: NewRepository(tx),
	}
}

// runChain drives one real order-equivalent all the way from an
// ad-hoc picking task (the same direct picking.Service.Create path
// internal/pickingreport's own tests use — bypassing internal/order
// entirely, which is fine here since deliveryreport's own join against
// orders is a LEFT JOIN that must degrade gracefully to its
// COALESCE(...,'SALES_ORDER') fallback when no orders row exists, and
// this IS that exact case) through Picking -> Packing -> Loading
// (trip created, shipment assigned, every carton scanned loaded,
// trip confirmed COMPLETED) -> Shipping Create. finalAction is one of
// "" (leave at READY — loading already completed, LoadedAt is set,
// but DISPATCH is never called), "DISPATCH", "DELIVER", or "ISSUE"
// (the latter two always dispatch first). Returns the shipment ID and
// the loading trip ID.
func runChain(t *testing.T, tx *gorm.DB, svcs chainServices, f fixture, lotNo string, qty float64, finalAction string) (shipmentID, tripID int64) {
	t.Helper()
	ctx := context.Background()

	seedBalance(t, tx, f, lotNo, qty)

	taskResp, err := svcs.picking.Create(ctx, f.tenantID, picking.CreateRequest{
		OrderNo: "SO-" + lotNo, WarehouseID: f.warehouseID,
		Items: []picking.CreateItemRequest{{ProductID: f.productID, LotNo: lotNo, LocationID: f.locationID, OrderedQty: qty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("runChain(%s): picking.Create: %v", lotNo, err)
	}
	taskID, itemID := taskResp.ID, taskResp.Items[0].ID

	if _, err := svcs.picking.Confirm(ctx, f.tenantID, taskID, picking.ConfirmRequest{
		IdempotencyKey: "runchain-pick-" + lotNo,
		Items:          []picking.ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: qty}},
	}, 1); err != nil {
		t.Fatalf("runChain(%s): picking.Confirm: %v", lotNo, err)
	}

	job, err := svcs.packing.Create(ctx, f.tenantID, packing.CreateRequest{PickingTaskID: taskID}, 1)
	if err != nil {
		t.Fatalf("runChain(%s): packing.Create: %v", lotNo, err)
	}
	carton, err := svcs.packing.CreateCarton(ctx, f.tenantID, job.ID, packing.CreateCartonRequest{}, 1)
	if err != nil {
		t.Fatalf("runChain(%s): packing.CreateCarton: %v", lotNo, err)
	}
	if _, err := svcs.packing.PackItem(ctx, f.tenantID, job.ID, packing.PackItemRequest{
		IdempotencyKey: "runchain-pack-" + lotNo, ItemID: job.Items[0].ID, CartonID: carton.ID,
		ScannedSKU: job.Items[0].SKU, Quantity: job.Items[0].PickedQty,
	}, 1); err != nil {
		t.Fatalf("runChain(%s): packing.PackItem: %v", lotNo, err)
	}
	if _, err := svcs.packing.CheckerConfirm(ctx, f.tenantID, job.ID, packing.CheckerConfirmRequest{}, 1); err != nil {
		t.Fatalf("runChain(%s): packing.CheckerConfirm: %v", lotNo, err)
	}

	carrier, tracking := "DHL", "TRACK-"+lotNo
	shipResp, err := svcs.shipping.Create(ctx, f.tenantID, shipping.CreateRequest{
		PackingJobID: job.ID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("runChain(%s): shipping.Create: %v", lotNo, err)
	}

	trip, err := svcs.loading.CreateTrip(ctx, f.tenantID, loading.CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("runChain(%s): loading.CreateTrip: %v", lotNo, err)
	}
	assigned, err := svcs.loading.AssignShipment(ctx, f.tenantID, trip.ID, loading.AssignShipmentRequest{ShipmentID: shipResp.ID}, 1)
	if err != nil {
		t.Fatalf("runChain(%s): loading.AssignShipment: %v", lotNo, err)
	}
	for _, c := range assigned.Cartons {
		if _, err := svcs.loading.ScanCarton(ctx, f.tenantID, trip.ID, loading.ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("runChain(%s): loading.ScanCarton: %v", lotNo, err)
		}
	}
	if _, err := svcs.loading.ConfirmTrip(ctx, f.tenantID, trip.ID, loading.ConfirmRequest{}, 1); err != nil {
		t.Fatalf("runChain(%s): loading.ConfirmTrip: %v", lotNo, err)
	}

	if finalAction == "" {
		return shipResp.ID, trip.ID
	}

	if _, err := svcs.shipping.Confirm(ctx, f.tenantID, shipResp.ID, shipping.ConfirmRequest{
		IdempotencyKey: "runchain-dispatch-" + lotNo, Action: "DISPATCH",
	}, 1); err != nil {
		t.Fatalf("runChain(%s): shipping.Confirm DISPATCH: %v", lotNo, err)
	}
	if finalAction == "DISPATCH" {
		return shipResp.ID, trip.ID
	}

	remark := ""
	if finalAction == "ISSUE" {
		remark = "Package damaged in transit"
	}
	if _, err := svcs.shipping.Confirm(ctx, f.tenantID, shipResp.ID, shipping.ConfirmRequest{
		IdempotencyKey: "runchain-" + finalAction + "-" + lotNo, Action: finalAction, Remark: remark,
	}, 1); err != nil {
		t.Fatalf("runChain(%s): shipping.Confirm %s: %v", lotNo, finalAction, err)
	}
	return shipResp.ID, trip.ID
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "list-a")
	b := newFixture(t, tx, "list-b")
	svcs := newChainServices(tx)

	shipmentA, _ := runChain(t, tx, svcs, a, "LOT-A", 10, "DISPATCH")
	runChain(t, tx, svcs, b, "LOT-B", 10, "DISPATCH")

	rows, total, err := svcs.repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's shipment)", len(rows), total)
	}
	if rows[0].ShipmentID != shipmentA {
		t.Errorf("ShipmentID = %d, want %d", rows[0].ShipmentID, shipmentA)
	}
}

// TestRepository_List_WarehouseAndCarrierFields_ViaPackingJobJoin
// proves the LEFT JOIN chain (shipments -> packing_jobs -> warehouses,
// and shipments -> packing_jobs -> orders) resolves real field values
// and degrades gracefully to COALESCE(...,'SALES_ORDER') when — as
// here, an ad-hoc picking task with no internal/order row at all —
// no matching orders row exists.
func TestRepository_List_WarehouseAndCarrierFields_ViaPackingJobJoin(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "fields")
	svcs := newChainServices(tx)
	runChain(t, tx, svcs, f, "LOT-F", 7, "DISPATCH")

	rows, _, err := svcs.repo.List(context.Background(), f.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.WarehouseName != f.warehouseName {
		t.Errorf("WarehouseName = %q, want %q (via packing_jobs.warehouse_id)", row.WarehouseName, f.warehouseName)
	}
	if row.OrderType != "SALES_ORDER" {
		t.Errorf("OrderType = %q, want SALES_ORDER (no orders row exists, must fall back to the default, not an empty string)", row.OrderType)
	}
	if row.Carrier == nil || *row.Carrier != "DHL" {
		t.Errorf("Carrier = %v, want DHL", row.Carrier)
	}
	if row.SKU != "SKU-fields" {
		t.Errorf("SKU = %q, want SKU-fields", row.SKU)
	}
	if row.Qty != 7 {
		t.Errorf("Qty = %v, want 7", row.Qty)
	}
	if row.Status != "DISPATCHED" {
		t.Errorf("Status = %q, want DISPATCHED", row.Status)
	}
}

// TestRepository_List_DeliveryLeadTime_NilUntilDelivered proves
// "Dispatched is NOT Delivered": DeliveredAt/DeliveryLeadTimeHours
// must stay nil for a merely-DISPATCHED shipment, and only become
// non-nil after a real DELIVER confirm.
func TestRepository_List_DeliveryLeadTime_NilUntilDelivered(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "leadtime")
	svcs := newChainServices(tx)
	shipmentID, _ := runChain(t, tx, svcs, f, "LOT-LT", 5, "DISPATCH")

	rowsDispatched, _, err := svcs.repo.List(context.Background(), f.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (dispatched): %v", err)
	}
	if len(rowsDispatched) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rowsDispatched))
	}
	if rowsDispatched[0].DeliveredAt != nil {
		t.Errorf("DeliveredAt = %v, want nil — merely dispatched is not delivered", rowsDispatched[0].DeliveredAt)
	}
	if rowsDispatched[0].DeliveryLeadTimeHours != nil {
		t.Errorf("DeliveryLeadTimeHours = %v, want nil while not yet delivered", *rowsDispatched[0].DeliveryLeadTimeHours)
	}

	if _, err := svcs.shipping.Confirm(context.Background(), f.tenantID, shipmentID, shipping.ConfirmRequest{
		IdempotencyKey: "leadtime-deliver", Action: "DELIVER",
	}, 1); err != nil {
		t.Fatalf("Confirm DELIVER: %v", err)
	}

	rowsDelivered, _, err := svcs.repo.List(context.Background(), f.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (delivered): %v", err)
	}
	if rowsDelivered[0].DeliveredAt == nil {
		t.Fatalf("DeliveredAt = nil after a real DELIVER confirm, want non-nil")
	}
	if rowsDelivered[0].DeliveryLeadTimeHours == nil {
		t.Errorf("DeliveryLeadTimeHours = nil after delivery, want non-nil (dispatched_at and delivered_at both set)")
	} else if *rowsDelivered[0].DeliveryLeadTimeHours < 0 {
		t.Errorf("DeliveryLeadTimeHours = %v, want >= 0", *rowsDelivered[0].DeliveryLeadTimeHours)
	}
}

// TestRepository_Summary_DispatchDeliveryExceptionRates_ExactNumerators
// seeds three real shipments in one tenant — one left READY (never
// dispatched), one DISPATCHED then DELIVERED, one DISPATCHED then
// ISSUE — and proves Summary's three rates are computed from the
// right numerator/denominator pairs, never double-counted by the
// item grain.
func TestRepository_Summary_DispatchDeliveryExceptionRates_ExactNumerators(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "rates")
	svcs := newChainServices(tx)

	runChain(t, tx, svcs, f, "LOT-READY", 3, "")
	runChain(t, tx, svcs, f, "LOT-DELIVERED", 4, "DELIVER")
	runChain(t, tx, svcs, f, "LOT-ISSUE", 5, "ISSUE")

	summary, err := svcs.repo.Summary(context.Background(), f.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalShipments != 3 {
		t.Fatalf("TotalShipments = %d, want 3", summary.TotalShipments)
	}
	if summary.DispatchRate.Numerator != 2 || summary.DispatchRate.Denominator != 3 {
		t.Errorf("DispatchRate = %+v, want 2/3 (READY shipment never dispatched)", summary.DispatchRate)
	}
	if summary.DeliveryConfirmationRate.Numerator != 1 || summary.DeliveryConfirmationRate.Denominator != 2 {
		t.Errorf("DeliveryConfirmationRate = %+v, want 1/2 (only 1 of the 2 dispatched shipments was ever marked delivered)", summary.DeliveryConfirmationRate)
	}
	if summary.ExceptionRate.Numerator != 1 || summary.ExceptionRate.Denominator != 3 {
		t.Errorf("ExceptionRate = %+v, want 1/3", summary.ExceptionRate)
	}
	if summary.AvgDeliveryLeadTimeHours == nil {
		t.Errorf("AvgDeliveryLeadTimeHours = nil, want non-nil (one shipment in scope has both dispatched_at and delivered_at)")
	}
}

// TestRepository_ListTrips_ShipmentAndCartonCounts proves the Loading
// & Dispatch Report's own trip-grain view resolves its shipment/carton
// aggregates correctly — a genuinely different grain than Row.
func TestRepository_ListTrips_ShipmentAndCartonCounts(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "trips")
	svcs := newChainServices(tx)
	_, tripID := runChain(t, tx, svcs, f, "LOT-TRIP", 6, "DISPATCH")

	trips, err := svcs.repo.ListTrips(context.Background(), f.tenantID, Filters{}, 100)
	if err != nil {
		t.Fatalf("ListTrips: %v", err)
	}
	if len(trips) != 1 {
		t.Fatalf("len(trips) = %d, want 1", len(trips))
	}
	trip := trips[0]
	if trip.TripID != tripID {
		t.Errorf("TripID = %d, want %d", trip.TripID, tripID)
	}
	if trip.Status != "COMPLETED" {
		t.Errorf("Status = %q, want COMPLETED", trip.Status)
	}
	if trip.ShipmentCount != 1 {
		t.Errorf("ShipmentCount = %d, want 1", trip.ShipmentCount)
	}
	if trip.CartonCount < 1 {
		t.Errorf("CartonCount = %d, want at least 1", trip.CartonCount)
	}
	if trip.ShortCartonCount != 0 {
		t.Errorf("ShortCartonCount = %d, want 0 (every carton was scanned loaded, none flagged short)", trip.ShortCartonCount)
	}
	if trip.LoadingDurationHours == nil {
		t.Errorf("LoadingDurationHours = nil, want non-nil (trip reached COMPLETED with both start/end timestamps set)")
	}
}

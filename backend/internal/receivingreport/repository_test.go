package receivingreport

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/putaway"
	"wms-backend/internal/qualitycheck"
	"wms-backend/internal/receiving"
	"wms-backend/internal/supplier"
	"wms-backend/internal/warehouse"
)

// fixture is one fully-seeded tenant, mirroring internal/receiving's
// own tenant_isolation_test.go fixture shape (real rows via the real
// repositories, not stubs).
type fixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
	supplierID  int64
}

func newFixture(t *testing.T, tx *gorm.DB, label string, qcRequired bool) fixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"rr-"+label, "Receiving Report Test Tenant "+label,
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
		BaseUOM: product.UOMPieces, Status: product.StatusActive, QCRequired: qcRequired,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'RECEIVING', 'ACTIVE')",
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

	sup := supplier.Supplier{TenantID: tenantID, Code: "SUP-" + label, CompanyName: "Supplier " + label, Status: supplier.StatusActive}
	if err := tx.Create(&sup).Error; err != nil {
		t.Fatalf("newFixture(%s): seed supplier: %v", label, err)
	}

	return fixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID, supplierID: sup.ID}
}

func newReceivingService(tx *gorm.DB) *receiving.Service {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	supplierRepo := supplier.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)
	return receiving.NewService(receiving.NewRepository(tx), productRepo, warehouseRepo, locationRepo, supplierRepo, invSvc)
}

// seedReceiptWithSplit creates one receipt with one line, confirmed
// with a single split (lot/pallet/temperature all set), via the real
// Create/Confirm path — so receipt_items/receipt_item_splits are
// exactly what production code would produce, not hand-built rows.
// Returns the receipt ID, the receipt item ID, and the split ID.
func seedReceiptWithSplit(t *testing.T, tx *gorm.DB, f fixture, qty float64, lotNo, palletID string, receivingDate time.Time) (receiptID, itemID, splitID int64) {
	t.Helper()
	svc := newReceivingService(tx)

	resp, err := svc.Create(context.Background(), f.tenantID, receiving.CreateRequest{
		ReceivingType: receiving.TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.locationID,
		SupplierID: &f.supplierID,
		Items:      []receiving.CreateItemRequest{{ProductID: f.productID, LocationID: f.locationID, ExpectedQty: qty, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("seedReceiptWithSplit: Create: %v", err)
	}
	itemID = resp.Items[0].ID

	confirmResp, err := svc.Confirm(context.Background(), f.tenantID, resp.ID, receiving.ConfirmRequest{
		IdempotencyKey: "seed-" + lotNo + "-" + palletID,
		Items: []receiving.ConfirmItemRequest{{
			ItemID: itemID, Quantity: qty,
			Splits: []receiving.ConfirmSplitRequest{{Quantity: qty, LotNo: lotNo, PalletID: palletID, TemperatureC: ptrF(4.5)}},
		}},
	}, 1)
	if err != nil {
		t.Fatalf("seedReceiptWithSplit: Confirm: %v", err)
	}
	receiptID = confirmResp.ID
	splitID = confirmResp.Items[0].Splits[0].ID

	// Confirm's own ReceivingDate is set at Create time to "now" — stamp
	// it explicitly to the caller's chosen value so date-boundary tests
	// can place rows precisely before/inside/after a filter window.
	if err := tx.Exec("UPDATE receipts SET receiving_date = ? WHERE id = ? AND tenant_id = ?", receivingDate, receiptID, f.tenantID).Error; err != nil {
		t.Fatalf("seedReceiptWithSplit: stamp receiving_date: %v", err)
	}

	return receiptID, itemID, splitID
}

func ptrF(v float64) *float64 { return &v }

// --- Tenant isolation ---

func TestRepository_List_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "list-a", false)
	b := newFixture(t, tx, "list-b", false)
	now := time.Now().UTC()

	seedReceiptWithSplit(t, tx, a, 10, "LOT-A", "PLT-A", now)
	seedReceiptWithSplit(t, tx, b, 10, "LOT-B", "PLT-B", now)

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's receipt)", len(rows), total)
	}
	if rows[0].LotNo == nil || *rows[0].LotNo != "LOT-A" {
		t.Errorf("rows[0].LotNo = %v, want LOT-A", rows[0].LotNo)
	}
}

func TestRepository_Summary_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "sum-a", false)
	b := newFixture(t, tx, "sum-b", false)
	now := time.Now().UTC()

	seedReceiptWithSplit(t, tx, a, 10, "LOT-A", "PLT-SA", now)
	seedReceiptWithSplit(t, tx, b, 999, "LOT-B", "PLT-SB", now)
	seedReceiptWithSplit(t, tx, b, 999, "LOT-C", "PLT-SC", now)

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalReceipts != 1 {
		t.Errorf("TotalReceipts = %d, want 1 (tenant B's 2 receipts of 999 each must not leak in)", summary.TotalReceipts)
	}
	for _, uq := range summary.TotalReceivedQtyByUOM {
		if uq.Qty >= 999 {
			t.Errorf("TotalReceivedQtyByUOM contains %v, tenant B's huge quantity leaked across tenant boundary", uq)
		}
	}
}

// --- Date boundary: inclusive From, exclusive To ---

func TestRepository_List_DateBoundary_InclusiveFromExclusiveTo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "date-a", false)

	windowStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)

	seedReceiptWithSplit(t, tx, a, 1, "LOT-BEFORE", "PLT-BEFORE", windowStart.Add(-time.Second)) // just before window: excluded
	seedReceiptWithSplit(t, tx, a, 1, "LOT-START", "PLT-START", windowStart)                     // exactly at From: included (inclusive)
	seedReceiptWithSplit(t, tx, a, 1, "LOT-MID", "PLT-MID", windowStart.Add(3*24*time.Hour))     // inside window: included
	seedReceiptWithSplit(t, tx, a, 1, "LOT-END", "PLT-END", windowEnd)                           // exactly at To: excluded (exclusive)

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{
		Page: 1, Limit: 20,
		Filters: Filters{From: &windowStart, To: &windowEnd},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (LOT-START and LOT-MID only)", total)
	}
	gotLots := map[string]bool{}
	for _, r := range rows {
		if r.LotNo != nil {
			gotLots[*r.LotNo] = true
		}
	}
	if !gotLots["LOT-START"] || !gotLots["LOT-MID"] {
		t.Errorf("rows = %+v, want LOT-START and LOT-MID present", rows)
	}
	if gotLots["LOT-BEFORE"] || gotLots["LOT-END"] {
		t.Errorf("rows = %+v, want LOT-BEFORE and LOT-END excluded (strict boundary)", rows)
	}
}

// --- QC status derivation ---

func TestRepository_List_QCStatus_NotRequired(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "qc-notreq", false) // qcRequired=false
	seedReceiptWithSplit(t, tx, a, 10, "LOT-NR", "PLT-NR", time.Now().UTC())

	repo := NewRepository(tx)
	rows, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].QCStatus != "NOT_REQUIRED" {
		t.Fatalf("QCStatus = %v, want NOT_REQUIRED", rows)
	}
}

func TestRepository_List_QCStatus_PendingThenPassed(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "qc-pass", true) // qcRequired=true
	_, _, splitID := seedReceiptWithSplit(t, tx, a, 10, "LOT-QC", "PLT-QC", time.Now().UTC())

	repo := NewRepository(tx)
	rowsBefore, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (before inspection): %v", err)
	}
	if len(rowsBefore) != 1 || rowsBefore[0].QCStatus != "PENDING" {
		t.Fatalf("QCStatus before any inspection = %v, want PENDING", rowsBefore)
	}

	inspection := qualitycheck.Inspection{
		TenantID: a.tenantID, InspectionNo: "QCI-TEST-1", ReceiptID: rowsBefore[0].ReceiptID,
		WarehouseID: a.warehouseID, QCType: qualitycheck.TypeNormal, Status: qualitycheck.StatusCompleted,
	}
	if err := tx.Create(&inspection).Error; err != nil {
		t.Fatalf("seed qc_inspections: %v", err)
	}
	item := qualitycheck.InspectionItem{
		TenantID: a.tenantID, InspectionID: inspection.ID, ReceiptItemSplitID: splitID,
		ProductID: a.productID, LotNo: "LOT-QC", LocationID: a.locationID,
		HeldQtySnapshot: 10, SampleQty: 10, Status: qualitycheck.ItemStatusCompleted,
	}
	if err := tx.Create(&item).Error; err != nil {
		t.Fatalf("seed qc_inspection_items: %v", err)
	}
	disposition := qualitycheck.InspectionDisposition{
		TenantID: a.tenantID, InspectionItemID: item.ID, Disposition: qualitycheck.DispositionPass, Quantity: 10,
	}
	if err := tx.Create(&disposition).Error; err != nil {
		t.Fatalf("seed qc_dispositions: %v", err)
	}

	rowsAfter, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (after PASS): %v", err)
	}
	if len(rowsAfter) != 1 || rowsAfter[0].QCStatus != "PASSED" {
		t.Fatalf("QCStatus after full PASS disposition = %v, want PASSED", rowsAfter)
	}
}

// --- Putaway status derivation ---

func TestRepository_List_PutawayStatus_NotStartedThenCompleted(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "pa-done", false)
	receiptID, itemID, _ := seedReceiptWithSplit(t, tx, a, 10, "LOT-PA", "PLT-PA", time.Now().UTC())

	repo := NewRepository(tx)
	before, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (before putaway): %v", err)
	}
	if len(before) != 1 || before[0].PutawayStatus != "NOT_STARTED" {
		t.Fatalf("PutawayStatus before any task = %v, want NOT_STARTED", before)
	}

	task := putaway.Task{
		TenantID: a.tenantID, PutawayNo: "PUT-TEST-1", ReceiptID: &receiptID,
		WarehouseID: a.warehouseID, Status: putaway.StatusCompleted, Priority: putaway.PriorityNormal,
	}
	if err := tx.Create(&task).Error; err != nil {
		t.Fatalf("seed putaway_tasks: %v", err)
	}
	taskItem := putaway.TaskItem{
		TenantID: a.tenantID, PutawayTaskID: task.ID, ReceiptItemID: &itemID,
		ProductID: a.productID, LotNo: "LOT-PA",
		SourceLocationID: a.locationID, DestinationLocationID: a.locationID,
		RequestedQty: 10, MovedQty: 10, UOM: "PCS", Status: putaway.ItemStatusCompleted,
	}
	if err := tx.Create(&taskItem).Error; err != nil {
		t.Fatalf("seed putaway_task_items: %v", err)
	}

	after, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (after putaway): %v", err)
	}
	if len(after) != 1 || after[0].PutawayStatus != "COMPLETED" {
		t.Fatalf("PutawayStatus after fully-moved task item = %v, want COMPLETED", after)
	}
}

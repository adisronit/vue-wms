package stockcountreport

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/stockadjustment"
	"wms-backend/internal/stockcount"
	"wms-backend/internal/warehouse"
	"wms-backend/internal/zone"
)

// fixture mirrors internal/stockcount's own tenant_isolation_test.go
// fixture shape (real rows via the real repositories, not stubs).
type fixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	zoneID      int64
	locationID  int64
}

func newFixture(t *testing.T, tx *gorm.DB, label string) fixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"scr-"+label, "Stock Count Report Test Tenant "+label,
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

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
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

	return fixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, zoneID: zoneID, locationID: l.ID}
}

func seedBalance(t *testing.T, tx *gorm.DB, f fixture, lotNo string, quantity float64) {
	t.Helper()
	inv := inventory.Inventory{TenantID: f.tenantID, ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo, Quantity: quantity}
	if err := tx.Create(&inv).Error; err != nil {
		t.Fatalf("seedBalance: %v", err)
	}
}

func newStockCountService(tx *gorm.DB) *stockcount.Service {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	zoneRepo := zone.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)
	adjSvc := stockadjustment.NewService(stockadjustment.NewRepository(tx), productRepo, locationRepo, invSvc, 48)
	return stockcount.NewService(stockcount.NewRepository(tx), warehouseRepo, zoneRepo, productRepo, locationRepo, invSvc, adjSvc)
}

// seedCountItem creates one count session with one line (system qty
// frozen from a real seeded inventory balance), records a counted
// quantity, and stamps count_date — via the real Create/RecordCount
// path, so stock_count_items looks exactly like production would
// produce. Returns the count ID and item ID; callers choose whether to
// Confirm or CreateAdjustments afterward.
func seedCountItem(t *testing.T, tx *gorm.DB, svc *stockcount.Service, f fixture, lotNo string, systemQty, countQty float64, countDate time.Time) (countID, itemID int64) {
	t.Helper()

	resp, err := svc.Create(context.Background(), f.tenantID, stockcount.CreateRequest{
		CountType: stockcount.CountTypeCycle, WarehouseID: f.warehouseID, ZoneID: &f.zoneID,
		Items: []stockcount.CreateItemRequest{{ProductID: f.productID, LocationID: f.locationID, LotNo: lotNo, UOM: "PCS"}},
	}, 1)
	if err != nil {
		t.Fatalf("seedCountItem: Create: %v", err)
	}
	countID = resp.ID
	itemID = resp.Items[0].ID

	if _, err := svc.RecordCount(context.Background(), f.tenantID, countID, itemID, stockcount.RecordCountRequest{
		CountQty: countQty, Condition: stockcount.ConditionGood, Status: stockcount.ItemStatusCounted,
	}); err != nil {
		t.Fatalf("seedCountItem: RecordCount: %v", err)
	}

	if err := tx.Exec("UPDATE stock_counts SET count_date = ? WHERE id = ? AND tenant_id = ?", countDate, countID, f.tenantID).Error; err != nil {
		t.Fatalf("seedCountItem: stamp count_date: %v", err)
	}

	return countID, itemID
}

// --- Tenant isolation ---

func TestRepository_List_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "list-a")
	b := newFixture(t, tx, "list-b")
	svc := newStockCountService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-A", 100)
	seedBalance(t, tx, b, "LOT-B", 100)
	seedCountItem(t, tx, svc, a, "LOT-A", 100, 95, now)
	seedCountItem(t, tx, svc, b, "LOT-B", 100, 50, now)

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's count)", len(rows), total)
	}
	if rows[0].LotNo != "LOT-A" {
		t.Errorf("rows[0].LotNo = %q, want LOT-A", rows[0].LotNo)
	}
}

func TestRepository_Summary_CrossTenant_Excluded(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "sum-a")
	b := newFixture(t, tx, "sum-b")
	svc := newStockCountService(tx)
	now := time.Now().UTC()

	seedBalance(t, tx, a, "LOT-A", 100)
	seedBalance(t, tx, b, "LOT-B", 999)
	seedCountItem(t, tx, svc, a, "LOT-A", 100, 95, now)
	seedCountItem(t, tx, svc, b, "LOT-B", 999, 1, now)

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.TotalCountSessions != 1 {
		t.Errorf("TotalCountSessions = %d, want 1 (tenant B's session must not leak in)", summary.TotalCountSessions)
	}
	if summary.QuantityVariance <= -500 {
		t.Errorf("QuantityVariance = %v, tenant B's huge variance (999->1) leaked across tenant boundary", summary.QuantityVariance)
	}
}

// --- Date boundary ---

func TestRepository_List_DateBoundary_InclusiveFromExclusiveTo(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "date-a")
	svc := newStockCountService(tx)
	seedBalance(t, tx, a, "LOT-BEFORE", 10)
	seedBalance(t, tx, a, "LOT-START", 10)
	seedBalance(t, tx, a, "LOT-MID", 10)
	seedBalance(t, tx, a, "LOT-END", 10)

	windowStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)

	seedCountItem(t, tx, svc, a, "LOT-BEFORE", 10, 10, windowStart.Add(-time.Second))
	seedCountItem(t, tx, svc, a, "LOT-START", 10, 10, windowStart)
	seedCountItem(t, tx, svc, a, "LOT-MID", 10, 10, windowStart.Add(3*24*time.Hour))
	seedCountItem(t, tx, svc, a, "LOT-END", 10, 10, windowEnd)

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
	got := map[string]bool{}
	for _, r := range rows {
		got[r.LotNo] = true
	}
	if !got["LOT-START"] || !got["LOT-MID"] {
		t.Errorf("rows = %+v, want LOT-START and LOT-MID present", rows)
	}
	if got["LOT-BEFORE"] || got["LOT-END"] {
		t.Errorf("rows = %+v, want LOT-BEFORE and LOT-END excluded (strict boundary)", rows)
	}
}

// --- Variance / accuracy / recount derivation ---

func TestRepository_List_VarianceOnly_ExcludesZeroVariance(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "var-a")
	svc := newStockCountService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-EXACT", 50)
	seedBalance(t, tx, a, "LOT-SHORT", 50)

	seedCountItem(t, tx, svc, a, "LOT-EXACT", 50, 50, now) // zero variance
	seedCountItem(t, tx, svc, a, "LOT-SHORT", 50, 40, now) // -10 variance

	repo := NewRepository(tx)
	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{
		Page: 1, Limit: 20, Filters: Filters{VarianceOnly: true},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].LotNo != "LOT-SHORT" {
		t.Fatalf("List(variance_only) = %+v (total %d), want exactly LOT-SHORT", rows, total)
	}
}

func TestRepository_Summary_AccuracyAndRecountRate(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "acc-a")
	svc := newStockCountService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-1", 50)
	seedBalance(t, tx, a, "LOT-2", 50)

	// LOT-1: counted exactly right (accurate). LOT-2: counted short, then
	// flagged RECOUNT.
	seedCountItem(t, tx, svc, a, "LOT-1", 50, 50, now)
	countID2, itemID2 := seedCountItem(t, tx, svc, a, "LOT-2", 50, 40, now)
	if _, err := svc.RecordCount(context.Background(), a.tenantID, countID2, itemID2, stockcount.RecordCountRequest{
		CountQty: 40, Condition: stockcount.ConditionGood, Status: stockcount.ItemStatusRecount,
	}); err != nil {
		t.Fatalf("flag RECOUNT: %v", err)
	}

	repo := NewRepository(tx)
	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.StockCountAccuracy == nil || *summary.StockCountAccuracy != 50 {
		t.Fatalf("StockCountAccuracy = %v, want 50 (1 of 2 counted items exactly matched)", summary.StockCountAccuracy)
	}
	if summary.RecountRate == nil || *summary.RecountRate != 50 {
		t.Fatalf("RecountRate = %v, want 50 (1 of 2 items flagged RECOUNT)", summary.RecountRate)
	}
}

// --- Adjustment reference + pending approval ---

func TestRepository_List_AdjustmentReference_PopulatedAfterCreateAdjustments(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newFixture(t, tx, "adj-a")
	svc := newStockCountService(tx)
	now := time.Now().UTC()
	seedBalance(t, tx, a, "LOT-ADJ", 50)

	countID, _ := seedCountItem(t, tx, svc, a, "LOT-ADJ", 50, 45, now)

	before, _, err := NewRepository(tx).List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (before CreateAdjustments): %v", err)
	}
	if len(before) != 1 || before[0].AdjustmentNo != nil {
		t.Fatalf("AdjustmentNo before CreateAdjustments = %v, want nil", before[0].AdjustmentNo)
	}

	if _, err := svc.CreateAdjustments(context.Background(), a.tenantID, countID, 1); err != nil {
		t.Fatalf("CreateAdjustments: %v", err)
	}

	repo := NewRepository(tx)
	after, _, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("List (after CreateAdjustments): %v", err)
	}
	if len(after) != 1 || after[0].AdjustmentNo == nil {
		t.Fatalf("AdjustmentNo after CreateAdjustments = %v, want a real adjustment number", after[0])
	}
	if after[0].AdjustmentStatus == nil || *after[0].AdjustmentStatus != "PENDING" {
		t.Errorf("AdjustmentStatus = %v, want PENDING (not yet approved)", after[0].AdjustmentStatus)
	}

	summary, err := repo.Summary(context.Background(), a.tenantID, Filters{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.PendingApproval != 1 {
		t.Errorf("PendingApproval = %d, want 1", summary.PendingApproval)
	}
}

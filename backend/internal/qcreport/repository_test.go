package qcreport

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/qualitycheck"
	"wms-backend/internal/receiving"
	"wms-backend/internal/warehouse"
)

type qcReportFixture struct {
	tenantID    int64
	productID   int64
	warehouseID int64
	locationID  int64
}

// newQCReportFixture mirrors internal/qualitycheck's own
// newQCFixture — a fresh tenant/product/warehouse/location per test.
func newQCReportFixture(t *testing.T, tx *gorm.DB, label string) qcReportFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"qcr-"+label, "QC Report Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newQCReportFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newQCReportFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive, QCRequired: true,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newQCReportFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newQCReportFixture(%s): seed warehouse: %v", label, err)
	}

	var zoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID, "Z-"+label, "Zone "+label,
	).Scan(&zoneID).Error; err != nil {
		t.Fatalf("newQCReportFixture(%s): seed zone: %v", label, err)
	}

	l := location.Location{
		TenantID: tenantID, ZoneID: zoneID, Code: "LOC-" + label, Name: "Location " + label,
		Status: location.StatusActive,
	}
	if err := tx.Create(&l).Error; err != nil {
		t.Fatalf("newQCReportFixture(%s): seed location: %v", label, err)
	}

	return qcReportFixture{tenantID: tenantID, productID: p.ID, warehouseID: warehouseID, locationID: l.ID}
}

// newQCServices wires the real receiving.Service and qualitycheck.Service
// (same construction internal/qualitycheck's own tests use), plus this
// package's own read-only Repository, all bound to tx.
func newQCServices(tx *gorm.DB) (*receiving.Service, *qualitycheck.Service, *Repository) {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	receivingRepo := receiving.NewRepository(tx)
	receivingSvc := receiving.NewService(receivingRepo, productRepo, warehouseRepo, locationRepo, nil, invSvc)

	qcRepo := qualitycheck.NewRepository(tx)
	qcSvc := qualitycheck.NewService(qcRepo, warehouseRepo, invSvc)

	return receivingSvc, qcSvc, NewRepository(tx)
}

// seedHeldReceipt drives a real Receiving Create->Confirm, producing a
// real receipt_item_splits row and a real inventory hold — mirrors
// internal/qualitycheck's own seedHeldReceipt exactly.
func seedHeldReceipt(t *testing.T, receivingSvc *receiving.Service, f qcReportFixture, lotNo string, qty float64) int64 {
	t.Helper()
	resp, err := receivingSvc.Create(context.Background(), f.tenantID, receiving.CreateRequest{
		ReceivingType: receiving.TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.locationID,
		Items: []receiving.CreateItemRequest{
			{ProductID: f.productID, LocationID: f.locationID, ExpectedQty: qty, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("seedHeldReceipt: Create: %v", err)
	}
	itemID := resp.Items[0].ID

	_, err = receivingSvc.Confirm(context.Background(), f.tenantID, resp.ID, receiving.ConfirmRequest{
		IdempotencyKey: "qcreport-seed-" + lotNo,
		Items:          []receiving.ConfirmItemRequest{{ItemID: itemID, Quantity: qty, LotNo: lotNo}},
	}, 1)
	if err != nil {
		t.Fatalf("seedHeldReceipt: Confirm: %v", err)
	}
	return resp.ID
}

func TestRepository_List_ScopedToTenant(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newQCReportFixture(t, tx, "list-a")
	b := newQCReportFixture(t, tx, "list-b")

	recvSvc, qcSvc, repo := newQCServices(tx)
	rA := seedHeldReceipt(t, recvSvc, a, "LOT-A", 10)
	rB := seedHeldReceipt(t, recvSvc, b, "LOT-B", 10)
	if _, err := qcSvc.Create(context.Background(), a.tenantID, qualitycheck.CreateRequest{ReceiptID: rA, WarehouseID: a.warehouseID}, 1); err != nil {
		t.Fatalf("qcSvc.Create A: %v", err)
	}
	if _, err := qcSvc.Create(context.Background(), b.tenantID, qualitycheck.CreateRequest{ReceiptID: rB, WarehouseID: b.warehouseID}, 1); err != nil {
		t.Fatalf("qcSvc.Create B: %v", err)
	}

	rows, total, err := repo.List(context.Background(), a.tenantID, ListParams{Page: 1, Limit: 20}, now())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(tenantA) = %d rows (total %d), want exactly 1 (must not include tenant B's inspection item)", len(rows), total)
	}
	if rows[0].ReceiptID != rA {
		t.Errorf("ReceiptID = %d, want %d", rows[0].ReceiptID, rA)
	}
}

// TestRepository_List_ResolutionStatus_PartialThenMixed_NoDoubleCounting
// drives the exact PASS 95 / HOLD 3 / REJECT 2 -> reinspect PASS 3
// scenario internal/qualitycheck's own mandatory-scenario test uses,
// and proves two things qcreport must get right: (1) Resolution
// Status transitions PENDING -> PARTIAL -> MIXED (not PASSED — some
// of the 100 units were rejected, so a fully-resolved item with both
// outcomes present must say MIXED, never silently collapse to
// PASSED), and (2) three separate qc_dispositions rows against the
// same inspection item never fan List out into more than one Row —
// the grain is one row per qc_inspection_item, not per disposition.
func TestRepository_List_ResolutionStatus_PartialThenMixed_NoDoubleCounting(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newQCReportFixture(t, tx, "lifecycle")
	recvSvc, qcSvc, repo := newQCServices(tx)
	receiptID := seedHeldReceipt(t, recvSvc, f, "LOT-M", 100)

	created, err := qcSvc.Create(context.Background(), f.tenantID, qualitycheck.CreateRequest{ReceiptID: receiptID, WarehouseID: f.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	itemID := created.Items[0].ID

	rowsPending, _, err := repo.List(context.Background(), f.tenantID, ListParams{Page: 1, Limit: 20}, now())
	if err != nil {
		t.Fatalf("List (pending): %v", err)
	}
	if len(rowsPending) != 1 || rowsPending[0].ResolutionStatus != "PENDING" {
		t.Fatalf("resolution_status before any disposition = %+v, want exactly 1 row, PENDING", rowsPending)
	}

	if _, err := qcSvc.Confirm(context.Background(), f.tenantID, created.ID, qualitycheck.ConfirmRequest{
		IdempotencyKey: "key-partial-1",
		Dispositions: []qualitycheck.ConfirmDispositionRequest{
			{ItemID: itemID, Disposition: qualitycheck.DispositionPass, Quantity: 95},
			{ItemID: itemID, Disposition: qualitycheck.DispositionHold, Quantity: 3},
			{ItemID: itemID, Disposition: qualitycheck.DispositionReject, Quantity: 2},
		},
	}, 1); err != nil {
		t.Fatalf("Confirm(PASS 95/HOLD 3/REJECT 2): %v", err)
	}

	rowsPartial, _, err := repo.List(context.Background(), f.tenantID, ListParams{Page: 1, Limit: 20}, now())
	if err != nil {
		t.Fatalf("List (partial): %v", err)
	}
	if len(rowsPartial) != 1 {
		t.Fatalf("List after 3 dispositions on the same item = %d rows, want exactly 1 (must not double-count by disposition)", len(rowsPartial))
	}
	if rowsPartial[0].ResolutionStatus != "PARTIAL" {
		t.Errorf("resolution_status = %q, want PARTIAL (97 of 100 decided, 3 still held)", rowsPartial[0].ResolutionStatus)
	}
	if rowsPartial[0].PassQty != 95 || rowsPartial[0].RejectQty != 2 || rowsPartial[0].RemainingHoldQty != 3 {
		t.Errorf("pass/reject/remaining = %v/%v/%v, want 95/2/3", rowsPartial[0].PassQty, rowsPartial[0].RejectQty, rowsPartial[0].RemainingHoldQty)
	}

	if _, err := qcSvc.Confirm(context.Background(), f.tenantID, created.ID, qualitycheck.ConfirmRequest{
		IdempotencyKey: "key-partial-2",
		Dispositions:   []qualitycheck.ConfirmDispositionRequest{{ItemID: itemID, Disposition: qualitycheck.DispositionPass, Quantity: 3}},
	}, 1); err != nil {
		t.Fatalf("Confirm(reinspection PASS 3): %v", err)
	}

	rowsFinal, _, err := repo.List(context.Background(), f.tenantID, ListParams{Page: 1, Limit: 20}, now())
	if err != nil {
		t.Fatalf("List (final): %v", err)
	}
	if len(rowsFinal) != 1 {
		t.Fatalf("List after reinspection = %d rows, want exactly 1", len(rowsFinal))
	}
	if rowsFinal[0].ResolutionStatus != "MIXED" {
		t.Errorf("resolution_status after full resolution = %q, want MIXED (98 passed + 2 rejected both present — must not collapse to PASSED)", rowsFinal[0].ResolutionStatus)
	}
	if rowsFinal[0].RemainingHoldQty != 0 {
		t.Errorf("remaining_hold_qty = %v, want 0", rowsFinal[0].RemainingHoldQty)
	}
	if rowsFinal[0].QCTurnaroundHours == nil {
		t.Errorf("qc_turnaround_hours = nil, want non-nil once resolved")
	}
	if rowsFinal[0].HoldAgingHours != nil {
		t.Errorf("hold_aging_hours = %v, want nil once resolved", *rowsFinal[0].HoldAgingHours)
	}
	if rowsFinal[0].DefectCode != nil {
		t.Errorf("defect_code = %q, want nil (no disposition in this test ever set one)", *rowsFinal[0].DefectCode)
	}
}

func TestRepository_Summary_PassRejectHoldRates_ExactNumerators(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newQCReportFixture(t, tx, "rates")
	recvSvc, qcSvc, repo := newQCServices(tx)
	receiptID := seedHeldReceipt(t, recvSvc, f, "LOT-R", 100)

	created, err := qcSvc.Create(context.Background(), f.tenantID, qualitycheck.CreateRequest{ReceiptID: receiptID, WarehouseID: f.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	itemID := created.Items[0].ID

	defectCode := "DEF-DAMAGED"
	if _, err := qcSvc.Confirm(context.Background(), f.tenantID, created.ID, qualitycheck.ConfirmRequest{
		IdempotencyKey: "key-rates-1",
		Dispositions: []qualitycheck.ConfirmDispositionRequest{
			{ItemID: itemID, Disposition: qualitycheck.DispositionPass, Quantity: 70},
			{ItemID: itemID, Disposition: qualitycheck.DispositionReject, Quantity: 30, DefectCode: defectCode},
		},
	}, 1); err != nil {
		t.Fatalf("Confirm(PASS 70/REJECT 30): %v", err)
	}

	summary, err := repo.Summary(context.Background(), f.tenantID, Filters{}, now())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.PassRate.RatePct == nil || *summary.PassRate.RatePct != 70 {
		t.Errorf("PassRate = %+v, want 70%%", summary.PassRate)
	}
	if summary.RejectionRate.RatePct == nil || *summary.RejectionRate.RatePct != 30 {
		t.Errorf("RejectionRate = %+v, want 30%%", summary.RejectionRate)
	}
	if summary.HoldRate.RatePct == nil || *summary.HoldRate.RatePct != 0 {
		t.Errorf("HoldRate = %+v, want 0%% (fully resolved, nothing remains held)", summary.HoldRate)
	}
	if len(summary.Charts.TopDefects) != 1 || summary.Charts.TopDefects[0].DefectCode != defectCode || summary.Charts.TopDefects[0].Qty != 30 {
		t.Errorf("TopDefects = %+v, want exactly one DEF-DAMAGED bucket with qty 30", summary.Charts.TopDefects)
	}
}

// TestRepository_Summary_PendingQCNotStarted_VsAwaitingDecision proves
// qcreport distinguishes the two sub-cases Receiving Report's own
// Pending QC Qty collapses together: a split with a live hold but no
// qc_inspection_items row at all (NotStarted) versus a split whose
// inspection was created but has received no disposition yet
// (AwaitingDecision).
func TestRepository_Summary_PendingQCNotStarted_VsAwaitingDecision(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newQCReportFixture(t, tx, "pending")
	recvSvc, qcSvc, repo := newQCServices(tx)

	// Not started: held receipt, no inspection created at all.
	notStartedReceiptID := seedHeldReceipt(t, recvSvc, f, "LOT-NS", 40)

	summaryBeforeInspection, err := repo.Summary(context.Background(), f.tenantID, Filters{}, now())
	if err != nil {
		t.Fatalf("Summary (before inspection): %v", err)
	}
	if summaryBeforeInspection.PendingQCNotStartedCount != 1 || summaryBeforeInspection.PendingQCNotStartedQty != 40 {
		t.Errorf("PendingQCNotStarted = count=%d qty=%v, want count=1 qty=40",
			summaryBeforeInspection.PendingQCNotStartedCount, summaryBeforeInspection.PendingQCNotStartedQty)
	}
	if summaryBeforeInspection.PendingQCAwaitingDecisionCount != 0 {
		t.Errorf("PendingQCAwaitingDecisionCount = %d, want 0 (no inspection item exists yet)", summaryBeforeInspection.PendingQCAwaitingDecisionCount)
	}

	// Now start the inspection (Create only, no Confirm) — the split
	// moves from NotStarted to AwaitingDecision.
	if _, err := qcSvc.Create(context.Background(), f.tenantID, qualitycheck.CreateRequest{ReceiptID: notStartedReceiptID, WarehouseID: f.warehouseID}, 1); err != nil {
		t.Fatalf("Create: %v", err)
	}

	summaryAfterInspection, err := repo.Summary(context.Background(), f.tenantID, Filters{}, now())
	if err != nil {
		t.Fatalf("Summary (after inspection started): %v", err)
	}
	if summaryAfterInspection.PendingQCNotStartedCount != 0 {
		t.Errorf("PendingQCNotStartedCount = %d, want 0 (inspection now started)", summaryAfterInspection.PendingQCNotStartedCount)
	}
	if summaryAfterInspection.PendingQCAwaitingDecisionCount != 1 || summaryAfterInspection.PendingQCAwaitingDecisionQty != 40 {
		t.Errorf("PendingQCAwaitingDecision = count=%d qty=%v, want count=1 qty=40",
			summaryAfterInspection.PendingQCAwaitingDecisionCount, summaryAfterInspection.PendingQCAwaitingDecisionQty)
	}
}

func TestRepository_List_HoldAgingHours_MeasuredFromFirstHoldDisposition(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newQCReportFixture(t, tx, "aging")
	recvSvc, qcSvc, repo := newQCServices(tx)
	receiptID := seedHeldReceipt(t, recvSvc, f, "LOT-AGE", 10)

	created, err := qcSvc.Create(context.Background(), f.tenantID, qualitycheck.CreateRequest{ReceiptID: receiptID, WarehouseID: f.warehouseID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	itemID := created.Items[0].ID

	if _, err := qcSvc.Confirm(context.Background(), f.tenantID, created.ID, qualitycheck.ConfirmRequest{
		IdempotencyKey: "key-aging-1",
		Dispositions:   []qualitycheck.ConfirmDispositionRequest{{ItemID: itemID, Disposition: qualitycheck.DispositionHold, Quantity: 10}},
	}, 1); err != nil {
		t.Fatalf("Confirm(HOLD 10): %v", err)
	}

	// "now" is 5 hours after the HOLD disposition was recorded —
	// Hold Aging must reflect that gap, not zero and not nil. Must use
	// this package's own now() (see service.go's doc) rather than a
	// plain time.Now().UTC(), or this assertion fails by exactly the
	// host's UTC offset — this is the test that originally caught
	// that quirk.
	future := now().Add(5 * time.Hour)
	rows, _, err := repo.List(context.Background(), f.tenantID, ListParams{Page: 1, Limit: 20}, future)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ResolutionStatus != "PENDING" {
		t.Fatalf("resolution_status = %q, want PENDING (HOLD resolves nothing)", rows[0].ResolutionStatus)
	}
	if rows[0].HoldAgingHours == nil {
		t.Fatalf("hold_aging_hours = nil, want non-nil while still open")
	}
	if *rows[0].HoldAgingHours < 4.9 || *rows[0].HoldAgingHours > 5.1 {
		t.Errorf("hold_aging_hours = %v, want ~5.0 (measured from the HOLD disposition's own created_at, not inspection.created_at)", *rows[0].HoldAgingHours)
	}
	if rows[0].QCTurnaroundHours != nil {
		t.Errorf("qc_turnaround_hours = %v, want nil while still open", *rows[0].QCTurnaroundHours)
	}
}

// Phase 33E.3 tests -- Reconciliation + the Inter-Warehouse Transfer
// Note PDF. Reuses the real fixture/stack helpers (newTRFixture,
// newTRStack, runTransferToShipment) pipeline_integration_test.go
// already established in this same package, and mirrors internal/
// shipping's own pdf_test.go shape closely for the PDF-specific tests
// (DRAFT watermark, immutable snapshot, side-effect-free, cross-tenant).
package transferreceipt

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/brand"
	"wms-backend/internal/docrender"
	"wms-backend/internal/order"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
)

// fakeDocRecorder is an in-memory docRecorder test double, mirroring
// internal/shipping/pdf_test.go's own copy exactly (each PDF package
// declares its own narrow interface, so each package's own tests
// declare their own narrow fake rather than sharing one across package
// boundaries).
type fakeDocRecorder struct {
	calls []fakeDocRenderCall
}

type fakeDocRenderCall struct {
	documentType, sourceTable string
	sourceID                  int64
	isDraft                   bool
	pdfData                   []byte
}

func (f *fakeDocRecorder) Record(_ context.Context, _ int64, documentType, _ string, sourceTable string, sourceID, _ int64, isDraft bool, pdfData []byte) error {
	f.calls = append(f.calls, fakeDocRenderCall{documentType, sourceTable, sourceID, isDraft, pdfData})
	return nil
}

// newTRPDFService wires a *PDFService bound to tx, fully equipped
// (products/brands/expiry, plus the shipment/order/warehouse lookups
// the header needs) -- the same "real repositories bound to the test's
// own tx" shape newTRStack itself already uses.
func newTRPDFService(tx *gorm.DB, st trStack) *PDFService {
	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	orderRepo := order.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	productRepo := product.NewRepository(tx)
	brandRepo := brand.NewRepository(tx)

	pdfSvc := NewPDFService(st.trSvc, st.shippingSvc, orderRepo, warehouseRepo, tenantRepo, userRepo)
	pdfSvc.SetProductInfo(productRepo, brandRepo)
	pdfSvc.SetExpiryInfo(st.pickingSvc)
	return pdfSvc
}

// --- Reconciliation correctness ---------------------------------------

// TestService_Reconciliation_FullyReceivedVsShortReceipt is the
// reconciliation-correctness test this batch's own spec calls for: a
// receipt with a short/damaged discrepancy (the exact same shape
// pipeline_integration_test.go's own
// TestTransferReceipt_ShortReceipt_DiscrepancyThenConfirm already
// proves at the stock-mutation level) must report fully_reconciled =
// false with the correct discrepancy numbers; a fully-received receipt
// must report fully_reconciled = true, both at the per-line AND header
// level.
func TestService_Reconciliation_FullyReceivedVsShortReceipt(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	// --- Fully received receipt. ---
	fFull := newTRFixture(t, tx, "recon-full")
	stFull := newTRStack(tx)
	const fullQty = 40.0
	fullShipmentID := runTransferToShipment(t, tx, stFull, fFull, "LOT-RECON-FULL", 100, fullQty, "recon-full")

	fullReceipt, err := stFull.trSvc.Create(context.Background(), fFull.tenantID, CreateRequest{ShipmentID: fullShipmentID}, 1)
	if err != nil {
		t.Fatalf("Create (full): %v", err)
	}
	fullLineID := fullReceipt.Items[0].ID
	if _, err := stFull.trSvc.ReceiveItem(context.Background(), fFull.tenantID, fullReceipt.ID, fullLineID, ReceiveItemRequest{
		IdempotencyKey: "recon-full-receive", LocationID: fFull.destLocationID, Quantity: fullQty,
	}, 1); err != nil {
		t.Fatalf("ReceiveItem (full): %v", err)
	}
	if _, err := stFull.trSvc.Confirm(context.Background(), fFull.tenantID, fullReceipt.ID, ConfirmRequest{IdempotencyKey: "recon-full-confirm"}, 1); err != nil {
		t.Fatalf("Confirm (full): %v", err)
	}

	fullRecon, err := stFull.trSvc.Reconciliation(context.Background(), fFull.tenantID, fullReceipt.ID)
	if err != nil {
		t.Fatalf("Reconciliation (full): %v", err)
	}
	if !fullRecon.FullyReconciled {
		t.Errorf("fully received receipt: FullyReconciled = false, want true")
	}
	if fullRecon.ReconciliationStatus != "RECONCILED" {
		t.Errorf("fully received receipt: ReconciliationStatus = %q, want RECONCILED", fullRecon.ReconciliationStatus)
	}
	if fullRecon.TotalDispatchedQty != fullQty || fullRecon.TotalReceivedQty != fullQty || fullRecon.TotalDiscrepancyQty != 0 {
		t.Errorf("fully received receipt: totals = dispatched=%v received=%v discrepancy=%v, want dispatched=%v received=%v discrepancy=0",
			fullRecon.TotalDispatchedQty, fullRecon.TotalReceivedQty, fullRecon.TotalDiscrepancyQty, fullQty, fullQty)
	}
	if len(fullRecon.Lines) != 1 || !fullRecon.Lines[0].FullyReconciled {
		t.Errorf("fully received receipt: line FullyReconciled = %v, want true", fullRecon.Lines)
	}

	// --- Short/damaged receipt, closed with a discrepancy. ---
	fShort := newTRFixture(t, tx, "recon-short")
	stShort := newTRStack(tx)
	const shortTransferQty = 50.0
	const shortActuallyReceived = 30.0
	shortShipmentID := runTransferToShipment(t, tx, stShort, fShort, "LOT-RECON-SHORT", 100, shortTransferQty, "recon-short")

	shortReceipt, err := stShort.trSvc.Create(context.Background(), fShort.tenantID, CreateRequest{ShipmentID: shortShipmentID}, 1)
	if err != nil {
		t.Fatalf("Create (short): %v", err)
	}
	shortLineID := shortReceipt.Items[0].ID
	if _, err := stShort.trSvc.ReceiveItem(context.Background(), fShort.tenantID, shortReceipt.ID, shortLineID, ReceiveItemRequest{
		IdempotencyKey: "recon-short-receive", LocationID: fShort.destLocationID, Quantity: shortActuallyReceived,
		CloseShort: true, DiscrepancyReason: "Two cartons damaged in transit",
	}, 1); err != nil {
		t.Fatalf("ReceiveItem (short, close short): %v", err)
	}
	if _, err := stShort.trSvc.Confirm(context.Background(), fShort.tenantID, shortReceipt.ID, ConfirmRequest{IdempotencyKey: "recon-short-confirm"}, 1); err != nil {
		t.Fatalf("Confirm (short): %v", err)
	}

	shortRecon, err := stShort.trSvc.Reconciliation(context.Background(), fShort.tenantID, shortReceipt.ID)
	if err != nil {
		t.Fatalf("Reconciliation (short): %v", err)
	}
	if shortRecon.FullyReconciled {
		t.Errorf("short receipt: FullyReconciled = true, want false")
	}
	if shortRecon.ReconciliationStatus != "DISCREPANCY" {
		t.Errorf("short receipt: ReconciliationStatus = %q, want DISCREPANCY", shortRecon.ReconciliationStatus)
	}
	wantDiscrepancy := shortTransferQty - shortActuallyReceived
	if shortRecon.TotalDispatchedQty != shortTransferQty || shortRecon.TotalReceivedQty != shortActuallyReceived || shortRecon.TotalDiscrepancyQty != wantDiscrepancy {
		t.Errorf("short receipt: totals = dispatched=%v received=%v discrepancy=%v, want dispatched=%v received=%v discrepancy=%v",
			shortRecon.TotalDispatchedQty, shortRecon.TotalReceivedQty, shortRecon.TotalDiscrepancyQty,
			shortTransferQty, shortActuallyReceived, wantDiscrepancy)
	}
	if len(shortRecon.Lines) != 1 {
		t.Fatalf("short receipt: lines = %d, want 1", len(shortRecon.Lines))
	}
	line := shortRecon.Lines[0]
	if line.FullyReconciled {
		t.Errorf("short receipt: line FullyReconciled = true, want false")
	}
	if line.DiscrepancyQty == nil || *line.DiscrepancyQty != wantDiscrepancy {
		t.Errorf("short receipt: line DiscrepancyQty = %v, want %v", line.DiscrepancyQty, wantDiscrepancy)
	}
	if line.DiscrepancyReason == nil || *line.DiscrepancyReason == "" {
		t.Errorf("short receipt: line DiscrepancyReason = %v, want a non-empty reason", line.DiscrepancyReason)
	}
	if line.ReceivedQty != shortActuallyReceived {
		t.Errorf("short receipt: line ReceivedQty = %v, want %v", line.ReceivedQty, shortActuallyReceived)
	}
}

// TestService_Reconciliation_DraftReceipt_IsPending covers the PENDING
// status: a receipt still DRAFT (not yet confirmed) reports
// ReconciliationStatus = PENDING even though a line may already carry a
// provisional discrepancy -- the overall verdict is not final until
// Confirm's own "every line settled" gate actually passes.
func TestService_Reconciliation_DraftReceipt_IsPending(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "recon-pending")
	st := newTRStack(tx)
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-RECON-PENDING", 100, 25, "recon-pending")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	recon, err := st.trSvc.Reconciliation(context.Background(), f.tenantID, receipt.ID)
	if err != nil {
		t.Fatalf("Reconciliation: %v", err)
	}
	if recon.ReconciliationStatus != "PENDING" {
		t.Errorf("DRAFT receipt: ReconciliationStatus = %q, want PENDING", recon.ReconciliationStatus)
	}
	if recon.FullyReconciled {
		t.Errorf("DRAFT, not-yet-received receipt: FullyReconciled = true, want false")
	}
}

// TestService_Reconciliation_CrossTenant_NotFound proves Reconciliation
// inherits Get's own tenant-scoped IDOR guard.
func TestService_Reconciliation_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTRFixture(t, tx, "recon-cross-a")
	b := newTRFixture(t, tx, "recon-cross-b")
	stA := newTRStack(tx)
	stB := newTRStack(tx)

	shipmentIDOnB := runTransferToShipment(t, tx, stB, b, "LOT-RECON-CROSS-B", 100, 25, "recon-cross-b")
	receiptOnB, err := stB.trSvc.Create(context.Background(), b.tenantID, CreateRequest{ShipmentID: shipmentIDOnB}, 1)
	if err != nil {
		t.Fatalf("Create (tenant B): %v", err)
	}

	if _, err := stA.trSvc.Reconciliation(context.Background(), a.tenantID, receiptOnB.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Reconciliation(tenantA, tenantB's receipt) = %v, want ErrNotFound", err)
	}
	if _, err := stB.trSvc.Reconciliation(context.Background(), b.tenantID, receiptOnB.ID); err != nil {
		t.Errorf("Reconciliation(tenantB, its own receipt) = %v, want success", err)
	}
}

// TestService_List_HasDiscrepancyFilter covers the list-level
// reconciliation view: GET /transfer-receipts?has_discrepancy=true
// returns only receipts carrying at least one discrepancy line.
func TestService_List_HasDiscrepancyFilter(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "list-disc")
	st := newTRStack(tx)

	cleanShipmentID := runTransferToShipment(t, tx, st, f, "LOT-LIST-CLEAN", 100, 20, "list-disc-clean")
	cleanReceipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: cleanShipmentID}, 1)
	if err != nil {
		t.Fatalf("Create (clean): %v", err)
	}
	if _, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, cleanReceipt.ID, cleanReceipt.Items[0].ID, ReceiveItemRequest{
		IdempotencyKey: "list-disc-clean-receive", LocationID: f.destLocationID, Quantity: 20,
	}, 1); err != nil {
		t.Fatalf("ReceiveItem (clean): %v", err)
	}

	discShipmentID := runTransferToShipment(t, tx, st, f, "LOT-LIST-DISC", 100, 20, "list-disc-bad")
	discReceipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: discShipmentID}, 1)
	if err != nil {
		t.Fatalf("Create (discrepancy): %v", err)
	}
	if _, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, discReceipt.ID, discReceipt.Items[0].ID, ReceiveItemRequest{
		IdempotencyKey: "list-disc-bad-receive", LocationID: f.destLocationID, Quantity: 10,
		CloseShort: true, DiscrepancyReason: "short",
	}, 1); err != nil {
		t.Fatalf("ReceiveItem (discrepancy): %v", err)
	}

	rows, total, err := st.trSvc.List(context.Background(), f.tenantID, ListParams{HasDiscrepancy: true})
	if err != nil {
		t.Fatalf("List(has_discrepancy=true): %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("List(has_discrepancy=true) returned %d rows (total %d), want exactly 1", len(rows), total)
	}
	if rows[0].ID != discReceipt.ID {
		t.Errorf("List(has_discrepancy=true) returned receipt %d, want the discrepancy receipt %d", rows[0].ID, discReceipt.ID)
	}

	allRows, allTotal, err := st.trSvc.List(context.Background(), f.tenantID, ListParams{})
	if err != nil {
		t.Fatalf("List(no filter): %v", err)
	}
	if allTotal != 2 || len(allRows) != 2 {
		t.Fatalf("List(no filter) returned %d rows (total %d), want exactly 2", len(allRows), allTotal)
	}
}

// --- Transfer Note PDF --------------------------------------------------

// TestPDFService_RenderTransferNote_DraftWatermark_DraftVsFinal renders
// the SAME receipt's Transfer Note once while still DRAFT (not yet
// confirmed) and once after Confirm, and proves the DRAFT/FINAL split
// mirrors every other PDF in this codebase.
func TestPDFService_RenderTransferNote_DraftWatermark_DraftVsFinal(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "pdf-draft-wm")
	st := newTRStack(tx)
	const transferQty = 15.0
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-PDF-DRAFT", 100, transferQty, "pdf-draft-wm")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pdfSvc := newTRPDFService(tx, st)
	recorder := &fakeDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	draftData, _, err := pdfSvc.RenderTransferNote(context.Background(), f.tenantID, 1, receipt.ID)
	if err != nil {
		t.Fatalf("RenderTransferNote (draft, not yet received): %v", err)
	}
	if len(draftData) == 0 || !bytes.HasPrefix(draftData, []byte("%PDF-")) {
		t.Fatal("RenderTransferNote (draft) did not return a valid, non-empty PDF")
	}

	if _, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, receipt.Items[0].ID, ReceiveItemRequest{
		IdempotencyKey: "pdf-draft-wm-receive", LocationID: f.destLocationID, Quantity: transferQty,
	}, 1); err != nil {
		t.Fatalf("ReceiveItem: %v", err)
	}
	if _, err := st.trSvc.Confirm(context.Background(), f.tenantID, receipt.ID, ConfirmRequest{IdempotencyKey: "pdf-draft-wm-confirm"}, 1); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	finalData, _, err := pdfSvc.RenderTransferNote(context.Background(), f.tenantID, 1, receipt.ID)
	if err != nil {
		t.Fatalf("RenderTransferNote (final, COMPLETED): %v", err)
	}
	if len(finalData) == 0 || !bytes.HasPrefix(finalData, []byte("%PDF-")) {
		t.Fatal("RenderTransferNote (final) did not return a valid, non-empty PDF")
	}

	if len(recorder.calls) != 2 {
		t.Fatalf("Record called %d times, want 2", len(recorder.calls))
	}
	if !recorder.calls[0].isDraft {
		t.Error("first render (DRAFT receipt) was recorded with is_draft = false, want true")
	}
	if recorder.calls[1].isDraft {
		t.Error("second render (COMPLETED receipt) was recorded with is_draft = true, want false")
	}
	if bytes.Equal(draftData, finalData) {
		t.Error("draft and final renders produced byte-identical PDFs — the DRAFT watermark/status text must change the output")
	}
}

// TestPDFService_RenderTransferNote_ImmutableSnapshot_RealIntegration is
// the end-to-end "immutable historical snapshot" proof wired through
// internal/docrender for real -- render the SAME receipt's Transfer
// Note before and after a real data change (Confirm) and confirm
// re-fetching the EARLIER render returns the ORIGINAL (draft) bytes,
// never the later (final) ones.
func TestPDFService_RenderTransferNote_ImmutableSnapshot_RealIntegration(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "pdf-snap-real")
	st := newTRStack(tx)
	const transferQty = 15.0
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-PDF-SNAP", 100, transferQty, "pdf-snap-real")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pdfSvc := newTRPDFService(tx, st)
	userRepo := user.NewRepository(tx)
	drSvc := docrender.NewService(docrender.NewRepository(tx), userRepo)
	pdfSvc.SetDocRender(drSvc)

	draftData, _, err := pdfSvc.RenderTransferNote(context.Background(), f.tenantID, 1, receipt.ID)
	if err != nil {
		t.Fatalf("RenderTransferNote (before confirm): %v", err)
	}

	if _, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, receipt.Items[0].ID, ReceiveItemRequest{
		IdempotencyKey: "pdf-snap-real-receive", LocationID: f.destLocationID, Quantity: transferQty,
	}, 1); err != nil {
		t.Fatalf("ReceiveItem: %v", err)
	}
	if _, err := st.trSvc.Confirm(context.Background(), f.tenantID, receipt.ID, ConfirmRequest{IdempotencyKey: "pdf-snap-real-confirm"}, 1); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	finalData, _, err := pdfSvc.RenderTransferNote(context.Background(), f.tenantID, 1, receipt.ID)
	if err != nil {
		t.Fatalf("RenderTransferNote (after confirm): %v", err)
	}
	if bytes.Equal(draftData, finalData) {
		t.Fatal("before/after renders were byte-identical — the test setup is not actually exercising a real data change")
	}

	history, err := drSvc.History(context.Background(), f.tenantID, docrender.TypeTransferNote, receipt.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("History returned %d rows, want 2", len(history))
	}
	// Newest first (Repository.ListBySource's own ordering).
	earlierRow, laterRow := history[1], history[0]

	earlier, err := drSvc.Fetch(context.Background(), f.tenantID, earlierRow.ID)
	if err != nil {
		t.Fatalf("Fetch(earlier): %v", err)
	}
	if !bytes.Equal(earlier.PDFData, draftData) {
		t.Error("Fetch(earlier) did not return the ORIGINAL draft bytes")
	}
	if bytes.Equal(earlier.PDFData, finalData) {
		t.Fatal("Fetch(earlier) returned the LATER (final) render's bytes — reprint re-rendered instead of returning the immutable snapshot")
	}

	later, err := drSvc.Fetch(context.Background(), f.tenantID, laterRow.ID)
	if err != nil {
		t.Fatalf("Fetch(later): %v", err)
	}
	if !bytes.Equal(later.PDFData, finalData) {
		t.Error("Fetch(later) did not return the final render's bytes")
	}
}

// TestPDFService_RenderTransferNote_RepeatedDownloads_NeverMutateState is
// the side-effect-free test this phase's own spec explicitly calls out:
// generate the SAME Transfer Note 5+ times in a row and assert zero
// change to transfer_receipt/item/inventory state — PDF generation must
// never mutate workflow state. Mirrors internal/shipping's own
// TestPDFService_RenderDeliveryNote_RepeatedDownloads_NeverMutateState.
func TestPDFService_RenderTransferNote_RepeatedDownloads_NeverMutateState(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newTRFixture(t, tx, "pdf-repeat-dl")
	st := newTRStack(tx)
	const transferQty = 15.0
	shipmentID := runTransferToShipment(t, tx, st, f, "LOT-PDF-REPEAT", 100, transferQty, "pdf-repeat-dl")

	receipt, err := st.trSvc.Create(context.Background(), f.tenantID, CreateRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := st.trSvc.ReceiveItem(context.Background(), f.tenantID, receipt.ID, receipt.Items[0].ID, ReceiveItemRequest{
		IdempotencyKey: "pdf-repeat-dl-receive", LocationID: f.destLocationID, Quantity: transferQty,
	}, 1); err != nil {
		t.Fatalf("ReceiveItem: %v", err)
	}

	before, err := st.trSvc.Get(context.Background(), f.tenantID, receipt.ID)
	if err != nil {
		t.Fatalf("Get(before): %v", err)
	}
	destBalanceBefore := sumInventoryQtyAtWarehouse(t, tx, f.tenantID, f.productID, f.destWarehouseID)

	pdfSvc := newTRPDFService(tx, st)

	for i := 0; i < 5; i++ {
		if _, _, err := pdfSvc.RenderTransferNote(context.Background(), f.tenantID, 1, receipt.ID); err != nil {
			t.Fatalf("RenderTransferNote (iteration %d): %v", i, err)
		}
	}

	after, err := st.trSvc.Get(context.Background(), f.tenantID, receipt.ID)
	if err != nil {
		t.Fatalf("Get(after): %v", err)
	}
	if after.Status != before.Status {
		t.Errorf("receipt status changed after 5x repeated PDF downloads: before=%v after=%v", before.Status, after.Status)
	}
	if len(after.Items) != len(before.Items) || after.Items[0].ReceivedQty != before.Items[0].ReceivedQty || after.Items[0].Status != before.Items[0].Status {
		t.Errorf("item state changed after 5x repeated PDF downloads: before=%+v after=%+v", before.Items, after.Items)
	}

	destBalanceAfter := sumInventoryQtyAtWarehouse(t, tx, f.tenantID, f.productID, f.destWarehouseID)
	if destBalanceAfter != destBalanceBefore {
		t.Errorf("destination warehouse inventory changed after 5x repeated PDF downloads: before=%v after=%v", destBalanceBefore, destBalanceAfter)
	}
}

// TestPDFService_RenderTransferNote_CrossTenant_NotFound proves the
// Transfer Note generator inherits Service.Get's own tenant-scoped IDOR
// guard.
func TestPDFService_RenderTransferNote_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newTRFixture(t, tx, "pdf-cross-a")
	b := newTRFixture(t, tx, "pdf-cross-b")
	stA := newTRStack(tx)
	stB := newTRStack(tx)

	shipmentIDOnB := runTransferToShipment(t, tx, stB, b, "LOT-PDF-CROSS-B", 100, 15, "pdf-cross-b")
	receiptOnB, err := stB.trSvc.Create(context.Background(), b.tenantID, CreateRequest{ShipmentID: shipmentIDOnB}, 1)
	if err != nil {
		t.Fatalf("Create (tenant B): %v", err)
	}

	pdfSvcA := newTRPDFService(tx, stA)
	pdfSvcB := newTRPDFService(tx, stB)

	if _, _, err := pdfSvcA.RenderTransferNote(context.Background(), a.tenantID, 1, receiptOnB.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("RenderTransferNote(tenantA, tenantB's receipt) = %v, want ErrNotFound", err)
	}
	if _, _, err := pdfSvcB.RenderTransferNote(context.Background(), b.tenantID, 1, receiptOnB.ID); err != nil {
		t.Errorf("RenderTransferNote(tenantB, its own receipt) = %v, want success", err)
	}
}

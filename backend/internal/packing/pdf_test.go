// Phase 33C.3 tests for internal/packing's own enhanced Packing List /
// Carton Label PDFs: the DRAFT watermark, the Carton Label's A4
// fallback + Carton Sequence, document-history recording (including a
// real end-to-end immutable-snapshot proof through internal/docrender),
// and the side-effect-free guarantee (PDF generation must never mutate
// inventory/job/item/carton state) — generalizing
// TestFullPackCycle_NeverTouchesInventory's own "snapshot before/after"
// pattern to repeated PDF downloads specifically.
package packing

import (
	"bytes"
	"context"
	"testing"

	"wms-backend/internal/docrender"
	"wms-backend/internal/inventory"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// fakeDocRecorder is an in-memory docRecorder test double, mirroring
// internal/picking/pdf_test.go's own copy exactly (each PDF package
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

// TestPDFService_RenderPackingList_DraftWatermark_DraftVsFinal is the
// DRAFT watermark test: render the SAME job's Packing List once while
// still IN_PROGRESS (not yet checker-confirmed) and once after
// CheckerConfirm has completed it, and prove:
//  1. The in-progress render is recorded with is_draft = true, the
//     final one with is_draft = false — the authoritative assertion,
//     since fpdf's own PDF stream is not plaintext-searchable for
//     "DRAFT" the way a plain-text format would be (the watermark
//     renders as glyph-positioning operators inside a compressed
//     content stream, not literal ASCII bytes a Go test could
//     reasonably grep for).
//  2. The two renders' byte lengths differ — a secondary, code-path
//     signal: MarkDraft draws extra content (a rotated, alpha-blended
//     "DRAFT" string plus the save/restore transform operators
//     surrounding it) only on the draft path, and the "Report Status"
//     line's own text differs between the two ("DRAFT (not yet
//     checker-confirmed)" vs the plain status), so the two outputs can
//     never be byte-identical.
func TestPDFService_RenderPackingList_DraftWatermark_DraftVsFinal(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "draft-wm")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := job.Items[0].ID
	carton, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{PackagingType: "BOX"}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "draft-wm-1", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-draft-wm", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)
	recorder := &fakeDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	// Render #1: job is IN_PROGRESS (packed, not yet checker-confirmed).
	draftData, _, err := pdfSvc.RenderPackingList(context.Background(), a.tenantID, 1, jobID)
	if err != nil {
		t.Fatalf("RenderPackingList (draft): %v", err)
	}
	if len(draftData) == 0 || !bytes.HasPrefix(draftData, []byte("%PDF-")) {
		t.Fatal("RenderPackingList (draft) did not return a valid, non-empty PDF")
	}

	// Checker-confirm the job to COMPLETED.
	if _, err := svc.CheckerConfirm(context.Background(), a.tenantID, jobID, CheckerConfirmRequest{}, 1); err != nil {
		t.Fatalf("CheckerConfirm: %v", err)
	}

	// Render #2: job is now COMPLETED — FINAL, no watermark.
	finalData, _, err := pdfSvc.RenderPackingList(context.Background(), a.tenantID, 1, jobID)
	if err != nil {
		t.Fatalf("RenderPackingList (final): %v", err)
	}
	if len(finalData) == 0 || !bytes.HasPrefix(finalData, []byte("%PDF-")) {
		t.Fatal("RenderPackingList (final) did not return a valid, non-empty PDF")
	}

	if len(recorder.calls) != 2 {
		t.Fatalf("Record called %d times, want 2", len(recorder.calls))
	}
	if !recorder.calls[0].isDraft {
		t.Error("first render (IN_PROGRESS job) was recorded with is_draft = false, want true")
	}
	if recorder.calls[1].isDraft {
		t.Error("second render (COMPLETED job) was recorded with is_draft = true, want false")
	}

	if bytes.Equal(draftData, finalData) {
		t.Error("draft and final renders produced byte-identical PDFs — the DRAFT watermark/report-status text must change the output")
	}
}

// TestPDFService_RenderCartonLabels_SequenceAndA4Fallback proves both
// the label format (default) and the new ?format=A4 fallback render
// valid, non-empty, and DIFFERENT (different page geometry) PDFs
// against the same multi-carton job — the same content-building logic
// driving two different page sizes, per RenderCartonLabels' own doc.
func TestPDFService_RenderCartonLabels_SequenceAndA4Fallback(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "a4-fallback")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 20)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := job.Items[0].ID

	carton1, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{PackagingType: "BOX"}, 1)
	if err != nil {
		t.Fatalf("CreateCarton 1: %v", err)
	}
	carton2, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{PackagingType: "BOX"}, 1)
	if err != nil {
		t.Fatalf("CreateCarton 2: %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "a4-1", ItemID: itemID, CartonID: carton1.ID, ScannedSKU: "SKU-a4-fallback", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem 1: %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "a4-2", ItemID: itemID, CartonID: carton2.ID, ScannedSKU: "SKU-a4-fallback", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem 2: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)

	labelData, _, err := pdfSvc.RenderCartonLabels(context.Background(), a.tenantID, 1, jobID, 0, "")
	if err != nil {
		t.Fatalf("RenderCartonLabels (label format, 2 cartons -> 2 pages): %v", err)
	}
	if len(labelData) == 0 || !bytes.HasPrefix(labelData, []byte("%PDF-")) {
		t.Fatal("RenderCartonLabels (label format) did not return a valid, non-empty PDF")
	}

	a4Data, _, err := pdfSvc.RenderCartonLabels(context.Background(), a.tenantID, 1, jobID, 0, "A4")
	if err != nil {
		t.Fatalf("RenderCartonLabels (A4 fallback): %v", err)
	}
	if len(a4Data) == 0 || !bytes.HasPrefix(a4Data, []byte("%PDF-")) {
		t.Fatal("RenderCartonLabels (A4 fallback) did not return a valid, non-empty PDF")
	}
	if bytes.Equal(labelData, a4Data) {
		t.Error("label-format and A4-format renders were byte-identical — the page geometry must differ")
	}

	// A single-carton reprint must still report its real sequence
	// ("2 of 2"), not "1 of 1" — exercised here by confirming it still
	// renders without error; the sequence text itself is not asserted
	// (fpdf's content stream is not plaintext-searchable, matching this
	// codebase's established "valid PDF, correct code path" testing
	// convention for every other PDF generator).
	singleData, _, err := pdfSvc.RenderCartonLabels(context.Background(), a.tenantID, 1, jobID, carton2.ID, "")
	if err != nil {
		t.Fatalf("RenderCartonLabels (single carton, format=\"\"): %v", err)
	}
	if len(singleData) == 0 || !bytes.HasPrefix(singleData, []byte("%PDF-")) {
		t.Fatal("RenderCartonLabels (single carton) did not return a valid, non-empty PDF")
	}
}

// TestPDFService_RenderPackingList_ImmutableSnapshot_RealIntegration is
// the end-to-end "immutable historical snapshot" proof wired through
// internal/docrender for real (internal/docrender's own service_test.go
// already proves the Service/Repository layer in isolation with
// synthetic bytes; this test proves the full integration: render the
// SAME job's Packing List before and after a real data change —
// CheckerConfirm, DRAFT -> COMPLETED — and confirm re-fetching the
// EARLIER render returns the ORIGINAL (draft) bytes, never the later
// (final) ones).
func TestPDFService_RenderPackingList_ImmutableSnapshot_RealIntegration(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "snap-real")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := job.Items[0].ID
	carton, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{PackagingType: "BOX"}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "snap-real-1", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-snap-real", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)
	drSvc := docrender.NewService(docrender.NewRepository(tx), userRepo)
	pdfSvc.SetDocRender(drSvc)

	draftData, _, err := pdfSvc.RenderPackingList(context.Background(), a.tenantID, 1, jobID)
	if err != nil {
		t.Fatalf("RenderPackingList (before checker-confirm): %v", err)
	}

	if _, err := svc.CheckerConfirm(context.Background(), a.tenantID, jobID, CheckerConfirmRequest{}, 1); err != nil {
		t.Fatalf("CheckerConfirm: %v", err)
	}

	finalData, _, err := pdfSvc.RenderPackingList(context.Background(), a.tenantID, 1, jobID)
	if err != nil {
		t.Fatalf("RenderPackingList (after checker-confirm): %v", err)
	}
	if bytes.Equal(draftData, finalData) {
		t.Fatal("before/after renders were byte-identical — the test setup is not actually exercising a real data change")
	}

	history, err := drSvc.History(context.Background(), a.tenantID, docrender.TypePackingList, jobID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("History returned %d rows, want 2", len(history))
	}
	// Newest first (Repository.ListBySource's own ordering).
	earlierRow, laterRow := history[1], history[0]

	earlier, err := drSvc.Fetch(context.Background(), a.tenantID, earlierRow.ID)
	if err != nil {
		t.Fatalf("Fetch(earlier): %v", err)
	}
	if !bytes.Equal(earlier.PDFData, draftData) {
		t.Error("Fetch(earlier) did not return the ORIGINAL draft bytes")
	}
	if bytes.Equal(earlier.PDFData, finalData) {
		t.Fatal("Fetch(earlier) returned the LATER (final) render's bytes — reprint re-rendered instead of returning the immutable snapshot")
	}

	later, err := drSvc.Fetch(context.Background(), a.tenantID, laterRow.ID)
	if err != nil {
		t.Fatalf("Fetch(later): %v", err)
	}
	if !bytes.Equal(later.PDFData, finalData) {
		t.Error("Fetch(later) did not return the final render's bytes")
	}
}

// TestPDFService_RepeatedDownloads_NeverMutateState is the side-effect-
// free test the spec explicitly calls out: generate the SAME Packing
// List and Carton Label PDFs 5+ times in a row and assert zero change
// to inventory balances or the job/item/carton's own stored state —
// PDF generation must never mutate workflow state. Reuses
// TestFullPackCycle_NeverTouchesInventory's own "snapshot quantities
// before/after" pattern, generalized to repeated reads rather than one
// full pack cycle.
func TestPDFService_RepeatedDownloads_NeverMutateState(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPackingFixture(t, tx, "repeat-dl")
	seedPackingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _, pickingSvc := newPackingServiceAndRepo(tx)
	pickTaskID := createCompletedPickingTask(t, pickingSvc, a, "LOT-A", 10)
	jobID := createPackingJob(t, svc, a.tenantID, pickTaskID)
	job, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := job.Items[0].ID
	carton, err := svc.CreateCarton(context.Background(), a.tenantID, jobID, CreateCartonRequest{PackagingType: "BOX"}, 1)
	if err != nil {
		t.Fatalf("CreateCarton: %v", err)
	}
	if _, err := svc.PackItem(context.Background(), a.tenantID, jobID, PackItemRequest{
		IdempotencyKey: "repeat-dl-1", ItemID: itemID, CartonID: carton.ID, ScannedSKU: "SKU-repeat-dl", Quantity: 10,
	}, 1); err != nil {
		t.Fatalf("PackItem: %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	before, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance(before): %v", err)
	}
	jobBefore, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get(before): %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)

	for i := 0; i < 5; i++ {
		if _, _, err := pdfSvc.RenderPackingList(context.Background(), a.tenantID, 1, jobID); err != nil {
			t.Fatalf("RenderPackingList (iteration %d): %v", i, err)
		}
		if _, _, err := pdfSvc.RenderCartonLabels(context.Background(), a.tenantID, 1, jobID, 0, ""); err != nil {
			t.Fatalf("RenderCartonLabels (iteration %d): %v", i, err)
		}
	}

	after, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance(after): %v", err)
	}
	if after.Quantity != before.Quantity || after.ReservedQuantity != before.ReservedQuantity || after.HoldQuantity != before.HoldQuantity {
		t.Errorf("inventory balance changed after 5x repeated PDF downloads: before=%+v after=%+v", before, after)
	}

	jobAfter, err := svc.Get(context.Background(), a.tenantID, jobID)
	if err != nil {
		t.Fatalf("Get(after): %v", err)
	}
	if jobAfter.Status != jobBefore.Status {
		t.Errorf("job status changed after 5x repeated PDF downloads: before=%v after=%v", jobBefore.Status, jobAfter.Status)
	}
	if jobAfter.Items[0].PackedQty != jobBefore.Items[0].PackedQty || jobAfter.Items[0].Status != jobBefore.Items[0].Status {
		t.Errorf("item state changed after 5x repeated PDF downloads: before=%+v after=%+v", jobBefore.Items[0], jobAfter.Items[0])
	}
	if jobAfter.Cartons[0].Status != jobBefore.Cartons[0].Status {
		t.Errorf("carton status changed after 5x repeated PDF downloads: before=%v after=%v", jobBefore.Cartons[0].Status, jobAfter.Cartons[0].Status)
	}
}

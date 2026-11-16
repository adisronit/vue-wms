// Phase 33D.3 tests for internal/loading's own new Loading Manifest
// PDF: the DRAFT watermark, document-history recording (including a
// real end-to-end immutable-snapshot proof through internal/docrender),
// the side-effect-free guarantee (PDF generation must never mutate
// trip/shipment/carton state), and cross-tenant isolation. Mirrors
// internal/packing/pdf_test.go's own shape closely, reusing the real
// fixture/service helpers tenant_isolation_test.go already established
// in this same package.
package loading

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"wms-backend/internal/docrender"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// fakeDocRecorder is an in-memory docRecorder test double, mirroring
// internal/packing/pdf_test.go's own copy exactly (each PDF package
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

// runFullLoadingCycle creates a trip, assigns shipmentID, scans every
// one of its own planned cartons loaded, and confirms the trip —
// returning the trip's own id, now COMPLETED. Mirrors
// TestFullHappyPath_TwoShipmentsTwoPackingJobs_BothDispatchableAfterConfirm's
// own cycle shape.
func runFullLoadingCycle(t *testing.T, loadingSvc *Service, tenantID, shipmentID int64) int64 {
	t.Helper()
	trip, err := loadingSvc.CreateTrip(context.Background(), tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("runFullLoadingCycle: CreateTrip: %v", err)
	}
	resp, err := loadingSvc.AssignShipment(context.Background(), tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("runFullLoadingCycle: AssignShipment: %v", err)
	}
	for _, c := range resp.Cartons {
		if _, err := loadingSvc.ScanCarton(context.Background(), tenantID, trip.ID, ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("runFullLoadingCycle: ScanCarton(%s): %v", c.LPN, err)
		}
	}
	completed, err := loadingSvc.ConfirmTrip(context.Background(), tenantID, trip.ID, ConfirmRequest{}, 1)
	if err != nil {
		t.Fatalf("runFullLoadingCycle: ConfirmTrip: %v", err)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("runFullLoadingCycle: trip status = %v, want COMPLETED", completed.Status)
	}
	return trip.ID
}

// TestPDFService_RenderLoadingManifest_DraftWatermark_DraftVsFinal
// renders the SAME trip's Loading Manifest once while still PLANNED/
// LOADING (not yet completed) and once after ConfirmTrip, and proves:
//  1. The not-yet-completed render is recorded with is_draft = true,
//     the completed one with is_draft = false.
//  2. The two renders' byte lengths differ (status/report text differs
//     between the two, and MarkDraft draws extra watermark content only
//     on the draft path) — the same secondary signal internal/packing's
//     own equivalent test uses, since fpdf's own content stream is not
//     plaintext-searchable for "DRAFT".
func TestPDFService_RenderLoadingManifest_DraftWatermark_DraftVsFinal(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "pdf-draft-wm")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)

	trip, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	resp, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(loadingSvc, tenantRepo, userRepo)
	recorder := &fakeDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	draftData, _, err := pdfSvc.RenderLoadingManifest(context.Background(), a.tenantID, 1, trip.ID)
	if err != nil {
		t.Fatalf("RenderLoadingManifest (draft, PLANNED): %v", err)
	}
	if len(draftData) == 0 || !bytes.HasPrefix(draftData, []byte("%PDF-")) {
		t.Fatal("RenderLoadingManifest (draft) did not return a valid, non-empty PDF")
	}

	for _, c := range resp.Cartons {
		if _, err := loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("ScanCarton(%s): %v", c.LPN, err)
		}
	}
	if _, err := loadingSvc.ConfirmTrip(context.Background(), a.tenantID, trip.ID, ConfirmRequest{}, 1); err != nil {
		t.Fatalf("ConfirmTrip: %v", err)
	}

	finalData, _, err := pdfSvc.RenderLoadingManifest(context.Background(), a.tenantID, 1, trip.ID)
	if err != nil {
		t.Fatalf("RenderLoadingManifest (final, COMPLETED): %v", err)
	}
	if len(finalData) == 0 || !bytes.HasPrefix(finalData, []byte("%PDF-")) {
		t.Fatal("RenderLoadingManifest (final) did not return a valid, non-empty PDF")
	}

	if len(recorder.calls) != 2 {
		t.Fatalf("Record called %d times, want 2", len(recorder.calls))
	}
	if !recorder.calls[0].isDraft {
		t.Error("first render (PLANNED trip) was recorded with is_draft = false, want true")
	}
	if recorder.calls[1].isDraft {
		t.Error("second render (COMPLETED trip) was recorded with is_draft = true, want false")
	}
	if bytes.Equal(draftData, finalData) {
		t.Error("draft and final renders produced byte-identical PDFs — the DRAFT watermark/report-status text must change the output")
	}
}

// TestPDFService_RenderLoadingManifest_ImmutableSnapshot_RealIntegration
// is the end-to-end "immutable historical snapshot" proof wired through
// internal/docrender for real: render the SAME trip's Loading Manifest
// before and after a real data change (ConfirmTrip -> COMPLETED) and
// confirm re-fetching the EARLIER render returns the ORIGINAL (draft)
// bytes, never the later (final) ones.
func TestPDFService_RenderLoadingManifest_ImmutableSnapshot_RealIntegration(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "pdf-snap-real")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)

	trip, err := loadingSvc.CreateTrip(context.Background(), a.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}
	resp, err := loadingSvc.AssignShipment(context.Background(), a.tenantID, trip.ID, AssignShipmentRequest{ShipmentID: shipmentID}, 1)
	if err != nil {
		t.Fatalf("AssignShipment: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(loadingSvc, tenantRepo, userRepo)
	drSvc := docrender.NewService(docrender.NewRepository(tx), userRepo)
	pdfSvc.SetDocRender(drSvc)

	draftData, _, err := pdfSvc.RenderLoadingManifest(context.Background(), a.tenantID, 1, trip.ID)
	if err != nil {
		t.Fatalf("RenderLoadingManifest (before confirm): %v", err)
	}

	for _, c := range resp.Cartons {
		if _, err := loadingSvc.ScanCarton(context.Background(), a.tenantID, trip.ID, ScanCartonRequest{LPN: c.LPN}, 1); err != nil {
			t.Fatalf("ScanCarton(%s): %v", c.LPN, err)
		}
	}
	if _, err := loadingSvc.ConfirmTrip(context.Background(), a.tenantID, trip.ID, ConfirmRequest{}, 1); err != nil {
		t.Fatalf("ConfirmTrip: %v", err)
	}

	finalData, _, err := pdfSvc.RenderLoadingManifest(context.Background(), a.tenantID, 1, trip.ID)
	if err != nil {
		t.Fatalf("RenderLoadingManifest (after confirm): %v", err)
	}
	if bytes.Equal(draftData, finalData) {
		t.Fatal("before/after renders were byte-identical — the test setup is not actually exercising a real data change")
	}

	history, err := drSvc.History(context.Background(), a.tenantID, docrender.TypeLoadingManifest, trip.ID)
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

// TestPDFService_RenderLoadingManifest_RepeatedDownloads_NeverMutateState
// is the side-effect-free test this phase's own spec explicitly calls
// out: generate the SAME Loading Manifest 5+ times in a row and assert
// zero change to the trip/shipment/carton's own stored state — PDF
// generation must never mutate workflow state. Mirrors internal/
// packing's own TestPDFService_RepeatedDownloads_NeverMutateState.
func TestPDFService_RenderLoadingManifest_RepeatedDownloads_NeverMutateState(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "pdf-repeat-dl")
	seedLoadingBalance(t, tx, a, "LOT-A", 100, 0)

	loadingSvc, shippingSvc, packingSvc, pickingSvc, _ := newLoadingStack(tx)
	shipmentID := createReadyShipment(t, pickingSvc, packingSvc, shippingSvc, a.tenantID, a, "LOT-A", 10)
	tripID := runFullLoadingCycle(t, loadingSvc, a.tenantID, shipmentID)

	tripBefore, err := loadingSvc.Get(context.Background(), a.tenantID, tripID)
	if err != nil {
		t.Fatalf("Get(before): %v", err)
	}
	shipmentBefore, err := shippingSvc.Get(context.Background(), a.tenantID, shipmentID)
	if err != nil {
		t.Fatalf("shipping Get(before): %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(loadingSvc, tenantRepo, userRepo)

	for i := 0; i < 5; i++ {
		if _, _, err := pdfSvc.RenderLoadingManifest(context.Background(), a.tenantID, 1, tripID); err != nil {
			t.Fatalf("RenderLoadingManifest (iteration %d): %v", i, err)
		}
	}

	tripAfter, err := loadingSvc.Get(context.Background(), a.tenantID, tripID)
	if err != nil {
		t.Fatalf("Get(after): %v", err)
	}
	if tripAfter.Status != tripBefore.Status || tripAfter.LoadedCount != tripBefore.LoadedCount || tripAfter.ShortCount != tripBefore.ShortCount {
		t.Errorf("trip state changed after 5x repeated PDF downloads: before=%+v after=%+v", tripBefore, tripAfter)
	}
	if len(tripAfter.Cartons) != len(tripBefore.Cartons) {
		t.Fatalf("carton count changed after 5x repeated PDF downloads: before=%d after=%d", len(tripBefore.Cartons), len(tripAfter.Cartons))
	}
	for i := range tripBefore.Cartons {
		if tripAfter.Cartons[i].Status != tripBefore.Cartons[i].Status {
			t.Errorf("carton %d status changed after 5x repeated PDF downloads: before=%v after=%v", i, tripBefore.Cartons[i].Status, tripAfter.Cartons[i].Status)
		}
	}

	shipmentAfter, err := shippingSvc.Get(context.Background(), a.tenantID, shipmentID)
	if err != nil {
		t.Fatalf("shipping Get(after): %v", err)
	}
	if shipmentAfter.Status != shipmentBefore.Status {
		t.Errorf("shipment status changed after 5x repeated PDF downloads: before=%v after=%v", shipmentBefore.Status, shipmentAfter.Status)
	}
}

// TestPDFService_RenderLoadingManifest_CrossTenant_NotFound proves the
// Loading Manifest generator inherits Service.Get's own tenant-scoped
// IDOR guard — a cross-tenant trip id is rejected as ErrNotFound exactly
// like a nonexistent one, never revealing whether the id exists in
// another tenant.
func TestPDFService_RenderLoadingManifest_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newLoadingFixture(t, tx, "pdf-cross-a")
	b := newLoadingFixture(t, tx, "pdf-cross-b")
	loadingSvc, _, _, _, _ := newLoadingStack(tx)

	trip, err := loadingSvc.CreateTrip(context.Background(), b.tenantID, CreateRequest{}, 1)
	if err != nil {
		t.Fatalf("CreateTrip: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(loadingSvc, tenantRepo, userRepo)

	if _, _, err := pdfSvc.RenderLoadingManifest(context.Background(), a.tenantID, 1, trip.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("RenderLoadingManifest(tenantA, tenantB's trip) = %v, want ErrNotFound", err)
	}
	if _, _, err := pdfSvc.RenderLoadingManifest(context.Background(), b.tenantID, 1, trip.ID); err != nil {
		t.Errorf("RenderLoadingManifest(tenantB, its own trip) = %v, want success", err)
	}
}

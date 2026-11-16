// Phase 33D.3 tests for internal/shipping's own new Delivery Note PDF:
// the DRAFT watermark, document-history recording (including a real
// end-to-end immutable-snapshot proof through internal/docrender), the
// side-effect-free guarantee (PDF generation must never mutate
// shipment/item/inventory state), and cross-tenant isolation. Mirrors
// internal/packing/pdf_test.go's own shape closely, reusing the real
// fixture/service helpers tenant_isolation_test.go already established
// in this same package.
package shipping

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"wms-backend/internal/docrender"
	"wms-backend/internal/inventory"
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

// dispatchShipment runs the real post-33D.2 path to DISPATCHED: MarkLoaded
// (simulating a completed Loading cycle, the same shortcut internal/
// shipping's own tenant_isolation_test.go already uses for this exact
// reason) followed by Confirm(DISPATCH).
func dispatchShipment(t *testing.T, svc *Service, tenantID, shipmentID int64) *Response {
	t.Helper()
	if err := svc.MarkLoaded(context.Background(), tenantID, shipmentID, 1); err != nil {
		t.Fatalf("dispatchShipment: MarkLoaded: %v", err)
	}
	resp, err := svc.Confirm(context.Background(), tenantID, shipmentID, ConfirmRequest{
		IdempotencyKey: "dispatch-pdf-test", Action: "DISPATCH",
	}, 1)
	if err != nil {
		t.Fatalf("dispatchShipment: Confirm(DISPATCH): %v", err)
	}
	if resp.Status != StatusDispatched {
		t.Fatalf("dispatchShipment: status = %v, want DISPATCHED", resp.Status)
	}
	return resp
}

// TestPDFService_RenderDeliveryNote_DraftWatermark_DraftVsFinal renders
// the SAME shipment's Delivery Note once while still READY (not yet
// dispatched) and once after DISPATCH, and proves:
//  1. The not-yet-dispatched render is recorded with is_draft = true,
//     the dispatched one with is_draft = false.
//  2. The two renders' byte lengths differ (status/report text differs
//     between the two, and MarkDraft draws extra watermark content only
//     on the draft path) — the same secondary signal internal/packing's
//     own equivalent test uses, since fpdf's own content stream is not
//     plaintext-searchable for "DRAFT".
func TestPDFService_RenderDeliveryNote_DraftWatermark_DraftVsFinal(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "pdf-draft-wm")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)
	carrier, tracking := "DHL", "DHL-PDF-DRAFT"
	shipment, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		PackingJobID: jobID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, packingSvc, tenantRepo, userRepo)
	recorder := &fakeDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	draftData, _, err := pdfSvc.RenderDeliveryNote(context.Background(), a.tenantID, 1, shipment.ID)
	if err != nil {
		t.Fatalf("RenderDeliveryNote (draft, READY): %v", err)
	}
	if len(draftData) == 0 || !bytes.HasPrefix(draftData, []byte("%PDF-")) {
		t.Fatal("RenderDeliveryNote (draft) did not return a valid, non-empty PDF")
	}

	dispatchShipment(t, svc, a.tenantID, shipment.ID)

	finalData, _, err := pdfSvc.RenderDeliveryNote(context.Background(), a.tenantID, 1, shipment.ID)
	if err != nil {
		t.Fatalf("RenderDeliveryNote (final, DISPATCHED): %v", err)
	}
	if len(finalData) == 0 || !bytes.HasPrefix(finalData, []byte("%PDF-")) {
		t.Fatal("RenderDeliveryNote (final) did not return a valid, non-empty PDF")
	}

	if len(recorder.calls) != 2 {
		t.Fatalf("Record called %d times, want 2", len(recorder.calls))
	}
	if !recorder.calls[0].isDraft {
		t.Error("first render (READY shipment) was recorded with is_draft = false, want true")
	}
	if recorder.calls[1].isDraft {
		t.Error("second render (DISPATCHED shipment) was recorded with is_draft = true, want false")
	}
	if bytes.Equal(draftData, finalData) {
		t.Error("draft and final renders produced byte-identical PDFs — the DRAFT watermark/report-status text must change the output")
	}
}

// TestPDFService_RenderDeliveryNote_ImmutableSnapshot_RealIntegration is
// the end-to-end "immutable historical snapshot" proof wired through
// internal/docrender for real: render the SAME shipment's Delivery Note
// before and after a real data change (DISPATCH) and confirm
// re-fetching the EARLIER render returns the ORIGINAL (draft) bytes,
// never the later (final) ones.
func TestPDFService_RenderDeliveryNote_ImmutableSnapshot_RealIntegration(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "pdf-snap-real")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)
	carrier, tracking := "DHL", "DHL-PDF-SNAP"
	shipment, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		PackingJobID: jobID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, packingSvc, tenantRepo, userRepo)
	drSvc := docrender.NewService(docrender.NewRepository(tx), userRepo)
	pdfSvc.SetDocRender(drSvc)

	draftData, _, err := pdfSvc.RenderDeliveryNote(context.Background(), a.tenantID, 1, shipment.ID)
	if err != nil {
		t.Fatalf("RenderDeliveryNote (before dispatch): %v", err)
	}

	dispatchShipment(t, svc, a.tenantID, shipment.ID)

	finalData, _, err := pdfSvc.RenderDeliveryNote(context.Background(), a.tenantID, 1, shipment.ID)
	if err != nil {
		t.Fatalf("RenderDeliveryNote (after dispatch): %v", err)
	}
	if bytes.Equal(draftData, finalData) {
		t.Fatal("before/after renders were byte-identical — the test setup is not actually exercising a real data change")
	}

	history, err := drSvc.History(context.Background(), a.tenantID, docrender.TypeDeliveryNote, shipment.ID)
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

// TestPDFService_RenderDeliveryNote_RepeatedDownloads_NeverMutateState is
// the side-effect-free test this phase's own spec explicitly calls out:
// generate the SAME Delivery Note 5+ times in a row and assert zero
// change to inventory balances or the shipment/item's own stored state
// — PDF generation must never mutate workflow state. Mirrors internal/
// packing's own TestPDFService_RepeatedDownloads_NeverMutateState.
func TestPDFService_RenderDeliveryNote_RepeatedDownloads_NeverMutateState(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "pdf-repeat-dl")
	seedShippingBalance(t, tx, a, "LOT-A", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, a.tenantID, a, "LOT-A", 10)
	carrier, tracking := "DHL", "DHL-PDF-REPEAT"
	shipment, err := svc.Create(context.Background(), a.tenantID, CreateRequest{
		PackingJobID: jobID, Carrier: &carrier, TrackingNo: &tracking,
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	invRepo := inventory.NewRepository(tx)
	before, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance(before): %v", err)
	}
	shipmentBefore, err := svc.Get(context.Background(), a.tenantID, shipment.ID)
	if err != nil {
		t.Fatalf("Get(before): %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, packingSvc, tenantRepo, userRepo)

	for i := 0; i < 5; i++ {
		if _, _, err := pdfSvc.RenderDeliveryNote(context.Background(), a.tenantID, 1, shipment.ID); err != nil {
			t.Fatalf("RenderDeliveryNote (iteration %d): %v", i, err)
		}
	}

	after, err := invRepo.FindBalance(context.Background(), a.tenantID, a.productID, a.locationID, "LOT-A")
	if err != nil {
		t.Fatalf("FindBalance(after): %v", err)
	}
	if after.Quantity != before.Quantity || after.ReservedQuantity != before.ReservedQuantity || after.HoldQuantity != before.HoldQuantity {
		t.Errorf("inventory balance changed after 5x repeated PDF downloads: before=%+v after=%+v", before, after)
	}

	shipmentAfter, err := svc.Get(context.Background(), a.tenantID, shipment.ID)
	if err != nil {
		t.Fatalf("Get(after): %v", err)
	}
	if shipmentAfter.Status != shipmentBefore.Status {
		t.Errorf("shipment status changed after 5x repeated PDF downloads: before=%v after=%v", shipmentBefore.Status, shipmentAfter.Status)
	}
	if shipmentAfter.DispatchedAt != nil || shipmentBefore.DispatchedAt != nil {
		t.Errorf("DispatchedAt changed after 5x repeated PDF downloads: before=%v after=%v", shipmentBefore.DispatchedAt, shipmentAfter.DispatchedAt)
	}
	if len(shipmentAfter.Items) != len(shipmentBefore.Items) || shipmentAfter.Items[0].Qty != shipmentBefore.Items[0].Qty {
		t.Errorf("item state changed after 5x repeated PDF downloads: before=%+v after=%+v", shipmentBefore.Items, shipmentAfter.Items)
	}
}

// TestPDFService_RenderDeliveryNote_CrossTenant_NotFound proves the
// Delivery Note generator inherits Service.Get's own tenant-scoped IDOR
// guard — a cross-tenant shipment id is rejected as ErrNotFound exactly
// like a nonexistent one, never revealing whether the id exists in
// another tenant.
func TestPDFService_RenderDeliveryNote_CrossTenant_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newShippingFixture(t, tx, "pdf-cross-a")
	b := newShippingFixture(t, tx, "pdf-cross-b")
	seedShippingBalance(t, tx, b, "LOT-B", 100, 0)

	pickingSvc := newPickingServiceForShipping(tx)
	packingSvc := newPackingServiceForShipping(tx, pickingSvc)
	svc, _ := newShippingServiceAndRepo(tx)
	jobID := createCompletedPackingJob(t, pickingSvc, packingSvc, b.tenantID, b, "LOT-B", 10)
	shipment, err := svc.Create(context.Background(), b.tenantID, CreateRequest{PackingJobID: jobID}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, packingSvc, tenantRepo, userRepo)

	if _, _, err := pdfSvc.RenderDeliveryNote(context.Background(), a.tenantID, 1, shipment.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("RenderDeliveryNote(tenantA, tenantB's shipment) = %v, want ErrNotFound", err)
	}
	if _, _, err := pdfSvc.RenderDeliveryNote(context.Background(), b.tenantID, 1, shipment.ID); err != nil {
		t.Errorf("RenderDeliveryNote(tenantB, its own shipment) = %v, want success", err)
	}
}

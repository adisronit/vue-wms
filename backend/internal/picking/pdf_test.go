package picking

import (
	"bytes"
	"context"
	"testing"

	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// TestPDFService_RenderPickList_ProducesValidPDF is Phase 33B's own
// smoke test for the Pick List document — proves RenderPickList (the
// barcode/QR-per-line generator) runs end to end against a real task
// and lines and returns well-formed, non-empty PDF bytes. Does not
// assert on the PDF's internal layout (no other PDF generator in this
// codebase does either) — only that generation succeeds and the result
// is a real PDF.
func TestPDFService_RenderPickList_ProducesValidPDF(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "pdf-smoke")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)
	generatedByUserID := seedPickerUser(t, tx, a, "pdf-smoke")

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)

	data, filename, err := pdfSvc.RenderPickList(context.Background(), a.tenantID, generatedByUserID, taskID)
	if err != nil {
		t.Fatalf("RenderPickList: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("RenderPickList returned empty PDF bytes")
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Error("RenderPickList did not return a valid PDF header")
	}
	if filename == "" {
		t.Error("RenderPickList returned an empty filename")
	}
}

// fakeDocRecorder is an in-memory docRecorder test double — records
// every call it receives so a test can assert on document_type/
// is_draft/source without needing the real internal/docrender package
// (which would pull in its own DB dependency this package's own tests
// otherwise have no reason to need).
type fakeDocRecorder struct {
	calls []fakeDocRenderCall
}

type fakeDocRenderCall struct {
	tenantID                              int64
	documentType, documentNo, sourceTable string
	sourceID, userID                      int64
	isDraft                               bool
	pdfData                               []byte
}

func (f *fakeDocRecorder) Record(_ context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error {
	f.calls = append(f.calls, fakeDocRenderCall{tenantID, documentType, documentNo, sourceTable, sourceID, userID, isDraft, pdfData})
	return nil
}

// TestPDFService_RenderPickList_RecordsDocumentHistory proves
// RenderPickList calls docrender's own Record (via SetDocRender) with
// the correct document type/no/source after a successful render — the
// additive integration point Phase 33C.3 adds to every pre-existing PDF
// generator in this codebase.
func TestPDFService_RenderPickList_RecordsDocumentHistory(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "docrender")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)
	generatedByUserID := seedPickerUser(t, tx, a, "docrender")

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)
	recorder := &fakeDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	if _, _, err := pdfSvc.RenderPickList(context.Background(), a.tenantID, generatedByUserID, taskID); err != nil {
		t.Fatalf("RenderPickList: %v", err)
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("Record called %d times, want 1", len(recorder.calls))
	}
	call := recorder.calls[0]
	if call.documentType != "PICK_LIST" || call.sourceTable != "picking_tasks" || call.sourceID != taskID || call.isDraft {
		t.Errorf("Record call = %+v, want PICK_LIST/picking_tasks/%d/isDraft=false", call, taskID)
	}
	if len(call.pdfData) == 0 {
		t.Error("Record was called with empty pdfData")
	}
}

// TestPDFService_RenderConfirmationReport_TerminalTask proves the
// Picking Confirmation Report renders a valid PDF against a COMPLETED
// task and records is_draft = false (the report is FINAL once the task
// itself has reached a terminal status).
func TestPDFService_RenderConfirmationReport_TerminalTask(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "confirm-report")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)
	pickerID := seedPickerUser(t, tx, a, "confirm-report")

	task, err := svc.Get(context.Background(), a.tenantID, taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	itemID := task.Items[0].ID

	if _, err := svc.AssignPicker(context.Background(), a.tenantID, taskID, pickerID, 1); err != nil {
		t.Fatalf("AssignPicker: %v", err)
	}
	if _, err := svc.ScanConfirm(context.Background(), a.tenantID, taskID, itemID, ScanRequest{
		LocationCode: task.Items[0].LocationCode, SKU: task.Items[0].SKU, LotNo: "LOT-A",
	}, 1); err != nil {
		t.Fatalf("ScanConfirm: %v", err)
	}
	resp, err := svc.Confirm(context.Background(), a.tenantID, taskID, ConfirmRequest{
		IdempotencyKey: "confirm-report-1",
		Items:          []ConfirmItemRequest{{ItemID: itemID, Action: "PICKED", Quantity: 10}},
	}, 1)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Fatalf("task status = %v, want COMPLETED", resp.Status)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)
	recorder := &fakeDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	data, filename, err := pdfSvc.RenderConfirmationReport(context.Background(), a.tenantID, 1, taskID)
	if err != nil {
		t.Fatalf("RenderConfirmationReport: %v", err)
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Error("RenderConfirmationReport did not return a valid, non-empty PDF")
	}
	if filename == "" {
		t.Error("RenderConfirmationReport returned an empty filename")
	}
	if len(recorder.calls) != 1 || recorder.calls[0].documentType != "PICKING_CONFIRMATION" || recorder.calls[0].isDraft {
		t.Errorf("Record call = %+v, want PICKING_CONFIRMATION/isDraft=false (task is terminal)", recorder.calls)
	}
}

// TestPDFService_RenderConfirmationReport_NonTerminalTask proves the
// report can still be generated against a PENDING (not yet terminal)
// task — not hard-blocked, per this method's own documented choice —
// and is recorded with is_draft = true, the honest "IN PROGRESS"
// signal.
func TestPDFService_RenderConfirmationReport_NonTerminalTask(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newPickingFixture(t, tx, "cr-wip")
	seedPickingBalance(t, tx, a, "LOT-A", 100, 0)

	svc, _ := newPickingServiceAndRepo(tx)
	taskID := createTask(t, svc, a.tenantID, a, "LOT-A", 10)

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := NewPDFService(svc, tenantRepo, userRepo)
	recorder := &fakeDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	data, _, err := pdfSvc.RenderConfirmationReport(context.Background(), a.tenantID, 1, taskID)
	if err != nil {
		t.Fatalf("RenderConfirmationReport: %v", err)
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Error("RenderConfirmationReport did not return a valid, non-empty PDF for a non-terminal task")
	}
	if len(recorder.calls) != 1 || !recorder.calls[0].isDraft {
		t.Errorf("Record call = %+v, want isDraft=true (task is still PENDING)", recorder.calls)
	}
}

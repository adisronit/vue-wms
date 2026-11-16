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

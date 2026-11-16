// Phase 33C.3 tests for the new Consolidated Batch Picking List PDF.
//
// Phase 33D.1: moved to the external "pickingbatch_test" package
// alongside service_test.go/sortation_test.go — see service_test.go's
// own doc for why.
package pickingbatch_test

import (
	"bytes"
	"context"
	"testing"

	"wms-backend/internal/pickingbatch"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// TestPDFService_RenderConsolidatedPickList_ProducesValidPDF reuses
// sortation_test.go's own buildSO001SO002SO003Batch fixture (a
// CONFIRMED batch, picked all the way to COMPLETED) — the exact
// scenario this document is meant to be printed for.
func TestPDFService_RenderConsolidatedPickList_ProducesValidPDF(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, f, batchID, _, _, _, _, _ := buildSO001SO002SO003Batch(t, tx, "pdf-smoke")

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := pickingbatch.NewPDFService(st.batchSvc, st.pickingSvc, tenantRepo, userRepo)

	data, filename, err := pdfSvc.RenderConsolidatedPickList(context.Background(), f.tenantID, 1, batchID)
	if err != nil {
		t.Fatalf("RenderConsolidatedPickList: %v", err)
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("RenderConsolidatedPickList did not return a valid, non-empty PDF")
	}
	if filename == "" {
		t.Error("RenderConsolidatedPickList returned an empty filename")
	}
}

// TestPDFService_RenderConsolidatedPickList_RejectsDraftBatch proves a
// still-DRAFT batch (no real picking task yet) cannot be rendered.
func TestPDFService_RenderConsolidatedPickList_RejectsDraftBatch(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newFixture(t, tx, "pdf-draft")
	seedBalance(t, tx, f, "LOT-X", 100)
	st := newStack(tx)

	r1 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "A", 20)
	r2 := createReleasedOrder(t, st.orderSvc, f.tenantID, f, "B", 30)

	draft, err := st.batchSvc.Create(context.Background(), f.tenantID, pickingbatch.CreateRequest{
		WarehouseID: f.warehouseID, OrderIDs: []int64{r1.ID, r2.ID},
	}, 1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := pickingbatch.NewPDFService(st.batchSvc, st.pickingSvc, tenantRepo, userRepo)

	if _, _, err := pdfSvc.RenderConsolidatedPickList(context.Background(), f.tenantID, 1, draft.ID); err != pickingbatch.ErrBatchNotConfirmed {
		t.Fatalf("RenderConsolidatedPickList(DRAFT batch) error = %v, want pickingbatch.ErrBatchNotConfirmed", err)
	}
}

// TestPDFService_RenderConsolidatedPickList_RecordsDocumentHistory
// proves the Record call fires with the correct document type/source.
func TestPDFService_RenderConsolidatedPickList_RecordsDocumentHistory(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	st, f, batchID, taskID, _, _, _, _ := buildSO001SO002SO003Batch(t, tx, "pdf-record")

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)
	pdfSvc := pickingbatch.NewPDFService(st.batchSvc, st.pickingSvc, tenantRepo, userRepo)
	recorder := &fakeBatchDocRecorder{}
	pdfSvc.SetDocRender(recorder)

	if _, _, err := pdfSvc.RenderConsolidatedPickList(context.Background(), f.tenantID, 1, batchID); err != nil {
		t.Fatalf("RenderConsolidatedPickList: %v", err)
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("Record called %d times, want 1", len(recorder.calls))
	}
	call := recorder.calls[0]
	if call.documentType != "PICKING_BATCH_LIST" || call.sourceTable != "picking_batches" || call.sourceID != batchID || call.isDraft {
		t.Errorf("Record call = %+v, want PICKING_BATCH_LIST/picking_batches/%d/isDraft=false", call, batchID)
	}
	_ = taskID
}

type fakeBatchDocRecorder struct {
	calls []fakeBatchDocRenderCall
}

type fakeBatchDocRenderCall struct {
	documentType, sourceTable string
	sourceID                  int64
	isDraft                   bool
}

func (f *fakeBatchDocRecorder) Record(_ context.Context, _ int64, documentType, _ string, sourceTable string, sourceID, _ int64, isDraft bool, _ []byte) error {
	f.calls = append(f.calls, fakeBatchDocRenderCall{documentType, sourceTable, sourceID, isDraft})
	return nil
}

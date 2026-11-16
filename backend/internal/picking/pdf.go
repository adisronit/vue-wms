package picking

import (
	"context"
	"fmt"
	"time"

	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// tenantNamer and userNamer are the narrow read-only slices this
// package's own PDF generator needs — the same copies internal/
// relocation's and internal/order's own PDFService each declare locally
// rather than importing one another's package for it.
type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// docRecorder is the narrow slice of internal/docrender's own Service
// this package needs to persist an immutable snapshot of each rendered
// PDF — satisfied structurally by *docrender.Service, wired in via
// SetDocRender (not the constructor, the same "optional dependency set
// after construction" convention SetUsers/SetBatches already
// established elsewhere) so every existing NewPDFService call site
// (including every test building one directly) keeps compiling
// unchanged. Nil-checked at every call site: a *PDFService never wired
// up with SetDocRender behaves exactly as it did before Phase 33C.3.
type docRecorder interface {
	Record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error
}

// PDFService composes Picking's own Service (for Get — the Pick List is
// just that same task detail, rendered) with the tenant/user display-
// name lookups every generated PDF's header/footer needs.
type PDFService struct {
	picking   *Service
	tenants   tenantNamer
	users     userNamer
	docrender docRecorder
}

func NewPDFService(pickingSvc *Service, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{picking: pickingSvc, tenants: tenants, users: users}
}

// SetDocRender wires the document-history recorder — see docRecorder's
// own doc.
func (s *PDFService) SetDocRender(d docRecorder) {
	s.docrender = d
}

// record is a best-effort call into docrender.Service.Record: a failure
// to WRITE the history row must never fail the PDF response itself (the
// caller already has their document) — this package has no logger
// dependency of its own to report the failure through, so it is simply
// swallowed, the same tradeoff documented on every other package's own
// pdf.go in this phase. s.docrender == nil (not wired via SetDocRender,
// e.g. an older test double) is a silent no-op, not an error.
func (s *PDFService) record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) {
	if s.docrender == nil {
		return
	}
	_ = s.docrender.Record(ctx, tenantID, documentType, documentNo, sourceTable, sourceID, userID, isDraft, pdfData)
}

func (s *PDFService) meta(ctx context.Context, tenantID, userID int64, title, docNo string) (pdfexport.Meta, error) {
	t, err := s.tenants.FindByID(ctx, tenantID)
	if err != nil {
		return pdfexport.Meta{}, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return pdfexport.Meta{}, err
	}
	return pdfexport.Meta{
		CompanyName: t.Name,
		DocTitle:    title,
		DocNo:       docNo,
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	}, nil
}

// ScanPayload builds the per-line "location|product|lot" string a
// warehouse operator's scanner is meant to read back and compare
// against via POST /picking/{id}/items/{itemId}/scan — the exact three
// fields Service.ScanConfirm validates, in the same order, pipe-
// separated so a single QR payload carries all three without needing a
// structured format the picker's scanner app would have to parse.
// Exported (Phase 33C.3) so internal/pickingbatch's own Consolidated
// Batch Picking List can reuse the exact same pick-face scan convention
// rather than reinventing it — pickingbatch's consolidated lines are
// the same location|SKU|lot identity, just before they have been
// confirmed into real picking_task_items rows.
func ScanPayload(locationCode, sku, lotNo string) string {
	return fmt.Sprintf("%s|%s|%s", locationCode, sku, lotNo)
}

// RenderPickList builds the Pick List / Picking Work Order document —
// header (work order no., source order no., customer/destination,
// warehouse, priority/due date, assigned picker) plus its lines
// (SKU/barcode/product/source location/lot/expiry/requested/allocated/
// picked qty), a Code128 barcode encoding the pick number itself (for a
// supervisor to scan the whole work order), and one QR code per line
// encoding that line's own location|product|lot (for the picker to
// scan-confirm against as they work it). Read-only: Service.Get never
// mutates anything, so printing/reprinting is always safe regardless of
// the task's current status.
func (s *PDFService) RenderPickList(ctx context.Context, tenantID, userID, taskID int64) ([]byte, string, error) {
	task, err := s.picking.Get(ctx, tenantID, taskID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Pick List / Picking Work Order", task.PickNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	assignedPicker := csvexport.FormatString(task.AssignedPickerName)
	dueDate := ""
	if task.DueDate != nil {
		dueDate = task.DueDate.Format("2006-01-02")
	}
	shipTo := csvexport.FormatString(task.ShipToName)
	if task.ShipToAddress != nil {
		shipTo = fmt.Sprintf("%s — %s", shipTo, *task.ShipToAddress)
	}

	doc.KeyValueGrid([][2]string{
		{"Pick No.", task.PickNo},
		{"Source Order No.", task.OrderNo},
		{"Status", string(task.Status)},
		{"Priority", string(task.Priority)},
		{"Warehouse", task.WarehouseCode},
		{"Customer", csvexport.FormatString(task.Customer)},
		{"Ship To / Destination", shipTo},
		{"Due Date", dueDate},
		{"Assigned Picker", assignedPicker},
		{"Remark", csvexport.FormatString(task.Remark)},
	}, 2)

	// The work order's own barcode — one scan identifies the whole task,
	// e.g. at a staging-lane handoff.
	doc.SectionTitle("Work Order Barcode")
	if err := doc.Barcode(task.PickNo, 12, doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)

	doc.SectionTitle("Pick Lines")
	rows := make([][]string, 0, len(task.Items))
	for _, item := range task.Items {
		expiry := ""
		if item.ExpiryDate != nil {
			expiry = item.ExpiryDate.Format("2006-01-02")
		}
		rows = append(rows, []string{
			item.SKU, item.ProductName, item.LocationCode, item.LotNo, expiry,
			csvexport.FormatFloat(item.OrderedQty), csvexport.FormatFloat(item.PickedQty), item.UOM, string(item.Status),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 20}, {Header: "Product", Width: 36}, {Header: "Source Loc.", Width: 20, Align: "C"},
		{Header: "Lot No.", Width: 20}, {Header: "Expiry", Width: 16, Align: "C"},
		{Header: "Req./Alloc. Qty", Width: 22, Align: "R"}, {Header: "Picked Qty", Width: 18, Align: "R"},
		{Header: "UOM", Width: 12, Align: "C"}, {Header: "Status", Width: 22, Align: "C"},
	}, rows)

	// One QR per line, encoding location|product|lot for the picker's
	// scanner — matching POST /picking/{id}/items/{itemId}/scan's own
	// validated fields exactly (see scanPayload's own doc).
	doc.SectionTitle("Scan Codes (per line: location | SKU | lot)")
	for _, item := range task.Items {
		payload := ScanPayload(item.LocationCode, item.SKU, item.LotNo)
		y := doc.CursorY()
		if err := doc.QRCode(payload, 12, y, 20); err != nil {
			return nil, "", err
		}
		doc.QRCaption(20, item.SKU+" @ "+item.LocationCode)
	}

	doc.SignatureBlock([]string{"Picker", "Warehouse Supervisor"})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "PICK_LIST", task.PickNo, "picking_tasks", taskID, userID, false, data)
	return data, fmt.Sprintf("PickList_%s.pdf", task.PickNo), nil
}

// RenderConfirmationReport builds the Picking Confirmation Report — a
// post-completion report for one task: ordered vs picked quantity per
// line (with shortage qty/reason where a line was flagged SHORT), a
// scan-verification summary derived cheaply from picking_scan_log
// (Phase 33B's own audit trail — see Repository.FindScanLogsByTask's
// own doc), and picker/checker signature fields.
//
// Generation is NOT hard-blocked before the task reaches a terminal
// status (COMPLETED/SHORT_PICK) — a supervisor mid-shift may
// legitimately want to see where a task currently stands. Instead, a
// report generated against a non-terminal task is honestly marked "IN
// PROGRESS" in its own header field (not a diagonal watermark the way
// Packing's own DRAFT marker is — this report is informational, not a
// document a customer/carrier ever receives, so a text field is
// sufficient) and recorded with docrender's own is_draft = true, the
// same semantic Packing's own draft flag carries. Read-only throughout:
// Service.Get never mutates anything, so generating/reprinting this
// report is always safe regardless of the task's current status.
func (s *PDFService) RenderConfirmationReport(ctx context.Context, tenantID, userID, taskID int64) ([]byte, string, error) {
	task, err := s.picking.Get(ctx, tenantID, taskID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Picking Confirmation Report", task.PickNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	terminal := task.Status == StatusCompleted || task.Status == StatusShortPick
	reportStatus := "FINAL"
	if !terminal {
		reportStatus = "IN PROGRESS — generated before the task reached a terminal status; quantities may still change"
	}

	doc.KeyValueGrid([][2]string{
		{"Pick No.", task.PickNo},
		{"Source Order No.", task.OrderNo},
		{"Task Status", string(task.Status)},
		{"Report Status", reportStatus},
		{"Warehouse", task.WarehouseCode},
		{"Assigned Picker", csvexport.FormatString(task.AssignedPickerName)},
		{"Completed At", csvexport.FormatTime(task.CompletedAt)},
	}, 2)

	doc.SectionTitle("Line Results — Ordered vs. Picked")
	rows := make([][]string, 0, len(task.Items))
	for _, item := range task.Items {
		shortQty := ""
		if item.ShortQty != nil {
			shortQty = csvexport.FormatFloat(*item.ShortQty)
		}
		rows = append(rows, []string{
			item.SKU, item.ProductName, item.LocationCode, item.LotNo,
			csvexport.FormatFloat(item.OrderedQty), csvexport.FormatFloat(item.PickedQty),
			shortQty, csvexport.FormatString(item.ShortReason), string(item.Status),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 16}, {Header: "Product", Width: 28}, {Header: "Location", Width: 16, Align: "C"},
		{Header: "Lot No.", Width: 16},
		{Header: "Ordered", Width: 12, Align: "R"}, {Header: "Picked", Width: 12, Align: "R"},
		{Header: "Short Qty", Width: 12, Align: "R"}, {Header: "Short Reason", Width: 20, Align: "C"}, {Header: "Status", Width: 14, Align: "C"},
	}, rows)

	logs, err := s.picking.repo.FindScanLogsByTask(ctx, tenantID, taskID)
	if err != nil {
		return nil, "", err
	}
	matched, mismatched := 0, 0
	for _, l := range logs {
		if l.Result == ScanResultMatch {
			matched++
		} else {
			mismatched++
		}
	}
	doc.SectionTitle("Scan Verification Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Scans", fmt.Sprintf("%d", len(logs))},
		{"Matched", fmt.Sprintf("%d", matched)},
		{"Mismatched", fmt.Sprintf("%d", mismatched)},
	}, 3)
	scanRows := make([][]string, 0, len(logs))
	for _, l := range logs {
		scanRows = append(scanRows, []string{
			l.ScannedLocationCode, l.ScannedSKU, l.ScannedLotNo, string(l.Result),
			csvexport.FormatString(l.MismatchDetail), csvexport.FormatTimeValue(l.CreatedAt),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "Scanned Location", Width: 28, Align: "C"}, {Header: "Scanned SKU", Width: 28},
		{Header: "Scanned Lot", Width: 24}, {Header: "Result", Width: 20, Align: "C"},
		{Header: "Mismatch Detail", Width: 40}, {Header: "When (UTC)", Width: 46, Align: "C"},
	}, scanRows)

	doc.SignatureBlock([]string{"Picker", "Checker"})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "PICKING_CONFIRMATION", task.PickNo, "picking_tasks", taskID, userID, !terminal, data)
	return data, fmt.Sprintf("PickingConfirmation_%s.pdf", task.PickNo), nil
}

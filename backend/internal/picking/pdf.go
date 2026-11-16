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

// PDFService composes Picking's own Service (for Get — the Pick List is
// just that same task detail, rendered) with the tenant/user display-
// name lookups every generated PDF's header/footer needs.
type PDFService struct {
	picking *Service
	tenants tenantNamer
	users   userNamer
}

func NewPDFService(pickingSvc *Service, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{picking: pickingSvc, tenants: tenants, users: users}
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

// scanPayload builds the per-line "location|product|lot" string a
// warehouse operator's scanner is meant to read back and compare
// against via POST /picking/{id}/items/{itemId}/scan — the exact three
// fields Service.ScanConfirm validates, in the same order, pipe-
// separated so a single QR payload carries all three without needing a
// structured format the picker's scanner app would have to parse.
func scanPayload(locationCode, sku, lotNo string) string {
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
		payload := scanPayload(item.LocationCode, item.SKU, item.LotNo)
		y := doc.CursorY()
		if err := doc.QRCode(payload, 12, y, 20); err != nil {
			return nil, "", err
		}
		doc.QRCaption(20, item.SKU+" @ "+item.LocationCode)
	}

	doc.SignatureBlock([]string{"Picker", "Warehouse Supervisor"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("PickList_%s.pdf", task.PickNo), err
}

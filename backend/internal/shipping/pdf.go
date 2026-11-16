// Phase 33D.3 -- Delivery Note. One document per shipments row, the
// final document in the outbound chain (Order -> Pick List -> Packing
// List -> Delivery Note). Reuses exactly the same PDF/reprint
// infrastructure 33C.3 already built (internal/pdfexport,
// internal/docrender) -- no new primitive is added here.
package shipping

import (
	"context"
	"fmt"
	"time"

	"wms-backend/internal/packing"
	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// tenantNamer and userNamer are the narrow read-only slices this
// package's own PDF generator needs -- the same local copies internal/
// packing's/internal/pickingbatch's own PDFService each declare rather
// than importing one another's package for it.
type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// packingJobLookup is the narrow, read-only slice of internal/packing's
// own Service this package's PDF generator needs, beyond the Service's
// own existing packingChecker dependency -- to resolve the Delivery
// Note's own warehouse code, packed-by/checked-by names, and each
// line's own carton LPN(s). shipment_items carries no carton reference
// of its own (only picking_task_item_id, copied verbatim from the
// source packing job at Create time -- see Item's own doc), so the
// match back to carton LPNs is keyed on that shared column against the
// linked packing job's own ItemResponse.PickingTaskItemID/CartonLPNs
// (Phase 33D.1/33C.3). Satisfied structurally by *packing.Service -- in
// practice the exact same instance main.go already wires as this
// package's own Service.packing.
type packingJobLookup interface {
	Get(ctx context.Context, tenantID, id int64) (*packing.Response, error)
}

// docRecorder is the narrow slice of internal/docrender's own Service
// this package needs -- see internal/picking/pdf.go's own docRecorder
// doc for the full rationale.
type docRecorder interface {
	Record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error
}

// PDFService composes Shipping's own Service (for Get -- the Delivery
// Note is just that same shipment detail, rendered) with the tenant/
// user display-name lookups and the packing read-only lookup the
// document's header/lines need.
type PDFService struct {
	shipping  *Service
	packing   packingJobLookup
	tenants   tenantNamer
	users     userNamer
	docrender docRecorder
}

func NewPDFService(shippingSvc *Service, packingSvc packingJobLookup, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{shipping: shippingSvc, packing: packingSvc, tenants: tenants, users: users}
}

// SetDocRender wires the document-history recorder (Phase 33C.3) -- see
// internal/picking/pdf.go's own docRecorder doc.
func (s *PDFService) SetDocRender(d docRecorder) {
	s.docrender = d
}

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

// packingJobDisplay resolves the linked packing job's own warehouse
// code, packed-by/checked-by display names, and a PickingTaskItemID ->
// CartonLPNs map for the Delivery Note's own item table -- tolerating
// any lookup failure (the packing job was already verified to exist at
// Create time; a lookup failure here is purely cosmetic) by degrading
// to "-"/an empty map rather than failing the whole document.
func (s *PDFService) packingJobDisplay(ctx context.Context, tenantID, packingJobID int64) (warehouseCode, packedBy, checkedBy string, cartonsByTaskItem map[int64][]string) {
	warehouseCode, packedBy, checkedBy = "-", "-", "-"
	cartonsByTaskItem = map[int64][]string{}
	if s.packing == nil {
		return
	}
	job, err := s.packing.Get(ctx, tenantID, packingJobID)
	if err != nil || job == nil {
		return
	}
	warehouseCode = job.WarehouseCode
	if job.AssignedPackerName != nil {
		packedBy = *job.AssignedPackerName
	}
	if job.CheckerName != nil {
		checkedBy = *job.CheckerName
	}
	for _, it := range job.Items {
		cartonsByTaskItem[it.PickingTaskItemID] = it.CartonLPNs
	}
	return
}

// RenderDeliveryNote builds the Delivery Note document -- header
// (delivery note no./shipment no., order no., customer, ship-to name +
// full delivery address snapshot, carrier, tracking no., ship date,
// warehouse, packed-by/checked-by resolved from the source packing job),
// every line (SKU/product, lot, UOM, qty shipped, carton LPN(s) it went
// into), a barcode for the shipment no. and order no., and a driver/
// recipient signature block. Read-only: Service.Get never mutates
// anything, so printing/reprinting is always safe regardless of the
// shipment's current status.
//
// DRAFT watermark: a shipment not yet DISPATCHED (DispatchedAt == nil,
// which covers READY and an ISSUE reported before ever dispatching) gets
// pdfexport's own diagonal DRAFT banner (doc.MarkDraft, Phase 33C.3) and
// is recorded with docrender's own is_draft = true, mirroring the exact
// "not yet [owning module's terminal confirmation] = DRAFT" precedent
// internal/packing's own RenderPackingList established, adapted to
// Shipping's own lifecycle: once DISPATCHED (or later -- IN_TRANSIT/
// DELIVERED/an ISSUE reported after dispatch all still carry a non-nil
// DispatchedAt), this document is FINAL.
func (s *PDFService) RenderDeliveryNote(ctx context.Context, tenantID, userID, shipmentID int64) ([]byte, string, error) {
	shipment, err := s.shipping.Get(ctx, tenantID, shipmentID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Delivery Note", shipment.ShipmentNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	isDraft := shipment.DispatchedAt == nil
	if isDraft {
		doc.MarkDraft()
	}

	warehouseCode, packedBy, checkedBy, cartonsByTaskItem := s.packingJobDisplay(ctx, tenantID, shipment.PackingJobID)

	reportStatus := string(shipment.Status)
	if isDraft {
		reportStatus = string(shipment.Status) + " — DRAFT (not yet dispatched)"
	}

	doc.KeyValueGrid([][2]string{
		{"Delivery Note No.", shipment.ShipmentNo},
		{"Shipment No.", shipment.ShipmentNo},
		{"Order No.", shipment.OrderNo},
		{"Status", reportStatus},
		{"Warehouse", warehouseCode},
		{"Customer", csvexport.FormatString(shipment.Customer)},
		{"Ship To Name", csvexport.FormatString(shipment.ShipToName)},
		{"Ship To Address", csvexport.FormatString(shipment.ShipToAddress)},
		{"Province / District", fmt.Sprintf("%s / %s", csvexport.FormatString(shipment.ShipToProvince), csvexport.FormatString(shipment.ShipToDistrict))},
		{"Postal Code", csvexport.FormatString(shipment.ShipToPostalCode)},
		{"Delivery Contact", csvexport.FormatString(shipment.ShipToContactPerson)},
		{"Delivery Contact Phone", csvexport.FormatString(shipment.ShipToContactPhone)},
		{"Carrier", csvexport.FormatString(shipment.Carrier)},
		{"Tracking No.", csvexport.FormatString(shipment.TrackingNo)},
		{"Ship Date", csvexport.FormatTime(shipment.ShipDate)},
		{"Packages", fmt.Sprintf("%d", shipment.Packages)},
		{"Packed By", packedBy},
		{"Checked By", checkedBy},
		{"Remark", csvexport.FormatString(shipment.Remark)},
	}, 2)

	doc.SectionTitle("Shipment Barcode")
	if err := doc.Barcode(shipment.ShipmentNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)
	doc.SectionTitle("Order Barcode")
	if err := doc.Barcode(shipment.OrderNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)

	doc.SectionTitle("Delivery Lines")
	rows := make([][]string, 0, len(shipment.Items))
	for i, item := range shipment.Items {
		cartons := "-"
		if lpns := cartonsByTaskItem[item.PickingTaskItemID]; len(lpns) > 0 {
			cartons = joinStrings(lpns, ", ")
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1), item.SKU, item.ProductName, item.LotNo,
			csvexport.FormatFloat(item.Qty), item.UOM, cartons,
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "#", Width: 10, Align: "C"}, {Header: "SKU", Width: 26}, {Header: "Product", Width: 50},
		{Header: "Lot No.", Width: 24}, {Header: "Qty", Width: 20, Align: "R"}, {Header: "UOM", Width: 18, Align: "C"},
		{Header: "Carton(s)", Width: 38, Align: "C"},
	}, rows)

	doc.SignatureBlock([]string{"Delivery Driver", "Recipient"})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "DELIVERY_NOTE", shipment.ShipmentNo, "shipments", shipmentID, userID, isDraft, data)
	return data, fmt.Sprintf("DeliveryNote_%s.pdf", shipment.ShipmentNo), nil
}

// joinStrings concatenates items with sep -- a local copy of internal/
// packing's own helper of the same name/shape (each PDF package
// declares its own rather than importing another's package just for
// this).
func joinStrings(items []string, sep string) string {
	result := ""
	for i, it := range items {
		if i > 0 {
			result += sep
		}
		result += it
	}
	return result
}

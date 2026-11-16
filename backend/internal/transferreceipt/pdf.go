// Phase 33E.3 -- Inter-Warehouse Transfer Note. One document per
// transfer_receipts row, the closing document of Phase 33E's own
// inter-warehouse transfer chain (Order -> Dispatch -> Destination
// Receiving -> Transfer Note). Reuses exactly the same PDF/reprint
// infrastructure 33C.3 already built (internal/pdfexport,
// internal/docrender) -- no new primitive is added here, and no new
// mutation: RenderTransferNote only ever reads Service.Get's own
// already-correct data.
package transferreceipt

import (
	"context"
	"fmt"
	"time"

	"wms-backend/internal/brand"
	"wms-backend/internal/order"
	"wms-backend/internal/pdfexport"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/product"
	"wms-backend/internal/shipping"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
)

// tenantNamer and userNamer are the narrow read-only slices this
// package's own PDF generator needs -- the same local copies every
// other PDF-rendering package in this codebase (internal/shipping's/
// internal/loading's/internal/packing's own PDFService) each declare
// rather than importing one another's package for it.
type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// shipmentLookup is the narrow, read-only slice of internal/shipping's
// own Service this package's PDF generator needs beyond this module's
// own existing shippingChecker dependency -- to resolve the Transfer
// Note's own "Dispatch Date" header field (shipment.DispatchedAt) and,
// via SetExpiryInfo's own expiryByReceiptItem, each line's own source
// picking_task_item_id. Satisfied structurally by *shipping.Service --
// in practice the exact same instance main.go already wires as this
// module's own Service.shipments.
type shipmentLookup interface {
	Get(ctx context.Context, tenantID, id int64) (*shipping.Response, error)
}

// orderLookup is the narrow, read-only slice of internal/order's own
// Repository this package's PDF generator needs -- resolving the
// receipt's own linked order BY ID (not by order_no, the way this
// module's own orderChecker already does for Create) to read that
// order's own WarehouseID -- the transfer's SOURCE warehouse, which
// Response/headerRow never carry (they only resolve the DESTINATION
// warehouse -- see FindReceiptDetail's own doc). Satisfied structurally
// by *order.Repository.
type orderLookup interface {
	FindByID(ctx context.Context, tenantID, id int64) (*order.Order, error)
}

// warehouseLookup resolves a warehouse id into its display code --
// satisfied structurally by *warehouse.Repository, used here only to
// resolve the SOURCE warehouse's own code (the destination warehouse
// code is already resolved by FindReceiptDetail).
type warehouseLookup interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*warehouse.Warehouse, error)
}

// productLookup and brandLookup (optional, wired via SetProductInfo)
// resolve a line's own Brand/Pack Size for the Transfer Note's item
// table -- the same optional-dependency shape internal/packing's own
// PDFService already established for the identical columns, satisfied
// structurally by *product.Repository/*brand.Repository.
type productLookup interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*product.Product, error)
}

type brandLookup interface {
	FindByID(ctx context.Context, tenantID, id int64) (*brand.Brand, error)
}

// pickingTaskLookup (optional, wired via SetExpiryInfo) resolves the
// SOURCE picking task's own already-resolved ItemResponse.ExpiryDate
// for each line -- transfer_receipt_items carries no expiry column of
// its own (frozen only from the shipment item's own dispatched_qty/lot_
// no -- see Item's own doc), so this goes one level below the shipment
// (via its own PickingTaskID and each item's own PickingTaskItemID) the
// exact same way internal/packing's own expiryByPackingItem already
// does for its Packing List. Satisfied structurally by *picking.Service.
type pickingTaskLookup interface {
	Get(ctx context.Context, tenantID, id int64) (*picking.Response, error)
}

// docRecorder is the narrow slice of internal/docrender's own Service
// this package needs -- see internal/picking/pdf.go's own docRecorder
// doc for the full rationale.
type docRecorder interface {
	Record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error
}

// PDFService composes this module's own Service (for Get -- the
// Transfer Note is just that same receipt detail, rendered) with the
// tenant/user display-name lookups every generated PDF needs plus the
// shipment/order/warehouse lookups the header's own SOURCE-warehouse/
// dispatch-date fields need. shipments/orders/warehouses are required
// constructor dependencies (core header fields, not a cosmetic join);
// products/brands/tasks are optional, wired via their own setters --
// unwired, those columns simply render "-" rather than erroring.
type PDFService struct {
	transferreceipt *Service
	shipments       shipmentLookup
	orders          orderLookup
	warehouses      warehouseLookup
	products        productLookup
	brands          brandLookup
	tasks           pickingTaskLookup
	tenants         tenantNamer
	users           userNamer
	docrender       docRecorder
}

func NewPDFService(trSvc *Service, shipments shipmentLookup, orders orderLookup, warehouses warehouseLookup, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{transferreceipt: trSvc, shipments: shipments, orders: orders, warehouses: warehouses, tenants: tenants, users: users}
}

// SetProductInfo wires the Brand/Pack Size lookups the Transfer Note's
// item table uses -- optional; a *PDFService never given one renders
// "-" for both columns instead of erroring.
func (s *PDFService) SetProductInfo(products productLookup, brands brandLookup) {
	s.products = products
	s.brands = brands
}

// SetExpiryInfo wires the picking-task lookup each line's own Expiry
// column uses -- optional; a *PDFService never given one renders "" for
// every line's expiry instead of erroring.
func (s *PDFService) SetExpiryInfo(tasks pickingTaskLookup) {
	s.tasks = tasks
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

// sourceWarehouse resolves the receipt's own linked order's SOURCE
// warehouse code (order.WarehouseID, never DestinationWarehouseID --
// that side is already resolved by FindReceiptDetail), tolerating any
// lookup failure (unwired dependency, deleted order/warehouse) by
// degrading to "-" rather than failing the whole document -- a Transfer
// Note must still render even if this cosmetic join can't complete.
func (s *PDFService) sourceWarehouse(ctx context.Context, tenantID, orderID int64) string {
	if s.orders == nil || s.warehouses == nil {
		return "-"
	}
	ord, err := s.orders.FindByID(ctx, tenantID, orderID)
	if err != nil || ord == nil {
		return "-"
	}
	wh, err := s.warehouses.FindByIDForTenant(ctx, tenantID, ord.WarehouseID)
	if err != nil || wh == nil {
		return "-"
	}
	return wh.Code
}

// dispatchDate resolves the source shipment's own DispatchedAt, the
// moment this receipt's own goods actually left the source warehouse --
// never stored on transfer_receipts itself. Tolerates any lookup
// failure by degrading to "-".
func (s *PDFService) dispatchDate(ctx context.Context, tenantID, shipmentID int64) string {
	if s.shipments == nil {
		return "-"
	}
	shipment, err := s.shipments.Get(ctx, tenantID, shipmentID)
	if err != nil || shipment == nil || shipment.DispatchedAt == nil {
		return "-"
	}
	return csvexport.FormatTime(shipment.DispatchedAt)
}

// brandPackSize resolves productID's own Brand name and Pack Size
// display string -- see internal/packing's own identical helper's doc
// for the full "tolerate any lookup failure" rationale, duplicated here
// rather than imported since each PDF package owns its own narrow
// interfaces/helpers.
func (s *PDFService) brandPackSize(ctx context.Context, tenantID, productID int64) (brandName, packSize string) {
	brandName, packSize = "-", "-"
	if s.products == nil {
		return
	}
	p, err := s.products.FindByIDForTenant(ctx, tenantID, productID)
	if err != nil || p == nil {
		return
	}
	if p.PackSize > 0 {
		packSize = fmt.Sprintf("%d %s", p.PackSize, p.PackSizeUnit)
	}
	if p.BrandID != nil && s.brands != nil {
		if b, err := s.brands.FindByID(ctx, tenantID, *p.BrandID); err == nil && b != nil {
			brandName = b.Name
		}
	}
	return
}

// expiryByReceiptItem resolves every receipt line's own expiry date by
// chasing: transfer_receipt_item.ShipmentItemID -> the matching
// shipment.Items[].PickingTaskItemID -> the SOURCE picking task's own
// already-resolved ItemResponse.ExpiryDate -- the same "go one level
// below an already-terminal source document, since expiry is never
// stored on THIS module's own rows" precedent internal/packing's own
// expiryByPackingItem already established, extended one hop further
// here (transfer_receipt_items -> shipment_items -> picking_task_items,
// vs. packing_job_items -> picking_task_items directly) because this
// module's own frozen snapshot is one layer further downstream than
// packing's. Returns an empty, non-nil map (never an error) on any
// lookup failure -- expiry is a cosmetic PDF field, not something worth
// failing the whole Transfer Note over.
func (s *PDFService) expiryByReceiptItem(ctx context.Context, tenantID, shipmentID int64, items []ItemResponse) map[int64]*time.Time {
	out := map[int64]*time.Time{}
	if s.shipments == nil || s.tasks == nil {
		return out
	}
	shipment, err := s.shipments.Get(ctx, tenantID, shipmentID)
	if err != nil || shipment == nil {
		return out
	}
	taskItemIDByShipmentItemID := make(map[int64]int64, len(shipment.Items))
	for _, si := range shipment.Items {
		taskItemIDByShipmentItemID[si.ID] = si.PickingTaskItemID
	}
	task, err := s.tasks.Get(ctx, tenantID, shipment.PickingTaskID)
	if err != nil || task == nil {
		return out
	}
	expiryByTaskItemID := make(map[int64]*time.Time, len(task.Items))
	for _, ti := range task.Items {
		expiryByTaskItemID[ti.ID] = ti.ExpiryDate
	}
	for _, item := range items {
		if taskItemID, ok := taskItemIDByShipmentItemID[item.ShipmentItemID]; ok {
			out[item.ID] = expiryByTaskItemID[taskItemID]
		}
	}
	return out
}

// RenderTransferNote builds the Inter-Warehouse Transfer Note document
// -- header (transfer note no./shipment no., source order no./transfer
// reference, source/destination warehouse, dispatch/receipt date,
// received by), every line (SKU/product/brand/pack size, lot/expiry,
// UOM, dispatched/received qty, discrepancy qty + reason), a
// discrepancy summary, received-by/checked-by signature fields, and
// barcodes for the receipt no. and the source order no. Read-only:
// Service.Get never mutates anything, so printing/reprinting is always
// safe regardless of the receipt's current status.
//
// DRAFT watermark: a receipt not yet COMPLETED (DRAFT, still being
// received line by line, or CANCELLED) gets pdfexport's own diagonal
// DRAFT banner (doc.MarkDraft, Phase 33C.3) and is recorded with
// docrender's own is_draft = true -- the same "not yet [this module's
// own terminal confirmation] = DRAFT" precedent every other PDF in this
// codebase already follows; once COMPLETED, this document is FINAL.
func (s *PDFService) RenderTransferNote(ctx context.Context, tenantID, userID, receiptID int64) ([]byte, string, error) {
	receipt, err := s.transferreceipt.Get(ctx, tenantID, receiptID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Inter-Warehouse Transfer Note", receipt.ReceiptNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	isDraft := receipt.Status != StatusCompleted
	if isDraft {
		doc.MarkDraft()
	}

	reportStatus := string(receipt.Status)
	if isDraft {
		reportStatus = string(receipt.Status) + " — DRAFT (not yet completed)"
	}

	sourceWarehouseCode := s.sourceWarehouse(ctx, tenantID, receipt.OrderID)
	dispatchDate := s.dispatchDate(ctx, tenantID, receipt.ShipmentID)
	receivedBy := "-"
	if receipt.ReceivedByName != nil {
		receivedBy = *receipt.ReceivedByName
	}

	doc.KeyValueGrid([][2]string{
		{"Transfer Note No.", receipt.ReceiptNo},
		{"Shipment No.", receipt.ShipmentNo},
		{"Source Order No.", receipt.OrderNo},
		{"Transfer Reference", csvexport.FormatString(receipt.TransferReference)},
		{"Status", reportStatus},
		{"Source Warehouse", sourceWarehouseCode},
		{"Destination Warehouse", receipt.DestinationWarehouseCode},
		{"Dispatch Date", dispatchDate},
		{"Receipt Date", csvexport.FormatTime(receipt.ReceivedAt)},
		{"Received By", receivedBy},
		{"Remark", csvexport.FormatString(receipt.Remark)},
	}, 2)

	doc.SectionTitle("Transfer Receipt Barcode")
	if err := doc.Barcode(receipt.ReceiptNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)
	doc.SectionTitle("Source Order QR")
	if err := doc.QRCode(receipt.OrderNo, doc.MarginX(), doc.CursorY(), 24); err != nil {
		return nil, "", err
	}
	doc.QRCaption(24, receipt.OrderNo)

	expiryByItem := s.expiryByReceiptItem(ctx, tenantID, receipt.ShipmentID, receipt.Items)

	doc.SectionTitle("Transfer Lines")
	rows := make([][]string, 0, len(receipt.Items))
	var totalDispatched, totalReceived, totalDiscrepancy float64
	var discrepancyNotes []string
	for i, item := range receipt.Items {
		brandName, packSize := s.brandPackSize(ctx, tenantID, item.ProductID)
		expiry := ""
		if exp := expiryByItem[item.ID]; exp != nil {
			expiry = exp.Format("2006-01-02")
		}
		discrepancyQty := "-"
		discrepancyReason := "-"
		if item.DiscrepancyQty != nil {
			discrepancyQty = csvexport.FormatFloat(*item.DiscrepancyQty)
			totalDiscrepancy += *item.DiscrepancyQty
			discrepancyReason = csvexport.FormatString(item.DiscrepancyReason)
			discrepancyNotes = append(discrepancyNotes, fmt.Sprintf("%s (lot %s): dispatched %s, received %s, short %s %s — %s",
				item.SKU, item.LotNo, csvexport.FormatFloat(item.DispatchedQty), csvexport.FormatFloat(item.ReceivedQty),
				csvexport.FormatFloat(*item.DiscrepancyQty), item.UOM, discrepancyReason))
		}
		totalDispatched += item.DispatchedQty
		totalReceived += item.ReceivedQty

		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1), item.SKU, item.ProductName, brandName, packSize, item.LotNo, expiry,
			csvexport.FormatFloat(item.DispatchedQty), csvexport.FormatFloat(item.ReceivedQty), item.UOM,
			discrepancyQty, discrepancyReason, string(item.Status),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "#", Width: 7, Align: "C"}, {Header: "SKU", Width: 16}, {Header: "Product", Width: 24}, {Header: "Brand", Width: 14},
		{Header: "Pack Size", Width: 13, Align: "C"}, {Header: "Lot No.", Width: 13}, {Header: "Expiry", Width: 13, Align: "C"},
		{Header: "Dispatched", Width: 14, Align: "R"}, {Header: "Received", Width: 14, Align: "R"}, {Header: "UOM", Width: 9, Align: "C"},
		{Header: "Discrep.", Width: 13, Align: "R"}, {Header: "Reason", Width: 22}, {Header: "Status", Width: 16, Align: "C"},
	}, rows)

	doc.SectionTitle("Totals")
	doc.KeyValueGrid([][2]string{
		{"Total Dispatched Qty", csvexport.FormatFloat(totalDispatched)},
		{"Total Received Qty", csvexport.FormatFloat(totalReceived)},
		{"Total Discrepancy Qty", csvexport.FormatFloat(totalDiscrepancy)},
	}, 2)

	if len(discrepancyNotes) > 0 {
		doc.SectionTitle("Discrepancy Summary")
		for _, n := range discrepancyNotes {
			doc.KeyValueGrid([][2]string{{"-", n}}, 1)
		}
	}

	doc.SignatureBlock([]string{"Received By", "Checked By"})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "TRANSFER_NOTE", receipt.ReceiptNo, "transfer_receipts", receiptID, userID, isDraft, data)
	return data, fmt.Sprintf("TransferNote_%s.pdf", receipt.ReceiptNo), nil
}

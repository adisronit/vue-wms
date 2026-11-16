// Phase 33C.3 -- Consolidated Batch Picking List. One document per
// CONFIRMED (or later COMPLETED, which only ever mirrors the underlying
// task's own terminal status, see Status' own doc) picking_batches row —
// every consolidated line, a barcode/QR for the batch no, and a
// per-line QR usable at the pick face, reusing exactly internal/
// picking's own Pick List conventions (ScanPayload, SignatureBlock,
// Barcode/QRCode layout) rather than reinventing them.
package pickingbatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"wms-backend/internal/brand"
	"wms-backend/internal/pdfexport"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/product"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// ErrBatchNotConfirmed is returned by RenderConsolidatedPickList against
// a still-DRAFT batch — a DRAFT batch has no real picking_tasks row and
// no reserved stock yet (see Status' own doc), so there is nothing a
// pick-face document could meaningfully drive; the preview itself is
// already visible via GET /picking-batches/{id}.
var ErrBatchNotConfirmed = errors.New("pickingbatch: batch must be CONFIRMED before its Consolidated Picking List can be generated")

type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// pickingReader is the narrow, read-only slice of internal/picking's
// own Service this package's PDF generator needs — to resolve each
// consolidated line's own expiry date once the batch has been confirmed
// into a real picking task (picking_batch_allocations/picking_task_items
// have no expiry column of their own — see picking.ItemResponse's own
// doc for why that is resolved from the matching inventory balance row
// instead). Satisfied structurally by *picking.Service; this package
// already depends on it for Create/Confirm (see service.go's own
// pickingOrchestrator), so this is simply reusing the same dependency
// for one more read-only call, never a second picking.Service instance.
type pickingReader interface {
	Get(ctx context.Context, tenantID, id int64) (*picking.Response, error)
}

// productLookup and brandLookup resolve a consolidated line's own
// Brand/Pack Size — the same narrow, optional-dependency shape internal/
// packing's own pdf.go uses for the identical need, duplicated locally
// rather than importing internal/packing for it (this package must
// never depend on internal/packing — see package doc elsewhere in this
// module for the one-way pickingbatch -> picking relationship).
type productLookup interface {
	FindByIDForTenant(ctx context.Context, tenantID, id int64) (*product.Product, error)
}

type brandLookup interface {
	FindByID(ctx context.Context, tenantID, id int64) (*brand.Brand, error)
}

// docRecorder is the narrow slice of internal/docrender's own Service
// this package needs — see internal/picking/pdf.go's own docRecorder
// doc for the full rationale.
type docRecorder interface {
	Record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error
}

// PDFService composes this package's own Service (for Get — the
// Consolidated Picking List is just that same batch detail, rendered)
// with the tenant/user display-name lookups and the picking/product/
// brand read-only lookups the document's lines need.
type PDFService struct {
	batches   *Service
	picking   pickingReader
	tenants   tenantNamer
	users     userNamer
	products  productLookup
	brands    brandLookup
	docrender docRecorder
}

func NewPDFService(batchSvc *Service, pickingSvc pickingReader, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{batches: batchSvc, picking: pickingSvc, tenants: tenants, users: users}
}

// SetProductInfo wires the optional Brand/Pack Size lookups — see
// internal/packing/pdf.go's own SetProductInfo doc for the identical
// "renders '-' rather than erroring when unwired" contract.
func (s *PDFService) SetProductInfo(products productLookup, brands brandLookup) {
	s.products = products
	s.brands = brands
}

// SetDocRender wires the document-history recorder — see
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

// RenderConsolidatedPickList builds the Consolidated Batch Picking List
// — batch/wave no., warehouse, created/confirmed dates, every
// consolidated line (SKU/product, brand/pack size, source location,
// lot, expiry, consolidated qty, UOM), a barcode/QR for the batch no.,
// and one QR per line for pick-face scanning (ScanPayload — the exact
// same location|SKU|lot convention internal/picking's own Pick List
// uses), plus picker/checker signature fields.
//
// LPN: picking_task_items (what each consolidated line becomes once
// confirmed) has no LPN column of its own — LPN only exists in this
// codebase for relocation_task_items and cartons, neither of which this
// document is about — so no LPN column is rendered; this is a real data-
// model gap, not an oversight, and is called out in the Phase 33C.3
// delivery notes.
//
// Only valid once the batch is CONFIRMED (a real picking_tasks row
// exists with real reservations) or COMPLETED (that task's own terminal
// mirror, see Status' own doc) — a DRAFT batch has nothing for a pick-
// face document to drive yet.
func (s *PDFService) RenderConsolidatedPickList(ctx context.Context, tenantID, userID, batchID int64) ([]byte, string, error) {
	batch, err := s.batches.Get(ctx, tenantID, batchID)
	if err != nil {
		return nil, "", err
	}
	if batch.Status != StatusConfirmed && batch.Status != StatusCompleted {
		return nil, "", ErrBatchNotConfirmed
	}

	meta, err := s.meta(ctx, tenantID, userID, "Consolidated Batch Picking List", batch.BatchNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	doc.KeyValueGrid([][2]string{
		{"Batch No.", batch.BatchNo},
		{"Status", string(batch.Status)},
		{"Warehouse", batch.WarehouseCode},
		{"Pick No.", csvexport.FormatString(batch.PickNo)},
		{"Order Count", fmt.Sprintf("%d", len(batch.Orders))},
		{"Created At", csvexport.FormatTimeValue(batch.CreatedAt)},
		{"Confirmed At", csvexport.FormatTime(batch.ConfirmedAt)},
	}, 2)

	doc.SectionTitle("Batch Barcode")
	if err := doc.Barcode(batch.BatchNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)

	doc.SectionTitle("Included Orders")
	orderRows := make([][]string, 0, len(batch.Orders))
	for _, o := range batch.Orders {
		orderRows = append(orderRows, []string{o.OrderNo, csvexport.FormatString(o.Customer), csvexport.FormatString(o.ShipToName), o.Status})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "Order No.", Width: 40}, {Header: "Customer", Width: 50},
		{Header: "Ship To", Width: 60}, {Header: "Status", Width: 36, Align: "C"},
	}, orderRows)

	// Resolve expiry per consolidated line via the underlying picking
	// task — see this method's own doc for why.
	expiryByTaskItem := map[int64]*time.Time{}
	if batch.PickingTaskID != nil {
		if task, err := s.picking.Get(ctx, tenantID, *batch.PickingTaskID); err == nil && task != nil {
			for _, it := range task.Items {
				expiryByTaskItem[it.ID] = it.ExpiryDate
			}
		}
	}

	doc.SectionTitle("Consolidated Pick Lines")
	rows := make([][]string, 0, len(batch.Lines))
	for i, line := range batch.Lines {
		brandName, packSize := s.brandPackSize(ctx, tenantID, line.ProductID)
		expiry := ""
		if len(line.Allocations) > 0 && line.Allocations[0].PickingTaskItemID != nil {
			if exp := expiryByTaskItem[*line.Allocations[0].PickingTaskItemID]; exp != nil {
				expiry = exp.Format("2006-01-02")
			}
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1), line.SKU, line.ProductName, brandName, packSize,
			line.LocationCode, line.LotNo, expiry, csvexport.FormatFloat(line.Qty), line.UOM,
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "#", Width: 8, Align: "C"}, {Header: "SKU", Width: 18}, {Header: "Product", Width: 28}, {Header: "Brand", Width: 16},
		{Header: "Pack Size", Width: 14, Align: "C"}, {Header: "Source Loc.", Width: 18, Align: "C"}, {Header: "Lot No.", Width: 16},
		{Header: "Expiry", Width: 14, Align: "C"}, {Header: "Qty", Width: 14, Align: "R"}, {Header: "UOM", Width: 12, Align: "C"},
	}, rows)

	doc.SectionTitle("Scan Codes (per line: location | SKU | lot)")
	for _, line := range batch.Lines {
		payload := picking.ScanPayload(line.LocationCode, line.SKU, line.LotNo)
		y := doc.CursorY()
		if err := doc.QRCode(payload, doc.MarginX(), y, 20); err != nil {
			return nil, "", err
		}
		doc.QRCaption(20, line.SKU+" @ "+line.LocationCode)
	}

	doc.SignatureBlock([]string{"Picker", "Checker"})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "PICKING_BATCH_LIST", batch.BatchNo, "picking_batches", batchID, userID, false, data)
	return data, fmt.Sprintf("BatchPickList_%s.pdf", batch.BatchNo), nil
}

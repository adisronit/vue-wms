package packing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"wms-backend/internal/brand"
	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/product"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// tenantNamer and userNamer are the narrow read-only slices this
// package's own PDF generator needs — the same local copies internal/
// picking's/internal/relocation's own PDFService each declare rather
// than importing one another's package for it.
type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// productLookup and brandLookup (Phase 33C.3) resolve a packing line's
// own Brand/Pack Size for the Packing List's item table — satisfied
// structurally by *product.Repository/*brand.Repository, wired via
// SetProductInfo (optional, like SetDocRender below) so a *PDFService
// never given one simply renders "-" for both columns rather than
// erroring.
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

// PDFService composes Packing's own Service (for Get — both PDFs are
// just that same job detail, rendered) with the tenant/user display-
// name lookups every generated PDF's header/footer needs.
type PDFService struct {
	packing   *Service
	tenants   tenantNamer
	users     userNamer
	products  productLookup
	brands    brandLookup
	docrender docRecorder
}

func NewPDFService(packingSvc *Service, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{packing: packingSvc, tenants: tenants, users: users}
}

// SetProductInfo wires the Brand/Pack Size lookups the Packing List's
// item table uses (Phase 33C.3) — optional; a *PDFService never given
// one renders "-" for both columns instead of erroring.
func (s *PDFService) SetProductInfo(products productLookup, brands brandLookup) {
	s.products = products
	s.brands = brands
}

// SetDocRender wires the document-history recorder (Phase 33C.3) — see
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

// brandPackSize resolves productID's own Brand name and Pack Size
// display string, tolerating any lookup failure (unwired dependency,
// deleted product, no brand assigned, ...) by degrading to "-" rather
// than failing the whole PDF — a Packing List must still render even
// if a cosmetic master-data join can't be completed.
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

// expiryByPackingItem resolves every line's own expiry date by reading
// the SOURCE picking task's already-resolved ItemResponse.ExpiryDate
// (internal/picking.Service.Get resolves it from the matching inventory
// balance row — see that field's own doc) and matching it back to each
// packing_job_items row via its own PickingTaskItemID column (read via
// Repository.FindItemsByJob's own raw Item model — ItemResponse does
// not expose that FK, so this goes one level below the DTO rather than
// adding a field to packing's own existing response shape just for this
// PDF). packing_job_items has no location_id of its own (stock already
// left the location at pick time — see this package's own doc), so it
// cannot run the same (product, location, lot) inventory lookup
// picking's Pick List uses directly; going through the already-terminal
// source task instead is free (two extra read-only calls) and exact,
// not an approximation. Returns an empty, non-nil map (never an error)
// on any lookup failure — expiry is a cosmetic PDF field, not something
// worth failing the whole Packing List over.
func (s *PDFService) expiryByPackingItem(ctx context.Context, tenantID, jobID, pickingTaskID int64) map[int64]*time.Time {
	out := map[int64]*time.Time{}
	rawItems, err := s.packing.repo.FindItemsByJob(ctx, tenantID, jobID)
	if err != nil {
		return out
	}
	task, err := s.packing.picking.Get(ctx, tenantID, pickingTaskID)
	if err != nil || task == nil {
		return out
	}
	expiryByTaskItemID := make(map[int64]*time.Time, len(task.Items))
	for _, ti := range task.Items {
		expiryByTaskItemID[ti.ID] = ti.ExpiryDate
	}
	for _, ri := range rawItems {
		out[ri.ID] = expiryByTaskItemID[ri.PickingTaskItemID]
	}
	return out
}

// RenderPackingList builds the Packing List document — job header
// (job/list no., sales order no., customer/full ship-to snapshot
// including delivery contact/phone, warehouse, packer/checker) plus
// every line (line no., SKU/product, brand/pack size, lot/expiry, UOM,
// ordered/picked/packed qty, carton(s) it went into) and a package
// summary (carton/pallet counts, gross/net weight totals, packed qty by
// UOM, partial/short notes). Read-only: Service.Get never mutates
// anything, so printing/reprinting is always safe regardless of the
// job's current status.
//
// DRAFT watermark: a job not yet checker-confirmed (Status != COMPLETED)
// gets pdfexport's own diagonal DRAFT banner (doc.MarkDraft, Phase
// 33C.3) and is recorded with docrender's own is_draft = true — this
// document must never be mistaken for FINAL before the two-person
// verification step Status' own doc describes has actually happened.
//
// Logo: Tenant.LogoURL exists as a DB column but has zero upload/
// storage/serving infrastructure anywhere in this codebase (confirmed
// by grep) — deliberately out of scope for this phase (a separate,
// larger feature). Branding here is text-only, via Meta.CompanyName,
// exactly like every other PDF this codebase generates.
func (s *PDFService) RenderPackingList(ctx context.Context, tenantID, userID, jobID int64) ([]byte, string, error) {
	job, err := s.packing.Get(ctx, tenantID, jobID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Packing List", job.JobNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	isDraft := job.Status != StatusCompleted
	if isDraft {
		doc.MarkDraft()
	}

	assignedPacker := csvexport.FormatString(job.AssignedPackerName)
	checker := csvexport.FormatString(job.CheckerName)
	shipTo := csvexport.FormatString(job.ShipToName)

	// packingDate: packing_jobs has no dedicated "packing date" column —
	// CheckerConfirmedAt (the moment this job actually finished) once
	// set, else CreatedAt (when packing activity began), the same
	// "best available timestamp" convention this codebase already uses
	// for CompletedAt-or-CreatedAt elsewhere.
	packingDate := csvexport.FormatTimeValue(job.CreatedAt)
	if job.CheckerConfirmedAt != nil {
		packingDate = csvexport.FormatTimeValue(*job.CheckerConfirmedAt)
	}

	reportStatus := string(job.Status)
	if isDraft {
		reportStatus = string(job.Status) + " — DRAFT (not yet checker-confirmed)"
	}

	doc.KeyValueGrid([][2]string{
		{"Packing List No.", job.JobNo},
		{"Job No.", job.JobNo},
		{"Sales Order No.", job.OrderNo},
		{"Source Pick No.", job.PickNo},
		{"Status", reportStatus},
		{"Warehouse", job.WarehouseCode},
		{"Customer", csvexport.FormatString(job.Customer)},
		{"Ship To Name", shipTo},
		{"Ship To Address", csvexport.FormatString(job.ShipToAddress)},
		{"Province / District", fmt.Sprintf("%s / %s", csvexport.FormatString(job.ShipToProvince), csvexport.FormatString(job.ShipToDistrict))},
		{"Postal Code", csvexport.FormatString(job.ShipToPostalCode)},
		{"Delivery Contact", csvexport.FormatString(job.ShipToContactPerson)},
		{"Delivery Contact Phone", csvexport.FormatString(job.ShipToContactPhone)},
		{"Packing Date", packingDate},
		{"Packed By", assignedPacker},
		{"Checked By", checker},
		{"Remark", csvexport.FormatString(job.Remark)},
	}, 2)

	doc.SectionTitle("Job Barcode")
	if err := doc.Barcode(job.JobNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)
	doc.SectionTitle("Sales Order Barcode")
	if err := doc.Barcode(job.OrderNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)

	expiryByItem := s.expiryByPackingItem(ctx, tenantID, jobID, job.PickingTaskID)

	doc.SectionTitle("Packing Lines")
	rows := make([][]string, 0, len(job.Items))
	for i, item := range job.Items {
		cartons := "-"
		if len(item.CartonLPNs) > 0 {
			cartons = joinStrings(item.CartonLPNs, ", ")
		}
		brandName, packSize := s.brandPackSize(ctx, tenantID, item.ProductID)
		expiry := ""
		if exp := expiryByItem[item.ID]; exp != nil {
			expiry = exp.Format("2006-01-02")
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1), item.SKU, item.ProductName, brandName, packSize, item.LotNo, expiry,
			csvexport.FormatFloat(item.OrderedQty), csvexport.FormatFloat(item.PickedQty), csvexport.FormatFloat(item.PackedQty),
			item.UOM, string(item.Status), cartons,
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "#", Width: 8, Align: "C"}, {Header: "SKU", Width: 16}, {Header: "Product", Width: 26}, {Header: "Brand", Width: 16},
		{Header: "Pack Size", Width: 14, Align: "C"}, {Header: "Lot No.", Width: 14}, {Header: "Expiry", Width: 14, Align: "C"},
		{Header: "Ordered", Width: 12, Align: "R"}, {Header: "Picked", Width: 12, Align: "R"}, {Header: "Packed", Width: 12, Align: "R"},
		{Header: "UOM", Width: 10, Align: "C"}, {Header: "Status", Width: 16, Align: "C"}, {Header: "Carton(s)", Width: 20, Align: "C"},
	}, rows)

	doc.SectionTitle("Cartons")
	cartonRows := make([][]string, 0, len(job.Cartons))
	for _, c := range job.Cartons {
		gross := "-"
		if c.GrossWeight != nil {
			gross = fmt.Sprintf("%.2f %s", *c.GrossWeight, c.WeightUnit)
		}
		dims := "-"
		if c.CartonLength != nil && c.CartonWidth != nil && c.CartonHeight != nil {
			dims = fmt.Sprintf("%.1fx%.1fx%.1f %s", *c.CartonLength, *c.CartonWidth, *c.CartonHeight, c.DimensionUnit)
		}
		cartonRows = append(cartonRows, []string{c.LPN, string(c.PackagingType), gross, dims, string(c.Status)})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "LPN", Width: 30}, {Header: "Type", Width: 24, Align: "C"}, {Header: "Gross Wt.", Width: 30, Align: "R"},
		{Header: "Dimensions", Width: 50, Align: "C"}, {Header: "Status", Width: 30, Align: "C"},
	}, cartonRows)

	// Package summary. "Total pallets" counts cartons whose own
	// packaging_type is PALLET — this codebase has no separate pallet
	// concept/table of its own (a pallet is just one more
	// PackagingType value on the existing cartons row, see model.go's
	// own PackagingType doc), so there is nothing further to summarize
	// beyond that count.
	doc.SectionTitle("Package Summary")
	totalCartons := len(job.Cartons)
	totalPallets := 0
	var grossTotal, netTotal float64
	weightUnit := "KG"
	for _, c := range job.Cartons {
		if c.PackagingType == PackagingPallet {
			totalPallets++
		}
		if c.GrossWeight != nil {
			grossTotal += *c.GrossWeight
			weightUnit = string(c.WeightUnit)
		}
		if c.NetWeight != nil {
			netTotal += *c.NetWeight
		}
	}
	packedByUOM := map[string]float64{}
	var shortNotes []string
	for _, item := range job.Items {
		packedByUOM[item.UOM] += item.PackedQty
		if item.ExceptionFlag || item.Status == ItemStatusPartiallyPacked {
			reason := csvexport.FormatString(item.ExceptionReason)
			shortNotes = append(shortNotes, fmt.Sprintf("%s (%s): packed %s of %s %s — %s",
				item.SKU, string(item.Status), csvexport.FormatFloat(item.PackedQty), csvexport.FormatFloat(item.PickedQty), item.UOM, reason))
		}
	}
	packedSummary := "-"
	for uom, qty := range packedByUOM {
		if packedSummary == "-" {
			packedSummary = fmt.Sprintf("%s %s", csvexport.FormatFloat(qty), uom)
		} else {
			packedSummary = fmt.Sprintf("%s, %s %s", packedSummary, csvexport.FormatFloat(qty), uom)
		}
	}
	doc.KeyValueGrid([][2]string{
		{"Total Cartons", fmt.Sprintf("%d", totalCartons)},
		{"Total Pallets", fmt.Sprintf("%d", totalPallets)},
		{"Total Gross Weight", fmt.Sprintf("%.2f %s", grossTotal, weightUnit)},
		{"Total Net Weight", fmt.Sprintf("%.2f %s", netTotal, weightUnit)},
		{"Total Packed Qty", packedSummary},
	}, 2)
	if len(shortNotes) > 0 {
		doc.SectionTitle("Partial / Short Packing Notes")
		for _, n := range shortNotes {
			doc.KeyValueGrid([][2]string{{"-", n}}, 1)
		}
	}

	doc.SignatureBlock([]string{"Packer", "Checker"})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "PACKING_LIST", job.JobNo, "packing_jobs", jobID, userID, isDraft, data)
	return data, fmt.Sprintf("PackingList_%s.pdf", job.JobNo), nil
}

// RenderCartonLabels builds the Carton/Pallet Label document — one
// compact 4x6" (101.6mm x 152.4mm) page per carton, each with the
// carton's own LPN as a Code128 barcode, destination/customer, a
// contents summary, gross/net weight/dimensions, and its own sequence
// within the job ("N of TOTAL", Phase 33C.3). If cartonID is 0, every
// carton in jobID is rendered (one per page, in LPN order); otherwise
// only that single carton is rendered (sequence is still relative to
// the job's FULL carton set, not just the one printed, so a reprinted
// single label still reads "3 of 5" rather than "1 of 1").
//
// format ("" or "LABEL" for the 4x6" media, "A4" for a full A4 page,
// Phase 33C.3) selects the page size the exact same content-building
// logic below renders against — pdfexport.Document's own layout
// primitives (KeyValueGrid/Barcode/Table/MarginX/PageContentWidth) are
// all instance methods that already adapt to whichever page size the
// Document was constructed with, so no content logic is duplicated
// between the two formats; only which constructor builds doc differs.
// Added for warehouses without a dedicated label printer.
//
// Page-size decision (Phase 33C, unchanged): a 4x6" label format (the
// de-facto shipping-label standard) rather than a scaled-down A4 page,
// so the printed label is actually usable at the size a warehouse label
// printer expects. pdfexport had no configurable page size before that
// phase (NewDocument was hardcoded to A4) — NewLabelDocument was added
// centrally to internal/pdfexport (not hacked around locally here) so
// any future small-format document can reuse the same custom-size +
// compact-header/footer support.
func (s *PDFService) RenderCartonLabels(ctx context.Context, tenantID, userID, jobID, cartonID int64, format string) ([]byte, string, error) {
	job, err := s.packing.Get(ctx, tenantID, jobID)
	if err != nil {
		return nil, "", err
	}

	allCartons := job.Cartons
	var targets []CartonResponse
	if cartonID == 0 {
		targets = allCartons
	} else {
		for _, c := range allCartons {
			if c.ID == cartonID {
				targets = append(targets, c)
				break
			}
		}
		if len(targets) == 0 {
			return nil, "", ErrCartonNotFound
		}
	}
	if len(targets) == 0 {
		return nil, "", errors.New("packing: job has no cartons to label")
	}

	// sequenceOf resolves one target carton's own "N of TOTAL" position
	// within the job's FULL carton set (allCartons), in the same LPN
	// order the "print every carton" case already iterates in — so a
	// single-carton reprint reports its real position, not "1 of 1".
	sequenceOf := func(id int64) (n, total int) {
		total = len(allCartons)
		for i, c := range allCartons {
			if c.ID == id {
				return i + 1, total
			}
		}
		return 0, total
	}

	meta, err := s.meta(ctx, tenantID, userID, "Carton Label", job.JobNo)
	if err != nil {
		return nil, "", err
	}

	// 101.6mm x 152.4mm = 4in x 6in, the standard shipping-label media
	// size.
	const labelWidthMM, labelHeightMM = 101.6, 152.4
	var doc *pdfexport.Document
	if format == "A4" {
		doc = pdfexport.NewDocument(meta)
	} else {
		doc = pdfexport.NewLabelDocument(meta, labelWidthMM, labelHeightMM)
	}

	shipTo := csvexport.FormatString(job.ShipToName)
	if job.ShipToAddress != nil {
		shipTo = fmt.Sprintf("%s / %s", shipTo, *job.ShipToAddress)
	}

	for i, c := range targets {
		if i > 0 {
			doc.AddPage()
		}

		n, total := sequenceOf(c.ID)
		doc.KeyValueGrid([][2]string{
			{"Order No.", job.OrderNo},
			{"Customer", csvexport.FormatString(job.Customer)},
			{"Ship To", shipTo},
			{"Carton Sequence", fmt.Sprintf("%d of %d", n, total)},
		}, 1)

		if err := doc.Barcode(c.LPN, doc.MarginX(), doc.CursorY(), doc.PageContentWidth(), 18); err != nil {
			return nil, "", err
		}
		doc.Spacer(20)

		gross := "-"
		if c.GrossWeight != nil {
			gross = fmt.Sprintf("%.2f %s", *c.GrossWeight, c.WeightUnit)
		}
		net := "-"
		if c.NetWeight != nil {
			net = fmt.Sprintf("%.2f %s", *c.NetWeight, c.WeightUnit)
		}
		dims := "-"
		if c.CartonLength != nil && c.CartonWidth != nil && c.CartonHeight != nil {
			dims = fmt.Sprintf("%.1fx%.1fx%.1f %s", *c.CartonLength, *c.CartonWidth, *c.CartonHeight, c.DimensionUnit)
		}
		doc.KeyValueGrid([][2]string{
			{"Packaging", string(c.PackagingType)},
			{"Gross Wt.", gross},
			{"Net Wt.", net},
			{"Dimensions", dims},
		}, 2)

		doc.SectionTitle("Contents")
		rows := make([][]string, 0, len(c.Contents))
		for _, item := range c.Contents {
			rows = append(rows, []string{item.SKU, item.LotNo, csvexport.FormatFloat(item.PackedQty), item.UOM})
		}
		doc.Table([]pdfexport.TableColumn{
			{Header: "SKU", Width: doc.PageContentWidth() * 0.4}, {Header: "Lot", Width: doc.PageContentWidth() * 0.3, Align: "C"},
			{Header: "Qty", Width: doc.PageContentWidth() * 0.15, Align: "R"}, {Header: "UOM", Width: doc.PageContentWidth() * 0.15, Align: "C"},
		}, rows)
	}

	filename := fmt.Sprintf("CartonLabel_%s.pdf", job.JobNo)
	sourceTable, sourceID := "packing_jobs", jobID
	if cartonID != 0 && len(targets) == 1 {
		filename = fmt.Sprintf("CartonLabel_%s.pdf", targets[0].LPN)
		sourceTable, sourceID = "cartons", targets[0].ID
	}
	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "CARTON_LABEL", job.JobNo, sourceTable, sourceID, userID, false, data)
	return data, filename, nil
}

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

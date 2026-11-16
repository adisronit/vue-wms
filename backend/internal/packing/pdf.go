package packing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
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

// PDFService composes Packing's own Service (for Get — both PDFs are
// just that same job detail, rendered) with the tenant/user display-
// name lookups every generated PDF's header/footer needs.
type PDFService struct {
	packing *Service
	tenants tenantNamer
	users   userNamer
}

func NewPDFService(packingSvc *Service, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{packing: packingSvc, tenants: tenants, users: users}
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

// RenderPackingList builds the Packing List document — job header
// (job no., source pick no./order no., customer/destination, warehouse,
// assigned packer/checker) plus every line (SKU/product/lot, ordered/
// picked/packed qty, which carton(s) it went into). Read-only:
// Service.Get never mutates anything, so printing/reprinting is always
// safe regardless of the job's current status.
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

	assignedPacker := csvexport.FormatString(job.AssignedPackerName)
	checker := csvexport.FormatString(job.CheckerName)
	shipTo := csvexport.FormatString(job.ShipToName)
	if job.ShipToAddress != nil {
		shipTo = fmt.Sprintf("%s — %s", shipTo, *job.ShipToAddress)
	}

	doc.KeyValueGrid([][2]string{
		{"Job No.", job.JobNo},
		{"Source Pick No.", job.PickNo},
		{"Source Order No.", job.OrderNo},
		{"Status", string(job.Status)},
		{"Warehouse", job.WarehouseCode},
		{"Customer", csvexport.FormatString(job.Customer)},
		{"Ship To / Destination", shipTo},
		{"Assigned Packer", assignedPacker},
		{"Checker", checker},
		{"Remark", csvexport.FormatString(job.Remark)},
	}, 2)

	doc.SectionTitle("Job Barcode")
	if err := doc.Barcode(job.JobNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)

	doc.SectionTitle("Packing Lines")
	rows := make([][]string, 0, len(job.Items))
	for _, item := range job.Items {
		cartons := "-"
		if len(item.CartonLPNs) > 0 {
			cartons = joinStrings(item.CartonLPNs, ", ")
		}
		rows = append(rows, []string{
			item.SKU, item.ProductName, item.LotNo,
			csvexport.FormatFloat(item.OrderedQty), csvexport.FormatFloat(item.PickedQty), csvexport.FormatFloat(item.PackedQty),
			item.UOM, string(item.Status), cartons,
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 18}, {Header: "Product", Width: 32}, {Header: "Lot No.", Width: 18},
		{Header: "Ordered", Width: 16, Align: "R"}, {Header: "Picked", Width: 16, Align: "R"}, {Header: "Packed", Width: 16, Align: "R"},
		{Header: "UOM", Width: 12, Align: "C"}, {Header: "Status", Width: 20, Align: "C"}, {Header: "Carton(s)", Width: 34, Align: "C"},
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

	doc.SignatureBlock([]string{"Packer", "Checker"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("PackingList_%s.pdf", job.JobNo), err
}

// RenderCartonLabels builds the Carton/Pallet Label document — one
// compact 4x6" (101.6mm x 152.4mm) page per carton, each with the
// carton's own LPN as a Code128 barcode, destination/customer, a
// contents summary, and gross/net weight/dimensions. If cartonID is 0,
// every carton in jobID is rendered (one per page, in LPN order);
// otherwise only that single carton is rendered.
//
// Page-size decision: a 4x6" label format (the de-facto shipping-label
// standard — matches a typical desktop label printer's own media size)
// rather than a scaled-down A4 page, so the printed label is actually
// usable at the size a warehouse label printer expects. pdfexport had
// no configurable page size before this phase (NewDocument was
// hardcoded to A4) — NewLabelDocument was added centrally to
// internal/pdfexport (not hacked around locally here) so any future
// small-format document can reuse the same custom-size + compact-
// header/footer support.
func (s *PDFService) RenderCartonLabels(ctx context.Context, tenantID, userID, jobID, cartonID int64) ([]byte, string, error) {
	job, err := s.packing.Get(ctx, tenantID, jobID)
	if err != nil {
		return nil, "", err
	}

	var targets []CartonResponse
	if cartonID == 0 {
		targets = job.Cartons
	} else {
		for _, c := range job.Cartons {
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

	meta, err := s.meta(ctx, tenantID, userID, "Carton Label", job.JobNo)
	if err != nil {
		return nil, "", err
	}

	// 101.6mm x 152.4mm = 4in x 6in, the standard shipping-label media
	// size.
	const labelWidthMM, labelHeightMM = 101.6, 152.4
	doc := pdfexport.NewLabelDocument(meta, labelWidthMM, labelHeightMM)

	shipTo := csvexport.FormatString(job.ShipToName)
	if job.ShipToAddress != nil {
		shipTo = fmt.Sprintf("%s / %s", shipTo, *job.ShipToAddress)
	}

	for i, c := range targets {
		if i > 0 {
			doc.AddPage()
		}

		doc.KeyValueGrid([][2]string{
			{"Order No.", job.OrderNo},
			{"Customer", csvexport.FormatString(job.Customer)},
			{"Ship To", shipTo},
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
	if cartonID != 0 && len(targets) == 1 {
		filename = fmt.Sprintf("CartonLabel_%s.pdf", targets[0].LPN)
	}
	data, err := doc.Bytes()
	return data, filename, err
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

package order

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
// relocation's own PDFService declares (not imported from there: an
// Order document is not a Relocation Transfer Note, and the two
// packages must not import each other for this).
type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// PDFService composes Order Management's own Service (for Get — the
// Order document is just that same order detail, rendered) with the
// tenant/user display-name lookups every generated PDF's header/footer
// needs — the same composition shape internal/relocation's own
// PDFService uses, kept local to this package.
// docRecorder is the narrow slice of internal/docrender's own Service
// this package needs — see internal/picking/pdf.go's own docRecorder
// doc for the full rationale (wired via SetDocRender, not the
// constructor, nil-checked at the one call site in record below).
type docRecorder interface {
	Record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error
}

type PDFService struct {
	order     *Service
	tenants   tenantNamer
	users     userNamer
	docrender docRecorder
}

func NewPDFService(orderSvc *Service, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{order: orderSvc, tenants: tenants, users: users}
}

// SetDocRender wires the document-history recorder (Phase 33C.3) — see
// docRecorder's own doc.
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

// RenderOrderDocument builds the full Order document — header
// (customer/ship-to, dates, carrier/shipping method, priority/status)
// plus its lines. Read-only: Service.Get never mutates anything, so
// printing/reprinting is always safe regardless of the order's current
// status.
func (s *PDFService) RenderOrderDocument(ctx context.Context, tenantID, userID, orderID int64) ([]byte, string, error) {
	o, err := s.order.Get(ctx, tenantID, orderID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Order Document", o.OrderNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	shipTo := csvexport.FormatString(o.ShipToName)
	if o.ShipToAddress != nil {
		shipTo = fmt.Sprintf("%s — %s", shipTo, *o.ShipToAddress)
	}

	doc.KeyValueGrid([][2]string{
		{"Order No.", o.OrderNo},
		{"Order Type", o.OrderType},
		{"Status", string(o.Status)},
		{"Fulfillment", string(o.FulfillmentStatus)},
		{"Priority", o.Priority},
		{"Warehouse", o.WarehouseCode},
		{"Customer", csvexport.FormatString(o.Customer)},
		{"Ship To", shipTo},
		{"External Reference", csvexport.FormatString(o.ExternalReference)},
		{"Required Shipping Date", formatDate(o.RequiredShippingDate)},
		{"Required Delivery Date", formatDate(o.RequiredDeliveryDate)},
		{"Carrier", csvexport.FormatString(o.Carrier)},
		{"Shipping Method", csvexport.FormatString(o.ShippingMethod)},
		{"Delivery Instructions", csvexport.FormatString(o.DeliveryInstructions)},
		{"Created At", csvexport.FormatTimeValue(o.CreatedAt)},
		{"Released At", csvexport.FormatTime(o.ReleasedAt)},
		{"Shortage Approved", formatBool(o.ShortageApproved)},
		{"Remark", csvexport.FormatString(o.Remark)},
	}, 2)

	doc.SectionTitle("Order Lines")
	rows := make([][]string, 0, len(o.Items))
	for _, item := range o.Items {
		shortfall := ""
		if item.ShortfallQty != nil {
			shortfall = csvexport.FormatFloat(*item.ShortfallQty)
		}
		rows = append(rows, []string{
			item.SKU, item.ProductName, item.StorageType,
			csvexport.FormatFloat(item.OrderedQty), item.UOM,
			csvexport.FormatString(item.PreferredLotNo),
			formatBool(item.ShortageFlag), shortfall,
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 16}, {Header: "Product", Width: 28}, {Header: "Storage", Width: 14, Align: "C"},
		{Header: "Ordered Qty", Width: 14, Align: "R"}, {Header: "UOM", Width: 10, Align: "C"},
		{Header: "Preferred Lot", Width: 16}, {Header: "Shortage", Width: 12, Align: "C"}, {Header: "Shortfall", Width: 14, Align: "R"},
	}, rows)

	doc.SignatureBlock([]string{
		signatureLabel(o.PreparedBy, "Prepared By"),
		signatureLabel(o.ApprovedBy, "Approved By"),
	})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "ORDER", o.OrderNo, "orders", orderID, userID, false, data)
	return data, fmt.Sprintf("Order_%s.pdf", o.OrderNo), nil
}

// formatDate renders a *time.Time as a date-only string (RequiredShipping/
// DeliveryDate are DATE columns, not full timestamps) — empty when nil,
// matching csvexport.FormatTime's own nil convention.
func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// formatBool renders a bool the same way every "Yes/No" PDF field in
// this codebase should — csvexport has no bool formatter of its own.
func formatBool(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// signatureLabel keeps the free-text prepared_by/approved_by name on
// the signature line when set, falling back to the plain role label
// (e.g. "Prepared By") when the field was left blank — a signature
// block still needs SOME label to print under the line either way.
func signatureLabel(name *string, fallback string) string {
	if name != nil && *name != "" {
		return fmt.Sprintf("%s (%s)", fallback, *name)
	}
	return fallback
}

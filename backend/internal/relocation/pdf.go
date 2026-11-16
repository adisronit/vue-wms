package relocation

import (
	"context"
	"fmt"
	"time"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// locationNamer, tenantNamer, and userNamer are the narrow read-only
// slices internal/inbounddoc's own PDF generators depend on, satisfied
// structurally by *location.Repository, *tenant.Repository, and
// *user.Repository. Relocation does not import internal/inbounddoc (and
// vice versa) — a Relocation Transfer Note is not an inbound document —
// so this package declares its own copy of the same narrow interfaces.
type locationNamer interface {
	FindByID(ctx context.Context, tenantID, id int64) (*location.Location, error)
}

type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// PDFService composes the Relocation workflow's own Service (for
// Get — the Transfer Note is just that same task detail, rendered) with
// the tenant/user/location display-name lookups every generated PDF's
// header/footer needs — the same composition shape internal/inbounddoc
// uses over receiving/qualitycheck/putaway, kept local to this package
// instead since a Relocation Transfer Note is not an inbound document.
type PDFService struct {
	relocation *Service
	inventory  *inventory.Service
	locations  locationNamer
	tenants    tenantNamer
	users      userNamer
}

func NewPDFService(relocationSvc *Service, inventorySvc *inventory.Service, locations locationNamer, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{relocation: relocationSvc, inventory: inventorySvc, locations: locations, tenants: tenants, users: users}
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

func (s *PDFService) locationCode(ctx context.Context, tenantID, id int64) string {
	loc, err := s.locations.FindByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Sprintf("#%d", id)
	}
	return loc.Code
}

// RenderTransferNote builds the Relocation Transfer Note — the task
// header plus its lines (source -> destination, requested vs moved,
// lot/expiry/LPN) and the ledger-derived Movement History, matching
// internal/inbounddoc.Service.RenderPutawayTaskSheet's structure.
// Read-only: never mutates inventory, print/reprint is always safe —
// it is just Service.Get's own data, laid out as a PDF.
func (s *PDFService) RenderTransferNote(ctx context.Context, tenantID, userID, taskID int64) ([]byte, string, error) {
	task, err := s.relocation.Get(ctx, tenantID, taskID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Stock Relocation Transfer Note", task.RelocationNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	doc.KeyValueGrid([][2]string{
		{"Relocation No.", task.RelocationNo},
		{"Status", string(task.Status)},
		{"Warehouse", task.WarehouseCode},
		{"Created At", csvexport.FormatTimeValue(task.CreatedAt)},
		{"Ready At", csvexport.FormatTime(task.ReadyAt)},
		{"Completed At", csvexport.FormatTime(task.CompletedAt)},
		{"Remark", csvexport.FormatString(task.Remark)},
	}, 2)

	doc.SectionTitle("Relocation Lines")
	rows := make([][]string, 0, len(task.Items))
	for _, item := range task.Items {
		expiry := ""
		if item.ExpiryDate != nil {
			expiry = item.ExpiryDate.Format("2006-01-02")
		}
		lpn := ""
		if item.LPN != nil {
			lpn = *item.LPN
		}
		rows = append(rows, []string{
			item.SKU, item.ProductName, item.LotNo, expiry, lpn,
			item.SourceLocationCode, item.DestinationLocationCode,
			csvexport.FormatFloat(item.RequestedQty), csvexport.FormatFloat(item.MovedQty), item.UOM, string(item.Status),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 16}, {Header: "Product", Width: 28}, {Header: "Lot No.", Width: 16}, {Header: "Expiry", Width: 16},
		{Header: "LPN", Width: 14},
		{Header: "Source", Width: 18, Align: "C"}, {Header: "Destination", Width: 18, Align: "C"},
		{Header: "Req.", Width: 14, Align: "R"}, {Header: "Moved", Width: 14, Align: "R"},
		{Header: "UOM", Width: 12, Align: "C"}, {Header: "Status", Width: 16, Align: "C"},
	}, rows)

	// Movement History, from the ledger rather than task.Items — a line
	// confirmed across multiple destinations only ever shows its MOST
	// RECENT destination and cumulative MovedQty in the table above (the
	// same documented limitation putaway's own Task Sheet carries); this
	// section is where every individual move this task actually made is
	// still visible. MoveAvailableStockTx always writes the negative
	// (source) leg immediately before the positive (destination) leg
	// within the same DB transaction, so consecutive pairs in id order
	// are always one move's two legs.
	doc.SectionTitle("Movement History")
	txns, err := s.inventory.ListTransactionsByReference(ctx, tenantID, "RELOCATION", task.RelocationNo)
	if err != nil {
		return nil, "", err
	}
	var moveRows [][]string
	for i := 0; i+1 < len(txns); i += 2 {
		from, to := txns[i], txns[i+1]
		sku := ""
		for _, item := range task.Items {
			if item.ProductID == to.ProductID && item.LotNo == to.LotNo {
				sku = item.SKU
				break
			}
		}
		moveRows = append(moveRows, []string{
			sku, to.LotNo, s.locationCode(ctx, tenantID, from.LocationID), s.locationCode(ctx, tenantID, to.LocationID),
			csvexport.FormatFloat(to.Quantity), csvexport.FormatTimeValue(to.CreatedAt),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 26}, {Header: "Lot No.", Width: 24}, {Header: "From", Width: 30, Align: "C"},
		{Header: "To", Width: 30, Align: "C"}, {Header: "Quantity", Width: 24, Align: "R"}, {Header: "When", Width: 52, Align: "C"},
	}, moveRows)

	doc.SignatureBlock([]string{"Relocation Operator", "Warehouse Supervisor"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("Relocation_%s.pdf", task.RelocationNo), err
}

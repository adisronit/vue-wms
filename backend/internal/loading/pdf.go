// Phase 33D.3 -- Loading Manifest. One document per loading_trips row,
// grouping every assigned shipment's own carton worklist under its own
// sub-section -- the same "multiple sub-entities, each with their own
// lines" shape internal/pickingbatch's own Consolidated Batch Picking
// List already established for a batch's own included orders, adapted
// here to a trip's own included shipments. Reuses exactly the same PDF/
// reprint infrastructure 33C.3 already built (internal/pdfexport,
// internal/docrender) -- no new primitive is added here.
package loading

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
// package's own PDF generator needs -- the same local copies internal/
// packing's/internal/pickingbatch's own PDFService each declare rather
// than importing one another's package for it.
type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// docRecorder is the narrow slice of internal/docrender's own Service
// this package needs -- see internal/picking/pdf.go's own docRecorder
// doc for the full rationale.
type docRecorder interface {
	Record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error
}

// PDFService composes Loading's own Service (for Get -- the Loading
// Manifest is just that same trip detail, rendered) with the tenant/
// user display-name lookups the document's header/footer needs.
type PDFService struct {
	loading   *Service
	tenants   tenantNamer
	users     userNamer
	docrender docRecorder
}

func NewPDFService(loadingSvc *Service, tenants tenantNamer, users userNamer) *PDFService {
	return &PDFService{loading: loadingSvc, tenants: tenants, users: users}
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

// RenderLoadingManifest builds the Loading Manifest document -- header
// (manifest/trip no., vehicle no., driver name, carrier, loading dock,
// route/destination, scheduled vs actual loading time, vehicle seal,
// status), one section PER SHIPMENT assigned to the trip (shipment no.,
// order no., customer) each followed by its own carton list (carton
// LPN, planned vs loaded/short status, short reason/remark), a trip-wide
// planned/loaded/short carton summary, a barcode for the trip no., and a
// driver/loader signature block. Read-only: Service.Get never mutates
// anything, so printing/reprinting is always safe regardless of the
// trip's current status.
//
// DRAFT watermark: a trip not yet COMPLETED gets pdfexport's own
// diagonal DRAFT banner (doc.MarkDraft, Phase 33C.3) and is recorded
// with docrender's own is_draft = true, the same "not yet [owning
// module's terminal status] = DRAFT" precedent internal/packing's own
// RenderPackingList established -- a manifest covering a trip that may
// still gain/lose cartons or shipments must never be mistaken for the
// FINAL record of what was actually loaded.
func (s *PDFService) RenderLoadingManifest(ctx context.Context, tenantID, userID, tripID int64) ([]byte, string, error) {
	trip, err := s.loading.Get(ctx, tenantID, tripID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Loading Manifest", trip.TripNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	isDraft := trip.Status != StatusCompleted
	if isDraft {
		doc.MarkDraft()
	}

	reportStatus := string(trip.Status)
	if isDraft {
		reportStatus = string(trip.Status) + " — DRAFT (not yet completed)"
	}

	doc.KeyValueGrid([][2]string{
		{"Manifest No.", trip.TripNo},
		{"Trip No.", trip.TripNo},
		{"Status", reportStatus},
		{"Vehicle No.", csvexport.FormatString(trip.VehicleNo)},
		{"Driver Name", csvexport.FormatString(trip.DriverName)},
		{"Carrier", csvexport.FormatString(trip.Carrier)},
		{"Loading Dock", csvexport.FormatString(trip.LoadingDock)},
		{"Route / Destination", csvexport.FormatString(trip.Route)},
		{"Scheduled Loading Time", csvexport.FormatTime(trip.ScheduledLoadingTime)},
		{"Actual Loading Start", csvexport.FormatTime(trip.ActualLoadingStartAt)},
		{"Actual Loading End", csvexport.FormatTime(trip.ActualLoadingEndAt)},
		{"Vehicle Seal", csvexport.FormatString(trip.VehicleSeal)},
		{"Remark", csvexport.FormatString(trip.Remark)},
	}, 2)

	doc.SectionTitle("Trip Barcode")
	if err := doc.Barcode(trip.TripNo, doc.MarginX(), doc.CursorY(), 70, 16); err != nil {
		return nil, "", err
	}
	doc.Spacer(20)

	cartonsByShipment := map[int64][]CartonResponse{}
	for _, c := range trip.Cartons {
		cartonsByShipment[c.ShipmentID] = append(cartonsByShipment[c.ShipmentID], c)
	}

	for _, sh := range trip.Shipments {
		doc.SectionTitle(fmt.Sprintf("Shipment %s — Order %s — %s", sh.ShipmentNo, sh.OrderNo, csvexport.FormatString(sh.Customer)))
		shipmentCartons := cartonsByShipment[sh.ShipmentID]
		rows := make([][]string, 0, len(shipmentCartons))
		for _, c := range shipmentCartons {
			shortInfo := "-"
			if c.ShortReason != nil {
				shortInfo = *c.ShortReason
				if c.ShortRemark != nil && *c.ShortRemark != "" {
					shortInfo = fmt.Sprintf("%s (%s)", shortInfo, *c.ShortRemark)
				}
			}
			rows = append(rows, []string{c.LPN, string(c.Status), shortInfo})
		}
		doc.Table([]pdfexport.TableColumn{
			{Header: "Carton LPN", Width: 60}, {Header: "Status", Width: 50, Align: "C"}, {Header: "Short Reason", Width: 76},
		}, rows)
	}

	doc.SectionTitle("Trip Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Planned Cartons", fmt.Sprintf("%d", trip.PlannedCount)},
		{"Total Loaded Cartons", fmt.Sprintf("%d", trip.LoadedCount)},
		{"Total Short Cartons", fmt.Sprintf("%d", trip.ShortCount)},
	}, 2)

	doc.SignatureBlock([]string{"Driver", "Loader"})

	data, err := doc.Bytes()
	if err != nil {
		return nil, "", err
	}
	s.record(ctx, tenantID, "LOADING_MANIFEST", trip.TripNo, "loading_trips", tripID, userID, isDraft, data)
	return data, fmt.Sprintf("LoadingManifest_%s.pdf", trip.TripNo), nil
}

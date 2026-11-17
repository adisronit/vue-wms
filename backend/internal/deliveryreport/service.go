package deliveryreport

import (
	"context"
	"fmt"
	"time"

	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

type tenantFinder interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userFinder interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// Service composes Repository with tenant/user lookups for the PDF
// export's header — every method is read-only.
type Service struct {
	repo    *Repository
	tenants tenantFinder
	users   userFinder
}

func NewService(repo *Repository, tenants tenantFinder, users userFinder) *Service {
	return &Service{repo: repo, tenants: tenants, users: users}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Row, int64, error) {
	return s.repo.List(ctx, tenantID, params)
}

func (s *Service) Summary(ctx context.Context, tenantID int64, filters Filters) (Summary, error) {
	return s.repo.Summary(ctx, tenantID, filters)
}

// ListForExport returns every matching row, capped at
// csvexport.MaxExportRows.
func (s *Service) ListForExport(ctx context.Context, tenantID int64, filters Filters) ([]Row, error) {
	return s.repo.ListForExport(ctx, tenantID, filters, csvexport.MaxExportRows)
}

// ListTrips returns the Loading & Dispatch Report's own trip-grain
// rows, capped at csvexport.MaxExportRows — CSV export only, see
// model.go's own doc on TripRow.
func (s *Service) ListTrips(ctx context.Context, tenantID int64, filters Filters) ([]TripRow, error) {
	return s.repo.ListTrips(ctx, tenantID, filters, csvexport.MaxExportRows)
}

func pctOrNA(v *float64) string {
	if v == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%%", *v)
}

func numOrNA(v *float64) string {
	if v == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", *v)
}

// RenderPDF builds the Delivery & Shipping Report PDF: a KPI summary
// grid followed by the (capped) shipment item detail table.
func (s *Service) RenderPDF(ctx context.Context, tenantID, userID int64, filters Filters) ([]byte, error) {
	t, err := s.tenants.FindByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	summary, err := s.repo.Summary(ctx, tenantID, filters)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListForExport(ctx, tenantID, filters, csvexport.MaxExportRows)
	if err != nil {
		return nil, err
	}

	doc := pdfexport.NewDocument(pdfexport.Meta{
		CompanyName: t.Name,
		DocTitle:    "Delivery & Shipping Report",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	})

	doc.SectionTitle("Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Shipments", fmt.Sprintf("%d", summary.TotalShipments)},
		{"Dispatch Rate", fmt.Sprintf("%s (%d/%d)", pctOrNA(summary.DispatchRate.RatePct), summary.DispatchRate.Numerator, summary.DispatchRate.Denominator)},
		{"Delivery Confirmation Rate (self-reported)", fmt.Sprintf("%s (%d/%d)", pctOrNA(summary.DeliveryConfirmationRate.RatePct), summary.DeliveryConfirmationRate.Numerator, summary.DeliveryConfirmationRate.Denominator)},
		{"Exception Rate", fmt.Sprintf("%s (%d/%d)", pctOrNA(summary.ExceptionRate.RatePct), summary.ExceptionRate.Numerator, summary.ExceptionRate.Denominator)},
		{"Avg. Delivery Lead Time", numOrNA(summary.AvgDeliveryLeadTimeHours) + " h"},
		{"Total Loading Trips", fmt.Sprintf("%d (%d completed)", summary.TotalTrips, summary.CompletedTrips)},
		{"Avg. Loading Duration", numOrNA(summary.AvgLoadingDurationHours) + " h"},
	}, 2)

	doc.SectionTitle("Shipment Item Detail")
	columns := []pdfexport.TableColumn{
		{Header: "Shipment No.", Width: 28},
		{Header: "Order No.", Width: 24},
		{Header: "Status", Width: 20},
		{Header: "SKU", Width: 20},
		{Header: "Qty", Width: 16, Align: "R"},
		{Header: "Carrier", Width: 18},
		{Header: "Tracking No.", Width: 24},
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.ShipmentNo, row.OrderNo, row.Status, row.SKU,
			csvexport.FormatFloat(row.Qty), csvexport.FormatString(row.Carrier), csvexport.FormatString(row.TrackingNo),
		})
	}
	doc.Table(columns, tableRows)

	return doc.Bytes()
}

package stockcountreport

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
// export's header — every method is read-only; see package doc.
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

func (s *Service) ListForExport(ctx context.Context, tenantID int64, filters Filters) ([]Row, error) {
	return s.repo.ListForExport(ctx, tenantID, filters, csvexport.MaxExportRows)
}

// RenderPDF builds the Stock Count Report PDF: a KPI summary grid
// followed by the (capped) detail table.
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
		DocTitle:    "Stock Count Report",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	})

	accuracy := "N/A"
	if summary.StockCountAccuracy != nil {
		accuracy = fmt.Sprintf("%.1f%%", *summary.StockCountAccuracy)
	}
	recountRate := "N/A"
	if summary.RecountRate != nil {
		recountRate = fmt.Sprintf("%.1f%%", *summary.RecountRate)
	}

	doc.SectionTitle("Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Count Sessions", fmt.Sprintf("%d", summary.TotalCountSessions)},
		{"Counted Locations", fmt.Sprintf("%d", summary.CountedLocations)},
		{"Counted SKUs", fmt.Sprintf("%d", summary.CountedSKUs)},
		{"Stock Count Accuracy", accuracy},
		{"Quantity Variance", csvexport.FormatFloat(summary.QuantityVariance)},
		{"Recount Rate", recountRate},
		{"Pending Approval", fmt.Sprintf("%d", summary.PendingApproval)},
	}, 2)

	doc.SectionTitle("Count Detail")
	columns := []pdfexport.TableColumn{
		{Header: "Count No.", Width: 28},
		{Header: "Location", Width: 24},
		{Header: "SKU", Width: 24},
		{Header: "System Qty", Width: 20, Align: "R"},
		{Header: "Counted Qty", Width: 20, Align: "R"},
		{Header: "Variance", Width: 18, Align: "R"},
		{Header: "Status", Width: 22},
		{Header: "Adjustment", Width: 30},
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		countedQty := ""
		if row.CountedQty != nil {
			countedQty = csvexport.FormatFloat(*row.CountedQty)
		}
		variance := ""
		if row.VarianceQty != nil {
			variance = csvexport.FormatFloat(*row.VarianceQty)
		}
		tableRows = append(tableRows, []string{
			row.CountNo, row.LocationCode, row.SKU,
			csvexport.FormatFloat(row.SystemQty), countedQty, variance,
			row.ItemStatus, csvexport.FormatString(row.AdjustmentNo),
		})
	}
	doc.Table(columns, tableRows)

	return doc.Bytes()
}

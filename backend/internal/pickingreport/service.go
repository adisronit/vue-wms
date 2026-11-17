package pickingreport

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

func (s *Service) ListBatchAllocations(ctx context.Context, tenantID int64, params BatchAllocationListParams) ([]BatchAllocationRow, int64, error) {
	return s.repo.ListBatchAllocations(ctx, tenantID, params)
}

func (s *Service) ListBatchAllocationsForExport(ctx context.Context, tenantID int64, filters BatchAllocationFilters) ([]BatchAllocationRow, error) {
	return s.repo.ListBatchAllocationsForExport(ctx, tenantID, filters, csvexport.MaxExportRows)
}

// RenderPDF builds the Picking Report PDF: a KPI summary grid followed
// by the (capped) detail table.
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
		DocTitle:    "Picking & Stock Issue Report",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	})

	shortRate := "N/A"
	if summary.ShortPickRate != nil {
		shortRate = fmt.Sprintf("%.1f%%", *summary.ShortPickRate)
	}

	doc.SectionTitle("Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Tasks", fmt.Sprintf("%d", summary.TotalTasks)},
		{"Completed Tasks", fmt.Sprintf("%d", summary.CompletedTasks)},
		{"Total Requested Qty", csvexport.FormatFloat(summary.TotalRequestedQty)},
		{"Total Picked Qty", csvexport.FormatFloat(summary.TotalPickedQty)},
		{"Total Short Qty", csvexport.FormatFloat(summary.TotalShortQty)},
		{"Short Pick Rate", shortRate},
	}, 2)

	doc.SectionTitle("Picking Detail")
	columns := []pdfexport.TableColumn{
		{Header: "Pick No.", Width: 26},
		{Header: "Order No.", Width: 24},
		{Header: "SKU", Width: 22},
		{Header: "Requested", Width: 18, Align: "R"},
		{Header: "Picked", Width: 18, Align: "R"},
		{Header: "Short", Width: 16, Align: "R"},
		{Header: "Status", Width: 22},
		{Header: "Picker", Width: 32},
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		shortQty := ""
		if row.ShortQty != nil {
			shortQty = csvexport.FormatFloat(*row.ShortQty)
		}
		tableRows = append(tableRows, []string{
			row.PickNo, row.OrderNo, row.SKU,
			csvexport.FormatFloat(row.RequestedQty), csvexport.FormatFloat(row.PickedQty), shortQty,
			row.ItemStatus, csvexport.FormatString(row.AssignedPicker),
		})
	}
	doc.Table(columns, tableRows)

	return doc.Bytes()
}

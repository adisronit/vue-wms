package receivingreport

import (
	"context"
	"fmt"
	"time"

	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// tenantFinder/userFinder are the minimal slices of internal/tenant's
// and internal/user's own repositories this package needs (just the
// PDF header's company name / generated-by display name) — mirrors
// internal/inbounddoc's own tenantNamer/userNamer dependency shape
// exactly.
type tenantFinder interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

type userFinder interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// Service composes Repository with tenant/user lookups for the PDF
// export's header — every method is read-only, matching the package
// doc's "a report can never move stock or change a workflow state"
// guarantee.
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
// csvexport.MaxExportRows — the one shared ceiling every synchronous
// export in this codebase uses, applied identically regardless of
// output format (CSV, XLSX, or the PDF table).
func (s *Service) ListForExport(ctx context.Context, tenantID int64, filters Filters) ([]Row, error) {
	return s.repo.ListForExport(ctx, tenantID, filters, csvexport.MaxExportRows)
}

// RenderPDF builds the Receiving Report PDF: a KPI summary grid
// followed by the (capped) detail table. It never calls anything but
// read-only lookups, so repeated rendering can never mutate any state.
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
		DocTitle:    "Receiving Report",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	})

	completionRate := "N/A"
	if summary.CompletionRate != nil {
		completionRate = fmt.Sprintf("%.1f%%", *summary.CompletionRate)
	}
	leadTime := "N/A"
	if summary.AvgReceivingLeadTimeHours != nil {
		leadTime = fmt.Sprintf("%.1f hrs", *summary.AvgReceivingLeadTimeHours)
	}

	doc.SectionTitle("Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Receipts", fmt.Sprintf("%d", summary.TotalReceipts)},
		{"Completion Rate", completionRate},
		{"Avg. Lead Time", leadTime},
		{"Pending QC Qty", csvexport.FormatFloat(summary.PendingQCQty)},
		{"Pending Putaway Qty", csvexport.FormatFloat(summary.PendingPutawayQty)},
	}, 2)

	doc.SectionTitle("Receiving Detail")
	columns := []pdfexport.TableColumn{
		{Header: "Receipt No.", Width: 30},
		{Header: "Date", Width: 24},
		{Header: "Supplier", Width: 34},
		{Header: "SKU", Width: 22},
		{Header: "Expected", Width: 18, Align: "R"},
		{Header: "Received", Width: 18, Align: "R"},
		{Header: "Lot", Width: 20},
		{Header: "Status", Width: 20},
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.ReceiptNo,
			csvexport.FormatTime(row.ReceivingDate),
			csvexport.FormatString(row.SupplierName),
			row.SKU,
			csvexport.FormatFloat(row.ExpectedQty),
			csvexport.FormatFloat(row.ReceivedQty),
			csvexport.FormatString(row.LotNo),
			row.ReceivingStatus,
		})
	}
	doc.Table(columns, tableRows)

	return doc.Bytes()
}

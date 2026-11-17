package inventoryreport

import (
	"context"
	"fmt"
	"strings"
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

type Service struct {
	repo    *Repository
	tenants tenantFinder
	users   userFinder
}

func NewService(repo *Repository, tenants tenantFinder, users userFinder) *Service {
	return &Service{repo: repo, tenants: tenants, users: users}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Row, int64, error) {
	return s.repo.List(ctx, tenantID, params, time.Now().UTC())
}

func (s *Service) Summary(ctx context.Context, tenantID int64, filters Filters) (Summary, error) {
	return s.repo.Summary(ctx, tenantID, filters, time.Now().UTC())
}

func (s *Service) ListForExport(ctx context.Context, tenantID int64, filters Filters) ([]Row, error) {
	return s.repo.ListForExport(ctx, tenantID, filters, time.Now().UTC(), csvexport.MaxExportRows)
}

func (s *Service) ListMovements(ctx context.Context, tenantID int64, params MovementListParams) ([]MovementRow, int64, error) {
	return s.repo.ListMovements(ctx, tenantID, params)
}

func (s *Service) ListMovementsForExport(ctx context.Context, tenantID int64, filters MovementFilters) ([]MovementRow, error) {
	return s.repo.ListMovementsForExport(ctx, tenantID, filters, csvexport.MaxExportRows)
}

func (s *Service) ListAsOf(ctx context.Context, tenantID int64, params AsOfListParams) ([]AsOfRow, int64, error) {
	return s.repo.ListAsOf(ctx, tenantID, params)
}

// RenderPDF builds the Inventory Report PDF: a KPI summary grid
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

	now := time.Now().UTC()
	summary, err := s.repo.Summary(ctx, tenantID, filters, now)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListForExport(ctx, tenantID, filters, now, csvexport.MaxExportRows)
	if err != nil {
		return nil, err
	}

	doc := pdfexport.NewDocument(pdfexport.Meta{
		CompanyName: t.Name,
		DocTitle:    "Inventory Balance Report",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	})

	onHandLine := "0"
	if len(summary.TotalOnHandByUOM) > 0 {
		parts := make([]string, 0, len(summary.TotalOnHandByUOM))
		for _, uq := range summary.TotalOnHandByUOM {
			parts = append(parts, fmt.Sprintf("%s %s", csvexport.FormatFloat(uq.Qty), uq.UOM))
		}
		onHandLine = strings.Join(parts, ", ")
	}
	intOrNA := func(v *int64) string {
		if v == nil {
			return "N/A"
		}
		return fmt.Sprintf("%d", *v)
	}

	doc.SectionTitle("Summary")
	doc.KeyValueGrid([][2]string{
		{"Total On-Hand", onHandLine},
		{"Low Stock Count", intOrNA(summary.LowStockCount)},
		{"Near Expiry Count", intOrNA(summary.NearExpiryCount)},
		{"Expired Count", intOrNA(summary.ExpiredCount)},
		{"Non-Moving Count", intOrNA(summary.NonMovingCount)},
	}, 2)

	doc.SectionTitle("Inventory Detail")
	columns := []pdfexport.TableColumn{
		{Header: "Warehouse", Width: 26},
		{Header: "Location", Width: 22},
		{Header: "SKU", Width: 22},
		{Header: "Lot", Width: 20},
		{Header: "On-Hand", Width: 18, Align: "R"},
		{Header: "Available", Width: 18, Align: "R"},
		{Header: "Status", Width: 24},
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.WarehouseCode, row.LocationCode, row.SKU, row.LotNo,
			csvexport.FormatFloat(row.OnHand), csvexport.FormatFloat(row.Available), row.InventoryStatus,
		})
	}
	doc.Table(columns, tableRows)

	return doc.Bytes()
}

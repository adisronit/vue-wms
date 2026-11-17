package pickingkpi

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

type Service struct {
	repo    *Repository
	tenants tenantFinder
	users   userFinder
}

func NewService(repo *Repository, tenants tenantFinder, users userFinder) *Service {
	return &Service{repo: repo, tenants: tenants, users: users}
}

func (s *Service) Summary(ctx context.Context, tenantID int64, f Filters) (Summary, error) {
	return s.repo.Summary(ctx, tenantID, f)
}

const maxSourceRows = csvexport.MaxExportRows

func (s *Service) ListSource(ctx context.Context, tenantID int64, f Filters) ([]SourceRow, error) {
	return s.repo.ListSource(ctx, tenantID, f, maxSourceRows)
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

// RenderPDF builds the Picking KPI Dashboard PDF: formula-labeled KPI
// grids followed by the operator/warehouse/picking-mode breakdowns.
func (s *Service) RenderPDF(ctx context.Context, tenantID, userID int64, f Filters) ([]byte, error) {
	t, err := s.tenants.FindByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	summary, err := s.repo.Summary(ctx, tenantID, f)
	if err != nil {
		return nil, err
	}

	doc := pdfexport.NewDocument(pdfexport.Meta{
		CompanyName: t.Name,
		DocTitle:    "Picking KPI Dashboard",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	})

	doc.SectionTitle("Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Tasks", fmt.Sprintf("%d", summary.TotalTasks)},
		{"Completed Tasks", fmt.Sprintf("%d", summary.CompletedTasks)},
		{"Completion Rate", pctOrNA(summary.CompletionRatePct)},
		{"Picked Lines", fmt.Sprintf("%d", summary.PickedLines)},
		{"Picked Units", csvexport.FormatFloat(summary.PickedUnits)},
		{"Lines/Hour", numOrNA(summary.LinesPerHour)},
		{"Units/Hour", numOrNA(summary.UnitsPerHour)},
		{"Accuracy", fmt.Sprintf("%s (%d/%d scans)", pctOrNA(summary.Accuracy.RatePct), summary.Accuracy.Numerator, summary.Accuracy.Denominator)},
		{"On-Time Rate", fmt.Sprintf("%s (%d/%d tasks with due date)", pctOrNA(summary.OnTime.RatePct), summary.OnTime.Numerator, summary.OnTime.Denominator)},
		{"Avg. Cycle Time", numOrNA(summary.AvgCycleTimeHours) + " h"},
		{"Short Pick Rate", pctOrNA(summary.ShortPickRatePct)},
		{"Re-pick Rate", pctOrNA(summary.RepickRatePct)},
	}, 2)

	doc.SectionTitle("By Operator")
	opColumns := []pdfexport.TableColumn{
		{Header: "Operator", Width: 50},
		{Header: "Tasks", Width: 24, Align: "R"},
		{Header: "Completed", Width: 28, Align: "R"},
		{Header: "Lines/Hour", Width: 30, Align: "R"},
		{Header: "Units/Hour", Width: 30, Align: "R"},
	}
	opRows := make([][]string, 0, len(summary.ByOperator))
	for _, row := range summary.ByOperator {
		opRows = append(opRows, []string{
			row.Key, fmt.Sprintf("%d", row.TaskCount), fmt.Sprintf("%d", row.CompletedTaskCount),
			numOrNA(row.LinesPerHour), numOrNA(row.UnitsPerHour),
		})
	}
	doc.Table(opColumns, opRows)

	doc.SectionTitle("By Picking Mode")
	modeColumns := []pdfexport.TableColumn{
		{Header: "Mode", Width: 40},
		{Header: "Tasks", Width: 30, Align: "R"},
		{Header: "Completion Rate", Width: 40, Align: "R"},
		{Header: "Avg. Cycle Time (h)", Width: 50, Align: "R"},
	}
	modeRows := make([][]string, 0, len(summary.ByPickingMode))
	for _, row := range summary.ByPickingMode {
		modeRows = append(modeRows, []string{
			row.PickingMode, fmt.Sprintf("%d", row.TaskCount), pctOrNA(row.CompletionRatePct), numOrNA(row.AvgCycleTimeHours),
		})
	}
	doc.Table(modeColumns, modeRows)

	return doc.Bytes()
}

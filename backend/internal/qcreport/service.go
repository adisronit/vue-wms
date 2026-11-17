package qcreport

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

// now returns the current moment using the same "local wall-clock
// reading, mislabeled UTC" convention every created_at/updated_at
// column in this entire database is actually written with. GORM's
// default clock populates those columns with the Go process's plain
// time.Now() (its LOCAL wall clock — e.g. host time +07:00), and the
// SQL Server driver writes a time.Time's wall-clock fields verbatim
// into DATETIME2, discarding the attached offset entirely rather than
// converting it to a true UTC instant first. A properly-converted
// time.Now().UTC() is therefore a DIFFERENT instant than what the
// write path actually stamped — off by exactly the host's UTC offset
// (confirmed directly against the disposable SQL Server: created_at
// read back as "11:33:02 UTC" while SYSUTCDATETIME() read "04:33:02
// UTC" for the same real moment, a 7-hour gap matching the host's
// +07:00 local offset). Hold Aging is the first Phase 34 report
// metric to compute an elapsed duration between a DB-stored
// created_at and a freshly computed reference instant (every other
// report's date-relative math either compares two DB-stored columns
// against each other, which cancels this out, or uses day-granularity
// range filters, which mask a few hours of drift) — see
// repository_test.go's TestRepository_List_HoldAgingHours for the test
// that caught it. This relabels Go's local wall-clock reading as UTC
// (same numbers, different label) so it lines up with the convention
// every existing timestamp in this schema already uses, rather than
// attempting to fix that systemic, pre-existing write-path quirk
// here — doing so would touch every GORM-timestamped table in the
// application, far outside this report's own scope.
func now() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), n.Second(), n.Nanosecond(), time.UTC)
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]Row, int64, error) {
	return s.repo.List(ctx, tenantID, params, now())
}

func (s *Service) Summary(ctx context.Context, tenantID int64, filters Filters) (Summary, error) {
	return s.repo.Summary(ctx, tenantID, filters, now())
}

// ListForExport returns every matching row, capped at
// csvexport.MaxExportRows.
func (s *Service) ListForExport(ctx context.Context, tenantID int64, filters Filters) ([]Row, error) {
	return s.repo.ListForExport(ctx, tenantID, filters, now(), csvexport.MaxExportRows)
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

// RenderPDF builds the QC Report PDF: a KPI summary grid, the top
// defects/pending-QC breakdown, followed by the (capped) detail table.
func (s *Service) RenderPDF(ctx context.Context, tenantID, userID int64, filters Filters) ([]byte, error) {
	t, err := s.tenants.FindByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	reportNow := now()
	summary, err := s.repo.Summary(ctx, tenantID, filters, reportNow)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListForExport(ctx, tenantID, filters, reportNow, csvexport.MaxExportRows)
	if err != nil {
		return nil, err
	}

	doc := pdfexport.NewDocument(pdfexport.Meta{
		CompanyName: t.Name,
		DocTitle:    "QC Report",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	})

	doc.SectionTitle("Summary")
	doc.KeyValueGrid([][2]string{
		{"Total Inspection Items", fmt.Sprintf("%d", summary.TotalInspectionItems)},
		{"Pass Rate", pctOrNA(summary.PassRate.RatePct)},
		{"Rejection Rate", pctOrNA(summary.RejectionRate.RatePct)},
		{"Hold Rate", pctOrNA(summary.HoldRate.RatePct)},
		{"Pending QC - Not Started", fmt.Sprintf("%d items / %s qty", summary.PendingQCNotStartedCount, csvexport.FormatFloat(summary.PendingQCNotStartedQty))},
		{"Pending QC - Awaiting Decision", fmt.Sprintf("%d items / %s qty", summary.PendingQCAwaitingDecisionCount, csvexport.FormatFloat(summary.PendingQCAwaitingDecisionQty))},
		{"Avg. QC Turnaround", numOrNA(summary.AvgTurnaroundHours) + " h"},
		{"Avg. Hold Aging", numOrNA(summary.AvgHoldAgingHours) + " h"},
		{"Max. Hold Aging", numOrNA(summary.MaxHoldAgingHours) + " h"},
	}, 2)

	doc.SectionTitle("Top Defects")
	defectColumns := []pdfexport.TableColumn{
		{Header: "Defect Code", Width: 50},
		{Header: "Count", Width: 30, Align: "R"},
		{Header: "Qty", Width: 30, Align: "R"},
	}
	defectRows := make([][]string, 0, len(summary.Charts.TopDefects))
	for _, d := range summary.Charts.TopDefects {
		defectRows = append(defectRows, []string{d.DefectCode, fmt.Sprintf("%d", d.Count), csvexport.FormatFloat(d.Qty)})
	}
	doc.Table(defectColumns, defectRows)

	doc.SectionTitle("QC Detail")
	columns := []pdfexport.TableColumn{
		{Header: "Inspection No.", Width: 28},
		{Header: "Receipt No.", Width: 24},
		{Header: "SKU", Width: 20},
		{Header: "Lot", Width: 18},
		{Header: "Pass Qty", Width: 16, Align: "R"},
		{Header: "Reject Qty", Width: 16, Align: "R"},
		{Header: "Status", Width: 20},
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.InspectionNo, row.ReceiptNo, row.SKU, row.LotNo,
			csvexport.FormatFloat(row.PassQty), csvexport.FormatFloat(row.RejectQty),
			row.ResolutionStatus,
		})
	}
	doc.Table(columns, tableRows)

	return doc.Bytes()
}

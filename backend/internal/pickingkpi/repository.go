package pickingkpi

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// scopeWhere builds the WHERE clause every task-grain query shares,
// referencing pt (picking_tasks, always joined), w (warehouses), and
// u (users, LEFT JOIN).
func scopeWhere(tenantID int64, f Filters) (string, []interface{}) {
	var sb strings.Builder
	args := []interface{}{tenantID}
	sb.WriteString("pt.tenant_id = ?")

	if f.From != nil {
		sb.WriteString(" AND pt.created_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		sb.WriteString(" AND pt.created_at < ?")
		args = append(args, *f.To)
	}
	if f.WarehouseID != nil {
		sb.WriteString(" AND pt.warehouse_id = ?")
		args = append(args, *f.WarehouseID)
	}
	if f.PickerID != nil {
		sb.WriteString(" AND pt.assigned_picker_id = ?")
		args = append(args, *f.PickerID)
	}

	return sb.String(), args
}

const taskBaseFrom = `FROM picking_tasks pt
JOIN warehouses w ON w.id = pt.warehouse_id
LEFT JOIN users u ON u.id = pt.assigned_picker_id
LEFT JOIN picking_batches pb ON pb.picking_task_id = pt.id AND pb.tenant_id = pt.tenant_id
`

const itemBaseFrom = `FROM picking_tasks pt
JOIN picking_task_items pti ON pti.picking_task_id = pt.id
JOIN warehouses w ON w.id = pt.warehouse_id
LEFT JOIN users u ON u.id = pt.assigned_picker_id
LEFT JOIN picking_batches pb ON pb.picking_task_id = pt.id AND pb.tenant_id = pt.tenant_id
`

const scanBaseFrom = `FROM picking_tasks pt
JOIN picking_scan_log psl ON psl.picking_task_id = pt.id
JOIN warehouses w ON w.id = pt.warehouse_id
LEFT JOIN users u ON u.id = pt.assigned_picker_id
`

type taskAgg struct {
	TotalTasks                int64
	CompletedTasks            int64
	AvgCycleHours             *float64
	AvgActiveHours            *float64
	MissingAssignedAt         int64
	TasksWithDueDate          int64
	TasksOnTime               int64
	TasksWithoutDueDate       int64
	SumCompletedDurationHours *float64
}

func (repo *Repository) taskAggregate(ctx context.Context, tenantID int64, f Filters) (taskAgg, error) {
	where, args := scopeWhere(tenantID, f)
	sql := `SELECT
		COUNT(*) AS total_tasks,
		SUM(CASE WHEN pt.status = 'COMPLETED' THEN 1 ELSE 0 END) AS completed_tasks,
		AVG(CASE WHEN pt.status = 'COMPLETED' THEN CAST(DATEDIFF(MINUTE, pt.created_at, pt.completed_at) AS FLOAT) / 60.0 END) AS avg_cycle_hours,
		AVG(CASE WHEN pt.status = 'COMPLETED' AND pt.assigned_at IS NOT NULL THEN CAST(DATEDIFF(MINUTE, pt.assigned_at, pt.completed_at) AS FLOAT) / 60.0 END) AS avg_active_hours,
		SUM(CASE WHEN pt.assigned_at IS NULL THEN 1 ELSE 0 END) AS missing_assigned_at,
		SUM(CASE WHEN pt.due_date IS NOT NULL THEN 1 ELSE 0 END) AS tasks_with_due_date,
		-- due_date is a DATE column (no time-of-day) — "on time" means
		-- completed by the end of that calendar day, so the comparison
		-- must be at date granularity, not against completed_at's full
		-- timestamp (which would wrongly fail for anything completed
		-- after midnight on the due date itself).
		SUM(CASE WHEN pt.due_date IS NOT NULL AND pt.status = 'COMPLETED' AND CAST(pt.completed_at AS DATE) <= pt.due_date THEN 1 ELSE 0 END) AS tasks_on_time,
		SUM(CASE WHEN pt.due_date IS NULL THEN 1 ELSE 0 END) AS tasks_without_due_date,
		SUM(CASE WHEN pt.status = 'COMPLETED' THEN CAST(DATEDIFF(MINUTE, pt.created_at, pt.completed_at) AS FLOAT) / 60.0 ELSE 0 END) AS sum_completed_duration_hours
	` + taskBaseFrom + ` WHERE ` + where

	var agg taskAgg
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&agg).Error; err != nil {
		return taskAgg{}, err
	}
	return agg, nil
}

type itemAgg struct {
	TotalLines    int64
	PickedLines   int64
	PickedUnits   *float64
	ShortLines    int64
	RepickedLines int64
	TotalRepicks  int64
}

func (repo *Repository) itemAggregate(ctx context.Context, tenantID int64, f Filters) (itemAgg, error) {
	where, args := scopeWhere(tenantID, f)
	sql := `SELECT
		COUNT(*) AS total_lines,
		SUM(CASE WHEN pti.status = 'COMPLETED' THEN 1 ELSE 0 END) AS picked_lines,
		SUM(pti.picked_qty) AS picked_units,
		SUM(CASE WHEN pti.status = 'SHORT' THEN 1 ELSE 0 END) AS short_lines,
		SUM(CASE WHEN pti.repick_count > 0 THEN 1 ELSE 0 END) AS repicked_lines,
		SUM(pti.repick_count) AS total_repicks
	` + itemBaseFrom + ` WHERE ` + where

	var agg itemAgg
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&agg).Error; err != nil {
		return itemAgg{}, err
	}
	return agg, nil
}

type scanAgg struct {
	MatchCount int64
	ScanCount  int64
}

func (repo *Repository) scanAggregate(ctx context.Context, tenantID int64, f Filters) (scanAgg, error) {
	where, args := scopeWhere(tenantID, f)
	sql := `SELECT
		SUM(CASE WHEN psl.result = 'MATCH' THEN 1 ELSE 0 END) AS match_count,
		COUNT(*) AS scan_count
	` + scanBaseFrom + ` WHERE ` + where

	var agg scanAgg
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&agg).Error; err != nil {
		return scanAgg{}, err
	}
	return agg, nil
}

// groupDuration/groupLines are joined by key in Go — see productivityByGroup.
type groupDuration struct {
	Key                string
	TaskCount          int64
	CompletedTaskCount int64
	DurationHours      float64
}

type groupLines struct {
	Key         string
	PickedLines int64
	PickedUnits float64
}

func (repo *Repository) productivityByGroup(ctx context.Context, tenantID int64, f Filters, groupExpr string) ([]ProductivityRow, error) {
	where, args := scopeWhere(tenantID, f)

	durationSQL := `SELECT ` + groupExpr + ` AS [key],
		COUNT(*) AS task_count,
		SUM(CASE WHEN pt.status = 'COMPLETED' THEN 1 ELSE 0 END) AS completed_task_count,
		SUM(CASE WHEN pt.status = 'COMPLETED' THEN CAST(DATEDIFF(MINUTE, pt.created_at, pt.completed_at) AS FLOAT) / 60.0 ELSE 0 END) AS duration_hours
	` + taskBaseFrom + ` WHERE ` + where + ` GROUP BY ` + groupExpr

	var durations []groupDuration
	if err := repo.db.WithContext(ctx).Raw(durationSQL, args...).Scan(&durations).Error; err != nil {
		return nil, err
	}

	linesSQL := `SELECT ` + groupExpr + ` AS [key],
		SUM(CASE WHEN pti.status = 'COMPLETED' THEN 1 ELSE 0 END) AS picked_lines,
		SUM(pti.picked_qty) AS picked_units
	` + itemBaseFrom + ` WHERE ` + where + ` GROUP BY ` + groupExpr

	var lines []groupLines
	if err := repo.db.WithContext(ctx).Raw(linesSQL, args...).Scan(&lines).Error; err != nil {
		return nil, err
	}

	linesByKey := make(map[string]groupLines, len(lines))
	for _, l := range lines {
		linesByKey[l.Key] = l
	}

	rows := make([]ProductivityRow, 0, len(durations))
	for _, d := range durations {
		l := linesByKey[d.Key]
		row := ProductivityRow{
			Key: d.Key, TaskCount: d.TaskCount, CompletedTaskCount: d.CompletedTaskCount,
			PickedLines: l.PickedLines, PickedUnits: l.PickedUnits,
		}
		if d.DurationHours > 0 {
			lph := float64(l.PickedLines) / d.DurationHours
			uph := l.PickedUnits / d.DurationHours
			row.LinesPerHour = &lph
			row.UnitsPerHour = &uph
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (repo *Repository) pickingModeComparison(ctx context.Context, tenantID int64, f Filters) ([]PickingModeRow, error) {
	where, args := scopeWhere(tenantID, f)
	const modeExpr = "CASE WHEN pb.id IS NOT NULL THEN 'BATCH' ELSE 'PICK_BY_ORDER' END"

	taskSQL := `SELECT ` + modeExpr + ` AS picking_mode,
		COUNT(*) AS task_count,
		SUM(CASE WHEN pt.status = 'COMPLETED' THEN 1 ELSE 0 END) AS completed_tasks,
		AVG(CASE WHEN pt.status = 'COMPLETED' THEN CAST(DATEDIFF(MINUTE, pt.created_at, pt.completed_at) AS FLOAT) / 60.0 END) AS avg_cycle_hours
	` + taskBaseFrom + ` WHERE ` + where + ` GROUP BY ` + modeExpr

	type rawMode struct {
		PickingMode    string
		TaskCount      int64
		CompletedTasks int64
		AvgCycleHours  *float64
	}
	var rawRows []rawMode
	if err := repo.db.WithContext(ctx).Raw(taskSQL, args...).Scan(&rawRows).Error; err != nil {
		return nil, err
	}

	itemSQL := `SELECT ` + modeExpr + ` AS picking_mode,
		COUNT(*) AS total_lines,
		SUM(CASE WHEN pti.status = 'SHORT' THEN 1 ELSE 0 END) AS short_lines
	` + itemBaseFrom + ` WHERE ` + where + ` GROUP BY ` + modeExpr

	type rawShort struct {
		PickingMode string
		TotalLines  int64
		ShortLines  int64
	}
	var shortRows []rawShort
	if err := repo.db.WithContext(ctx).Raw(itemSQL, args...).Scan(&shortRows).Error; err != nil {
		return nil, err
	}
	shortByMode := make(map[string]rawShort, len(shortRows))
	for _, s := range shortRows {
		shortByMode[s.PickingMode] = s
	}

	rows := make([]PickingModeRow, 0, len(rawRows))
	for _, r := range rawRows {
		row := PickingModeRow{PickingMode: r.PickingMode, TaskCount: r.TaskCount, AvgCycleTimeHours: r.AvgCycleHours}
		if r.TaskCount > 0 {
			rate := float64(r.CompletedTasks) / float64(r.TaskCount) * 100
			row.CompletionRatePct = &rate
		}
		if s, ok := shortByMode[r.PickingMode]; ok && s.TotalLines > 0 {
			rate := float64(s.ShortLines) / float64(s.TotalLines) * 100
			row.ShortPickRatePct = &rate
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (repo *Repository) dailyTrend(ctx context.Context, tenantID int64, f Filters) ([]TrendPoint, error) {
	where, args := scopeWhere(tenantID, f)
	sql := `SELECT
		CONVERT(varchar(10), pt.created_at, 23) AS date,
		SUM(CASE WHEN pt.status = 'COMPLETED' THEN 1 ELSE 0 END) AS completed_tasks,
		SUM(CASE WHEN pti.status = 'COMPLETED' THEN 1 ELSE 0 END) AS picked_lines,
		SUM(pti.picked_qty) AS picked_units,
		SUM(CASE WHEN pti.status = 'SHORT' THEN 1 ELSE 0 END) AS short_lines
	` + itemBaseFrom + ` WHERE ` + where + `
	GROUP BY CONVERT(varchar(10), pt.created_at, 23)
	ORDER BY date`

	var rows []TrendPoint
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (repo *Repository) cycleTimeDistribution(ctx context.Context, tenantID int64, f Filters) ([]DurationBucket, error) {
	where, args := scopeWhere(tenantID, f)
	sql := `SELECT
		CASE
			WHEN h < 1 THEN '0-1h'
			WHEN h < 4 THEN '1-4h'
			WHEN h < 8 THEN '4-8h'
			WHEN h < 24 THEN '8-24h'
			ELSE '24h+'
		END AS label,
		COUNT(*) AS count
	FROM (
		SELECT CAST(DATEDIFF(MINUTE, pt.created_at, pt.completed_at) AS FLOAT) / 60.0 AS h
		` + taskBaseFrom + ` WHERE ` + where + ` AND pt.status = 'COMPLETED'
	) d
	GROUP BY CASE
		WHEN h < 1 THEN '0-1h'
		WHEN h < 4 THEN '1-4h'
		WHEN h < 8 THEN '4-8h'
		WHEN h < 24 THEN '8-24h'
		ELSE '24h+'
	END`

	var rows []DurationBucket
	if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (repo *Repository) Summary(ctx context.Context, tenantID int64, f Filters) (Summary, error) {
	var summary Summary

	ta, err := repo.taskAggregate(ctx, tenantID, f)
	if err != nil {
		return Summary{}, err
	}
	summary.TotalTasks = ta.TotalTasks
	summary.CompletedTasks = ta.CompletedTasks
	if ta.TotalTasks > 0 {
		rate := float64(ta.CompletedTasks) / float64(ta.TotalTasks) * 100
		summary.CompletionRatePct = &rate
	}
	summary.AvgCycleTimeHours = ta.AvgCycleHours
	summary.AvgActivePickingDurationHours = ta.AvgActiveHours
	summary.TasksMissingAssignedAt = ta.MissingAssignedAt
	summary.TasksWithoutDueDate = ta.TasksWithoutDueDate
	summary.OnTime = RateWithDenominator{Numerator: ta.TasksOnTime, Denominator: ta.TasksWithDueDate}
	if ta.TasksWithDueDate > 0 {
		rate := float64(ta.TasksOnTime) / float64(ta.TasksWithDueDate) * 100
		summary.OnTime.RatePct = &rate
	}

	ia, err := repo.itemAggregate(ctx, tenantID, f)
	if err != nil {
		return Summary{}, err
	}
	summary.PickedLines = ia.PickedLines
	if ia.PickedUnits != nil {
		summary.PickedUnits = *ia.PickedUnits
	}
	summary.TotalRepicks = ia.TotalRepicks
	if ia.TotalLines > 0 {
		shortRate := float64(ia.ShortLines) / float64(ia.TotalLines) * 100
		summary.ShortPickRatePct = &shortRate
		repickRate := float64(ia.RepickedLines) / float64(ia.TotalLines) * 100
		summary.RepickRatePct = &repickRate
	}
	if ta.SumCompletedDurationHours != nil && *ta.SumCompletedDurationHours > 0 {
		lph := float64(ia.PickedLines) / *ta.SumCompletedDurationHours
		summary.LinesPerHour = &lph
		if ia.PickedUnits != nil {
			uph := *ia.PickedUnits / *ta.SumCompletedDurationHours
			summary.UnitsPerHour = &uph
		}
	}

	sa, err := repo.scanAggregate(ctx, tenantID, f)
	if err != nil {
		return Summary{}, err
	}
	summary.Accuracy = RateWithDenominator{Numerator: sa.MatchCount, Denominator: sa.ScanCount}
	if sa.ScanCount > 0 {
		rate := float64(sa.MatchCount) / float64(sa.ScanCount) * 100
		summary.Accuracy.RatePct = &rate
	}

	byOperator, err := repo.productivityByGroup(ctx, tenantID, f, "COALESCE(u.full_name, 'Unassigned')")
	if err != nil {
		return Summary{}, err
	}
	summary.ByOperator = byOperator

	byWarehouse, err := repo.productivityByGroup(ctx, tenantID, f, "w.name")
	if err != nil {
		return Summary{}, err
	}
	summary.ByWarehouse = byWarehouse

	byMode, err := repo.pickingModeComparison(ctx, tenantID, f)
	if err != nil {
		return Summary{}, err
	}
	summary.ByPickingMode = byMode

	trend, err := repo.dailyTrend(ctx, tenantID, f)
	if err != nil {
		return Summary{}, err
	}
	distribution, err := repo.cycleTimeDistribution(ctx, tenantID, f)
	if err != nil {
		return Summary{}, err
	}
	summary.Charts = Charts{
		DailyTrend: trend, ByOperator: byOperator, CycleTimeDistribution: distribution,
	}

	return summary, nil
}

func (repo *Repository) ListSource(ctx context.Context, tenantID int64, f Filters, maxRows int) ([]SourceRow, error) {
	where, args := scopeWhere(tenantID, f)
	const modeExpr = "CASE WHEN pb.id IS NOT NULL THEN 'BATCH' ELSE 'PICK_BY_ORDER' END"
	sql := `SELECT
		pt.id AS task_id, pt.pick_no, w.name AS warehouse_name, ` + modeExpr + ` AS picking_mode,
		u.full_name AS assigned_picker, pt.created_at, pt.assigned_at, pt.completed_at, pt.due_date,
		p.sku, pti.ordered_qty, pti.picked_qty, pti.short_qty, pti.repick_count,
		pti.status AS item_status, pt.status AS task_status
	` + itemBaseFrom + `
	JOIN products p ON p.id = pti.product_id
	WHERE ` + where + `
	ORDER BY pt.created_at DESC, pt.id DESC
	OFFSET 0 ROWS FETCH NEXT ? ROWS ONLY`
	dataArgs := append(append([]interface{}{}, args...), maxRows)

	var rows []SourceRow
	if err := repo.db.WithContext(ctx).Raw(sql, dataArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Package pickingkpi implements Phase 34F's Picking KPI Dashboard. It
// is a KPI dashboard, not a new detail-row report — it computes real
// aggregates from picking_tasks/picking_task_items/picking_scan_log/
// picking_batches directly via raw SQL (same no-Go-dependency shape
// every other Phase 34 report package uses), never fabricating a
// metric it cannot reliably measure.
//
// Every formula below was decided from a direct audit of
// internal/picking's own service.go, not assumed:
//
//   - Picked Lines/Hour, Picked Units/Hour: SUM(lines or units) over
//     COMPLETED tasks in scope, divided by SUM(each task's own
//     CompletedAt-CreatedAt duration in hours) — a throughput-vs-task-
//     duration rate, NOT a true per-worked-hour rate (no clock-in/
//     clock-out or shift-log table exists anywhere in this schema).
//     Nil when there are zero completed tasks with a positive
//     duration in scope.
//
//   - Picking Accuracy: SUM(MATCH)/SUM(MATCH+MISMATCH) from
//     picking_scan_log rows in scope ONLY. ScanConfirm is a fully
//     separate action from Confirm (confirmed directly in
//     service.go) — a task can reach COMPLETED with zero scan log
//     rows. This KPI is therefore computed over, and only
//     representative of, the scanned subset; ScannedLines/TotalLines
//     are both returned so the frontend can show exactly how partial
//     that coverage is. Nil when zero scan log rows exist in scope.
//
//   - Picking Completion Rate: COMPLETED tasks / all tasks in scope.
//     Always computable when total > 0.
//
//   - On-Time Picking Rate: among tasks that HAVE a due_date set
//     (DueDate is genuinely optional, confirmed no binding:"required"
//     anywhere), the fraction completed by that due date. due_date is
//     a DATE column with no time-of-day, so "on time" compares at
//     date granularity (CAST(completed_at AS DATE) <= due_date) — a
//     task completed any time on its due date counts as on time, not
//     only if completed before midnight. TasksWithoutDueDate is
//     always returned alongside it so a caller never mistakes "100%
//     on time" for "every task had a deadline." Nil when zero tasks
//     in scope have a due date.
//
//   - Average Picking Cycle Time: AVG(CompletedAt - CreatedAt) over
//     COMPLETED tasks in scope — available for every terminal task.
//     Average Active Picking Duration: AVG(CompletedAt - AssignedAt),
//     computed only over the subset where AssignedAt is actually set
//     (a task can be Confirmed without ever being assigned) — this is
//     the closer proxy for "time actually spent picking" but is not
//     universally available, so it is reported as its own separate,
//     independently-nullable figure, never silently substituted for
//     the cycle time above.
//
//   - Short Pick Rate / Re-pick Rate: straightforward fractions over
//     picking_task_items in scope (short_qty IS NOT NULL /
//     repick_count > 0 respectively) — both reliably computable.
//     TotalRepicks (SUM(repick_count), confirmed to increment by
//     exactly 1 per real Repick call, never more) is also returned.
//
//   - Picker/Warehouse Productivity: the same Lines/Hour, Units/Hour
//     formula above, grouped by assigned_picker_id / warehouse_id.
//
//   - Picking Mode Comparison: grouped by whether a picking_batches
//     row links to the task (BATCH) or not (PICK_BY_ORDER) — the
//     task-grain-consistent axis (orders.picking_mode lives on the
//     order, and one BATCH task can serve many orders with different
//     values, so it cannot be used as a single per-task label).
//
//   - Shift Performance: NOT IMPLEMENTED. No shift/schedule field
//     exists anywhere on users or picking_tasks (confirmed by direct
//     inspection). An hour-of-day bucket could be derived from
//     timestamps, but that would not be a "shift" and labeling it as
//     one would misrepresent what the data actually shows — per the
//     spec's own "return N/A when measurement is not trustworthy"
//     rule, this KPI section is omitted entirely rather than
//     mislabeled.
//
//   - Target vs Actual: NOT IMPLEMENTED. No quota/target field exists
//     anywhere in this schema ("where targets exist" per the spec's
//     own conditional wording — they don't).
package pickingkpi

import "time"

// Filters is every server-side filter the dashboard and its export
// share.
type Filters struct {
	From        *time.Time // inclusive, on picking_tasks.created_at
	To          *time.Time // exclusive
	WarehouseID *int64
	PickerID    *int64
}

// RateWithDenominator pairs a percentage (nil when the denominator is
// zero) with the counts that produced it, so the frontend can always
// show exactly what was and wasn't measured — never a bare percentage
// with hidden exclusions.
type RateWithDenominator struct {
	RatePct     *float64 `json:"rate_pct"`
	Numerator   int64    `json:"numerator"`
	Denominator int64    `json:"denominator"`
}

// ProductivityRow is one grouping bucket (by picker or warehouse) for
// the Lines/Hour and Units/Hour throughput formula.
type ProductivityRow struct {
	Key                string  `json:"key"` // picker or warehouse display name
	TaskCount          int64   `json:"task_count"`
	CompletedTaskCount int64   `json:"completed_task_count"`
	PickedLines        int64   `json:"picked_lines"`
	PickedUnits        float64 `json:"picked_units"`
	// LinesPerHour/UnitsPerHour are nil when this group has zero
	// completed-task duration in scope (never a divide-by-zero 0).
	LinesPerHour *float64 `json:"lines_per_hour"`
	UnitsPerHour *float64 `json:"units_per_hour"`
}

// PickingModeRow is one bucket (BATCH or PICK_BY_ORDER) of the Picking
// Mode Comparison — see package doc for why this is the task-grain
// axis, not orders.picking_mode.
type PickingModeRow struct {
	PickingMode       string   `json:"picking_mode"`
	TaskCount         int64    `json:"task_count"`
	CompletionRatePct *float64 `json:"completion_rate_pct"`
	AvgCycleTimeHours *float64 `json:"avg_cycle_time_hours"`
	ShortPickRatePct  *float64 `json:"short_pick_rate_pct"`
}

// TrendPoint is one day's picking activity, for the Picking Trend and
// Short Pick Trend charts.
type TrendPoint struct {
	Date           string  `json:"date"`
	CompletedTasks int64   `json:"completed_tasks"`
	PickedLines    int64   `json:"picked_lines"`
	PickedUnits    float64 `json:"picked_units"`
	ShortLines     int64   `json:"short_lines"`
}

// DurationBucket is one bucket of the Cycle Time Distribution chart.
type DurationBucket struct {
	Label string `json:"label"` // e.g. "0-1h", "1-4h", "4-8h", "8-24h", "24h+"
	Count int64  `json:"count"`
}

// Charts bundles every Picking KPI Dashboard chart's data.
// DailyTrend doubles as the Short Pick Trend data source — each
// TrendPoint already carries ShortLines alongside PickedLines, so the
// frontend derives the daily short-pick rate from the same series
// rather than fetching it twice.
type Charts struct {
	DailyTrend            []TrendPoint      `json:"daily_trend"`
	ByOperator            []ProductivityRow `json:"by_operator"`
	CycleTimeDistribution []DurationBucket  `json:"cycle_time_distribution"`
}

// Summary is the Picking KPI Dashboard's full payload. Every nullable
// field is nil, never a fabricated 0, when its denominator is zero or
// its backing data doesn't exist — see package doc for the exact rule
// behind each one.
type Summary struct {
	TotalTasks        int64    `json:"total_tasks"`
	CompletedTasks    int64    `json:"completed_tasks"`
	CompletionRatePct *float64 `json:"completion_rate_pct"`

	PickedLines  int64    `json:"picked_lines"`
	PickedUnits  float64  `json:"picked_units"`
	LinesPerHour *float64 `json:"lines_per_hour"`
	UnitsPerHour *float64 `json:"units_per_hour"`

	Accuracy RateWithDenominator `json:"accuracy"` // over scanned lines only — see package doc

	OnTime              RateWithDenominator `json:"on_time"` // over tasks with a due_date only
	TasksWithoutDueDate int64               `json:"tasks_without_due_date"`

	AvgCycleTimeHours             *float64 `json:"avg_cycle_time_hours"`
	AvgActivePickingDurationHours *float64 `json:"avg_active_picking_duration_hours"`
	TasksMissingAssignedAt        int64    `json:"tasks_missing_assigned_at"`

	ShortPickRatePct *float64 `json:"short_pick_rate_pct"`
	RepickRatePct    *float64 `json:"repick_rate_pct"`
	TotalRepicks     int64    `json:"total_repicks"`

	ByOperator    []ProductivityRow `json:"by_operator"`
	ByWarehouse   []ProductivityRow `json:"by_warehouse"`
	ByPickingMode []PickingModeRow  `json:"by_picking_mode"`

	Charts Charts `json:"charts"`
}

// SourceRow is one picking_task_item row feeding the KPIs above —
// exported so a caller can audit exactly which lines produced the
// aggregate numbers ("export KPI source details" per the spec).
type SourceRow struct {
	TaskID         int64      `json:"task_id"`
	PickNo         string     `json:"pick_no"`
	WarehouseName  string     `json:"warehouse_name"`
	PickingMode    string     `json:"picking_mode"`
	AssignedPicker *string    `json:"assigned_picker"`
	CreatedAt      time.Time  `json:"created_at"`
	AssignedAt     *time.Time `json:"assigned_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	DueDate        *time.Time `json:"due_date"`
	SKU            string     `json:"sku"`
	OrderedQty     float64    `json:"ordered_qty"`
	PickedQty      float64    `json:"picked_qty"`
	ShortQty       *float64   `json:"short_qty"`
	RepickCount    int        `json:"repick_count"`
	ItemStatus     string     `json:"item_status"`
	TaskStatus     string     `json:"task_status"`
}

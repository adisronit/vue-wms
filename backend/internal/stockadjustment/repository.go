package stockadjustment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("stockadjustment: not found")
var ErrItemNotFound = errors.New("stockadjustment: item not found")
var ErrDuplicateIdempotencyKey = errors.New("stockadjustment: idempotency key already used")

// Repository is a thin GORM wrapper — business rules (status machine,
// validation, the approve workflow) live in Service.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it both the
// raw *gorm.DB (so callers can construct an inventory.Repository bound
// to the exact same transaction — see service.go's Confirm) and a
// Repository bound to it, the same pattern every prior phase established.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB, txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx, &Repository{db: tx})
	})
}

// CreateAdjustment inserts the header, then derives adjustment_no from
// the new IDENTITY id in the same transaction — the same race-free-by-
// construction approach as every prior document number in this codebase.
func (r *Repository) CreateAdjustment(ctx context.Context, a *Adjustment) error {
	if err := r.db.WithContext(ctx).Create(a).Error; err != nil {
		return err
	}
	a.AdjustmentNo = fmt.Sprintf("ADJ-%06d", a.ID)
	return r.db.WithContext(ctx).Model(&Adjustment{}).Where("id = ?", a.ID).
		Update("adjustment_no", a.AdjustmentNo).Error
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Adjustment, error) {
	var a Adjustment
	err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) CreateItem(ctx context.Context, item *Item) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// FindItemByAdjustmentID returns the one line belonging to an
// adjustment — every document currently has exactly one (see model.go's
// doc); Service uses this rather than a list query since there is
// never more than a single row to fetch.
func (r *Repository) FindItemByAdjustmentID(ctx context.Context, adjustmentID int64) (*Item, error) {
	var item Item
	err := r.db.WithContext(ctx).First(&item, "stock_adjustment_id = ?", adjustmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// UpdateItemFields writes the editable line fields — only ever called
// while the adjustment is still DRAFT (see Status.CanEdit).
func (r *Repository) UpdateItemFields(ctx context.Context, item *Item) error {
	return r.db.WithContext(ctx).Model(&Item{}).Where("id = ?", item.ID).Updates(map[string]any{
		"lot_no":      item.LotNo,
		"adjust_type": item.AdjustType,
		"adjust_qty":  item.AdjustQty,
		"reason":      item.Reason,
		"remark":      item.Remark,
	}).Error
}

// UpdateItemAfterApprove writes before_qty/after_qty — deliberately
// separate from UpdateItemFields so Approve's own write can never
// clobber a concurrent (theoretical, DRAFT-only) field edit, the same
// field-scoped-write discipline Phase 9's race fix established.
func (r *Repository) UpdateItemAfterApprove(ctx context.Context, item *Item) error {
	return r.db.WithContext(ctx).Model(&Item{}).Where("id = ?", item.ID).Updates(map[string]any{
		"before_qty": item.BeforeQty,
		"after_qty":  item.AfterQty,
	}).Error
}

func (r *Repository) UpdateStatus(ctx context.Context, a *Adjustment) error {
	return r.db.WithContext(ctx).Model(&Adjustment{}).Where("id = ?", a.ID).Updates(map[string]any{
		"status":      a.Status,
		"approved_by": a.ApprovedBy,
		"approved_at": a.ApprovedAt,
	}).Error
}

type listRow struct {
	ID            int64
	AdjustmentNo  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	SKU           string
	ProductName   string
	LocationCode  string
	BeforeQty     *float64
	AdjustQty     float64
	AdjustType    string
	AfterQty      *float64
	Reason        string
	Remark        *string
	Status        string
	CreatedBy     *int64
	CreatedByName *string
}

// List returns adjustment-level rows joined with their one line and
// product/location display fields — matches StockAdjustmentView.vue's
// table directly (one row per document). warningHours is Phase 20's
// configured follow-up threshold (config.StockAdjustmentPendingWarningHours,
// passed down from Service): used both to compute each row's own
// IsOverdue and, when params.Overdue is set, as the actual SQL filter
// cutoff — so "overdue" always means the same thing in the filter as it
// does in the returned flag, never two different definitions.
func (r *Repository) List(ctx context.Context, params ListParams, warningHours int) ([]ListResponse, int64, error) {
	base := `
		FROM stock_adjustments sa
		JOIN stock_adjustment_items sai ON sai.stock_adjustment_id = sa.id
		JOIN products p ON p.id = sai.product_id
		JOIN locations l ON l.id = sai.location_id
		LEFT JOIN users u ON u.id = sa.created_by
	`
	// Every filter is independently optional (its own "empty/zero means
	// unfiltered" sentinel) and AND-ed together, the same design the
	// pre-existing search/type/status/date filters already use —
	// older_than_days and overdue both compare sa.updated_at against a
	// cutoff timestamp computed in Go (not SQL DATEDIFF/GETUTCDATE), the
	// same parameterized-timestamp style the existing date filter uses.
	// overdue additionally forces status = PENDING regardless of
	// params.Status: an "overdue" adjustment is by definition still
	// pending, so combining overdue=true with a contradictory status
	// filter yields zero rows rather than a confusing partial match.
	where := `WHERE (? = '' OR sa.adjustment_no LIKE ? OR p.sku LIKE ? OR sai.reason LIKE ?)
		AND (? = '' OR sai.adjust_type = ?)
		AND (? = '' OR sa.status = ?)
		AND (? IS NULL OR CAST(sa.created_at AS DATE) = ?)
		AND (? = '' OR sai.reason = ?)
		AND (? = 0 OR sa.created_by = ?)
		AND (? IS NULL OR sa.updated_at <= ?)
		AND (? = 0 OR (sa.status = 'PENDING' AND sa.updated_at <= ?))`

	now := time.Now().UTC()
	var olderThanCutoffPtr *time.Time
	if params.OlderThanDays > 0 {
		c := olderThanCutoffTime(now, params.OlderThanDays)
		olderThanCutoffPtr = &c
	}
	overdueFlag := 0
	var overdueCutoffPtr *time.Time
	if params.Overdue {
		overdueFlag = 1
		c := overdueCutoff(now, warningHours)
		overdueCutoffPtr = &c
	}

	like := "%" + params.Search + "%"
	args := []any{
		params.Search, like, like, like,
		string(params.Type), string(params.Type),
		string(params.Status), string(params.Status),
		params.Date, params.Date,
		string(params.Reason), string(params.Reason),
		params.CreatedBy, params.CreatedBy,
		olderThanCutoffPtr, olderThanCutoffPtr,
		overdueFlag, overdueCutoffPtr,
	}

	countSQL := "SELECT COUNT(*) " + base + where
	var total int64
	if err := r.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.Limit
	listSQL := `
		SELECT sa.id AS id, sa.adjustment_no AS adjustment_no, sa.created_at AS created_at,
			sa.updated_at AS updated_at,
			p.sku AS sku, p.name AS product_name, l.code AS location_code,
			sai.before_qty AS before_qty, sai.adjust_qty AS adjust_qty, sai.adjust_type AS adjust_type,
			sai.after_qty AS after_qty, sai.reason AS reason, sai.remark AS remark, sa.status AS status,
			sa.created_by AS created_by, u.full_name AS created_by_name
	` + base + where + `
		ORDER BY sa.id DESC
		OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`
	listArgs := append(append([]any{}, args...), offset, params.Limit)

	var rows []listRow
	if err := r.db.WithContext(ctx).Raw(listSQL, listArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		aging := pendingAging(Status(row.Status), row.UpdatedAt, now, warningHours)
		responses = append(responses, ListResponse{
			ID: row.ID, AdjustmentNo: row.AdjustmentNo, Date: row.CreatedAt,
			SKU: row.SKU, ProductName: row.ProductName, LocationCode: row.LocationCode,
			BeforeQty: row.BeforeQty, AdjustQty: row.AdjustQty, AdjustType: AdjustType(row.AdjustType),
			AfterQty: row.AfterQty, Reason: Reason(row.Reason), Remark: row.Remark, Status: Status(row.Status),
			CreatedBy: row.CreatedBy, CreatedByName: row.CreatedByName,
			PendingSince: aging.PendingSince, AgeHours: aging.AgeHours, AgeDays: aging.AgeDays,
			IsOverdue: aging.IsOverdue, PendingWarningHours: aging.PendingWarningHours,
		})
	}
	return responses, total, nil
}

type detailRow struct {
	ID                 int64
	AdjustmentNo       string
	ProductID          int64
	SKU                string
	ProductName        string
	LocationID         int64
	LocationCode       string
	LotNo              string
	AdjustType         string
	AdjustQty          float64
	SourceStockCountID *int64
	SourceStockCountNo *string
	BeforeQty          *float64
	AfterQty           *float64
	Reason             string
	Remark             *string
	Status             string
	CreatedBy          *int64
	CreatedByName      *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ApprovedAt         *time.Time
}

// FindDetail returns the joined header+item+product+location row for
// GET /stock-adjustments/{id}'s response. The LEFT JOIN to stock_counts
// (Phase 17) resolves source_stock_count_id to its count_no for
// display — LEFT, not JOIN, since most adjustments are manually created
// and have no source count at all. The LEFT JOIN to users (Phase 20)
// resolves created_by to a display name, the same pattern Phase 14/18
// already established for this codebase's other created_by_name/
// cancelled_by_name columns.
func (r *Repository) FindDetail(ctx context.Context, id int64) (*detailRow, error) {
	var row detailRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT sa.id AS id, sa.adjustment_no AS adjustment_no,
			sai.product_id AS product_id, p.sku AS sku, p.name AS product_name,
			sai.location_id AS location_id, l.code AS location_code, sai.lot_no AS lot_no,
			sai.adjust_type AS adjust_type, sai.adjust_qty AS adjust_qty,
			sa.source_stock_count_id AS source_stock_count_id, sc.count_no AS source_stock_count_no,
			sai.before_qty AS before_qty, sai.after_qty AS after_qty,
			sai.reason AS reason, sai.remark AS remark, sa.status AS status,
			sa.created_by AS created_by, u.full_name AS created_by_name,
			sa.created_at AS created_at, sa.updated_at AS updated_at, sa.approved_at AS approved_at
		FROM stock_adjustments sa
		JOIN stock_adjustment_items sai ON sai.stock_adjustment_id = sa.id
		JOIN products p ON p.id = sai.product_id
		JOIN locations l ON l.id = sai.location_id
		LEFT JOIN stock_counts sc ON sc.id = sa.source_stock_count_id
		LEFT JOIN users u ON u.id = sa.created_by
		WHERE sa.id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// FindBySourceCountID returns every adjustment generated from a
// specific stock count, oldest first — Phase 17's addition backing
// internal/stockcount's GeneratedAdjustments. reason lives on the item
// table, not the header, but every document CreateAdjustments produces
// has exactly one line (the same "one line per generated document"
// shape it always used), so this join is unambiguous.
func (r *Repository) FindBySourceCountID(ctx context.Context, countID int64) ([]SourceCountSummary, error) {
	var rows []SourceCountSummary
	err := r.db.WithContext(ctx).Raw(`
		SELECT sa.id AS id, sa.adjustment_no AS adjustment_no, sa.status AS status,
			sai.reason AS reason, sa.created_at AS created_at
		FROM stock_adjustments sa
		JOIN stock_adjustment_items sai ON sai.stock_adjustment_id = sa.id
		WHERE sa.source_stock_count_id = ?
		ORDER BY sa.id ASC
	`, countID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// InsertConfirmation is the idempotency guard itself — see
// migrations/0025's doc. Must be called inside the same transaction as
// the approval mutation it's guarding.
func (r *Repository) InsertConfirmation(ctx context.Context, idempotencyKey string, adjustmentID int64) error {
	err := r.db.WithContext(ctx).Create(&Confirmation{IdempotencyKey: idempotencyKey, AdjustmentID: adjustmentID}).Error
	if err != nil && isUniqueViolation(err) {
		return ErrDuplicateIdempotencyKey
	}
	return err
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) — built correct from
// the start, the same as internal/picking and internal/shipping.
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

// FindOverduePending returns every currently-PENDING adjustment whose
// updated_at is at or before cutoff — Phase 21's reminder scan, reusing
// the exact overdue determination Phase 20's own List(overdue=true)
// filter already established (see the ClaimAndListOverdueForReminder
// caller). Scoped to only the columns ReminderCandidate actually needs
// (see its own doc on why a reminder payload stays minimal), not the
// full product/location joins List/FindDetail carry.
func (r *Repository) FindOverduePending(ctx context.Context, cutoff time.Time) ([]ReminderCandidate, error) {
	var rows []struct {
		ID           int64
		AdjustmentNo string
		CreatedBy    *int64
		UpdatedAt    time.Time
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT id AS id, adjustment_no AS adjustment_no, created_by AS created_by, updated_at AS updated_at
		FROM stock_adjustments
		WHERE status = 'PENDING' AND updated_at <= ?
		ORDER BY id ASC
	`, cutoff).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	candidates := make([]ReminderCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, ReminderCandidate{
			ID: row.ID, AdjustmentNo: row.AdjustmentNo, CreatedBy: row.CreatedBy,
			PendingSince: row.UpdatedAt, AgeHours: int(now.Sub(row.UpdatedAt).Hours()),
		})
	}
	return candidates, nil
}

// ClaimReminder attempts to claim the right to send a reminder for
// adjustmentID: succeeds (returns true) either the first time this
// adjustment is ever reminded, or once cooldown has fully elapsed since
// the last reminder; returns false (no error, not a failure) while
// still inside the cooldown window, meaning the caller must not
// publish — this is the duplicate-reminder guard Phase 21 requires.
// Runs inside its own short transaction so the insert-or-conditional-
// update read+write is atomic against a concurrent scan (defensive —
// this codebase only ever runs one scanner, but the guarantee costs
// nothing to hold, matching requirement 11's own instruction).
func (r *Repository) ClaimReminder(ctx context.Context, adjustmentID int64, cooldown time.Duration) (bool, error) {
	claimed := false
	err := r.Transaction(ctx, func(tx *gorm.DB, _ *Repository) error {
		now := time.Now().UTC()

		// First-ever reminder for this adjustment: insert only if no row
		// exists yet — the PRIMARY KEY on stock_adjustment_id makes a
		// concurrent double-insert impossible even without this WHERE
		// NOT EXISTS guard, but the guard avoids a needless constraint-
		// violation round trip in the common case.
		insertResult := tx.Exec(`
			INSERT INTO stock_adjustment_reminders (stock_adjustment_id, last_reminded_at, reminder_count)
			SELECT ?, ?, 1
			WHERE NOT EXISTS (SELECT 1 FROM stock_adjustment_reminders WHERE stock_adjustment_id = ?)
		`, adjustmentID, now, adjustmentID)
		if insertResult.Error != nil {
			return insertResult.Error
		}
		if insertResult.RowsAffected > 0 {
			claimed = true
			return nil
		}

		// A row already exists — only re-claim once cooldown has fully
		// elapsed since the last reminder.
		updateResult := tx.Exec(`
			UPDATE stock_adjustment_reminders
			SET last_reminded_at = ?, reminder_count = reminder_count + 1
			WHERE stock_adjustment_id = ? AND last_reminded_at <= ?
		`, now, adjustmentID, now.Add(-cooldown))
		if updateResult.Error != nil {
			return updateResult.Error
		}
		claimed = updateResult.RowsAffected > 0
		return nil
	})
	return claimed, err
}

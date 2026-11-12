// Package stockcount implements physical stock counting, independent
// from internal/stockadjustment: Create Count -> freeze each line's
// expected quantity from live inventory -> operator records counted
// quantity -> Confirm -> apply each line's variance through Inventory
// Core (AddStockTx on a positive variance, RemoveStockTx on a negative
// one) -> Inventory Ledger. It never overwrites inventory.quantity with
// the counted value directly — only the DELTA (counted - expected) is
// ever applied, through Inventory Core's own atomic guarded UPDATE.
//
// Matches CheckStockView.vue — unlike every other module, this screen
// has no list-of-past-counts table at all; it is a single, perpetual
// working page ("+ New Count Task" was a bare alert() with no state
// change in the mock). See PROJECT_STATUS.md for the snapshot,
// stale-conflict, and Create-Adjustment-bridge decisions.
package stockcount

import "time"

// Status is the count header's state machine. DRAFT while items are
// being added/counted; from there, exactly one of two mutually
// exclusive terminal states is reachable — CONFIRMED (Confirm Count
// applied every line's variance directly) or ADJUSTMENT_CREATED (Create
// Adjustment routed the variance through separate
// internal/stockadjustment approval instead). Before Phase 16, neither
// path checked whether the other had already run, so the same count's
// variance could be applied via both — see PROJECT_STATUS.md's Phase 16
// section for the full overlap this closes. Both CONFIRMED and
// ADJUSTMENT_CREATED are claimed via the same guarded UPDATE
// (Repository.ClaimStatus), never a plain Go-side status check followed
// by a blind write, so the exclusion holds even under two genuinely
// concurrent requests (e.g. Confirm and Create Adjustment fired at the
// same instant with different idempotency keys), not just sequential
// calls.
type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusConfirmed Status = "CONFIRMED"
	// StatusAdjustmentCreated is Phase 16's addition — see the Status
	// doc above.
	StatusAdjustmentCreated Status = "ADJUSTMENT_CREATED"
)

func (s Status) CanEdit() bool {
	return s == StatusDraft
}

func (s Status) CanConfirm() bool {
	return s == StatusDraft
}

// CanCreateAdjustments reports whether Create Adjustment may run
// against a count in this status — Phase 16's addition. Identical
// predicate to CanConfirm/CanEdit today (DRAFT only), kept as its own
// method for the same reason Phase 13 gave for internal/picking's
// CanCancel: a future rule could diverge it without disturbing
// Confirm's own gate. The real enforcement is Repository.ClaimStatus's
// guarded UPDATE, not this method — this exists for a clear, documented
// name at the call site.
func (s Status) CanCreateAdjustments() bool {
	return s == StatusDraft
}

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusConfirmed, StatusAdjustmentCreated:
		return true
	default:
		return false
	}
}

// CountType mirrors CheckStockView.vue's Count Type <select> exactly.
// Purely descriptive/filtering this phase — no automatic item-
// population logic is tied to any of these values (the operator always
// adds/scans lines by hand, matching the mock's own manual "+ Add Item"
// and scan-bar behavior; see the task's own "do not build a full
// cycle-count engine unless required").
type CountType string

const (
	CountTypeCycle    CountType = "CYCLE_COUNT"
	CountTypeFull     CountType = "FULL_COUNT"
	CountTypeLocation CountType = "LOCATION_COUNT"
	CountTypeSKU      CountType = "SKU_COUNT"
)

func (t CountType) IsValid() bool {
	switch t {
	case CountTypeCycle, CountTypeFull, CountTypeLocation, CountTypeSKU:
		return true
	default:
		return false
	}
}

// Condition mirrors CheckStockView.vue's per-line Condition <select>.
type Condition string

const (
	ConditionGood    Condition = "GOOD"
	ConditionDamaged Condition = "DAMAGED"
	ConditionHold    Condition = "HOLD"
)

func (c Condition) IsValid() bool {
	switch c {
	case ConditionGood, ConditionDamaged, ConditionHold:
		return true
	default:
		return false
	}
}

// ItemStatus mirrors CheckStockView.vue's per-line Status <select>.
type ItemStatus string

const (
	ItemStatusPending ItemStatus = "PENDING"
	ItemStatusCounted ItemStatus = "COUNTED"
	ItemStatusRecount ItemStatus = "RECOUNT"
)

func (s ItemStatus) IsValid() bool {
	switch s {
	case ItemStatusPending, ItemStatusCounted, ItemStatusRecount:
		return true
	default:
		return false
	}
}

// Count is the GORM model for the stock_counts table (migrations/0026).
type Count struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0045, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID    int64      `gorm:"column:tenant_id;not null;default:1"`
	CountNo     string     `gorm:"column:count_no"`
	CountType   CountType  `gorm:"column:count_type"`
	WarehouseID int64      `gorm:"column:warehouse_id"`
	ZoneID      *int64     `gorm:"column:zone_id"`
	CountDate   *time.Time `gorm:"column:count_date"`
	Status      Status     `gorm:"column:status"`
	CreatedBy   *int64     `gorm:"column:created_by"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	ConfirmedAt *time.Time `gorm:"column:confirmed_at"`
}

func (Count) TableName() string { return "stock_counts" }

// Item is the GORM model for the stock_count_items table
// (migrations/0027). SystemQty is captured once, when the line is
// added, and never refreshed afterward (the task's own explicit
// "do not recompute the historical expected quantity" requirement) —
// see repository.go's CreateItem. VarianceQty is computed and stored
// the moment CountQty is recorded (repository.go's RecordCount), not
// left as a query-time derivation, since it is a historical fact about
// one specific count event that must never silently change.
type Item struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0045, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID    int64      `gorm:"column:tenant_id;not null;default:1"`
	CountID     int64      `gorm:"column:stock_count_id"`
	ProductID   int64      `gorm:"column:product_id"`
	LocationID  int64      `gorm:"column:location_id"`
	LotNo       string     `gorm:"column:lot_no"`
	UOM         string     `gorm:"column:uom"`
	SystemQty   float64    `gorm:"column:system_qty"`
	CountQty    *float64   `gorm:"column:count_qty"`
	VarianceQty *float64   `gorm:"column:variance_qty"`
	Condition   Condition  `gorm:"column:condition"`
	Status      ItemStatus `gorm:"column:status"`
	Remark      *string    `gorm:"column:remark"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (Item) TableName() string { return "stock_count_items" }

// Confirmation is the GORM model for the stock_count_confirmations
// table (migrations/0028) — the idempotency guard for Confirm, the same
// pattern as every prior phase's own Confirmation type.
type Confirmation struct {
	IdempotencyKey string `gorm:"column:idempotency_key;primaryKey"`
	// TenantID (migrations/0045, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID  int64     `gorm:"column:tenant_id;not null;default:1"`
	CountID   int64     `gorm:"column:stock_count_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "stock_count_confirmations" }

// Package qualitycheck implements Phase 31B's real QC inspection
// workflow, replacing QualityCheckView.vue/NewQualityCheckView.vue's
// previous local-only mock data (see those files' own doc comments from
// Phase 31A, which explicitly deferred this rather than fabricate an
// API). An inspection targets one Receiving document (internal/receiving)
// and inspects the receipt_item_splits (internal/receiving's own
// migrations/0069) lines whose product requires QC and still carry
// inventory.HoldQuantity > 0 — see Service's own doc for why this
// package reads those tables directly via raw SQL rather than taking a
// Go dependency on internal/receiving.
//
// PASS/HOLD/REJECT dispositions call Phase 5/31A's internal/inventory
// (ReleaseHoldTx/RejectHoldTx) for every stock mutation — no balance or
// ledger logic is duplicated here, mirroring internal/receiving's own
// "calls Inventory Core, never mutates stock itself" discipline exactly.
package qualitycheck

import "time"

// Status is the inspection header's state machine — derived from its
// items' own statuses (see computeInspectionStatus), not independently
// settable.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
)

// QCType mirrors NewQualityCheckView.vue's pre-existing QC Type <select>.
type QCType string

const (
	TypeNormal QCType = "NORMAL"
	TypeFull   QCType = "FULL"
	TypeRandom QCType = "RANDOM"
	TypeDamage QCType = "DAMAGE"
)

func (t QCType) IsValid() bool {
	switch t {
	case TypeNormal, TypeFull, TypeRandom, TypeDamage:
		return true
	default:
		return false
	}
}

// ItemStatus is one inspection line's own state, independent of the
// header — derived from the live inventory.HoldQuantity remaining for
// its (product, location, lot) tuple, not stored authoritatively (see
// itemStatus's own doc).
type ItemStatus string

const (
	ItemStatusPending    ItemStatus = "PENDING"
	ItemStatusInProgress ItemStatus = "IN_PROGRESS"
	ItemStatusCompleted  ItemStatus = "COMPLETED"
)

// Disposition is one QC decision's outcome.
type Disposition string

const (
	DispositionPass   Disposition = "PASS"
	DispositionHold   Disposition = "HOLD"
	DispositionReject Disposition = "REJECT"
)

func (d Disposition) IsValid() bool {
	switch d {
	case DispositionPass, DispositionHold, DispositionReject:
		return true
	default:
		return false
	}
}

// Inspection is the GORM model for the qc_inspections table
// (migrations/0070).
type Inspection struct {
	ID           int64     `gorm:"column:id;primaryKey"`
	TenantID     int64     `gorm:"column:tenant_id;not null"`
	InspectionNo string    `gorm:"column:inspection_no"`
	ReceiptID    int64     `gorm:"column:receipt_id"`
	WarehouseID  int64     `gorm:"column:warehouse_id"`
	InspectorID  *int64    `gorm:"column:inspector_id"`
	QCType       QCType    `gorm:"column:qc_type"`
	Status       Status    `gorm:"column:status"`
	Remark       *string   `gorm:"column:remark"`
	CreatedBy    *int64    `gorm:"column:created_by"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (Inspection) TableName() string { return "qc_inspections" }

// InspectionItem is the GORM model for the qc_inspection_items table
// (migrations/0071) — one row per receipt_item_splits line being
// inspected. HeldQtySnapshot is the hold quantity that existed at
// creation time, kept only for display ("how much was there to
// inspect") — the authoritative "remaining to inspect" is always the
// live inventory.HoldQuantity for this item's own tuple, never this
// snapshot (see Service.itemStatus's own doc).
type InspectionItem struct {
	ID                 int64      `gorm:"column:id;primaryKey"`
	TenantID           int64      `gorm:"column:tenant_id;not null"`
	InspectionID       int64      `gorm:"column:inspection_id"`
	ReceiptItemSplitID int64      `gorm:"column:receipt_item_split_id"`
	ProductID          int64      `gorm:"column:product_id"`
	LotNo              string     `gorm:"column:lot_no"`
	ExpiryDate         *time.Time `gorm:"column:expiry_date"`
	LocationID         int64      `gorm:"column:location_id"`
	HeldQtySnapshot    float64    `gorm:"column:held_qty_snapshot"`
	SampleQty          float64    `gorm:"column:sample_qty"`
	TemperatureC       *float64   `gorm:"column:temperature_c"`
	PackagingOK        *bool      `gorm:"column:packaging_ok"`
	LabelOK            *bool      `gorm:"column:label_ok"`
	Status             ItemStatus `gorm:"column:status"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (InspectionItem) TableName() string { return "qc_inspection_items" }

// InspectionDisposition is the GORM model for the qc_dispositions table
// (migrations/0072) — append-only, mirroring inventory_transactions'
// own never-updated-or-deleted shape. Quantity is this decision's own
// magnitude, not a running total.
type InspectionDisposition struct {
	ID                int64       `gorm:"column:id;primaryKey"`
	TenantID          int64       `gorm:"column:tenant_id;not null"`
	InspectionItemID  int64       `gorm:"column:inspection_item_id"`
	Disposition       Disposition `gorm:"column:disposition"`
	Quantity          float64     `gorm:"column:quantity"`
	DefectCode        *string     `gorm:"column:defect_code"`
	EvidenceReference *string     `gorm:"column:evidence_reference"`
	Remark            *string     `gorm:"column:remark"`
	DecidedBy         *int64      `gorm:"column:decided_by"`
	CreatedAt         time.Time   `gorm:"column:created_at"`
}

func (InspectionDisposition) TableName() string { return "qc_dispositions" }

// Confirmation is the GORM model for the qc_confirmations table
// (migrations/0073) — the idempotency guard for Confirm, identical
// shape to internal/receiving.Confirmation.
type Confirmation struct {
	IdempotencyKey string    `gorm:"column:idempotency_key;primaryKey"`
	TenantID       int64     `gorm:"column:tenant_id;not null"`
	InspectionID   int64     `gorm:"column:inspection_id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "qc_confirmations" }

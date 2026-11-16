// Package receiving implements the first real transactional workflow:
// Receiving Document -> Receiving Lines -> Confirm Receive -> Inventory
// AddStock -> Inventory Transaction Ledger. It calls Phase 5's
// internal/inventory (AddStockTx) for every stock mutation — no balance
// or ledger logic is duplicated here.
//
// Matches ReceiveView.vue / NewReceivingView.vue. See PROJECT_STATUS.md
// for the staging-vs-direct-to-stock, vendor, receipt numbering, status
// consolidation, over-receipt, and idempotency decisions.
package receiving

import "time"

// Status is the receipt header's state machine.
type Status string

const (
	StatusDraft             Status = "DRAFT"
	StatusWaiting           Status = "WAITING"
	StatusPartiallyReceived Status = "PARTIALLY_RECEIVED"
	StatusCompleted         Status = "COMPLETED"
	StatusCancelled         Status = "CANCELLED"
)

// CanConfirm reports whether Confirm may run against a receipt in this
// status. COMPLETED and CANCELLED are terminal.
func (s Status) CanConfirm() bool {
	switch s {
	case StatusDraft, StatusWaiting, StatusPartiallyReceived:
		return true
	default:
		return false
	}
}

// CanEdit reports whether header/line fields may still be changed —
// once any quantity has been received, the document that produced that
// stock movement shouldn't be edited out from under it.
func (s Status) CanEdit() bool {
	switch s {
	case StatusDraft, StatusWaiting:
		return true
	default:
		return false
	}
}

// CanCancel reports whether Cancel may run against a receipt in this
// status — Phase 15's addition. Identical predicate to CanEdit today
// (DRAFT/WAITING, i.e. before any quantity has been received: per
// computeReceiptStatus's own doc, WAITING is only ever reached while
// every line's received_qty is still zero), kept as its own method so
// a future rule could diverge the two without disturbing CanEdit's own
// callers. Once any line has received_qty > 0 the header is already
// PARTIALLY_RECEIVED or COMPLETED — a real AddStockTx call already
// happened for that quantity, and Phase 15 does not invent a
// compensating reversal for it, so cancellation is refused from that
// point on rather than attempting to undo it.
func (s Status) CanCancel() bool {
	switch s {
	case StatusDraft, StatusWaiting:
		return true
	default:
		return false
	}
}

// ReceivingType mirrors NewReceivingView.vue's Receiving Type <select>.
type ReceivingType string

const (
	TypePO       ReceivingType = "PO"
	TypeASN      ReceivingType = "ASN"
	TypeReturn   ReceivingType = "RETURN"
	TypeTransfer ReceivingType = "TRANSFER"
)

func (t ReceivingType) IsValid() bool {
	switch t {
	case TypePO, TypeASN, TypeReturn, TypeTransfer:
		return true
	default:
		return false
	}
}

// ItemStatus is a receipt line's own state, independent of the header.
type ItemStatus string

const (
	ItemStatusPending           ItemStatus = "PENDING"
	ItemStatusPartiallyReceived ItemStatus = "PARTIALLY_RECEIVED"
	ItemStatusCompleted         ItemStatus = "COMPLETED"
)

// Receipt is the GORM model for the receipts table (migrations/0011).
// CancelledAt/CancelledBy/CancelRemark (migrations/0032, Phase 18) are
// nil until a successful Cancel — see Service.Cancel's own doc.
type Receipt struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) is now a verified,
	// per-request value threaded through every Service method — see
	// Phase 31A's own report. The default:1 tag is kept only so a bare
	// Receipt{} literal in a test fixture that forgets to set it still
	// satisfies the NOT NULL column, matching every other tenant-scoped
	// model in this codebase.
	TenantID      int64         `gorm:"column:tenant_id;not null;default:1"`
	ReceiptNo     string        `gorm:"column:receipt_no"`
	ReceivingType ReceivingType `gorm:"column:receiving_type"`
	ReferenceNo   *string       `gorm:"column:reference_no"`
	Vendor        *string       `gorm:"column:vendor"`
	// SupplierID/SupplierNameSnapshot (migrations/0065-0066, Phase 31A)
	// integrate the real internal/supplier master. SupplierID is
	// nullable — Vendor (free text, migrations/0011) is untouched and
	// still the only supplier field some ReceivingTypes (RETURN/
	// TRANSFER) need. SupplierNameSnapshot is a one-time copy at
	// Create/Update time, never re-synced from the live supplier row
	// later — the same snapshot-immutability pattern Customer Ship-to
	// established (see internal/order's own applyCustomerShipTo doc):
	// renaming a supplier later must not silently change what an
	// already-created receipt displays.
	SupplierID           *int64  `gorm:"column:supplier_id"`
	SupplierNameSnapshot *string `gorm:"column:supplier_name_snapshot"`
	// InvoiceNo..Currency (migrations/0065, Phase 31A) are the detailed
	// logistics/document header fields NewReceivingView.vue's redesigned
	// Document step collects. All nullable: a receipt created before
	// this phase, or one whose ReceivingType has no meaningful
	// container/transport details (e.g. an internal TRANSFER), simply
	// leaves them unset.
	InvoiceNo          *string    `gorm:"column:invoice_no"`
	DeliveryNoteNo     *string    `gorm:"column:delivery_note_no"`
	ContainerNo        *string    `gorm:"column:container_no"`
	SealNo             *string    `gorm:"column:seal_no"`
	TransportCompany   *string    `gorm:"column:transport_company"`
	DriverName         *string    `gorm:"column:driver_name"`
	VehicleRegNo       *string    `gorm:"column:vehicle_reg_no"`
	DockDoor           *string    `gorm:"column:dock_door"`
	ArrivalTime        *time.Time `gorm:"column:arrival_time"`
	UnloadingStartTime *time.Time `gorm:"column:unloading_start_time"`
	UnloadingEndTime   *time.Time `gorm:"column:unloading_end_time"`
	Currency           *string    `gorm:"column:currency"`
	WarehouseID        int64      `gorm:"column:warehouse_id"`
	StagingLocationID  int64      `gorm:"column:staging_location_id"`
	Status             Status     `gorm:"column:status"`
	ReceivingDate      *time.Time `gorm:"column:receiving_date"`
	Remark             *string    `gorm:"column:remark"`
	CreatedBy          *int64     `gorm:"column:created_by"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
	CancelledAt        *time.Time `gorm:"column:cancelled_at"`
	CancelledBy        *int64     `gorm:"column:cancelled_by"`
	CancelRemark       *string    `gorm:"column:cancel_remark"`
}

func (Receipt) TableName() string { return "receipts" }

// applyCancel mutates the receipt in place for a successful cancellation:
// sets Status to CANCELLED and stamps CancelledAt/CancelledBy, storing
// remark in the dedicated CancelRemark column only when a non-empty
// remark was supplied. Callers (Service.Cancel) only ever invoke this
// after Status.CanCancel() has already gated the guard, so it never runs
// twice for the same receipt and never overwrites an existing
// cancellation's audit trail.
func (r *Receipt) applyCancel(userID int64, remark *string, now time.Time) {
	r.Status = StatusCancelled
	r.CancelledAt = &now
	r.CancelledBy = &userID
	if remark != nil && *remark != "" {
		r.CancelRemark = remark
	}
}

// ReceiptItem is the GORM model for the receipt_items table
// (migrations/0012). ReceivedQty is cumulative across every confirm
// call for this line, not one batch's delta.
type ReceiptItem struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID    int64      `gorm:"column:tenant_id;not null;default:1"`
	ReceiptID   int64      `gorm:"column:receipt_id"`
	ProductID   int64      `gorm:"column:product_id"`
	ExpectedQty float64    `gorm:"column:expected_qty"`
	ReceivedQty float64    `gorm:"column:received_qty"`
	UOM         string     `gorm:"column:uom"`
	LotNo       string     `gorm:"column:lot_no"`
	ExpiryDate  *time.Time `gorm:"column:expiry_date"`
	LocationID  int64      `gorm:"column:location_id"`
	Status      ItemStatus `gorm:"column:status"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (ReceiptItem) TableName() string { return "receipt_items" }

// Confirmation is the GORM model for the receipt_confirmations table
// (migrations/0013) — the idempotency guard for Confirm.
type Confirmation struct {
	IdempotencyKey string `gorm:"column:idempotency_key;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID  int64     `gorm:"column:tenant_id;not null;default:1"`
	ReceiptID int64     `gorm:"column:receipt_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "receipt_confirmations" }

// ReceiptItemSplit is the GORM model for the receipt_item_splits table
// (migrations/0069, Phase 31A) — one immutable row per distinct lot/
// expiry/pallet batch a Confirm call actually posted for a line. Added
// so a single ReceiptItem (one expected SKU/qty/location line) can be
// received across multiple lots, expiry dates, or pallets/LPNs without
// losing any of them — receipt_items.lot_no/expiry_date themselves stay
// exactly what they always were (the most recent confirm call's values,
// see ReceiptItem's own doc), so every receipt confirmed before this
// phase displays identically. Rows here are never updated or deleted.
type ReceiptItemSplit struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	TenantID        int64      `gorm:"column:tenant_id;not null"`
	ReceiptID       int64      `gorm:"column:receipt_id"`
	ReceiptItemID   int64      `gorm:"column:receipt_item_id"`
	LotNo           string     `gorm:"column:lot_no"`
	MfgDate         *time.Time `gorm:"column:mfg_date"`
	ExpiryDate      *time.Time `gorm:"column:expiry_date"`
	Quantity        float64    `gorm:"column:quantity"`
	UOM             string     `gorm:"column:uom"`
	PalletID        *string    `gorm:"column:pallet_id"`
	CartonCount     *int       `gorm:"column:carton_count"`
	LocationID      int64      `gorm:"column:location_id"`
	TemperatureC    *float64   `gorm:"column:temperature_c"`
	ConditionNote   *string    `gorm:"column:condition_note"`
	CountryOfOrigin *string    `gorm:"column:country_of_origin"`
	CreatedBy       *int64     `gorm:"column:created_by"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
}

func (ReceiptItemSplit) TableName() string { return "receipt_item_splits" }

// itemStatus derives a line's status from its own quantities — mirrors
// the same "fully received once received >= expected, given expected=0
// never blocks completion" rule used for the receipt header.
func itemStatus(expectedQty, receivedQty float64) ItemStatus {
	if receivedQty <= 0 {
		return ItemStatusPending
	}
	if expectedQty > 0 && receivedQty < expectedQty {
		return ItemStatusPartiallyReceived
	}
	return ItemStatusCompleted
}

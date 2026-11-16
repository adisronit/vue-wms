// Package transferreceipt implements Phase 33E.2 — Destination
// Receiving: the action that finally credits the DESTINATION warehouse
// once an INTERNAL_TRANSFER order's dispatched shipment physically
// arrives, closing the status-window loop 33E.1 proved (see
// internal/order's own transfer_pipeline_test.go). A transfer receipt
// is created from an already-dispatched internal/shipping Shipment —
// never from the original order lines directly, since those may have
// been short-picked; the shipment's own items (post-33D.1, the real
// packed_qty-sourced quantities) are the authoritative "what was
// shipped."
//
// Deliberately its own new, lightweight module rather than an
// extension of internal/receiving — that package is deeply
// supplier-centric (vendor, PO reference, QC/hold integration from
// Phase 31) and forcing transfer semantics into it would distort an
// already-complex module for a workflow that needs none of that (no QC
// hold applies to goods a company shipped to itself). Mirrors
// Receiving's OWN simple header+lines+confirm shape instead, the same
// "own focused module" precedent internal/relocation/internal/packing/
// internal/pickingbatch/internal/loading each already established.
//
// Calls inventory.Service.AddStockTx for every stock mutation, exactly
// as internal/receiving already does — no debit/credit logic is
// duplicated here. Never touches the SOURCE warehouse at all: that
// side already dropped at Picking.Confirm(PICKED) time, via the
// ordinary PICK ledger row an equivalent sales order would also
// produce.
package transferreceipt

import "time"

// Status is the transfer receipt header's state machine: DRAFT ->
// COMPLETED / CANCELLED. Deliberately no intermediate
// PARTIALLY_RECEIVED state the way receiving.Status has — a transfer
// receipt stays DRAFT across any number of incremental ReceiveItem
// calls; it only reaches COMPLETED via the explicit, idempotency-
// guarded Confirm action once every line is either fully received or
// has a recorded discrepancy (the same explicit "ready to confirm"
// gate shape internal/loading's own ConfirmTrip established, not a
// status derived automatically from line quantities).
type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusCompleted Status = "COMPLETED"
	StatusCancelled Status = "CANCELLED"
)

// CanReceive reports whether ReceiveItem may run against a receipt in
// this status.
func (s Status) CanReceive() bool {
	return s == StatusDraft
}

// CanConfirm reports whether Confirm may run against a receipt in this
// status.
func (s Status) CanConfirm() bool {
	return s == StatusDraft
}

// CanCancel reports whether Cancel may run against a receipt in this
// status — only DRAFT, the same "before anything terminal happened"
// boundary every other module's own CanCancel uses.
func (s Status) CanCancel() bool {
	return s == StatusDraft
}

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// ItemStatus is a receipt line's own state, independent of the header.
// DISCREPANCY is terminal: a line explicitly closed short/damaged (see
// Service.ReceiveItem's own "close short" branch) never receives
// again, even though received_qty may remain below dispatched_qty
// forever.
type ItemStatus string

const (
	ItemStatusPending           ItemStatus = "PENDING"
	ItemStatusPartiallyReceived ItemStatus = "PARTIALLY_RECEIVED"
	ItemStatusCompleted         ItemStatus = "COMPLETED"
	ItemStatusDiscrepancy       ItemStatus = "DISCREPANCY"
)

// CanReceive reports whether more ReceiveItem calls may target this
// line — false once COMPLETED (nothing left to receive) or DISCREPANCY
// (explicitly closed short, terminal).
func (s ItemStatus) CanReceive() bool {
	switch s {
	case ItemStatusPending, ItemStatusPartiallyReceived:
		return true
	default:
		return false
	}
}

// IsSettled reports whether this line counts toward Confirm's own
// "every line is either fully received or has a recorded discrepancy"
// gate.
func (s ItemStatus) IsSettled() bool {
	return s == ItemStatusCompleted || s == ItemStatusDiscrepancy
}

// Receipt is the GORM model for the transfer_receipts table
// (migrations/0116). ShipmentID is UNIQUE per tenant — at most one
// receipt may ever exist for a given transfer shipment. OrderID and
// DestinationWarehouseID are both resolved server-side at Create time
// from the shipment's own linked INTERNAL_TRANSFER order, never from
// client input — see Service.Create's own doc.
type Receipt struct {
	ID                     int64      `gorm:"column:id;primaryKey"`
	TenantID               int64      `gorm:"column:tenant_id;not null"`
	ReceiptNo              string     `gorm:"column:receipt_no"`
	ShipmentID             int64      `gorm:"column:shipment_id"`
	OrderID                int64      `gorm:"column:order_id"`
	DestinationWarehouseID int64      `gorm:"column:destination_warehouse_id"`
	Status                 Status     `gorm:"column:status"`
	ReceivedBy             *int64     `gorm:"column:received_by"`
	ReceivedAt             *time.Time `gorm:"column:received_at"`
	Remark                 *string    `gorm:"column:remark"`
	CreatedBy              *int64     `gorm:"column:created_by"`
	CreatedAt              time.Time  `gorm:"column:created_at"`
	UpdatedAt              time.Time  `gorm:"column:updated_at"`
	CancelledAt            *time.Time `gorm:"column:cancelled_at"`
	CancelledBy            *int64     `gorm:"column:cancelled_by"`
	CancelRemark           *string    `gorm:"column:cancel_remark"`
}

func (Receipt) TableName() string { return "transfer_receipts" }

// applyCancel mutates the receipt in place for a successful
// cancellation, matching every other module's own applyCancel shape
// exactly.
func (r *Receipt) applyCancel(userID int64, remark *string, now time.Time) {
	r.Status = StatusCancelled
	r.CancelledAt = &now
	r.CancelledBy = &userID
	if remark != nil && *remark != "" {
		r.CancelRemark = remark
	}
}

// Item is the GORM model for the transfer_receipt_items table
// (migrations/0117) — one row per shipped line, created from the
// source shipment_item at receipt-creation time. DispatchedQty is a
// frozen snapshot of that shipment item's own shipped quantity — the
// hard ceiling every ReceiveItem call is bounded against. ReceivedQty
// is cumulative across every ReceiveItem call for this line, not one
// call's delta. LocationID is nullable: unset until the first
// ReceiveItem call actually chooses a destination staging location
// (see Service.ReceiveItem's own doc), then updated to each call's own
// value purely for display, the same "most recent call's values"
// meaning receiving.ReceiptItem.LocationID already carries.
// DiscrepancyQty/DiscrepancyReason stay nil for every ordinary, fully-
// received line.
type Item struct {
	ID                int64      `gorm:"column:id;primaryKey"`
	TenantID          int64      `gorm:"column:tenant_id;not null"`
	TransferReceiptID int64      `gorm:"column:transfer_receipt_id"`
	ShipmentItemID    int64      `gorm:"column:shipment_item_id"`
	ProductID         int64      `gorm:"column:product_id"`
	LotNo             string     `gorm:"column:lot_no"`
	UOM               string     `gorm:"column:uom"`
	DispatchedQty     float64    `gorm:"column:dispatched_qty"`
	ReceivedQty       float64    `gorm:"column:received_qty"`
	LocationID        *int64     `gorm:"column:location_id"`
	DiscrepancyQty    *float64   `gorm:"column:discrepancy_qty"`
	DiscrepancyReason *string    `gorm:"column:discrepancy_reason"`
	Status            ItemStatus `gorm:"column:status"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (Item) TableName() string { return "transfer_receipt_items" }

// Confirmation is the GORM model for the transfer_receipt_confirmations
// table (migrations/0118) — the idempotency guard shared by BOTH
// ReceiveItem and Confirm, the same pattern as every other module's own
// Confirmation type.
type Confirmation struct {
	IdempotencyKey    string    `gorm:"column:idempotency_key;primaryKey"`
	TenantID          int64     `gorm:"column:tenant_id;not null"`
	TransferReceiptID int64     `gorm:"column:transfer_receipt_id"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "transfer_receipt_confirmations" }

// itemStatus derives a line's status from its own quantities when it
// is NOT being closed with a discrepancy this call — mirrors every
// other module's own itemStatus helper. The DISCREPANCY status is set
// directly by Service.ReceiveItem's own "close short" branch, never by
// this helper.
func itemStatus(dispatchedQty, receivedQty float64) ItemStatus {
	if receivedQty <= 0 {
		return ItemStatusPending
	}
	if receivedQty < dispatchedQty {
		return ItemStatusPartiallyReceived
	}
	return ItemStatusCompleted
}

// computeReceiptStatus derives whether every line is "settled" (fully
// received or closed with a discrepancy) — Confirm's own gate. Does
// NOT return a Status: the header never auto-transitions off DRAFT the
// way receiving's header does; Confirm is always the explicit trigger.
func allItemsSettled(items []Item) bool {
	if len(items) == 0 {
		return false
	}
	for _, it := range items {
		if !it.Status.IsSettled() {
			return false
		}
	}
	return true
}

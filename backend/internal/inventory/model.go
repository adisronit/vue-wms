// Package inventory owns current stock balance and its immutable
// transaction ledger — the central model
// Product -> Inventory -> Location -> Zone -> Warehouse later Receiving/
// Putaway/Move/Picking/Shipping/Adjustment/Count phases will all build
// on. See PROJECT_STATUS.md for the lot/serial/expiry design reasoning.
//
// This phase exposes read-only HTTP endpoints (matching
// StockInquiryView.vue) plus a reusable, unexported-from-HTTP stock
// mutation foundation (AddStock/RemoveStock/MoveStock in service.go) —
// no workflow calls that foundation yet from a real user action, since
// Receiving/Putaway/Picking/Shipping/Adjustment/Count are later phases.
package inventory

import "time"

// TransactionType is a closed set matching the CHECK constraint on
// inventory_transactions.transaction_type. Only ADJUST is actually
// exercised by anything in this phase (via live verification/tests);
// the rest exist so later phases can use this ledger without a schema
// or Go-type change.
type TransactionType string

const (
	TransactionReceive TransactionType = "RECEIVE"
	TransactionPutaway TransactionType = "PUTAWAY"
	TransactionMove    TransactionType = "MOVE"
	TransactionPick    TransactionType = "PICK"
	TransactionShip    TransactionType = "SHIP"
	TransactionAdjust  TransactionType = "ADJUST"
	TransactionReturn  TransactionType = "RETURN"
	TransactionCount   TransactionType = "COUNT"
)

func (t TransactionType) IsValid() bool {
	switch t {
	case TransactionReceive, TransactionPutaway, TransactionMove, TransactionPick,
		TransactionShip, TransactionAdjust, TransactionReturn, TransactionCount:
		return true
	default:
		return false
	}
}

// Inventory is the GORM model for the inventory table (migrations/0009)
// — current balance for one (product, location, lot) tuple.
type Inventory struct {
	ID               int64      `gorm:"column:id;primaryKey"`
	ProductID        int64      `gorm:"column:product_id"`
	LocationID       int64      `gorm:"column:location_id"`
	LotNo            string     `gorm:"column:lot_no"`
	ExpiryDate       *time.Time `gorm:"column:expiry_date"`
	Quantity         float64    `gorm:"column:quantity"`
	ReservedQuantity float64    `gorm:"column:reserved_quantity"`
	OnHold           bool       `gorm:"column:on_hold"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (Inventory) TableName() string { return "inventory" }

// Available is the derived available-to-use quantity — never stored,
// always computed, so it can never drift from quantity/reserved.
func (i Inventory) Available() float64 {
	return i.Quantity - i.ReservedQuantity
}

// Transaction is the GORM model for the inventory_transactions table
// (migrations/0010) — append-only; nothing in this codebase updates or
// deletes a row here.
type Transaction struct {
	ID              int64           `gorm:"column:id;primaryKey"`
	TransactionType TransactionType `gorm:"column:transaction_type"`
	ProductID       int64           `gorm:"column:product_id"`
	LocationID      int64           `gorm:"column:location_id"`
	LotNo           string          `gorm:"column:lot_no"`
	Quantity        float64         `gorm:"column:quantity"`
	ReferenceType   *string         `gorm:"column:reference_type"`
	ReferenceID     *string         `gorm:"column:reference_id"`
	Reason          *string         `gorm:"column:reason"`
	CreatedBy       *int64          `gorm:"column:created_by"`
	CreatedAt       time.Time       `gorm:"column:created_at"`
}

func (Transaction) TableName() string { return "inventory_transactions" }

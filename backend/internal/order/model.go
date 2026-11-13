// Package order implements Phase 25's Order Management — a lightweight
// header+lines layer that orchestrates an existing internal/picking
// task rather than ever touching inventory itself. Order Management
// never calls inventory.Service directly and never writes an
// inventory_transactions row: internal/picking retains sole ownership
// of both reservation (ReserveStockTx, at task-creation time) and
// deduction (ConsumeReservationTx/RemoveStockTx, at Confirm) — Order's
// own CreatePickingTask action is nothing more than a thin call into
// picking.Service.Create with this order's own items.
package order

import "time"

// Status is the order's own explicit state machine — deliberately
// narrow (3 states, only ever set by Order's own 3 actions). The
// richer PICKED/SHIPPING/DELIVERED states a real order lifecycle needs
// are NOT stored here — see dto.go's FulfillmentStatus doc for why.
type Status string

const (
	StatusPending   Status = "PENDING"
	StatusPicking   Status = "PICKING"
	StatusCancelled Status = "CANCELLED"
)

// CanCreatePickingTask reports whether CreatePickingTask may run —
// only ever the first and only orchestration call for this order
// (picking_task_id is UNIQUE; a second call would violate it anyway,
// but this status check gives a clean 409 instead of a raw constraint
// error).
func (s Status) CanCreatePickingTask() bool {
	return s == StatusPending
}

// CanCancel reports whether Cancel may run — only before a picking
// task has ever been created for this order. Once orchestration has
// happened, cancelling the underlying picking task (via
// internal/picking's own Cancel, which already releases its own
// reservation) is the correct action, not a second, competing cancel
// path here.
func (s Status) CanCancel() bool {
	return s == StatusPending
}

// CanUpdate reports whether header/items may still be edited — same
// condition as CanCreatePickingTask/CanCancel (only before a picking
// task has ever been created), kept as its own named predicate so a
// future business rule could diverge it without disturbing the other
// two — the same reasoning internal/picking's own CanCancel doc gives
// for not just reusing CanConfirm.
func (s Status) CanUpdate() bool {
	return s == StatusPending
}

// Order is the GORM model for the orders table (migrations/0038).
type Order struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID          int64      `gorm:"column:tenant_id;not null;default:1"`
	OrderNo           string     `gorm:"column:order_no"`
	Customer          *string    `gorm:"column:customer"`
	ExternalReference *string    `gorm:"column:external_reference"`
	WarehouseID       int64      `gorm:"column:warehouse_id"`
	PickingTaskID     *int64     `gorm:"column:picking_task_id"`
	Status            Status     `gorm:"column:status"`
	Remark            *string    `gorm:"column:remark"`
	CreatedBy         *int64     `gorm:"column:created_by"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
	CancelledAt       *time.Time `gorm:"column:cancelled_at"`
	CancelledBy       *int64     `gorm:"column:cancelled_by"`
	CancelRemark      *string    `gorm:"column:cancel_remark"`
}

func (Order) TableName() string { return "orders" }

// Item is the GORM model for order_items — product + ordered_qty only,
// no location/lot: an order is location-agnostic, exactly like a real
// sales order. Location/lot are chosen at CreatePickingTask time.
type Item struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID   int64   `gorm:"column:tenant_id;not null;default:1"`
	OrderID    int64   `gorm:"column:order_id"`
	ProductID  int64   `gorm:"column:product_id"`
	OrderedQty float64 `gorm:"column:ordered_qty"`
	UOM        string  `gorm:"column:uom"`
}

func (Item) TableName() string { return "order_items" }

// applyCancel populates the cancellation-audit columns — same shape as
// picking.Task's/replenishment.Task's own applyCancel.
func (o *Order) applyCancel(userID int64, remark string, now time.Time) {
	o.Status = StatusCancelled
	o.CancelledAt = &now
	o.CancelledBy = &userID
	if remark != "" {
		o.CancelRemark = &remark
	}
}

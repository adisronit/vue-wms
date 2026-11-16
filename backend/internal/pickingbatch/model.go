// Package pickingbatch implements Phase 33C.1 — Batch Picking / Wave
// Management: consolidating picking demand from MULTIPLE RELEASED,
// PICK_BY_PRODUCT orders (internal/order) into one real picking task
// (internal/picking), grouped by product+location+lot+UOM+warehouse,
// while preserving full traceability back to each original order/order
// line via picking_batch_allocations.
//
// This package never duplicates internal/picking's own execution
// engine: once a batch is Confirmed, the resulting picking_tasks row is
// nothing more than an ordinary task with ordinary (consolidated)
// picking_task_items rows, and every subsequent action against it
// (Confirm/ScanConfirm/Repick/AssignPicker/the Pick List PDF) runs
// through internal/picking's own completely unmodified public API
// (Service.CreateTx is the one and only call this package makes into
// it). It also never calls internal/inventory directly — ReserveStockTx
// is only ever reached indirectly, through picking.Service.CreateTx's
// own existing createCore/createItem path, exactly as if each
// consolidated line had been submitted as a normal picking.CreateRequest
// line.
//
// Traceability design: orders.picking_task_id (migrations/0060) carries
// a filtered UNIQUE index — only one order may ever reference a given
// picking task. A batch-created task is referenced by MANY orders at
// once, so this package never writes orders.picking_task_id for a
// batch-picked order. Instead:
//   - orders.picking_batch_id (migrations/0097-0098, plain FK+index, no
//     uniqueness) is set once, at Service.Confirm's own atomic
//     multi-order claim, as the order's own display/traceability link
//     back to this package's picking_batches row.
//   - picking_batch_allocations (migrations/0096) is the real
//     traceability ledger: one row per (order_item, lot, location)
//     slice, recording exactly how much of a consolidated (product, lot,
//     location) picking_task_items line belongs to which original order
//     line — this is what Phase 33C.2's future Sortation will read.
package pickingbatch

import "time"

// Status is the picking batch header's own state machine.
//
//   - DRAFT: the consolidation preview. Lines and allocations are
//     already computed and persisted (picking_batch_allocations exist
//     with picking_task_item_id still NULL), but no picking_tasks row
//     exists yet and no stock has been reserved — Create (the
//     consolidation step) is entirely read-only against inventory.
//   - CONFIRMED: Service.Confirm has run — a real picking_tasks row now
//     exists (created via picking.Service.CreateTx, which performed its
//     own normal ReserveStockTx reservation per line) and every included
//     order has been atomically claimed into PICKING. From this point on,
//     execution proceeds entirely through internal/picking's own engine;
//     this package's own Status no longer drives anything.
//   - COMPLETED/CANCELLED: mirrors the underlying picking task's own
//     terminal status (COMPLETED/SHORT_PICK -> COMPLETED, CANCELLED ->
//     CANCELLED) — written lazily by Service.Get/List when it observes
//     the linked task has reached one, never by a cross-package DB
//     trigger. A DRAFT batch may also reach CANCELLED directly, via
//     Service.Cancel, before any task/reservation ever existed.
type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusConfirmed Status = "CONFIRMED"
	StatusCompleted Status = "COMPLETED"
	StatusCancelled Status = "CANCELLED"
)

// CanConfirm reports whether Service.Confirm may run — only from DRAFT,
// the one and only transition into CONFIRMED. The real guard against a
// concurrent double-confirm is Repository.ClaimConfirm's own atomic
// UPDATE (0 rows affected = already confirmed/cancelled, or a race this
// call lost) — this status check exists only to give a clean 409 before
// ever opening that transaction.
func (s Status) CanConfirm() bool {
	return s == StatusDraft
}

// CanCancel reports whether Service.Cancel may run — DRAFT only. A
// CONFIRMED batch's own picking task must be cancelled directly via
// internal/picking's own POST /picking/{id}/cancel (which already
// releases its own reservation) — see this package's own Cancel doc.
func (s Status) CanCancel() bool {
	return s == StatusDraft
}

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusConfirmed, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// Batch is the GORM model for the picking_batches table
// (migrations/0094).
type Batch struct {
	ID            int64      `gorm:"column:id;primaryKey"`
	TenantID      int64      `gorm:"column:tenant_id;not null"`
	BatchNo       string     `gorm:"column:batch_no"`
	WarehouseID   int64      `gorm:"column:warehouse_id"`
	Status        Status     `gorm:"column:status"`
	PickingTaskID *int64     `gorm:"column:picking_task_id"`
	CreatedBy     *int64     `gorm:"column:created_by"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
	ConfirmedAt   *time.Time `gorm:"column:confirmed_at"`
	CancelledAt   *time.Time `gorm:"column:cancelled_at"`
	CancelledBy   *int64     `gorm:"column:cancelled_by"`
	CancelRemark  *string    `gorm:"column:cancel_remark"`
}

func (Batch) TableName() string { return "picking_batches" }

// BatchOrder is the GORM model for the picking_batch_orders table
// (migrations/0095) — the set of orders consolidated into one batch.
// Inserted at DRAFT creation time, before Confirm — see migrations/
// 0095's own doc for why that is what reserves an order against a
// second concurrent DRAFT batch.
type BatchOrder struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	TenantID  int64     `gorm:"column:tenant_id;not null"`
	BatchID   int64     `gorm:"column:batch_id"`
	OrderID   int64     `gorm:"column:order_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (BatchOrder) TableName() string { return "picking_batch_orders" }

// Allocation is the GORM model for the picking_batch_allocations table
// (migrations/0096) — the critical traceability ledger. See the
// package doc above for the full shape/purpose.
type Allocation struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	TenantID          int64     `gorm:"column:tenant_id;not null"`
	BatchID           int64     `gorm:"column:batch_id"`
	OrderID           int64     `gorm:"column:order_id"`
	OrderItemID       int64     `gorm:"column:order_item_id"`
	ProductID         int64     `gorm:"column:product_id"`
	LotNo             string    `gorm:"column:lot_no"`
	LocationID        int64     `gorm:"column:location_id"`
	PickingTaskItemID *int64    `gorm:"column:picking_task_item_id"`
	AllocatedQty      float64   `gorm:"column:allocated_qty"`
	UOM               string    `gorm:"column:uom"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (Allocation) TableName() string { return "picking_batch_allocations" }

// applyCancel mutates b in place for a successful DRAFT -> CANCELLED
// Cancel — mirrors every other workflow package's own applyCancel
// shape (order.Order, picking.Task, relocation.Task, ...).
func (b *Batch) applyCancel(userID int64, remark *string, now time.Time) {
	b.Status = StatusCancelled
	b.CancelledAt = &now
	b.CancelledBy = &userID
	if remark != nil && *remark != "" {
		b.CancelRemark = remark
	}
}

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

// Status is the order's own explicit state machine. Phase 33A inserts
// RELEASED between PENDING and PICKING: PENDING -> RELEASED (Service.
// Release, the new Review & Release step: a read-only availability
// check, gated on order.confirm, optionally overridden with a
// "controlled shortage approval") -> PICKING (the existing
// CreatePickingTask orchestration, now gated on RELEASED instead of
// PENDING directly — see CanCreatePickingTask below) -> CANCELLED. The
// richer PICKED/SHIPPING/DELIVERED states a real order lifecycle needs
// are still NOT stored here — see dto.go's FulfillmentStatus doc for
// why.
type Status string

const (
	StatusPending   Status = "PENDING"
	StatusReleased  Status = "RELEASED"
	StatusPicking   Status = "PICKING"
	StatusCancelled Status = "CANCELLED"
)

// CanRelease reports whether Service.Release may run — only from
// PENDING, the one and only transition into RELEASED. A second Release
// call (already RELEASED/PICKING) is rejected with a clean 409 rather
// than silently re-running the availability check and re-stamping
// released_at/released_by.
func (s Status) CanRelease() bool {
	return s == StatusPending
}

// CanCreatePickingTask reports whether CreatePickingTask may run — only
// once RELEASED (Service.Release has already run its availability check
// and, if needed, recorded a controlled shortage approval). Phase 33A
// moves this off PENDING directly: Release is now the one and only gate
// in front of this orchestration call, never bypassable by going
// straight from PENDING. picking_task_id is still UNIQUE, so a second
// call would fail that constraint anyway, but this status check gives a
// clean 409 instead of a raw constraint error — unchanged reasoning from
// before Phase 33A, just evaluated against RELEASED now.
func (s Status) CanCreatePickingTask() bool {
	return s == StatusReleased
}

// CanCancel reports whether Cancel may run — before a picking task has
// ever been created for this order, i.e. PENDING or RELEASED. Once
// orchestration has happened, cancelling the underlying picking task
// (via internal/picking's own Cancel, which already releases its own
// reservation) is the correct action, not a second, competing cancel
// path here.
func (s Status) CanCancel() bool {
	return s == StatusPending || s == StatusReleased
}

// CanUpdate reports whether header/items may still be edited — only
// PENDING, strictly narrower than CanCancel now: once Service.Release
// has run (RELEASED), the header/lines a shortage decision was made
// against must stay frozen — editing them after the fact would make
// released_at/shortage_approved describe lines that no longer exist.
// An order that needs changes after Release must be Cancelled and
// recreated, the same "frozen once committed to its next real step"
// precedent CanCreatePickingTask's own pre-33A PENDING-only gate already
// established.
func (s Status) CanUpdate() bool {
	return s == StatusPending
}

// Order is the GORM model for the orders table (migrations/0038).
type Order struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0044, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	TenantID int64   `gorm:"column:tenant_id;not null;default:1"`
	OrderNo  string  `gorm:"column:order_no"`
	Customer *string `gorm:"column:customer"`
	// CustomerID/ShipTo* (migrations/0062) are the Customer Ship-to
	// integration: CustomerID links to internal/customer's own tenant-
	// scoped master; ShipTo* is a server-generated snapshot of the
	// chosen internal/customerdeliverylocation.Location's own fields at
	// the moment Create/Update ran — see Service.resolveShipTo's own
	// doc. Never re-synced if the source delivery location is edited
	// afterward: an order's delivery details are frozen the moment the
	// order itself was placed/edited, not a live view onto the master.
	CustomerID          *int64  `gorm:"column:customer_id"`
	ShipToID            *int64  `gorm:"column:ship_to_id"`
	ShipToName          *string `gorm:"column:ship_to_name"`
	ShipToAddress       *string `gorm:"column:ship_to_address"`
	ShipToProvince      *string `gorm:"column:ship_to_province"`
	ShipToDistrict      *string `gorm:"column:ship_to_district"`
	ShipToPostalCode    *string `gorm:"column:ship_to_postal_code"`
	ShipToContactPerson *string `gorm:"column:ship_to_contact_person"`
	ShipToContactPhone  *string `gorm:"column:ship_to_contact_phone"`
	ExternalReference   *string `gorm:"column:external_reference"`
	WarehouseID         int64   `gorm:"column:warehouse_id"`
	PickingTaskID       *int64  `gorm:"column:picking_task_id"`
	Status              Status  `gorm:"column:status"`
	Remark              *string `gorm:"column:remark"`
	// OrderType (migrations/0079, Phase 33A) defaults to SALES_ORDER.
	// INTERNAL_TRANSFER is also a valid value (CK_orders_order_type) so
	// Phase 33E can reuse this same column later — nothing in this
	// package's own Service builds any transfer-specific behavior around
	// it yet (CLAUDE.md's own "do not build transfer logic now"
	// instruction).
	OrderType            string     `gorm:"column:order_type"`
	Priority             string     `gorm:"column:priority"`
	RequiredShippingDate *time.Time `gorm:"column:required_shipping_date"`
	RequiredDeliveryDate *time.Time `gorm:"column:required_delivery_date"`
	Carrier              *string    `gorm:"column:carrier"`
	ShippingMethod       *string    `gorm:"column:shipping_method"`
	DeliveryInstructions *string    `gorm:"column:delivery_instructions"`
	PreparedBy           *string    `gorm:"column:prepared_by"`
	ApprovedBy           *string    `gorm:"column:approved_by"`
	// ReleasedAt/ReleasedBy/ShortageApproved (migrations/0079) are
	// Service.Release's own write — see Status.CanRelease's doc and
	// Release's own doc for the full Review & Release step.
	ReleasedAt       *time.Time `gorm:"column:released_at"`
	ReleasedBy       *int64     `gorm:"column:released_by"`
	ShortageApproved bool       `gorm:"column:shortage_approved"`
	CreatedBy        *int64     `gorm:"column:created_by"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	CancelledAt      *time.Time `gorm:"column:cancelled_at"`
	CancelledBy      *int64     `gorm:"column:cancelled_by"`
	CancelRemark     *string    `gorm:"column:cancel_remark"`
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
	// PreferredLotNo/FEFOPreferred (migrations/0080, Phase 33A) are a
	// picker HINT only — surfaced to whoever later fills in
	// CreatePickingTaskRequest's own LocationAssignment.LotNo, never
	// consulted by any inventory call Order itself makes. See
	// migrations/0080's own doc.
	PreferredLotNo *string `gorm:"column:preferred_lot_no"`
	FEFOPreferred  bool    `gorm:"column:fefo_preferred"`
	// ShortageFlag/ShortfallQty are written once, by Service.Release's
	// own availability check — never by Create/Update.
	ShortageFlag bool     `gorm:"column:shortage_flag"`
	ShortfallQty *float64 `gorm:"column:shortfall_qty"`
	LineNotes    *string  `gorm:"column:line_notes"`
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

// applyRelease populates the Review & Release audit columns and
// transitions the header into RELEASED — called only after
// Service.Release's own availability check has already decided this may
// proceed (no shortage, or an explicit, order.confirm-gated override).
func (o *Order) applyRelease(userID int64, shortageApproved bool, now time.Time) {
	o.Status = StatusReleased
	o.ReleasedAt = &now
	o.ReleasedBy = &userID
	o.ShortageApproved = shortageApproved
}

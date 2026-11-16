// Package packing implements Phase 33C's Packing module — the physical-
// handling/traceability layer that sits between Picking and Shipping:
// Picking Task (COMPLETED/SHORT_PICK) -> Packing Job -> Packing Job
// Items -> Cartons -> Carton Contents -> Checker Confirm. Physical stock
// already left the building at Picking.Confirm(PICKED) (via
// inventory.ConsumeReservationTx) — this package is a pure record of how
// already-picked quantities get boxed into cartons and NEVER calls any
// internal/inventory mutation function (no AddStockTx/RemoveStockTx/
// MoveStockTx/ReserveStockTx/ConsumeReservationTx/ReleaseReservationTx —
// grepped to confirm internal/inventory's own Service type is not even
// imported for mutation purposes). Its job is to pack already-picked
// quantities into cartons, prevent overpacking (packed qty per line can
// never exceed that line's own frozen PickedQty snapshot), and produce
// the Packing List and Carton/Pallet Label PDFs.
//
// Does NOT gate Shipping.Create — that DB-level "dispatch re-sequencing
// behind pack/load validation" gate is explicitly deferred to Phase 33D.
// internal/shipping is not modified by this package at all.
package packing

import "time"

// Status is the packing job header's state machine:
// DRAFT -> IN_PROGRESS -> COMPLETED / CANCELLED. Unlike Picking/
// Relocation's own COMPLETED (derived automatically once every line is
// terminal), COMPLETED here is reached ONLY via the explicit
// CheckerConfirm action — Phase 33C specifically asks for a second
// person (a "checker") to verify the job before it is considered done,
// so completion can never be a side effect of the packer's own last
// pack action.
type Status string

const (
	StatusDraft      Status = "DRAFT"
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
	StatusCancelled  Status = "CANCELLED"
)

// CanAssign reports whether AssignPacker may run — any non-terminal
// status.
func (s Status) CanAssign() bool {
	switch s {
	case StatusDraft, StatusInProgress:
		return true
	default:
		return false
	}
}

// CanAddCarton reports whether CreateCarton may run — cartons are
// container prep and may be added any time before the job itself is
// COMPLETED or CANCELLED.
func (s Status) CanAddCarton() bool {
	return s.CanAssign()
}

// CanPack reports whether PackItem may run — same non-terminal
// predicate.
func (s Status) CanPack() bool {
	return s.CanAssign()
}

// CanCheckerConfirm reports whether CheckerConfirm may transition this
// job to COMPLETED. Deliberately excludes DRAFT: a job with zero
// packing activity has nothing for a checker to verify yet (see
// Service.CheckerConfirm's own doc for the additional "every line must
// be terminal" requirement checked separately).
func (s Status) CanCheckerConfirm() bool {
	return s == StatusInProgress
}

// CanCancel reports whether Cancel may run — before either terminal
// state, matching every other module's own CanCancel shape.
func (s Status) CanCancel() bool {
	switch s {
	case StatusDraft, StatusInProgress:
		return true
	default:
		return false
	}
}

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusInProgress, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// ItemStatus is a job line's own state, independent of the header.
// EXCEPTION is terminal regardless of packed_qty, mirroring
// picking_task_items' own SHORT-is-terminal-regardless-of-picked_qty
// precedent (internal/picking/model.go's itemStatus).
type ItemStatus string

const (
	ItemStatusPending         ItemStatus = "PENDING"
	ItemStatusPartiallyPacked ItemStatus = "PARTIALLY_PACKED"
	ItemStatusPacked          ItemStatus = "PACKED"
	ItemStatusException       ItemStatus = "EXCEPTION"
)

// ExceptionReason is a closed set for a packing-stage damage/shortage
// exception — deliberately separate from picking_task_items.short_reason
// (ShortReasonCode in internal/picking): a packing-stage exception is a
// different event at a different workflow stage than a picking-stage
// short pick.
type ExceptionReason string

const (
	ExceptionDamaged   ExceptionReason = "DAMAGED"
	ExceptionShortage  ExceptionReason = "SHORTAGE"
	ExceptionWrongItem ExceptionReason = "WRONG_ITEM"
	ExceptionOther     ExceptionReason = "OTHER"
)

func (r ExceptionReason) IsValid() bool {
	switch r {
	case ExceptionDamaged, ExceptionShortage, ExceptionWrongItem, ExceptionOther:
		return true
	default:
		return false
	}
}

// CartonStatus is a carton's own lifecycle — OPEN (still accepting
// packed items) -> CLOSED (sealed, no further packing into it). No
// CANCELLED: a carton is never its own transactional unit the way a job
// is (see migrations/0089's own doc).
type CartonStatus string

const (
	CartonStatusOpen   CartonStatus = "OPEN"
	CartonStatusClosed CartonStatus = "CLOSED"
)

func (s CartonStatus) IsValid() bool {
	switch s {
	case CartonStatusOpen, CartonStatusClosed:
		return true
	default:
		return false
	}
}

// PackagingType mirrors cartons.packaging_type's CHECK constraint.
type PackagingType string

const (
	PackagingBox    PackagingType = "BOX"
	PackagingPallet PackagingType = "PALLET"
	PackagingBag    PackagingType = "BAG"
	PackagingCrate  PackagingType = "CRATE"
	PackagingOther  PackagingType = "OTHER"
)

func (p PackagingType) IsValid() bool {
	switch p {
	case PackagingBox, PackagingPallet, PackagingBag, PackagingCrate, PackagingOther:
		return true
	default:
		return false
	}
}

// WeightUnit/DimensionUnit mirror cartons' own CHECK constraints.
type WeightUnit string

const (
	WeightKG WeightUnit = "KG"
	WeightLB WeightUnit = "LB"
)

func (u WeightUnit) IsValid() bool {
	return u == WeightKG || u == WeightLB
}

type DimensionUnit string

const (
	DimensionCM DimensionUnit = "CM"
	DimensionIN DimensionUnit = "IN"
)

func (u DimensionUnit) IsValid() bool {
	return u == DimensionCM || u == DimensionIN
}

// Job is the GORM model for the packing_jobs table (migrations/0087,
// extended by migrations/0101-0102 with OrderID). OrderNo/Customer/
// CustomerID/ShipTo* are a one-way snapshot copy at Create time — for
// the ordinary (non-batch) flow, copied from the source picking_tasks
// row, matching picking_tasks' own copy from orders (migrations/0062);
// for a batch-sourced job (OrderID set), copied directly from THAT
// order's own row instead, since the shared picking task's own copied
// fields only ever held one (arbitrary, now-meaningless) order's
// snapshot — see Service.Create's own doc for why.
//
// OrderID is NULL for every ordinary PICK_BY_ORDER job — byte-for-byte
// the same shape as before Phase 33C.2 — and set to a specific order_id
// only when the source picking_task_id is batch-sourced (internal/
// pickingbatch), letting one shared task fan out into one packing job
// PER ORDER. See migrations/0102's own doc for the dual filtered-unique-
// index scheme this enables.
type Job struct {
	ID                  int64      `gorm:"column:id;primaryKey"`
	TenantID            int64      `gorm:"column:tenant_id;not null"`
	JobNo               string     `gorm:"column:job_no"`
	PickingTaskID       int64      `gorm:"column:picking_task_id"`
	OrderID             *int64     `gorm:"column:order_id"`
	OrderNo             string     `gorm:"column:order_no"`
	Customer            *string    `gorm:"column:customer"`
	CustomerID          *int64     `gorm:"column:customer_id"`
	ShipToID            *int64     `gorm:"column:ship_to_id"`
	ShipToName          *string    `gorm:"column:ship_to_name"`
	ShipToAddress       *string    `gorm:"column:ship_to_address"`
	ShipToProvince      *string    `gorm:"column:ship_to_province"`
	ShipToDistrict      *string    `gorm:"column:ship_to_district"`
	ShipToPostalCode    *string    `gorm:"column:ship_to_postal_code"`
	ShipToContactPerson *string    `gorm:"column:ship_to_contact_person"`
	ShipToContactPhone  *string    `gorm:"column:ship_to_contact_phone"`
	WarehouseID         int64      `gorm:"column:warehouse_id"`
	Status              Status     `gorm:"column:status"`
	AssignedPackerID    *int64     `gorm:"column:assigned_packer_id"`
	AssignedAt          *time.Time `gorm:"column:assigned_at"`
	CheckerID           *int64     `gorm:"column:checker_id"`
	CheckerConfirmedAt  *time.Time `gorm:"column:checker_confirmed_at"`
	Remark              *string    `gorm:"column:remark"`
	CreatedBy           *int64     `gorm:"column:created_by"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	CompletedAt         *time.Time `gorm:"column:completed_at"`
	CancelledAt         *time.Time `gorm:"column:cancelled_at"`
	CancelledBy         *int64     `gorm:"column:cancelled_by"`
	CancelRemark        *string    `gorm:"column:cancel_remark"`
}

func (Job) TableName() string { return "packing_jobs" }

// applyCancel mutates the job in place for a successful cancellation —
// matches picking.Task.applyCancel/relocation.Task.applyCancel exactly.
func (j *Job) applyCancel(userID int64, remark *string, now time.Time) {
	j.Status = StatusCancelled
	j.CancelledAt = &now
	j.CancelledBy = &userID
	if remark != nil && *remark != "" {
		j.CancelRemark = remark
	}
}

// Item is the GORM model for the packing_job_items table
// (migrations/0088). OrderedQty/PickedQty are a frozen snapshot copied
// once from picking_task_items at job-creation time — see that
// migration's own doc for why this is never re-derived or re-synced.
// PackedQty is cumulative across every pack action for this line
// (potentially split across multiple cartons — see CartonContent),
// written via an atomic guarded UPDATE (repository.go's AdjustPackedQty)
// rather than a Go read-modify-write — the same AdjustPickedQty/
// AdjustMovedQty convention this codebase already established.
type Item struct {
	ID                int64      `gorm:"column:id;primaryKey"`
	TenantID          int64      `gorm:"column:tenant_id;not null"`
	PackingJobID      int64      `gorm:"column:packing_job_id"`
	PickingTaskItemID int64      `gorm:"column:picking_task_item_id"`
	ProductID         int64      `gorm:"column:product_id"`
	LotNo             string     `gorm:"column:lot_no"`
	OrderedQty        float64    `gorm:"column:ordered_qty"`
	PickedQty         float64    `gorm:"column:picked_qty"`
	PackedQty         float64    `gorm:"column:packed_qty"`
	UOM               string     `gorm:"column:uom"`
	Status            ItemStatus `gorm:"column:status"`
	ExceptionFlag     bool       `gorm:"column:exception_flag"`
	ExceptionReason   *string    `gorm:"column:exception_reason"`
	ExceptionQty      *float64   `gorm:"column:exception_qty"`
	Remark            *string    `gorm:"column:remark"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (Item) TableName() string { return "packing_job_items" }

// Carton is the GORM model for the cartons table (migrations/0089). LPN
// is system-generated from the new IDENTITY id (never user-supplied —
// see that migration's own doc for why this differs from relocation's
// purely-descriptive LPN column).
type Carton struct {
	ID            int64         `gorm:"column:id;primaryKey"`
	TenantID      int64         `gorm:"column:tenant_id;not null"`
	PackingJobID  int64         `gorm:"column:packing_job_id"`
	LPN           string        `gorm:"column:lpn"`
	PackagingType PackagingType `gorm:"column:packaging_type"`
	GrossWeight   *float64      `gorm:"column:gross_weight"`
	NetWeight     *float64      `gorm:"column:net_weight"`
	WeightUnit    WeightUnit    `gorm:"column:weight_unit"`
	CartonLength  *float64      `gorm:"column:carton_length"`
	CartonWidth   *float64      `gorm:"column:carton_width"`
	CartonHeight  *float64      `gorm:"column:carton_height"`
	DimensionUnit DimensionUnit `gorm:"column:dimension_unit"`
	Status        CartonStatus  `gorm:"column:status"`
	Remark        *string       `gorm:"column:remark"`
	CreatedBy     *int64        `gorm:"column:created_by"`
	CreatedAt     time.Time     `gorm:"column:created_at"`
	UpdatedAt     time.Time     `gorm:"column:updated_at"`
}

func (Carton) TableName() string { return "cartons" }

// CartonContent is the GORM model for the carton_contents table
// (migrations/0090) — append-only, see that migration's own doc. Each
// row is both one join entry AND its own audit trail of a pack action.
type CartonContent struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	TenantID         int64     `gorm:"column:tenant_id;not null"`
	CartonID         int64     `gorm:"column:carton_id"`
	PackingJobItemID int64     `gorm:"column:packing_job_item_id"`
	PackedQty        float64   `gorm:"column:packed_qty"`
	CreatedBy        *int64    `gorm:"column:created_by"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (CartonContent) TableName() string { return "carton_contents" }

// Confirmation is the GORM model for the packing_confirmations table
// (migrations/0091) — the idempotency guard for PackItem, the same
// pattern as picking.Confirmation/relocation.Confirmation.
type Confirmation struct {
	IdempotencyKey string    `gorm:"column:idempotency_key;primaryKey"`
	TenantID       int64     `gorm:"column:tenant_id;not null"`
	PackingJobID   int64     `gorm:"column:packing_job_id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (Confirmation) TableName() string { return "packing_confirmations" }

// itemStatus derives a line's status from its own quantities and
// whether it has been flagged with an exception — mirrors
// picking.itemStatus exactly, EXCEPTION standing in for SHORT.
func itemStatus(pickedQty, packedQty float64, exception bool) ItemStatus {
	if exception {
		return ItemStatusException
	}
	if packedQty <= 0 {
		return ItemStatusPending
	}
	if pickedQty > 0 && packedQty < pickedQty {
		return ItemStatusPartiallyPacked
	}
	return ItemStatusPacked
}

// itemTerminal reports whether a line has reached a state CheckerConfirm
// will accept as "resolved" — PACKED or EXCEPTION, the same two-terminal-
// states shape picking.computeTaskStatus checks for COMPLETED/SHORT.
func itemTerminal(status ItemStatus) bool {
	return status == ItemStatusPacked || status == ItemStatusException
}

// computeJobStatus derives the header status from its lines, but
// deliberately NEVER returns StatusCompleted — unlike Picking/
// Relocation, completion here requires the explicit CheckerConfirm
// action (see Status' own doc), so this only ever distinguishes DRAFT
// (no activity on any line yet) from IN_PROGRESS (at least one line has
// packed_qty > 0 or has been flagged with an exception).
func computeJobStatus(items []Item) Status {
	for _, it := range items {
		if it.PackedQty > 0 || it.ExceptionFlag {
			return StatusInProgress
		}
	}
	return StatusDraft
}

// allItemsTerminal reports whether every line in items is PACKED or
// EXCEPTION — the precondition CheckerConfirm additionally enforces on
// top of Status.CanCheckerConfirm().
func allItemsTerminal(items []Item) bool {
	if len(items) == 0 {
		return false
	}
	for _, it := range items {
		if !itemTerminal(it.Status) {
			return false
		}
	}
	return true
}

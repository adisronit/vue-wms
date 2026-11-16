package pickingbatch

import "time"

// CreateRequest is POST /picking-batches — the consolidation step
// (DRAFT only, no reservation). OrderIDs must each be RELEASED,
// PICK_BY_PRODUCT, belong to the caller's own tenant, belong to
// WarehouseID, and not already be a member of another active (DRAFT or
// CONFIRMED) batch — every failing order is reported individually (see
// ErrOrderNotEligible), never silently dropped.
type CreateRequest struct {
	WarehouseID int64   `json:"warehouse_id,string" binding:"required" example:"1"`
	OrderIDs    []int64 `json:"order_ids" binding:"required,min=2" example:"1,2"`
} //	@name	CreatePickingBatchRequest

// OrderSummaryResponse is one order included in a batch — enough to
// show on the preview/detail screen that each order's own customer/
// ship-to snapshot remains distinct (this package never merges or
// rewrites it), and its own live status.
type OrderSummaryResponse struct {
	OrderID    int64   `json:"order_id,string" example:"1"`
	OrderNo    string  `json:"order_no" example:"ORD-000001"`
	Customer   *string `json:"customer"`
	ShipToName *string `json:"ship_to_name"`
	Status     string  `json:"status" example:"PICKING"`
} //	@name	PickingBatchOrderSummary

// AllocationResponse is one order's own share of a consolidated line —
// matches picking_batch_allocations exactly, the breakdown the preview
// and the future Sortation step both need ("of the 100 units picked for
// SKU-A, 20 belong to ORD-000001, 30 to ORD-000002, 50 to ORD-000003").
type AllocationResponse struct {
	OrderID           int64  `json:"order_id,string" example:"1"`
	OrderNo           string `json:"order_no" example:"ORD-000001"`
	OrderItemID       int64  `json:"order_item_id,string" example:"1"`
	PickingTaskItemID *int64 `json:"picking_task_item_id,string"`
	AllocatedQty      float64
} //	@name	PickingBatchAllocationResponse

// ConsolidatedLineResponse is one (product, lot, location) consolidated
// demand line — becomes exactly one picking_task_items row at Confirm.
// AllocatedQty summed across Allocations always equals Qty.
type ConsolidatedLineResponse struct {
	ProductID    int64                `json:"product_id,string" example:"1"`
	SKU          string               `json:"sku" example:"SKU-0001"`
	ProductName  string               `json:"product_name" example:"Barcode Label Roll"`
	LotNo        string               `json:"lot_no" example:"LOT-0001"`
	LocationID   int64                `json:"location_id,string" example:"1"`
	LocationCode string               `json:"location_code" example:"A-01-01"`
	Qty          float64              `json:"qty" example:"100"`
	UOM          string               `json:"uom" example:"PCS"`
	Allocations  []AllocationResponse `json:"allocations"`
} //	@name	PickingBatchConsolidatedLine

// Response is a full picking batch — GET /picking-batches/{id} and the
// result of Create/Confirm. Sortation* fields (Phase 33C.2) mirror
// picking_batches' own sortation_* columns for at-a-glance progress —
// the full per-line worklist lives at GET /picking-batches/{id}/
// sortation instead (SortationResponse below).
type Response struct {
	ID                   int64                      `json:"id,string" example:"1"`
	BatchNo              string                     `json:"batch_no" example:"BATCH-000001"`
	WarehouseID          int64                      `json:"warehouse_id,string" example:"1"`
	WarehouseCode        string                     `json:"warehouse_code" example:"WH-01"`
	Status               Status                     `json:"status" example:"DRAFT"`
	PickingTaskID        *int64                     `json:"picking_task_id,string"`
	PickNo               *string                    `json:"pick_no"`
	Orders               []OrderSummaryResponse     `json:"orders"`
	Lines                []ConsolidatedLineResponse `json:"lines"`
	CreatedBy            *int64                     `json:"created_by,string"`
	CreatedAt            time.Time                  `json:"created_at"`
	UpdatedAt            time.Time                  `json:"updated_at"`
	ConfirmedAt          *time.Time                 `json:"confirmed_at"`
	CancelledAt          *time.Time                 `json:"cancelled_at"`
	CancelledBy          *int64                     `json:"cancelled_by,string"`
	CancelRemark         *string                    `json:"cancel_remark"`
	SortationStatus      SortationStatus            `json:"sortation_status" example:"NOT_STARTED"`
	SortationStation     *string                    `json:"sortation_station"`
	SortationAssignedTo  *int64                     `json:"sortation_assigned_to,string"`
	SortationStartedAt   *time.Time                 `json:"sortation_started_at"`
	SortationCompletedAt *time.Time                 `json:"sortation_completed_at"`
} //	@name	PickingBatchResponse

// ListResponse is one row of GET /picking-batches.
type ListResponse struct {
	ID            int64     `json:"id,string" example:"1"`
	BatchNo       string    `json:"batch_no" example:"BATCH-000001"`
	WarehouseCode string    `json:"warehouse_code" example:"WH-01"`
	OrderCount    int       `json:"order_count" example:"3"`
	LineCount     int       `json:"line_count" example:"2"`
	Status        Status    `json:"status" example:"DRAFT"`
	PickNo        *string   `json:"pick_no"`
	CreatedAt     time.Time `json:"created_at"`
} //	@name	PickingBatchListResponse

// ListParams narrows GET /picking-batches.
type ListParams struct {
	Status      Status
	WarehouseID int64
	Page        int
	Limit       int
}

// AvailableOrderResponse is one row of GET /picking-batches/
// available-orders?warehouse_id= — the multi-select source: every
// RELEASED, PICK_BY_PRODUCT order in that warehouse not already a
// member of another active (DRAFT/CONFIRMED) batch.
type AvailableOrderResponse struct {
	OrderID    int64   `json:"order_id,string" example:"1"`
	OrderNo    string  `json:"order_no" example:"ORD-000001"`
	Customer   *string `json:"customer"`
	ShipToName *string `json:"ship_to_name"`
	ItemCount  int     `json:"item_count" example:"2"`
} //	@name	PickingBatchAvailableOrder

// CancelRequest is POST /picking-batches/{id}/cancel — DRAFT only, see
// Service.Cancel's own doc.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Orders being re-planned"`
} //	@name	CancelPickingBatchRequest

// ---------------------------------------------------------------------
// Phase 33C.2 -- Sortation
// ---------------------------------------------------------------------

// SortationLineResponse is one allocation row of the Sortation worklist
// — the order-share granularity the pick floor physically handles.
// Remaining is a derived convenience (never persisted): AllocatedQty -
// SortedQty - ShortageQty, clamped at 0.
type SortationLineResponse struct {
	AllocationID      int64    `json:"allocation_id,string" example:"1"`
	OrderID           int64    `json:"order_id,string" example:"1"`
	OrderNo           string   `json:"order_no" example:"ORD-000001"`
	ShipToName        *string  `json:"ship_to_name"`
	OrderItemID       int64    `json:"order_item_id,string" example:"1"`
	ProductID         int64    `json:"product_id,string" example:"1"`
	SKU               string   `json:"sku" example:"SKU-0001"`
	ProductName       string   `json:"product_name" example:"Barcode Label Roll"`
	LotNo             string   `json:"lot_no" example:"LOT-0001"`
	LocationID        int64    `json:"location_id,string" example:"1"`
	LocationCode      string   `json:"location_code" example:"A-01-01"`
	PickingTaskItemID *int64   `json:"picking_task_item_id,string"`
	AllocatedQty      float64  `json:"allocated_qty" example:"20"`
	SortedQty         float64  `json:"sorted_qty" example:"0"`
	ShortageQty       *float64 `json:"shortage_qty"`
	ExceptionReason   *string  `json:"exception_reason"`
	Remaining         float64  `json:"remaining" example:"20"`
	UOM               string   `json:"uom" example:"PCS"`
	Resolved          bool     `json:"resolved" example:"false"`
} //	@name	SortationLineResponse

// SortationResponse is the full Sortation worklist for one batch — GET
// /picking-batches/{id}/sortation, and the result of every mutating
// sortation action.
type SortationResponse struct {
	BatchID              int64                   `json:"batch_id,string" example:"1"`
	BatchNo              string                  `json:"batch_no" example:"BATCH-000001"`
	PickingTaskID        *int64                  `json:"picking_task_id,string"`
	PickNo               *string                 `json:"pick_no"`
	SortationStatus      SortationStatus         `json:"sortation_status" example:"IN_PROGRESS"`
	SortationStation     *string                 `json:"sortation_station"`
	SortationAssignedTo  *int64                  `json:"sortation_assigned_to,string"`
	SortationStartedAt   *time.Time              `json:"sortation_started_at"`
	SortationCompletedAt *time.Time              `json:"sortation_completed_at"`
	Lines                []SortationLineResponse `json:"lines"`
} //	@name	SortationResponse

// SortationScanRequest is POST /picking-batches/{id}/sortation/scan —
// one physical "move N units of this order's own share onto its pile"
// confirmation. OrderBarcode is matched against order_no (case-
// insensitive/trimmed) among the batch's OWN member orders only — a
// real order_no that belongs to a different batch (or no batch at all)
// is rejected exactly like a typo. SKU/LotNo are matched against the
// scanned order's own allocation rows for this batch, the same
// scan-verification shape picking.ScanConfirm/packing.PackItem already
// use.
type SortationScanRequest struct {
	OrderBarcode string  `json:"order_barcode" binding:"required" example:"ORD-000001"`
	SKU          string  `json:"sku" binding:"required" example:"SKU-0001"`
	LotNo        string  `json:"lot_no" example:"LOT-0001"`
	Quantity     float64 `json:"quantity" binding:"required" example:"20"`
} //	@name	SortationScanRequest

// SortationExceptionRequest is POST /picking-batches/{id}/sortation/
// exception — flags a sortation-stage damage/shortage discovery on one
// specific allocation row (identified by AllocationID, taken from the
// worklist response), terminating that row's need for further sortation
// regardless of its current SortedQty, as long as SortedQty + ShortageQty
// does not exceed AllocatedQty (checked by Service.FlagSortationException
// before writing).
type SortationExceptionRequest struct {
	AllocationID int64   `json:"allocation_id,string" binding:"required" example:"1"`
	ShortageQty  float64 `json:"shortage_qty" binding:"required" example:"5"`
	Reason       string  `json:"reason" binding:"required" example:"Found damaged while sorting"`
} //	@name	SortationExceptionRequest

// SortationAssignRequest is PUT /picking-batches/{id}/sortation/assign
// — the station/operator planning annotation, mirroring picking.
// AssignPicker/packing.AssignPacker's own authority level (never an
// authorization gate). Both fields are optional; either may be omitted
// to leave that field unchanged.
type SortationAssignRequest struct {
	Station *string `json:"station" example:"SORT-01"`
	UserID  *int64  `json:"user_id,string" example:"1"`
} //	@name	SortationAssignRequest

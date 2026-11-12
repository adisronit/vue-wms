package picking

import "time"

// ItemResponse is one picking task line, matching PickingView.vue's
// detail drawer "Picking Items" cards (sku, productName, location,
// orderQty, pickedQty).
type ItemResponse struct {
	ID           int64      `json:"id,string" example:"1"`
	ProductID    int64      `json:"product_id,string" example:"1"`
	SKU          string     `json:"sku" example:"SKU-0001"`
	ProductName  string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo        string     `json:"lot_no"`
	LocationID   int64      `json:"location_id,string" example:"1"`
	LocationCode string     `json:"location_code" example:"A-01-01"`
	OrderedQty   float64    `json:"ordered_qty" example:"20"`
	PickedQty    float64    `json:"picked_qty" example:"0"`
	UOM          string     `json:"uom" example:"PCS"`
	Status       ItemStatus `json:"status" example:"PENDING"`
	Remark       *string    `json:"remark"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
} //	@name	PickingItemResponse

// Response is a full picking task with its lines — GET /picking/{id}.
// CancelledAt/CancelledBy/CancelledByName/CancelRemark (Phase 18) are
// all nil/null unless the task was actually cancelled — see
// Service.Cancel's own doc.
type Response struct {
	ID              int64          `json:"id,string" example:"1"`
	PickNo          string         `json:"pick_no" example:"PICK-000001"`
	OrderNo         string         `json:"order_no" example:"SO-20260608-001"`
	Customer        *string        `json:"customer" example:"ABC Customer"`
	WarehouseID     int64          `json:"warehouse_id,string" example:"1"`
	WarehouseCode   string         `json:"warehouse_code" example:"WH-01"`
	Status          Status         `json:"status" example:"PENDING"`
	Priority        Priority       `json:"priority" example:"NORMAL"`
	DueDate         *time.Time     `json:"due_date"`
	Remark          *string        `json:"remark"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	CompletedAt     *time.Time     `json:"completed_at"`
	CancelledAt     *time.Time     `json:"cancelled_at"`
	CancelledBy     *int64         `json:"cancelled_by,string"`
	CancelledByName *string        `json:"cancelled_by_name"`
	CancelRemark    *string        `json:"cancel_remark"`
	Items           []ItemResponse `json:"items"`
} //	@name	PickingTaskResponse

// ListResponse is one row of GET /picking — task-level, matching
// PickingView.vue's table (Pick No./Order No./Customer/Warehouse/
// Priority/Items/Progress/Due Date/Status) directly.
type ListResponse struct {
	ID            int64      `json:"id,string" example:"1"`
	PickNo        string     `json:"pick_no" example:"PICK-000001"`
	OrderNo       string     `json:"order_no" example:"SO-20260608-001"`
	Customer      *string    `json:"customer" example:"ABC Customer"`
	WarehouseCode string     `json:"warehouse_code" example:"WH-01"`
	Priority      Priority   `json:"priority" example:"NORMAL"`
	ItemCount     int        `json:"item_count" example:"3"`
	ProgressPct   float64    `json:"progress_pct" example:"45"`
	DueDate       *time.Time `json:"due_date"`
	Status        Status     `json:"status" example:"PENDING"`
} //	@name	PickingListResponse

// CreateRequest is POST /picking — the drawer's single "+ New Picking
// Task" flow submits the whole task (header + lines) at once. A new task
// always starts PENDING; use POST /picking/{id}/confirm to actually
// remove stock.
type CreateRequest struct {
	OrderNo     string              `json:"order_no" binding:"required" example:"SO-20260608-005"`
	Customer    *string             `json:"customer" example:"ABC Customer"`
	WarehouseID int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Priority    Priority            `json:"priority" example:"NORMAL"`
	DueDate     *time.Time          `json:"due_date"`
	Remark      *string             `json:"remark"`
	Items       []CreateItemRequest `json:"items"`
} //	@name	CreatePickingRequest

// CreateItemRequest is one line within CreateRequest.Items — the
// product/quantity/source-location a picker must draw down.
type CreateItemRequest struct {
	ProductID  int64   `json:"product_id,string" binding:"required" example:"1"`
	LotNo      string  `json:"lot_no" example:"LOT-0001"`
	LocationID int64   `json:"location_id,string" binding:"required" example:"1"`
	OrderedQty float64 `json:"ordered_qty" binding:"required" example:"20"`
	UOM        string  `json:"uom" example:"PCS"`
} //	@name	CreatePickingItemRequest

// ConfirmRequest is POST /picking/{id}/confirm — the drawer's
// "Confirm Pick"/"Short Pick"/"Complete Picking" actions submitting one
// or more line actions at once. IdempotencyKey is required — see
// picking_confirmations' migration doc for why.
type ConfirmRequest struct {
	IdempotencyKey string               `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	Items          []ConfirmItemRequest `json:"items" binding:"required,min=1"`
} //	@name	ConfirmPickingRequest

// ConfirmItemRequest is one line's confirm action. Action "PICKED"
// (matching PickingView.vue's "Confirm Pick" button) removes Quantity
// from the line's source location; action "SHORT" (matching its
// "Short Pick" button) records Remark and removes nothing.
type ConfirmItemRequest struct {
	ItemID   int64   `json:"item_id,string" binding:"required" example:"1"`
	Action   string  `json:"action" binding:"required,oneof=PICKED SHORT" example:"PICKED"`
	Quantity float64 `json:"quantity" example:"20"`
	Remark   string  `json:"remark" example:"Location empty, short 5 units"`
} //	@name	ConfirmPickingItemRequest

// CancelRequest is POST /picking/{id}/cancel — Phase 13's addition.
// Remark is optional; when provided it is stored in the dedicated
// cancel_remark column (Phase 18) rather than overwriting the task's own
// Remark field — see Service.Cancel's doc for why this changed. No
// idempotency_key: unlike Confirm, Cancel's own mutation (releasing
// whatever remains reserved) is naturally safe under a duplicate/retry —
// the first call transitions the task to CANCELLED and the status guard
// (Status.CanCancel) alone rejects any repeat, the same way re-confirming
// an already-COMPLETED task is rejected without a dedicated idempotency
// table.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Customer changed the order"`
} //	@name	CancelPickingRequest

// ListParams narrows GET /picking to the Picking screen's actual search
// box (matches pick no, order no, customer, or a line's SKU/product
// name) plus status/priority/date filters, with pagination.
type ListParams struct {
	Search      string
	Status      Status
	Priority    Priority
	WarehouseID int64
	Date        *time.Time
	Page        int
	Limit       int
}

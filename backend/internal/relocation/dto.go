package relocation

import "time"

// ItemResponse is one relocation task line, matching PutawayItemResponse's
// shape with the addition of LPN (pallet/license-plate) and ExpiryDate
// (resolved live from the current source inventory balance — see
// repository.go's ListItems doc, never a stored column).
type ItemResponse struct {
	ID                      int64      `json:"id,string" example:"1"`
	ProductID               int64      `json:"product_id,string" example:"1"`
	SKU                     string     `json:"sku" example:"SKU-0001"`
	ProductName             string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo                   string     `json:"lot_no"`
	LPN                     *string    `json:"lpn"`
	ExpiryDate              *time.Time `json:"expiry_date"`
	SourceLocationID        int64      `json:"source_location_id,string" example:"1"`
	SourceLocationCode      string     `json:"source_location_code" example:"A-01-01"`
	DestinationLocationID   int64      `json:"destination_location_id,string" example:"2"`
	DestinationLocationCode string     `json:"destination_location_code" example:"B-02-03"`
	RequestedQty            float64    `json:"requested_qty" example:"100"`
	MovedQty                float64    `json:"moved_qty" example:"0"`
	UOM                     string     `json:"uom" example:"ROLL"`
	Status                  ItemStatus `json:"status" example:"PENDING"`
	Remark                  *string    `json:"remark"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
} //	@name	RelocationItemResponse

// Response is a full relocation task with its lines — GET /relocations/{id}.
type Response struct {
	ID              int64          `json:"id,string" example:"1"`
	RelocationNo    string         `json:"relocation_no" example:"RL-000001"`
	WarehouseID     int64          `json:"warehouse_id,string" example:"1"`
	WarehouseCode   string         `json:"warehouse_code" example:"WH-01"`
	Status          Status         `json:"status" example:"DRAFT"`
	Remark          *string        `json:"remark"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	ReadyAt         *time.Time     `json:"ready_at"`
	CompletedAt     *time.Time     `json:"completed_at"`
	CancelledAt     *time.Time     `json:"cancelled_at"`
	CancelledBy     *int64         `json:"cancelled_by,string"`
	CancelledByName *string        `json:"cancelled_by_name"`
	CancelRemark    *string        `json:"cancel_remark"`
	Items           []ItemResponse `json:"items"`
} //	@name	RelocationTaskResponse

// ListResponse is one row of GET /relocations — item-level, matching
// PutawayListResponse's own shape, with TaskStatus added so the list
// screen can filter/show the header's own state alongside the line's.
type ListResponse struct {
	TaskID                  int64      `json:"task_id,string" example:"1"`
	ItemID                  int64      `json:"item_id,string" example:"1"`
	RelocationNo            string     `json:"relocation_no" example:"RL-000001"`
	SKU                     string     `json:"sku" example:"SKU-0001"`
	ProductName             string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo                   string     `json:"lot_no"`
	RequestedQty            float64    `json:"requested_qty" example:"100"`
	MovedQty                float64    `json:"moved_qty" example:"0"`
	SourceLocationCode      string     `json:"source_location_code" example:"A-01-01"`
	DestinationLocationCode string     `json:"destination_location_code" example:"B-02-03"`
	Status                  ItemStatus `json:"status" example:"PENDING"`
	TaskStatus              Status     `json:"task_status" example:"DRAFT"`
} //	@name	RelocationListResponse

// CreateRequest is POST /relocations — the New Relocation screen's
// single submit sends the whole task (header + lines) at once, mirroring
// putaway.CreateRequest's shape. A new task always starts DRAFT; use
// POST /relocations/{id}/ready, then POST /relocations/{id}/confirm to
// actually move stock.
type CreateRequest struct {
	WarehouseID int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Remark      *string             `json:"remark"`
	Items       []CreateItemRequest `json:"items"`
} //	@name	CreateRelocationRequest

// CreateItemRequest is one line within CreateRequest.Items.
type CreateItemRequest struct {
	ProductID             int64   `json:"product_id,string" binding:"required" example:"1"`
	LotNo                 string  `json:"lot_no" example:"LOT-0001"`
	LPN                   *string `json:"lpn" example:"PLT-0001"`
	SourceLocationID      int64   `json:"source_location_id,string" binding:"required" example:"1"`
	DestinationLocationID int64   `json:"destination_location_id,string" binding:"required" example:"2"`
	RequestedQty          float64 `json:"requested_qty" binding:"required" example:"100"`
	UOM                   string  `json:"uom" example:"ROLL"`
	Remark                *string `json:"remark"`
} //	@name	CreateRelocationItemRequest

// ReadyRequest is POST /relocations/{id}/ready — no body needed, kept
// as a named (empty) type for Swagger symmetry with the other action
// endpoints.
type ReadyRequest struct{} //	@name	ReadyRelocationRequest

// ConfirmRequest is POST /relocations/{id}/confirm — IdempotencyKey is
// required, matching putaway_confirmations' idempotency guard.
type ConfirmRequest struct {
	IdempotencyKey string               `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	Items          []ConfirmItemRequest `json:"items" binding:"required,min=1"`
} //	@name	ConfirmRelocationRequest

// ConfirmItemRequest is one line's confirm action — moves Quantity from
// the line's source to its destination (or DestinationLocationID, if
// this call overrides it, matching putaway's own per-confirm override
// for partial relocation across multiple destinations).
type ConfirmItemRequest struct {
	ItemID                int64   `json:"item_id,string" binding:"required" example:"1"`
	Quantity              float64 `json:"quantity" binding:"required" example:"50"`
	DestinationLocationID int64   `json:"destination_location_id,string" example:"2"`
} //	@name	ConfirmRelocationItemRequest

// CancelRequest is POST /relocations/{id}/cancel. Remark is optional;
// when provided it is stored in the dedicated cancel_remark column
// rather than overwriting the task's own Remark field.
type CancelRequest struct {
	Remark *string `json:"remark" example:"No longer needed"`
} //	@name	CancelRelocationRequest

// ListParams narrows GET /relocations to the Relocation List screen's
// search box plus item-status/task-status/warehouse filters, with
// pagination.
type ListParams struct {
	Search      string
	Status      ItemStatus
	TaskStatus  Status
	WarehouseID int64
	Page        int
	Limit       int
}

package putaway

import "time"

// ItemResponse is one putaway task line, matching PutawayView.vue's
// list row shape (each row IS one item, joined with its task header —
// see repository.go's list query doc) and NewPutawayView.vue's items
// table.
type ItemResponse struct {
	ID                      int64      `json:"id,string" example:"1"`
	ReceiptItemID           *int64     `json:"receipt_item_id,string"`
	ProductID               int64      `json:"product_id,string" example:"1"`
	SKU                     string     `json:"sku" example:"SKU-0001"`
	ProductName             string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo                   string     `json:"lot_no"`
	SourceLocationID        int64      `json:"source_location_id,string" example:"1"`
	SourceLocationCode      string     `json:"source_location_code" example:"STAGING-01"`
	DestinationLocationID   int64      `json:"destination_location_id,string" example:"2"`
	DestinationLocationCode string     `json:"destination_location_code" example:"A-01-01"`
	RequestedQty            float64    `json:"requested_qty" example:"100"`
	MovedQty                float64    `json:"moved_qty" example:"0"`
	UOM                     string     `json:"uom" example:"ROLL"`
	Status                  ItemStatus `json:"status" example:"PENDING"`
	Remark                  *string    `json:"remark"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
} //	@name	PutawayItemResponse

// Response is a full putaway task with its lines — GET /putaway/{id}.
// CancelledAt/CancelledBy/CancelledByName/CancelRemark (Phase 18) are
// all nil/null unless the task was actually cancelled — see
// Service.Cancel's own doc.
type Response struct {
	ID              int64          `json:"id,string" example:"1"`
	PutawayNo       string         `json:"putaway_no" example:"PA-000001"`
	ReceiptID       *int64         `json:"receipt_id,string"`
	ReceiptNo       *string        `json:"receipt_no" example:"RCV-000001"`
	WarehouseID     int64          `json:"warehouse_id,string" example:"1"`
	WarehouseCode   string         `json:"warehouse_code" example:"WH-01"`
	Status          Status         `json:"status" example:"PENDING"`
	Priority        Priority       `json:"priority" example:"NORMAL"`
	AssignedTo      *string        `json:"assigned_to"`
	Remark          *string        `json:"remark"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	CompletedAt     *time.Time     `json:"completed_at"`
	CancelledAt     *time.Time     `json:"cancelled_at"`
	CancelledBy     *int64         `json:"cancelled_by,string"`
	CancelledByName *string        `json:"cancelled_by_name"`
	CancelRemark    *string        `json:"cancel_remark"`
	Items           []ItemResponse `json:"items"`
} //	@name	PutawayTaskResponse

// ListResponse is one row of GET /putaway — item-level, matching
// PutawayView.vue's table (Task No./Receiving No./SKU/Product/Qty/
// From Location/Suggested Bin/Actual Bin/Status) directly, with no
// separate item-list drawer the way Receiving needed one.
type ListResponse struct {
	TaskID                  int64      `json:"task_id,string" example:"1"`
	ItemID                  int64      `json:"item_id,string" example:"1"`
	PutawayNo               string     `json:"putaway_no" example:"PA-000001"`
	ReceiptNo               *string    `json:"receipt_no" example:"RCV-000001"`
	SKU                     string     `json:"sku" example:"SKU-0001"`
	ProductName             string     `json:"product_name" example:"Barcode Label Roll"`
	RequestedQty            float64    `json:"requested_qty" example:"100"`
	MovedQty                float64    `json:"moved_qty" example:"0"`
	SourceLocationCode      string     `json:"source_location_code" example:"STAGING-01"`
	DestinationLocationCode string     `json:"destination_location_code" example:"A-01-01"`
	Status                  ItemStatus `json:"status" example:"PENDING"`
} //	@name	PutawayListResponse

// CreateRequest is POST /putaway — the create screen's single "Save
// Draft"/"Confirm Putaway" click submits the whole task (header +
// lines) at once, mirroring Receiving's CreateRequest shape.
type CreateRequest struct {
	ReceiptID   *int64              `json:"receipt_id,string" example:"1"`
	WarehouseID int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Priority    Priority            `json:"priority" example:"NORMAL"`
	AssignedTo  *string             `json:"assigned_to" example:"Warehouse Staff"`
	Remark      *string             `json:"remark"`
	Items       []CreateItemRequest `json:"items"`
} //	@name	CreatePutawayRequest

// CreateItemRequest is one line within CreateRequest.Items.
// ReceiptItemID is set when the line was loaded from a real receipt
// line (GET /putaway/sources/{receiptId}); left nil for a line added
// manually or via scan (real NewPutawayView.vue behavior — see
// migrations/0015's doc).
type CreateItemRequest struct {
	ReceiptItemID         *int64  `json:"receipt_item_id,string" example:"1"`
	ProductID             int64   `json:"product_id,string" binding:"required" example:"1"`
	LotNo                 string  `json:"lot_no" example:"LOT-0001"`
	SourceLocationID      int64   `json:"source_location_id,string" binding:"required" example:"1"`
	DestinationLocationID int64   `json:"destination_location_id,string" binding:"required" example:"2"`
	RequestedQty          float64 `json:"requested_qty" binding:"required" example:"100"`
	UOM                   string  `json:"uom" example:"ROLL"`
	Remark                *string `json:"remark"`
} //	@name	CreatePutawayItemRequest

// ConfirmRequest is POST /putaway/{id}/confirm — the frontend's single
// "Confirm Putaway" button submitting an action for every line at once
// (mirrors PutawayView.vue's simpler one-item drawer, which just sends
// one line). IdempotencyKey is required — see putaway_confirmations'
// migration doc for why.
type ConfirmRequest struct {
	IdempotencyKey string               `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	Items          []ConfirmItemRequest `json:"items" binding:"required,min=1"`
} //	@name	ConfirmPutawayRequest

// ConfirmItemRequest is one line's confirm action. Action "CONFIRMED"
// (matching NewPutawayView.vue's own per-item status vocabulary) moves
// Quantity from the line's source to its destination (or
// DestinationLocationID, if this call overrides it — matching the scan
// bar's ability to update actualBin at confirm time); action "ISSUE"
// (matching PutawayView.vue's "Report Issue" button) records Remark and
// moves nothing.
type ConfirmItemRequest struct {
	ItemID                int64   `json:"item_id,string" binding:"required" example:"1"`
	Action                string  `json:"action" binding:"required,oneof=CONFIRMED ISSUE" example:"CONFIRMED"`
	Quantity              float64 `json:"quantity" example:"100"`
	DestinationLocationID int64   `json:"destination_location_id,string" example:"2"`
	Remark                string  `json:"remark" example:"Actual bin does not match suggested bin"`
} //	@name	ConfirmPutawayItemRequest

// SourceLineResponse is one candidate line for GET
// /putaway/sources/{receiptId} — powers NewPutawayView.vue's
// loadSourceItems(), replacing its mockSourceItems object with a real
// receipt's lines still eligible for putaway.
type SourceLineResponse struct {
	ReceiptItemID      int64   `json:"receipt_item_id,string" example:"1"`
	ProductID          int64   `json:"product_id,string" example:"1"`
	SKU                string  `json:"sku" example:"SKU-0001"`
	ProductName        string  `json:"product_name" example:"Barcode Label Roll"`
	Barcode            *string `json:"barcode"`
	LotNo              string  `json:"lot_no"`
	SourceLocationID   int64   `json:"source_location_id,string" example:"1"`
	SourceLocationCode string  `json:"source_location_code" example:"STAGING-01"`
	UOM                string  `json:"uom" example:"ROLL"`
	ReceivedQty        float64 `json:"received_qty" example:"100"`
	AlreadyPutAwayQty  float64 `json:"already_put_away_qty" example:"0"`
	RemainingQty       float64 `json:"remaining_qty" example:"100"`
} //	@name	PutawaySourceLineResponse

// CancelRequest is POST /putaway/{id}/cancel — Phase 15's addition.
// Remark is optional; when provided it is stored in the dedicated
// cancel_remark column (Phase 18) rather than overwriting the task's own
// Remark field — see Service.Cancel's doc for why this changed. No
// idempotency_key — see internal/picking's own CancelRequest doc
// (Phase 13) for why Cancel's status guard alone is sufficient here.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Source location found empty"`
} //	@name	CancelPutawayRequest

// ListParams narrows GET /putaway to the Putaway screen's actual search
// box plus status/zone/warehouse filters, with pagination. ZoneID
// filters by the line's destination location's zone — the mock's Zone
// filter used hardcoded "A"/"B"/"C" values with nothing behind them;
// this is the real-data equivalent (a real zones(id)).
type ListParams struct {
	Search      string
	Status      Status
	WarehouseID int64
	ZoneID      int64
	Page        int
	Limit       int
}

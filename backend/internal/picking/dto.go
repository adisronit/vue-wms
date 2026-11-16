package picking

import "time"

// ItemResponse is one picking task line, matching PickingView.vue's
// detail drawer "Picking Items" cards (sku, productName, location,
// orderQty, pickedQty).
type ItemResponse struct {
	ID          int64  `json:"id,string" example:"1"`
	ProductID   int64  `json:"product_id,string" example:"1"`
	SKU         string `json:"sku" example:"SKU-0001"`
	ProductName string `json:"product_name" example:"Barcode Label Roll"`
	LotNo       string `json:"lot_no"`
	// ExpiryDate (Phase 33B) is resolved from the matching inventory
	// balance row (product_id, location_id, lot_no), the same tuple
	// identity inventory.Inventory itself uses — not stored on
	// picking_task_items (which has no expiry_date column of its own,
	// matching putaway_task_items' own precedent of deferring to
	// inventory for that). Null when no such balance row exists
	// (e.g. it was fully consumed since this line was created).
	ExpiryDate   *time.Time `json:"expiry_date"`
	LocationID   int64      `json:"location_id,string" example:"1"`
	LocationCode string     `json:"location_code" example:"A-01-01"`
	OrderedQty   float64    `json:"ordered_qty" example:"20"`
	PickedQty    float64    `json:"picked_qty" example:"0"`
	UOM          string     `json:"uom" example:"PCS"`
	Status       ItemStatus `json:"status" example:"PENDING"`
	Remark       *string    `json:"remark"`
	// ShortQty/ShortReason (Phase 33B) are set only while/after this
	// line has been flagged SHORT — both nil otherwise, and both cleared
	// back to nil by a successful Repick. RepickCount is 0 until the
	// first successful Repick call on this line.
	ShortQty    *float64  `json:"short_qty"`
	ShortReason *string   `json:"short_reason" example:"OUT_OF_STOCK"`
	RepickCount int       `json:"repick_count" example:"0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
} //	@name	PickingItemResponse

// Response is a full picking task with its lines — GET /picking/{id}.
// CancelledAt/CancelledBy/CancelledByName/CancelRemark (Phase 18) are
// all nil/null unless the task was actually cancelled — see
// Service.Cancel's own doc.
type Response struct {
	ID                  int64      `json:"id,string" example:"1"`
	PickNo              string     `json:"pick_no" example:"PICK-000001"`
	OrderNo             string     `json:"order_no" example:"SO-20260608-001"`
	Customer            *string    `json:"customer" example:"ABC Customer"`
	CustomerID          *int64     `json:"customer_id,string"`
	ShipToID            *int64     `json:"ship_to_id,string"`
	ShipToName          *string    `json:"ship_to_name"`
	ShipToAddress       *string    `json:"ship_to_address"`
	ShipToProvince      *string    `json:"ship_to_province"`
	ShipToDistrict      *string    `json:"ship_to_district"`
	ShipToPostalCode    *string    `json:"ship_to_postal_code"`
	ShipToContactPerson *string    `json:"ship_to_contact_person"`
	ShipToContactPhone  *string    `json:"ship_to_contact_phone"`
	WarehouseID         int64      `json:"warehouse_id,string" example:"1"`
	WarehouseCode       string     `json:"warehouse_code" example:"WH-01"`
	Status              Status     `json:"status" example:"PENDING"`
	Priority            Priority   `json:"priority" example:"NORMAL"`
	DueDate             *time.Time `json:"due_date"`
	Remark              *string    `json:"remark"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	CompletedAt         *time.Time `json:"completed_at"`
	CancelledAt         *time.Time `json:"cancelled_at"`
	CancelledBy         *int64     `json:"cancelled_by,string"`
	CancelledByName     *string    `json:"cancelled_by_name"`
	CancelRemark        *string    `json:"cancel_remark"`
	// AssignedPickerID/AssignedPickerName/AssignedAt (Phase 33B) are all
	// nil/null until the task has been assigned via POST
	// /picking/{id}/assign — see Service.AssignPicker's own doc.
	AssignedPickerID   *int64         `json:"assigned_picker_id,string"`
	AssignedPickerName *string        `json:"assigned_picker_name"`
	AssignedAt         *time.Time     `json:"assigned_at"`
	Items              []ItemResponse `json:"items"`
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
	OrderNo  string  `json:"order_no" binding:"required" example:"SO-20260608-005"`
	Customer *string `json:"customer" example:"ABC Customer"`
	// CustomerID/ShipTo* are set only by internal/order's own
	// CreatePickingTask — a one-way copy of that order's own already-
	// validated/snapshotted ship-to fields (see Service.createCore's own
	// doc). A direct POST /picking caller may still set them, but
	// nothing in this package re-validates customer_id/ship_to_id
	// ownership itself: that verification happened once, in
	// internal/order, before this request was ever built.
	CustomerID          *int64              `json:"customer_id,string"`
	ShipToID            *int64              `json:"ship_to_id,string"`
	ShipToName          *string             `json:"ship_to_name"`
	ShipToAddress       *string             `json:"ship_to_address"`
	ShipToProvince      *string             `json:"ship_to_province"`
	ShipToDistrict      *string             `json:"ship_to_district"`
	ShipToPostalCode    *string             `json:"ship_to_postal_code"`
	ShipToContactPerson *string             `json:"ship_to_contact_person"`
	ShipToContactPhone  *string             `json:"ship_to_contact_phone"`
	WarehouseID         int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Priority            Priority            `json:"priority" example:"NORMAL"`
	DueDate             *time.Time          `json:"due_date"`
	Remark              *string             `json:"remark"`
	Items               []CreateItemRequest `json:"items"`
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
// "Short Pick" button) records ShortReason (+ optional free-text Remark)
// and removes nothing.
//
// Phase 33B breaking change: ShortReason (one of OUT_OF_STOCK, DAMAGED,
// LOCATION_EMPTY, WRONG_LOT, MISCOUNT, OTHER) is now required for a
// SHORT action instead of Remark — Remark alone used to be the only
// thing Confirm required and is now optional free text layered on top
// of the structured reason. A pre-33B caller sending only Remark on a
// SHORT line now gets a 422 (short_reason is required) rather than
// succeeding — this is a deliberate, documented break, not an oversight:
// a free-text reason can never be grouped/filtered/reported on the way
// a closed code can, and every other module's own confirm action in
// this codebase (QC's dispositions, Relocation's own reasons) already
// uses a closed set rather than prose for exactly this reason.
type ConfirmItemRequest struct {
	ItemID      int64   `json:"item_id,string" binding:"required" example:"1"`
	Action      string  `json:"action" binding:"required,oneof=PICKED SHORT" example:"PICKED"`
	Quantity    float64 `json:"quantity" example:"20"`
	ShortReason string  `json:"short_reason" example:"OUT_OF_STOCK"`
	Remark      string  `json:"remark" example:"Counted twice, shelf was empty"`
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

// AssignRequest is PUT /picking/{id}/assign — claims (or reassigns) the
// task to a picker. PickerID must be a real user id (see Service.
// AssignPicker's own doc) — no role/membership check is performed
// against it, matching CreatedBy/CancelledBy's own existing precedent.
type AssignRequest struct {
	PickerID int64 `json:"picker_id,string" binding:"required" example:"1"`
} //	@name	AssignPickerRequest

// ScanRequest is POST /picking/{id}/items/{itemId}/scan — the barcode-
// scanner-driven location -> product -> lot confirmation step a picker
// runs before Confirm. Each field is matched against the line's own
// assigned location/product/lot — see Service.ScanConfirm's own doc.
type ScanRequest struct {
	LocationCode string `json:"location_code" binding:"required" example:"A-01-01"`
	SKU          string `json:"sku" binding:"required" example:"SKU-0001"`
	LotNo        string `json:"lot_no" example:"LOT-0001"`
} //	@name	PickingScanRequest

// ScanResponse is ScanRequest's result — Matched is false (and Detail
// explains which field(s) disagreed) on a MISMATCH; the row is written
// to picking_scan_log either way (see migrations/0086's own doc).
type ScanResponse struct {
	Matched bool   `json:"matched" example:"true"`
	Detail  string `json:"detail" example:""`
} //	@name	PickingScanResponse

// LotSuggestionResponse is one row of GET /picking/lots/suggest — a
// dedicated local DTO (rather than returning inventory.LotAvailability
// directly) matching this codebase's own convention of every HTTP
// response having its own @name-tagged type, and keeping Picking's own
// Swagger schema independent of internal/inventory's internal shape.
type LotSuggestionResponse struct {
	LocationID   int64      `json:"location_id,string" example:"1"`
	LocationCode string     `json:"location_code" example:"A-01-01"`
	LotNo        string     `json:"lot_no" example:"LOT-0001"`
	ExpiryDate   *time.Time `json:"expiry_date"`
	AvailableQty float64    `json:"available_qty" example:"50"`
	OnHandQty    float64    `json:"on_hand_qty" example:"60"`
} //	@name	PickingLotSuggestionResponse

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

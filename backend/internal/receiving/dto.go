package receiving

import "time"

// ItemResponse is one receiving line, matching NewReceivingView.vue's
// item table columns.
type ItemResponse struct {
	ID          int64      `json:"id,string" example:"1"`
	ProductID   int64      `json:"product_id,string" example:"1"`
	SKU         string     `json:"sku" example:"SKU-0001"`
	ProductName string     `json:"product_name" example:"Barcode Label Roll"`
	Barcode     *string    `json:"barcode"`
	ExpectedQty float64    `json:"expected_qty" example:"100"`
	ReceivedQty float64    `json:"received_qty" example:"0"`
	UOM         string     `json:"uom" example:"ROLL"`
	LotNo       string     `json:"lot_no"`
	ExpiryDate  *time.Time `json:"expiry_date"`
	LocationID  int64      `json:"location_id,string" example:"1"`
	Status      ItemStatus `json:"status" example:"PENDING"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
} //	@name	ReceiptItemResponse

// Response is a full receipt with its lines — GET /receipts/{id}.
// CancelledAt/CancelledBy/CancelledByName/CancelRemark (Phase 18) are
// all nil/null unless the receipt was actually cancelled — see
// Service.Cancel's own doc.
type Response struct {
	ID                int64          `json:"id,string" example:"1"`
	ReceiptNo         string         `json:"receipt_no" example:"RCV-000001"`
	ReceivingType     ReceivingType  `json:"receiving_type" example:"PO"`
	ReferenceNo       *string        `json:"reference_no" example:"PO-20260608-001"`
	Vendor            *string        `json:"vendor" example:"ABC Supplier"`
	WarehouseID       int64          `json:"warehouse_id,string" example:"1"`
	WarehouseCode     string         `json:"warehouse_code" example:"WH-01"`
	StagingLocationID int64          `json:"staging_location_id,string" example:"1"`
	StagingLocation   string         `json:"staging_location" example:"A-01-01"`
	Status            Status         `json:"status" example:"WAITING"`
	ReceivingDate     *time.Time     `json:"receiving_date"`
	Remark            *string        `json:"remark"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	CancelledAt       *time.Time     `json:"cancelled_at"`
	CancelledBy       *int64         `json:"cancelled_by,string"`
	CancelledByName   *string        `json:"cancelled_by_name"`
	CancelRemark      *string        `json:"cancel_remark"`
	Items             []ItemResponse `json:"items"`
} //	@name	ReceiptResponse

// ListResponse is one row of GET /receipts — matches ReceiveView.vue's
// table (Document No./ASN No./Vendor/Warehouse/Expected Date/Items/
// Progress/Status) without the full items array.
type ListResponse struct {
	ID            int64      `json:"id,string" example:"1"`
	ReceiptNo     string     `json:"receipt_no" example:"RCV-000001"`
	ReferenceNo   *string    `json:"reference_no" example:"PO-20260608-001"`
	Vendor        *string    `json:"vendor" example:"ABC Supplier"`
	WarehouseCode string     `json:"warehouse_code" example:"WH-01"`
	ReceivingDate *time.Time `json:"receiving_date"`
	ItemCount     int        `json:"item_count" example:"3"`
	ProgressPct   float64    `json:"progress_pct" example:"45"`
	Status        Status     `json:"status" example:"WAITING"`
} //	@name	ReceiptListResponse

// CreateRequest is POST /receipts. Items is optional — the frontend's
// single "Confirm Receiving"/"Save Draft" click submits the whole form
// (header + lines) in one call, but a receipt can also be created
// header-only and have lines added later via POST .../items (the
// scan-to-add flow on an existing receipt).
type CreateRequest struct {
	ReceivingType     ReceivingType       `json:"receiving_type" binding:"required" example:"PO"`
	ReferenceNo       *string             `json:"reference_no" example:"PO-20260608-005"`
	Vendor            *string             `json:"vendor" example:"ABC Supplier"`
	WarehouseID       int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	StagingLocationID int64               `json:"staging_location_id,string" binding:"required" example:"1"`
	Status            Status              `json:"status" example:"DRAFT"`
	ReceivingDate     *time.Time          `json:"receiving_date"`
	Remark            *string             `json:"remark"`
	Items             []CreateItemRequest `json:"items"`
} //	@name	CreateReceiptRequest

// CreateItemRequest is one line within CreateRequest.Items or the body
// of POST /receipts/{id}/items.
type CreateItemRequest struct {
	ProductID   int64      `json:"product_id,string" binding:"required" example:"1"`
	ExpectedQty float64    `json:"expected_qty" example:"100"`
	UOM         string     `json:"uom" example:"ROLL"`
	LotNo       string     `json:"lot_no" example:"LOT-0001"`
	ExpiryDate  *time.Time `json:"expiry_date"`
	LocationID  int64      `json:"location_id,string" example:"1"`
} //	@name	CreateReceiptItemRequest

// UpdateRequest is PUT /receipts/{id} — header fields only, and only
// while the receipt is still editable (see Status.CanEdit).
type UpdateRequest struct {
	ReferenceNo       *string    `json:"reference_no"`
	Vendor            *string    `json:"vendor"`
	StagingLocationID int64      `json:"staging_location_id,string" binding:"required" example:"1"`
	ReceivingDate     *time.Time `json:"receiving_date"`
	Remark            *string    `json:"remark"`
} //	@name	UpdateReceiptRequest

// UpdateItemRequest is PUT /receipts/{id}/items/{itemId} — only while
// the line hasn't started receiving (status PENDING).
type UpdateItemRequest struct {
	ExpectedQty float64    `json:"expected_qty" example:"100"`
	UOM         string     `json:"uom" example:"ROLL"`
	LotNo       string     `json:"lot_no" example:"LOT-0001"`
	ExpiryDate  *time.Time `json:"expiry_date"`
	LocationID  int64      `json:"location_id,string" binding:"required" example:"1"`
} //	@name	UpdateReceiptItemRequest

// ConfirmRequest is POST /receipts/{id}/confirm — the frontend's single
// "Confirm Receiving" button submitting the current receiveQty/lot/
// expiry/location for every line at once. IdempotencyKey is required —
// see receipt_confirmations' migration doc for why.
type ConfirmRequest struct {
	IdempotencyKey string               `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	Items          []ConfirmItemRequest `json:"items" binding:"required,min=1"`
} //	@name	ConfirmReceiptRequest

// ConfirmItemRequest is one line's confirm delta. Quantity is how much
// to add THIS call, not the line's new total.
type ConfirmItemRequest struct {
	ItemID     int64      `json:"item_id,string" binding:"required" example:"1"`
	Quantity   float64    `json:"quantity" binding:"required" example:"100"`
	LotNo      string     `json:"lot_no" example:"LOT-0001"`
	ExpiryDate *time.Time `json:"expiry_date"`
	LocationID int64      `json:"location_id,string" example:"1"`
} //	@name	ConfirmReceiptItemRequest

// CancelRequest is POST /receipts/{id}/cancel — Phase 15's addition.
// Remark is optional; when provided it is stored in the dedicated
// cancel_remark column (Phase 18) rather than overwriting the receipt's
// own Remark field — see Service.Cancel's doc for why this changed. No
// idempotency_key: Cancel's only mutation is the status/audit-column
// write itself, which the CanCancel status guard alone makes safe under
// a duplicate/retry — the same reasoning Phase 13 documented for
// internal/picking's own Cancel.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Vendor cancelled the order"`
} //	@name	CancelReceiptRequest

// ListParams narrows GET /receipts to the Goods Receiving screen's
// actual search box plus status/date filters, with pagination.
type ListParams struct {
	Search      string
	Status      Status
	WarehouseID int64
	Date        *time.Time
	Page        int
	Limit       int
}

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
	// Splits (Phase 31A) is the immutable, per-confirm-call history of
	// every lot/expiry/pallet batch actually posted for this line — see
	// SplitResponse's own doc. Empty until this line's first Confirm
	// call under Phase 31A.
	Splits []SplitResponse `json:"splits"`
} //	@name	ReceiptItemResponse

// SplitResponse is one receipt_item_splits row — the Review step's
// lot/expiry/pallet breakdown for a line that was received across more
// than one batch.
type SplitResponse struct {
	ID              int64      `json:"id,string" example:"1"`
	LotNo           string     `json:"lot_no" example:"LOT-0001"`
	MfgDate         *time.Time `json:"mfg_date"`
	ExpiryDate      *time.Time `json:"expiry_date"`
	Quantity        float64    `json:"quantity" example:"50"`
	UOM             string     `json:"uom" example:"ROLL"`
	PalletID        *string    `json:"pallet_id"`
	CartonCount     *int       `json:"carton_count"`
	LocationID      int64      `json:"location_id,string" example:"1"`
	LocationCode    string     `json:"location_code" example:"A-01-01"`
	TemperatureC    *float64   `json:"temperature_c"`
	ConditionNote   *string    `json:"condition_note"`
	CountryOfOrigin *string    `json:"country_of_origin"`
	CreatedAt       time.Time  `json:"created_at"`
} //	@name	ReceiptItemSplitResponse

// Response is a full receipt with its lines — GET /receipts/{id}.
// CancelledAt/CancelledBy/CancelledByName/CancelRemark (Phase 18) are
// all nil/null unless the receipt was actually cancelled — see
// Service.Cancel's own doc.
type Response struct {
	ID            int64         `json:"id,string" example:"1"`
	ReceiptNo     string        `json:"receipt_no" example:"RCV-000001"`
	ReceivingType ReceivingType `json:"receiving_type" example:"PO"`
	ReferenceNo   *string       `json:"reference_no" example:"PO-20260608-001"`
	Vendor        *string       `json:"vendor" example:"ABC Supplier"`
	// SupplierID/SupplierName (Phase 31A): SupplierName is whichever of
	// the live supplier's company_name or (if the supplier was since
	// deleted/renamed) the stored SupplierNameSnapshot resolves — see
	// FindReceiptDetail's own doc.
	SupplierID         *int64         `json:"supplier_id,string" example:"null"`
	SupplierName       *string        `json:"supplier_name"`
	InvoiceNo          *string        `json:"invoice_no"`
	DeliveryNoteNo     *string        `json:"delivery_note_no"`
	ContainerNo        *string        `json:"container_no"`
	SealNo             *string        `json:"seal_no"`
	TransportCompany   *string        `json:"transport_company"`
	DriverName         *string        `json:"driver_name"`
	VehicleRegNo       *string        `json:"vehicle_reg_no"`
	DockDoor           *string        `json:"dock_door"`
	ArrivalTime        *time.Time     `json:"arrival_time"`
	UnloadingStartTime *time.Time     `json:"unloading_start_time"`
	UnloadingEndTime   *time.Time     `json:"unloading_end_time"`
	Currency           *string        `json:"currency"`
	WarehouseID        int64          `json:"warehouse_id,string" example:"1"`
	WarehouseCode      string         `json:"warehouse_code" example:"WH-01"`
	StagingLocationID  int64          `json:"staging_location_id,string" example:"1"`
	StagingLocation    string         `json:"staging_location" example:"A-01-01"`
	Status             Status         `json:"status" example:"WAITING"`
	ReceivingDate      *time.Time     `json:"receiving_date"`
	Remark             *string        `json:"remark"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	CancelledAt        *time.Time     `json:"cancelled_at"`
	CancelledBy        *int64         `json:"cancelled_by,string"`
	CancelledByName    *string        `json:"cancelled_by_name"`
	CancelRemark       *string        `json:"cancel_remark"`
	Items              []ItemResponse `json:"items"`
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
	ReceivingType ReceivingType `json:"receiving_type" binding:"required" example:"PO"`
	ReferenceNo   *string       `json:"reference_no" example:"PO-20260608-005"`
	Vendor        *string       `json:"vendor" example:"ABC Supplier"`
	// SupplierID (Phase 31A) optionally links a real internal/supplier
	// row — see Response.SupplierName's own doc for the snapshot this
	// produces. Independent of Vendor: both may be set, only one, or
	// neither.
	SupplierID         *int64              `json:"supplier_id,string" example:"null"`
	InvoiceNo          *string             `json:"invoice_no"`
	DeliveryNoteNo     *string             `json:"delivery_note_no"`
	ContainerNo        *string             `json:"container_no"`
	SealNo             *string             `json:"seal_no"`
	TransportCompany   *string             `json:"transport_company"`
	DriverName         *string             `json:"driver_name"`
	VehicleRegNo       *string             `json:"vehicle_reg_no"`
	DockDoor           *string             `json:"dock_door"`
	ArrivalTime        *time.Time          `json:"arrival_time"`
	UnloadingStartTime *time.Time          `json:"unloading_start_time"`
	UnloadingEndTime   *time.Time          `json:"unloading_end_time"`
	Currency           *string             `json:"currency"`
	WarehouseID        int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	StagingLocationID  int64               `json:"staging_location_id,string" binding:"required" example:"1"`
	Status             Status              `json:"status" example:"DRAFT"`
	ReceivingDate      *time.Time          `json:"receiving_date"`
	Remark             *string             `json:"remark"`
	Items              []CreateItemRequest `json:"items"`
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
	ReferenceNo        *string    `json:"reference_no"`
	Vendor             *string    `json:"vendor"`
	SupplierID         *int64     `json:"supplier_id,string" example:"null"`
	InvoiceNo          *string    `json:"invoice_no"`
	DeliveryNoteNo     *string    `json:"delivery_note_no"`
	ContainerNo        *string    `json:"container_no"`
	SealNo             *string    `json:"seal_no"`
	TransportCompany   *string    `json:"transport_company"`
	DriverName         *string    `json:"driver_name"`
	VehicleRegNo       *string    `json:"vehicle_reg_no"`
	DockDoor           *string    `json:"dock_door"`
	ArrivalTime        *time.Time `json:"arrival_time"`
	UnloadingStartTime *time.Time `json:"unloading_start_time"`
	UnloadingEndTime   *time.Time `json:"unloading_end_time"`
	Currency           *string    `json:"currency"`
	StagingLocationID  int64      `json:"staging_location_id,string" binding:"required" example:"1"`
	ReceivingDate      *time.Time `json:"receiving_date"`
	Remark             *string    `json:"remark"`
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
	// Splits (Phase 31A) optionally breaks this call's Quantity into
	// more than one lot/expiry/pallet batch — e.g. the same SKU/line
	// arriving as two different lot numbers in one delivery. Omitted:
	// LotNo/ExpiryDate/LocationID/Quantity above are treated as a
	// single implicit split covering the whole call, exactly preserving
	// pre-Phase-31A request bodies. Provided: every split's Quantity
	// must sum to exactly this call's Quantity, and LotNo/ExpiryDate/
	// LocationID above are ignored in favor of each split's own values.
	Splits []ConfirmSplitRequest `json:"splits"`
} //	@name	ConfirmReceiptItemRequest

// ConfirmSplitRequest is one lot/expiry/pallet batch within a
// ConfirmItemRequest's Splits.
type ConfirmSplitRequest struct {
	Quantity        float64    `json:"quantity" binding:"required" example:"50"`
	LotNo           string     `json:"lot_no" example:"LOT-0001"`
	MfgDate         *time.Time `json:"mfg_date"`
	ExpiryDate      *time.Time `json:"expiry_date"`
	LocationID      int64      `json:"location_id,string" example:"1"`
	PalletID        string     `json:"pallet_id" example:"PLT-0001"`
	CartonCount     int        `json:"carton_count" example:"10"`
	TemperatureC    *float64   `json:"temperature_c"`
	ConditionNote   string     `json:"condition_note"`
	CountryOfOrigin string     `json:"country_of_origin"`
} //	@name	ConfirmReceiptItemSplitRequest

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

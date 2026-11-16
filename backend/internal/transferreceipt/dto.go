package transferreceipt

import "time"

// ItemResponse is one transfer receipt line.
type ItemResponse struct {
	ID                int64      `json:"id,string" example:"1"`
	ShipmentItemID    int64      `json:"shipment_item_id,string" example:"1"`
	ProductID         int64      `json:"product_id,string" example:"1"`
	SKU               string     `json:"sku" example:"SKU-0001"`
	ProductName       string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo             string     `json:"lot_no"`
	UOM               string     `json:"uom" example:"PCS"`
	DispatchedQty     float64    `json:"dispatched_qty" example:"100"`
	ReceivedQty       float64    `json:"received_qty" example:"0"`
	LocationID        *int64     `json:"location_id,string"`
	LocationCode      *string    `json:"location_code"`
	DiscrepancyQty    *float64   `json:"discrepancy_qty"`
	DiscrepancyReason *string    `json:"discrepancy_reason"`
	Status            ItemStatus `json:"status" example:"PENDING"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
} //	@name	TransferReceiptItemResponse

// Response is a full transfer receipt with its lines — GET
// /transfer-receipts/{id}.
type Response struct {
	ID                       int64          `json:"id,string" example:"1"`
	ReceiptNo                string         `json:"receipt_no" example:"TRC-000001"`
	ShipmentID               int64          `json:"shipment_id,string" example:"1"`
	ShipmentNo               string         `json:"shipment_no" example:"SHP-000001"`
	OrderID                  int64          `json:"order_id,string" example:"1"`
	OrderNo                  string         `json:"order_no" example:"SO-20260608-001"`
	TransferReference        *string        `json:"transfer_reference"`
	DestinationWarehouseID   int64          `json:"destination_warehouse_id,string" example:"1"`
	DestinationWarehouseCode string         `json:"destination_warehouse_code" example:"WH-02"`
	Status                   Status         `json:"status" example:"DRAFT"`
	ReceivedBy               *int64         `json:"received_by,string"`
	ReceivedByName           *string        `json:"received_by_name"`
	ReceivedAt               *time.Time     `json:"received_at"`
	Remark                   *string        `json:"remark"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	CancelledAt              *time.Time     `json:"cancelled_at"`
	CancelledBy              *int64         `json:"cancelled_by,string"`
	CancelledByName          *string        `json:"cancelled_by_name"`
	CancelRemark             *string        `json:"cancel_remark"`
	Items                    []ItemResponse `json:"items"`
} //	@name	TransferReceiptResponse

// ListResponse is one row of GET /transfer-receipts — header-level,
// matching the Transfer Receiving worklist's table.
type ListResponse struct {
	ID                       int64     `json:"id,string" example:"1"`
	ReceiptNo                string    `json:"receipt_no" example:"TRC-000001"`
	ShipmentID               int64     `json:"shipment_id,string" example:"1"`
	ShipmentNo               string    `json:"shipment_no" example:"SHP-000001"`
	OrderNo                  string    `json:"order_no" example:"SO-20260608-001"`
	TransferReference        *string   `json:"transfer_reference"`
	DestinationWarehouseID   int64     `json:"destination_warehouse_id,string" example:"1"`
	DestinationWarehouseCode string    `json:"destination_warehouse_code" example:"WH-02"`
	Status                   Status    `json:"status" example:"DRAFT"`
	CreatedAt                time.Time `json:"created_at"`
} //	@name	TransferReceiptListResponse

// EligibleShipmentResponse is one candidate shipment for POST
// /transfer-receipts — a DISPATCHED (or later) INTERNAL_TRANSFER
// shipment, scoped to destination_warehouse_id, that has no transfer
// receipt yet. Powers the Transfer Receiving screen's "+ New Receipt"
// shipment picker.
type EligibleShipmentResponse struct {
	ShipmentID             int64   `json:"shipment_id,string" example:"1"`
	ShipmentNo             string  `json:"shipment_no" example:"SHP-000001"`
	OrderID                int64   `json:"order_id,string" example:"1"`
	OrderNo                string  `json:"order_no" example:"SO-20260608-001"`
	TransferReference      *string `json:"transfer_reference"`
	SourceWarehouseID      int64   `json:"source_warehouse_id,string" example:"1"`
	SourceWarehouseCode    string  `json:"source_warehouse_code" example:"WH-01"`
	DestinationWarehouseID int64   `json:"destination_warehouse_id,string" example:"2"`
	ShipmentStatus         string  `json:"shipment_status" example:"DISPATCHED"`
	ItemCount              int     `json:"item_count" example:"2"`
} //	@name	EligibleTransferShipmentResponse

// CreateRequest is POST /transfer-receipts — creates a DRAFT receipt
// from an already-dispatched transfer shipment. Items are never
// accepted from the client — they are always copied server-side from
// the shipment's own lines (see Service.Create's own doc).
type CreateRequest struct {
	ShipmentID int64   `json:"shipment_id,string" binding:"required" example:"1"`
	Remark     *string `json:"remark"`
} //	@name	CreateTransferReceiptRequest

// ReceiveItemRequest is POST
// /transfer-receipts/{id}/items/{itemId}/receive. IdempotencyKey is
// required — a fresh UUID per call, matching every other module's own
// stock-mutating action. LocationID is the destination STAGING
// location this call's quantity is being received into — required
// whenever Quantity > 0, validated to belong to the receipt's own
// destination_warehouse_id and to a RECEIVING-type zone.
//
// CloseShort/DiscrepancyReason together record a short or damaged
// receipt: set CloseShort=true with a non-empty DiscrepancyReason to
// permanently stop expecting more of this line even though
// received_qty (after this call's own Quantity, which may be zero) is
// still below dispatched_qty. This is a deliberate, explicit action on
// the same call that may also receive a final partial quantity — not
// inferred automatically when Confirm runs — so the receiver always
// states why a shortfall is being accepted before it is recorded.
type ReceiveItemRequest struct {
	IdempotencyKey    string  `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	LocationID        int64   `json:"location_id,string" example:"1"`
	Quantity          float64 `json:"quantity" example:"40"`
	CloseShort        bool    `json:"close_short" example:"false"`
	DiscrepancyReason string  `json:"discrepancy_reason" example:"Two cartons damaged in transit"`
} //	@name	ReceiveTransferReceiptItemRequest

// ConfirmRequest is POST /transfer-receipts/{id}/confirm — transitions
// DRAFT -> COMPLETED once every line is either fully received or has a
// recorded discrepancy. IdempotencyKey is required.
type ConfirmRequest struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
} //	@name	ConfirmTransferReceiptRequest

// CancelRequest is POST /transfer-receipts/{id}/cancel — DRAFT only.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Shipment recalled before arrival"`
} //	@name	CancelTransferReceiptRequest

// ListParams narrows GET /transfer-receipts to the worklist's own
// search/status/destination-warehouse filters, with pagination.
// HasDiscrepancy (Phase 33E.3) is a list-level reconciliation view: when
// true, only receipts with at least one line carrying a recorded
// discrepancy (discrepancy_qty > 0) are returned -- the "needs follow-
// up" worklist. False (the zero value, "?has_discrepancy" omitted or
// "false") applies no such filter, matching every other optional filter
// in this struct's own "zero value means no filter" convention
// (Status == "", DestinationWarehouseID == 0).
type ListParams struct {
	Search                 string
	Status                 Status
	DestinationWarehouseID int64
	HasDiscrepancy         bool
	Page                   int
	Limit                  int
}

// ReconciliationLineResponse is one line of GET
// /transfer-receipts/{id}/reconciliation -- pure computation over the
// already-stored dispatched_qty/received_qty/discrepancy_qty columns
// (ItemResponse's own fields), never a new stored value.
// FullyReconciled is true iff received_qty equals dispatched_qty AND no
// discrepancy was recorded for this line -- the per-line reconciliation
// verdict an operator's worklist needs at a glance.
type ReconciliationLineResponse struct {
	ItemID            int64      `json:"item_id,string" example:"1"`
	ProductID         int64      `json:"product_id,string" example:"1"`
	SKU               string     `json:"sku" example:"SKU-0001"`
	ProductName       string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo             string     `json:"lot_no"`
	UOM               string     `json:"uom" example:"PCS"`
	DispatchedQty     float64    `json:"dispatched_qty" example:"100"`
	ReceivedQty       float64    `json:"received_qty" example:"100"`
	DiscrepancyQty    *float64   `json:"discrepancy_qty"`
	DiscrepancyReason *string    `json:"discrepancy_reason"`
	Status            ItemStatus `json:"status" example:"COMPLETED"`
	FullyReconciled   bool       `json:"fully_reconciled" example:"true"`
} //	@name	TransferReceiptReconciliationLineResponse

// ReconciliationResponse is GET /transfer-receipts/{id}/reconciliation --
// a read-only summary over an already-created/received/confirmed
// receipt's own stored lines; no new table, no mutation.
// ReconciliationStatus is PENDING while the receipt itself has not yet
// been confirmed (receiving is still in progress -- any discrepancy
// recorded so far is provisional), RECONCILED once COMPLETED with zero
// total discrepancy, or DISCREPANCY once COMPLETED with a non-zero
// total discrepancy. FullyReconciled mirrors that at the boolean level
// every line's own FullyReconciled already uses (true iff every line is
// FullyReconciled).
type ReconciliationResponse struct {
	ReceiptID            int64                         `json:"receipt_id,string" example:"1"`
	ReceiptNo            string                        `json:"receipt_no" example:"TRC-000001"`
	Status               Status                        `json:"status" example:"COMPLETED"`
	TotalDispatchedQty   float64                       `json:"total_dispatched_qty" example:"100"`
	TotalReceivedQty     float64                       `json:"total_received_qty" example:"90"`
	TotalDiscrepancyQty  float64                       `json:"total_discrepancy_qty" example:"10"`
	FullyReconciled      bool                          `json:"fully_reconciled" example:"false"`
	ReconciliationStatus string                        `json:"reconciliation_status" example:"DISCREPANCY"`
	Lines                []ReconciliationLineResponse `json:"lines"`
} //	@name	TransferReceiptReconciliationResponse

// EligibleShipmentParams narrows GET
// /transfer-receipts/eligible-shipments.
type EligibleShipmentParams struct {
	DestinationWarehouseID int64
	Search                 string
}

package qualitycheck

import "time"

// EligibleSplitResponse is one receipt_item_splits line (internal/
// receiving) eligible to be inspected — its product requires QC and it
// still carries inventory.HoldQuantity > 0. Returned by GET
// /qc/eligible-splits?receipt_id=... so NewQualityCheckView.vue's
// "Receiving No." picker can show what's actually waiting for
// inspection on that receipt, instead of a free-text SKU field.
type EligibleSplitResponse struct {
	ReceiptItemSplitID int64      `json:"receipt_item_split_id,string" example:"1"`
	ProductID          int64      `json:"product_id,string" example:"1"`
	SKU                string     `json:"sku" example:"SKU-0001"`
	ProductName        string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo              string     `json:"lot_no" example:"LOT-0001"`
	ExpiryDate         *time.Time `json:"expiry_date"`
	LocationID         int64      `json:"location_id,string" example:"1"`
	LocationCode       string     `json:"location_code" example:"A-01-01"`
	HeldQty            float64    `json:"held_qty" example:"100"`
} //	@name	QCEligibleSplitResponse

// DispositionResponse is one qc_dispositions row.
type DispositionResponse struct {
	ID                int64       `json:"id,string" example:"1"`
	Disposition       Disposition `json:"disposition" example:"PASS"`
	Quantity          float64     `json:"quantity" example:"95"`
	DefectCode        *string     `json:"defect_code"`
	EvidenceReference *string     `json:"evidence_reference"`
	Remark            *string     `json:"remark"`
	DecidedBy         *int64      `json:"decided_by,string"`
	DecidedByName     *string     `json:"decided_by_name"`
	CreatedAt         time.Time   `json:"created_at"`
} //	@name	QCDispositionResponse

// ItemResponse is one inspection line with its full disposition
// history, matching NewQualityCheckView.vue's item table columns plus
// the checklist fields and the decision list the redesigned screen
// needs. RemainingHeldQty is read live from inventory.HoldQuantity at
// response time — see Service.itemStatus's own doc for why that, not
// HeldQtySnapshot, is the authoritative "how much is left to inspect."
type ItemResponse struct {
	ID                 int64                 `json:"id,string" example:"1"`
	ReceiptItemSplitID int64                 `json:"receipt_item_split_id,string" example:"1"`
	ProductID          int64                 `json:"product_id,string" example:"1"`
	SKU                string                `json:"sku" example:"SKU-0001"`
	ProductName        string                `json:"product_name" example:"Barcode Label Roll"`
	LotNo              string                `json:"lot_no" example:"LOT-0001"`
	ExpiryDate         *time.Time            `json:"expiry_date"`
	LocationID         int64                 `json:"location_id,string" example:"1"`
	LocationCode       string                `json:"location_code" example:"A-01-01"`
	HeldQtySnapshot    float64               `json:"held_qty_snapshot" example:"100"`
	RemainingHeldQty   float64               `json:"remaining_held_qty" example:"3"`
	SampleQty          float64               `json:"sample_qty" example:"10"`
	TemperatureC       *float64              `json:"temperature_c"`
	PackagingOK        *bool                 `json:"packaging_ok"`
	LabelOK            *bool                 `json:"label_ok"`
	Status             ItemStatus            `json:"status" example:"IN_PROGRESS"`
	Dispositions       []DispositionResponse `json:"dispositions"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
} //	@name	QCInspectionItemResponse

// Response is a full inspection with its lines — GET /qc/inspections/{id}.
type Response struct {
	ID            int64          `json:"id,string" example:"1"`
	InspectionNo  string         `json:"inspection_no" example:"QC-000001"`
	ReceiptID     int64          `json:"receipt_id,string" example:"1"`
	ReceiptNo     string         `json:"receipt_no" example:"RCV-000001"`
	WarehouseID   int64          `json:"warehouse_id,string" example:"1"`
	WarehouseCode string         `json:"warehouse_code" example:"WH-01"`
	InspectorID   *int64         `json:"inspector_id,string"`
	InspectorName *string        `json:"inspector_name"`
	QCType        QCType         `json:"qc_type" example:"NORMAL"`
	Status        Status         `json:"status" example:"PENDING"`
	Remark        *string        `json:"remark"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Items         []ItemResponse `json:"items"`
} //	@name	QCInspectionResponse

// ListResponse is one row of GET /qc/inspections — the Quality Check
// screen's table, without the full items array.
type ListResponse struct {
	ID            int64     `json:"id,string" example:"1"`
	InspectionNo  string    `json:"inspection_no" example:"QC-000001"`
	ReceiptNo     string    `json:"receipt_no" example:"RCV-000001"`
	WarehouseCode string    `json:"warehouse_code" example:"WH-01"`
	QCType        QCType    `json:"qc_type" example:"NORMAL"`
	Status        Status    `json:"status" example:"PENDING"`
	ItemCount     int       `json:"item_count" example:"3"`
	CreatedAt     time.Time `json:"created_at"`
} //	@name	QCInspectionListResponse

// CreateRequest is POST /qc/inspections. Items are auto-populated from
// every eligible receipt_item_splits line on ReceiptID (see
// EligibleSplitResponse's own doc) — the caller does not hand-pick
// lines, mirroring how the mock screen's "Receiving No." selector
// always implied "inspect everything on this receipt that needs it."
type CreateRequest struct {
	ReceiptID   int64   `json:"receipt_id,string" binding:"required" example:"1"`
	WarehouseID int64   `json:"warehouse_id,string" binding:"required" example:"1"`
	InspectorID *int64  `json:"inspector_id,string"`
	QCType      QCType  `json:"qc_type" example:"NORMAL"`
	Remark      *string `json:"remark"`
} //	@name	CreateQCInspectionRequest

// UpdateItemRequest is PUT /qc/inspections/{id}/items/{itemId} — records
// the checklist/sample fields before a disposition is confirmed. Never
// touches inventory.
type UpdateItemRequest struct {
	SampleQty    float64  `json:"sample_qty" example:"10"`
	TemperatureC *float64 `json:"temperature_c"`
	PackagingOK  *bool    `json:"packaging_ok"`
	LabelOK      *bool    `json:"label_ok"`
} //	@name	UpdateQCInspectionItemRequest

// ConfirmRequest is POST /qc/inspections/{id}/confirm — the batch
// disposition-recording action, mirroring internal/receiving's own
// Confirm shape exactly (idempotency_key + a list of per-line deltas
// applied atomically in one transaction). A single call may record
// PASS, HOLD, and REJECT entries for different lines together — the
// mandatory "receive 100 -> PASS 95 / HOLD 3 / REJECT 2" scenario is
// three ConfirmDispositionRequest entries against the SAME inspection
// item in one call.
type ConfirmRequest struct {
	IdempotencyKey string                      `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	Dispositions   []ConfirmDispositionRequest `json:"dispositions" binding:"required,min=1"`
} //	@name	ConfirmQCInspectionRequest

// ConfirmDispositionRequest is one disposition entry within
// ConfirmRequest.Dispositions.
type ConfirmDispositionRequest struct {
	ItemID            int64       `json:"item_id,string" binding:"required" example:"1"`
	Disposition       Disposition `json:"disposition" binding:"required" example:"PASS"`
	Quantity          float64     `json:"quantity" binding:"required" example:"95"`
	DefectCode        string      `json:"defect_code" example:"DAMAGED"`
	EvidenceReference string      `json:"evidence_reference"`
	Remark            string      `json:"remark"`
} //	@name	ConfirmQCDispositionRequest

// ListParams narrows GET /qc/inspections to the Quality Check screen's
// search box plus status/warehouse filters, with pagination.
// ReceiptID (Phase 31D) is not exposed on the screen's own filter bar —
// it exists so internal/inbounddoc can find every inspection for one
// receipt when composing the Inbound Completion Report, the same
// zero-means-unfiltered convention WarehouseID already uses.
type ListParams struct {
	Search      string
	Status      Status
	WarehouseID int64
	ReceiptID   int64
	Page        int
	Limit       int
}

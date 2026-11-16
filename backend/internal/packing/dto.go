package packing

import "time"

// ItemResponse is one packing job line.
type ItemResponse struct {
	ID              int64      `json:"id,string" example:"1"`
	ProductID       int64      `json:"product_id,string" example:"1"`
	SKU             string     `json:"sku" example:"SKU-0001"`
	ProductName     string     `json:"product_name" example:"Barcode Label Roll"`
	LotNo           string     `json:"lot_no"`
	OrderedQty      float64    `json:"ordered_qty" example:"20"`
	PickedQty       float64    `json:"picked_qty" example:"20"`
	PackedQty       float64    `json:"packed_qty" example:"0"`
	UOM             string     `json:"uom" example:"PCS"`
	Status          ItemStatus `json:"status" example:"PENDING"`
	ExceptionFlag   bool       `json:"exception_flag" example:"false"`
	ExceptionReason *string    `json:"exception_reason" example:"DAMAGED"`
	ExceptionQty    *float64   `json:"exception_qty"`
	Remark          *string    `json:"remark"`
	// CartonLPNs is the distinct set of carton LPNs this line has been
	// packed into so far (empty until the first successful pack action)
	// — resolved live from carton_contents, never stored on the line
	// itself.
	CartonLPNs []string  `json:"carton_lpns"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
} //	@name	PackingItemResponse

// ContentResponse is one carton_contents row, joined for display — used
// by CartonResponse.Contents.
type ContentResponse struct {
	ID               int64   `json:"id,string" example:"1"`
	PackingJobItemID int64   `json:"packing_job_item_id,string" example:"1"`
	SKU              string  `json:"sku" example:"SKU-0001"`
	ProductName      string  `json:"product_name" example:"Barcode Label Roll"`
	LotNo            string  `json:"lot_no"`
	PackedQty        float64 `json:"packed_qty" example:"10"`
	UOM              string  `json:"uom" example:"PCS"`
} //	@name	PackingCartonContentResponse

// CartonResponse is one carton with its contents.
type CartonResponse struct {
	ID            int64             `json:"id,string" example:"1"`
	PackingJobID  int64             `json:"packing_job_id,string" example:"1"`
	LPN           string            `json:"lpn" example:"CTN-000001"`
	PackagingType PackagingType     `json:"packaging_type" example:"BOX"`
	GrossWeight   *float64          `json:"gross_weight" example:"5.2"`
	NetWeight     *float64          `json:"net_weight" example:"4.8"`
	WeightUnit    WeightUnit        `json:"weight_unit" example:"KG"`
	CartonLength  *float64          `json:"carton_length" example:"30"`
	CartonWidth   *float64          `json:"carton_width" example:"20"`
	CartonHeight  *float64          `json:"carton_height" example:"15"`
	DimensionUnit DimensionUnit     `json:"dimension_unit" example:"CM"`
	Status        CartonStatus      `json:"status" example:"OPEN"`
	Remark        *string           `json:"remark"`
	Contents      []ContentResponse `json:"contents"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
} //	@name	PackingCartonResponse

// Response is a full packing job with its lines and cartons — GET
// /packing-jobs/{id}. OrderID is nil for the ordinary (non-batch) flow;
// set to a specific order when this job is one of a batch-sourced
// task's own per-order packing jobs (Phase 33C.2).
type Response struct {
	ID                  int64            `json:"id,string" example:"1"`
	JobNo               string           `json:"job_no" example:"PACK-000001"`
	PickingTaskID       int64            `json:"picking_task_id,string" example:"1"`
	PickNo              string           `json:"pick_no" example:"PICK-000001"`
	OrderID             *int64           `json:"order_id,string"`
	OrderNo             string           `json:"order_no" example:"SO-20260608-001"`
	Customer            *string          `json:"customer" example:"ABC Customer"`
	CustomerID          *int64           `json:"customer_id,string"`
	ShipToID            *int64           `json:"ship_to_id,string"`
	ShipToName          *string          `json:"ship_to_name"`
	ShipToAddress       *string          `json:"ship_to_address"`
	ShipToProvince      *string          `json:"ship_to_province"`
	ShipToDistrict      *string          `json:"ship_to_district"`
	ShipToPostalCode    *string          `json:"ship_to_postal_code"`
	ShipToContactPerson *string          `json:"ship_to_contact_person"`
	ShipToContactPhone  *string          `json:"ship_to_contact_phone"`
	WarehouseID         int64            `json:"warehouse_id,string" example:"1"`
	WarehouseCode       string           `json:"warehouse_code" example:"WH-01"`
	Status              Status           `json:"status" example:"DRAFT"`
	AssignedPackerID    *int64           `json:"assigned_packer_id,string"`
	AssignedPackerName  *string          `json:"assigned_packer_name"`
	AssignedAt          *time.Time       `json:"assigned_at"`
	CheckerID           *int64           `json:"checker_id,string"`
	CheckerName         *string          `json:"checker_name"`
	CheckerConfirmedAt  *time.Time       `json:"checker_confirmed_at"`
	Remark              *string          `json:"remark"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
	CompletedAt         *time.Time       `json:"completed_at"`
	CancelledAt         *time.Time       `json:"cancelled_at"`
	CancelledBy         *int64           `json:"cancelled_by,string"`
	CancelledByName     *string          `json:"cancelled_by_name"`
	CancelRemark        *string          `json:"cancel_remark"`
	Items               []ItemResponse   `json:"items"`
	Cartons             []CartonResponse `json:"cartons"`
} //	@name	PackingJobResponse

// ListResponse is one row of GET /packing-jobs — job-level, mirroring
// picking.ListResponse's own task-level shape.
type ListResponse struct {
	ID                 int64     `json:"id,string" example:"1"`
	JobNo              string    `json:"job_no" example:"PACK-000001"`
	PickingTaskID      int64     `json:"picking_task_id,string" example:"1"`
	OrderID            *int64    `json:"order_id,string"`
	OrderNo            string    `json:"order_no" example:"SO-20260608-001"`
	Customer           *string   `json:"customer" example:"ABC Customer"`
	WarehouseCode      string    `json:"warehouse_code" example:"WH-01"`
	Status             Status    `json:"status" example:"DRAFT"`
	ItemCount          int       `json:"item_count" example:"3"`
	ProgressPct        float64   `json:"progress_pct" example:"45"`
	AssignedPackerName *string   `json:"assigned_packer_name"`
	CreatedAt          time.Time `json:"created_at"`
} //	@name	PackingListResponse

// PendingOrderResponse (Phase 33C.2) is one of a batch-sourced source
// task's own included orders that does not yet have its own packing
// job — the picker for the UI's "create a job for which order" choice.
type PendingOrderResponse struct {
	OrderID  int64   `json:"order_id,string" example:"1"`
	OrderNo  string  `json:"order_no" example:"ORD-000001"`
	Customer *string `json:"customer" example:"ABC Customer"`
} //	@name	PackingPendingOrderResponse

// SourceResponse is one row of GET /packing-jobs/sources — a completed
// or short-picked picking task that still has at least one packable
// packing job left to create, mirroring shipping.SourceResponse's own
// shape exactly for the ordinary (non-batch) case.
//
// IsBatch/BatchID/PendingOrders (Phase 33C.2) are populated only when
// this task is batch-sourced (internal/pickingbatch): a batch-sourced
// task stays in Sources as long as ANY of its own included orders still
// lacks a job (unlike the ordinary case's strict "no job exists yet at
// all" rule — multiple jobs, one per order, are expected here), and
// PendingOrders lists exactly which orders those are, since the caller
// must choose one via CreateRequest.OrderID.
type SourceResponse struct {
	PickingTaskID int64                  `json:"picking_task_id,string" example:"1"`
	PickNo        string                 `json:"pick_no" example:"PICK-000001"`
	OrderNo       string                 `json:"order_no" example:"SO-20260608-001"`
	Customer      *string                `json:"customer" example:"ABC Customer"`
	ItemCount     int                    `json:"item_count" example:"3"`
	IsBatch       bool                   `json:"is_batch" example:"false"`
	BatchID       *int64                 `json:"batch_id,string"`
	PendingOrders []PendingOrderResponse `json:"pending_orders,omitempty"`
} //	@name	PackingSourceResponse

// CreateRequest is POST /packing-jobs — creates a job from an already-
// terminal (COMPLETED or SHORT_PICK) picking task. There is no Items
// field: lines are always copied verbatim from already-picked/already-
// sorted quantities (see Service.Create's own doc) — a client can never
// specify or inflate a line's picked_qty.
//
// OrderID (Phase 33C.2) is required if, and only if, PickingTaskID is
// batch-sourced (internal/pickingbatch) — it selects which one of that
// batch's own included orders this specific job is for, since a
// consolidated task must fan out into one job PER ORDER, never one job
// for the whole batch. Supplying OrderID against an ordinary (non-batch)
// task is rejected: that task has exactly one implicit "order" and a
// client cannot choose a different one.
type CreateRequest struct {
	PickingTaskID int64   `json:"picking_task_id,string" binding:"required" example:"1"`
	OrderID       *int64  `json:"order_id,string" example:"1"`
	Remark        *string `json:"remark"`
} //	@name	CreatePackingJobRequest

// AssignRequest is PUT /packing-jobs/{id}/assign — claims (or reassigns)
// the job to a packer.
type AssignRequest struct {
	PackerID int64 `json:"packer_id,string" binding:"required" example:"1"`
} //	@name	AssignPackerRequest

// PackItemRequest is POST /packing-jobs/{id}/pack — one "scan picked
// goods into a carton" action. ScannedSKU/ScannedLotNo are compared
// against the line's own expected product/lot (see Service.PackItem's
// own doc, adapted from picking.ScanConfirm's shape minus the location
// concept — a carton has no location) before any quantity is applied.
// IdempotencyKey is required, matching picking_confirmations/
// relocation_confirmations' own idempotency guard.
type PackItemRequest struct {
	IdempotencyKey string  `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	ItemID         int64   `json:"item_id,string" binding:"required" example:"1"`
	CartonID       int64   `json:"carton_id,string" binding:"required" example:"1"`
	ScannedSKU     string  `json:"scanned_sku" binding:"required" example:"SKU-0001"`
	ScannedLotNo   string  `json:"scanned_lot_no" example:"LOT-0001"`
	Quantity       float64 `json:"quantity" binding:"required" example:"10"`
} //	@name	PackItemRequest

// FlagExceptionRequest is POST /packing-jobs/{id}/items/{itemId}/exception
// — records a packing-stage damage/shortage/wrong-item exception,
// terminating the line regardless of its current packed_qty.
type FlagExceptionRequest struct {
	Reason   string  `json:"reason" binding:"required,oneof=DAMAGED SHORTAGE WRONG_ITEM OTHER" example:"DAMAGED"`
	Quantity float64 `json:"quantity" example:"2"`
	Remark   *string `json:"remark" example:"Carton crushed in transit from picking face"`
} //	@name	FlagPackingExceptionRequest

// CreateCartonRequest is POST /packing-jobs/{id}/cartons.
type CreateCartonRequest struct {
	PackagingType string   `json:"packaging_type" example:"BOX"`
	GrossWeight   *float64 `json:"gross_weight" example:"5.2"`
	NetWeight     *float64 `json:"net_weight" example:"4.8"`
	WeightUnit    string   `json:"weight_unit" example:"KG"`
	CartonLength  *float64 `json:"carton_length" example:"30"`
	CartonWidth   *float64 `json:"carton_width" example:"20"`
	CartonHeight  *float64 `json:"carton_height" example:"15"`
	DimensionUnit string   `json:"dimension_unit" example:"CM"`
	Remark        *string  `json:"remark"`
} //	@name	CreateCartonRequest

// UpdateCartonRequest is PUT /packing-jobs/{id}/cartons/{cartonId} —
// updates descriptive metadata and/or closes the carton (Status
// "CLOSED" stops any further PackItem call from targeting it). lpn is
// never part of this request — it is immutable once generated.
type UpdateCartonRequest struct {
	PackagingType string   `json:"packaging_type" example:"BOX"`
	GrossWeight   *float64 `json:"gross_weight" example:"5.2"`
	NetWeight     *float64 `json:"net_weight" example:"4.8"`
	WeightUnit    string   `json:"weight_unit" example:"KG"`
	CartonLength  *float64 `json:"carton_length" example:"30"`
	CartonWidth   *float64 `json:"carton_width" example:"20"`
	CartonHeight  *float64 `json:"carton_height" example:"15"`
	DimensionUnit string   `json:"dimension_unit" example:"CM"`
	Status        string   `json:"status" example:"OPEN"`
	Remark        *string  `json:"remark"`
} //	@name	UpdateCartonRequest

// CheckerConfirmRequest is POST /packing-jobs/{id}/checker-confirm — the
// second-person verification step. No idempotency_key: this is a pure
// status transition with no quantity math (see migrations/0091's own
// doc for why), naturally safe under a duplicate/retry via the status
// guard alone, the same reason Cancel never needed one either.
type CheckerConfirmRequest struct {
	Remark *string `json:"remark" example:"Verified all cartons sealed and labeled"`
} //	@name	CheckerConfirmRequest

// CancelRequest is POST /packing-jobs/{id}/cancel.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Order cancelled by customer"`
} //	@name	CancelPackingJobRequest

// ListParams narrows GET /packing-jobs to its search box plus status/
// warehouse filters, with pagination.
type ListParams struct {
	Search      string
	Status      Status
	WarehouseID int64
	Page        int
	Limit       int
}

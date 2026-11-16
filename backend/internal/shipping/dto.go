package shipping

import "time"

// ItemResponse is one shipment line, matching ShippingView.vue's
// "Package Items" cards (sku, productName, qty).
type ItemResponse struct {
	ID           int64     `json:"id,string" example:"1"`
	ProductID    int64     `json:"product_id,string" example:"1"`
	SKU          string    `json:"sku" example:"SKU-0001"`
	ProductName  string    `json:"product_name" example:"Barcode Label Roll"`
	LotNo        string    `json:"lot_no"`
	LocationID   int64     `json:"location_id,string" example:"1"`
	LocationCode string    `json:"location_code" example:"BIN-C01"`
	Qty          float64   `json:"qty" example:"20"`
	UOM          string    `json:"uom" example:"PCS"`
	CreatedAt    time.Time `json:"created_at"`
} //	@name	ShipmentItemResponse

// Response is a full shipment with its lines — GET /shipping/{id}.
// OrderNo/Customer/ShipTo* (Phase 33D.1) are resolved from the linked
// packing job's own order snapshot — not picking_tasks — so a batch-
// sourced shipment correctly shows ITS OWN order, not whichever
// (arbitrary) order the shared picking task happened to be created
// under; see internal/packing's own Job.OrderNo doc (Phase 33C.2).
// Neither is stored on shipments itself — see migrations/0020's doc.
// CancelledAt/CancelledBy/CancelledByName/CancelRemark (Phase 18) are
// all nil/null unless the shipment was actually cancelled — see
// Service.Cancel's own doc.
type Response struct {
	ID                  int64          `json:"id,string" example:"1"`
	ShipmentNo          string         `json:"shipment_no" example:"SHP-000001"`
	PackingJobID        int64          `json:"packing_job_id,string" example:"1"`
	JobNo               string         `json:"job_no" example:"PACK-000001"`
	PickingTaskID       int64          `json:"picking_task_id,string" example:"1"`
	PickNo              string         `json:"pick_no" example:"PICK-000001"`
	OrderNo             string         `json:"order_no" example:"SO-20260608-001"`
	Customer            *string        `json:"customer" example:"ABC Customer"`
	CustomerID          *int64         `json:"customer_id,string"`
	ShipToID            *int64         `json:"ship_to_id,string"`
	ShipToName          *string        `json:"ship_to_name"`
	ShipToAddress       *string        `json:"ship_to_address"`
	ShipToProvince      *string        `json:"ship_to_province"`
	ShipToDistrict      *string        `json:"ship_to_district"`
	ShipToPostalCode    *string        `json:"ship_to_postal_code"`
	ShipToContactPerson *string        `json:"ship_to_contact_person"`
	ShipToContactPhone  *string        `json:"ship_to_contact_phone"`
	Carrier             *string        `json:"carrier" example:"DHL"`
	TrackingNo          *string        `json:"tracking_no" example:"DHL123456"`
	Packages            int            `json:"packages" example:"2"`
	Weight              *float64       `json:"weight" example:"18.5"`
	ShipDate            *time.Time     `json:"ship_date"`
	Address             *string        `json:"address"`
	Remark              *string        `json:"remark"`
	Status              Status         `json:"status" example:"READY"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DispatchedAt        *time.Time     `json:"dispatched_at"`
	DeliveredAt         *time.Time     `json:"delivered_at"`
	CancelledAt         *time.Time     `json:"cancelled_at"`
	CancelledBy         *int64         `json:"cancelled_by,string"`
	CancelledByName     *string        `json:"cancelled_by_name"`
	CancelRemark        *string        `json:"cancel_remark"`
	Items               []ItemResponse `json:"items"`
} //	@name	ShipmentResponse

// ListResponse is one row of GET /shipping — matches ShippingView.vue's
// table (Shipment No./Order No./Customer/Carrier/Tracking No./Packages/
// Weight/Ship Date/Status) without the full items array.
type ListResponse struct {
	ID         int64      `json:"id,string" example:"1"`
	ShipmentNo string     `json:"shipment_no" example:"SHP-000001"`
	OrderNo    string     `json:"order_no" example:"SO-20260608-001"`
	Customer   *string    `json:"customer" example:"ABC Customer"`
	Carrier    *string    `json:"carrier" example:"DHL"`
	TrackingNo *string    `json:"tracking_no" example:"DHL123456"`
	Packages   int        `json:"packages" example:"2"`
	Weight     *float64   `json:"weight" example:"18.5"`
	ShipDate   *time.Time `json:"ship_date"`
	Status     Status     `json:"status" example:"READY"`
} //	@name	ShipmentListResponse

// SourceResponse is one candidate packing job for GET /shipping/sources
// (Phase 33D.1) — a COMPLETED (two-person checker-verified, see
// internal/packing's own Status doc) packing job that has no shipment
// yet. Powers ShippingView.vue's "+ New Shipment" flow with a real
// "Source Packing Job" selector. IsBatch/BatchNo mirror
// packing.SourceResponse's own shape (Phase 33C.2) so the UI can show
// "Order SO-001 (Batch WAVE-000001)" per job — IsBatch is true exactly
// when the job's own order_id is set (i.e. it was created from one
// order's own share of a batch-sourced picking task), and BatchNo is
// only ever non-nil in that case.
type SourceResponse struct {
	PackingJobID  int64   `json:"packing_job_id,string" example:"1"`
	JobNo         string  `json:"job_no" example:"PACK-000001"`
	PickingTaskID int64   `json:"picking_task_id,string" example:"1"`
	PickNo        string  `json:"pick_no" example:"PICK-000001"`
	OrderNo       string  `json:"order_no" example:"SO-20260608-001"`
	Customer      *string `json:"customer" example:"ABC Customer"`
	ItemCount     int     `json:"item_count" example:"2"`
	IsBatch       bool    `json:"is_batch" example:"false"`
	BatchNo       *string `json:"batch_no,omitempty" example:"WAVE-000001"`
} //	@name	ShipmentSourceResponse

// CreateRequest is POST /shipping. Phase 33D.1: re-keyed off
// PackingJobID (a deliberate, documented breaking API change — see
// Service.Create's own doc for why PickingTaskID could not simply stay
// alongside it). Items are never accepted from the client — they are
// always copied server-side from the linked, COMPLETED packing job's
// own packed lines, so there is no way for a request to ask to ship
// more than was actually packed into a carton.
type CreateRequest struct {
	PackingJobID int64      `json:"packing_job_id,string" binding:"required" example:"1"`
	Carrier      *string    `json:"carrier" example:"DHL"`
	TrackingNo   *string    `json:"tracking_no" example:"DHL123456"`
	Packages     int        `json:"packages" example:"2"`
	Weight       *float64   `json:"weight" example:"18.5"`
	ShipDate     *time.Time `json:"ship_date"`
	Address      *string    `json:"address"`
	Remark       *string    `json:"remark"`
} //	@name	CreateShipmentRequest

// UpdateRequest is PUT /shipping/{id} — header fields only, and only
// while the shipment is still READY (see Status.CanEdit).
type UpdateRequest struct {
	Carrier    *string    `json:"carrier" example:"DHL"`
	TrackingNo *string    `json:"tracking_no" example:"DHL123456"`
	Packages   int        `json:"packages" example:"2"`
	Weight     *float64   `json:"weight" example:"18.5"`
	ShipDate   *time.Time `json:"ship_date"`
	Address    *string    `json:"address"`
	Remark     *string    `json:"remark"`
} //	@name	UpdateShipmentRequest

// ConfirmRequest is POST /shipping/{id}/confirm — the drawer's "Confirm
// Dispatch"/"Mark Delivered"/"Report Issue" buttons each submit one
// action. IdempotencyKey is required — see shipment_confirmations'
// migration doc for why. Remark is used only by the ISSUE action: it is
// accepted here rather than requiring the header's own Remark field to
// already be non-empty, because header edits (PUT /shipping/{id}) are
// only allowed while a shipment is still READY (Status.CanEdit), but
// ISSUE itself may legitimately be reported after DISPATCHED — a
// remark captured only through the header-edit path could never be set
// at that point. Persists to the same `remark` column as an update.
type ConfirmRequest struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
	Action         string `json:"action" binding:"required,oneof=DISPATCH DELIVER ISSUE" example:"DISPATCH"`
	Remark         string `json:"remark" example:"Package damaged in transit"`
} //	@name	ConfirmShipmentRequest

// CancelRequest is POST /shipping/{id}/cancel — Phase 15's addition.
// Remark is optional; when provided it is stored in the dedicated
// cancel_remark column (Phase 18) rather than overwriting the shipment's
// own Remark field — see Service.Cancel's doc for why this changed. No
// idempotency_key — see internal/picking's own CancelRequest doc
// (Phase 13) for why Cancel's status guard alone is sufficient.
type CancelRequest struct {
	Remark *string `json:"remark" example:"Order cancelled by customer before dispatch"`
} //	@name	CancelShipmentRequest

// ListParams narrows GET /shipping to the Shipping screen's actual
// search box plus status/carrier/date filters, with pagination.
type ListParams struct {
	Search  string
	Status  Status
	Carrier string
	Date    *time.Time
	Page    int
	Limit   int
}

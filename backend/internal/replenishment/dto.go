package replenishment

import "time"

// Response is a full replenishment task — GET /replenishment/{id}.
type Response struct {
	ID               int64      `json:"id,string" example:"1"`
	TaskNo           string     `json:"task_no" example:"RPL-000001"`
	ProductID        int64      `json:"product_id,string" example:"1"`
	SKU              string     `json:"sku" example:"SKU-0001"`
	ProductName      string     `json:"product_name" example:"Barcode Label Roll"`
	FromLocationID   int64      `json:"from_location_id,string" example:"1"`
	FromLocationCode string     `json:"from_location_code" example:"A-01-01"`
	ToLocationID     int64      `json:"to_location_id,string" example:"2"`
	ToLocationCode   string     `json:"to_location_code" example:"A-02-01"`
	LotNo            string     `json:"lot_no" example:"LOT-0001"`
	Quantity         float64    `json:"quantity" example:"50"`
	WarehouseID      int64      `json:"warehouse_id,string" example:"1"`
	WarehouseCode    string     `json:"warehouse_code" example:"WH-01"`
	Status           Status     `json:"status" example:"PENDING"`
	Remark           *string    `json:"remark"`
	IssueRemark      *string    `json:"issue_remark"`
	CreatedBy        *int64     `json:"created_by,string"`
	CreatedByName    *string    `json:"created_by_name"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	StartedAt        *time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	CancelledAt      *time.Time `json:"cancelled_at"`
	CancelledBy      *int64     `json:"cancelled_by,string"`
	CancelledByName  *string    `json:"cancelled_by_name"`
	CancelRemark     *string    `json:"cancel_remark"`
} //	@name	ReplenishmentTaskResponse

// ListResponse is one row of GET /replenishment.
type ListResponse struct {
	ID               int64     `json:"id,string" example:"1"`
	TaskNo           string    `json:"task_no" example:"RPL-000001"`
	SKU              string    `json:"sku" example:"SKU-0001"`
	ProductName      string    `json:"product_name" example:"Barcode Label Roll"`
	FromLocationCode string    `json:"from_location_code" example:"A-01-01"`
	ToLocationCode   string    `json:"to_location_code" example:"A-02-01"`
	Quantity         float64   `json:"quantity" example:"50"`
	WarehouseCode    string    `json:"warehouse_code" example:"WH-01"`
	Status           Status    `json:"status" example:"PENDING"`
	CreatedAt        time.Time `json:"created_at"`
} //	@name	ReplenishmentTaskListResponse

// ListParams narrows GET /replenishment — empty/zero means unfiltered,
// the same convention every other list endpoint in this codebase uses.
type ListParams struct {
	Search      string
	Status      Status
	WarehouseID int64
	ProductID   int64
	Page        int
	Limit       int
}

// CreateRequest is POST /replenishment — manual creation, this phase's
// primary path. warehouse_id is required and validated against both
// locations (see Service.Create) rather than derived, matching
// Receiving/Picking's own explicit-warehouse_id convention.
type CreateRequest struct {
	ProductID      int64   `json:"product_id,string" binding:"required" example:"1"`
	FromLocationID int64   `json:"from_location_id,string" binding:"required" example:"1"`
	ToLocationID   int64   `json:"to_location_id,string" binding:"required" example:"2"`
	LotNo          string  `json:"lot_no" example:"LOT-0001"`
	Quantity       float64 `json:"quantity" binding:"required" example:"50"`
	WarehouseID    int64   `json:"warehouse_id,string" binding:"required" example:"1"`
	Remark         *string `json:"remark"`
} //	@name	CreateReplenishmentTaskRequest

// ConfirmRequest is POST /replenishment/{id}/confirm.
type ConfirmRequest struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required" example:"a1b2c3d4-..."`
} //	@name	ConfirmReplenishmentTaskRequest

// CancelRequest is POST /replenishment/{id}/cancel.
type CancelRequest struct {
	Remark string `json:"remark" example:"No longer needed"`
} //	@name	CancelReplenishmentTaskRequest

// IssueRequest is POST /replenishment/{id}/issue — reports a real-world
// problem (damaged stock, location mismatch, ...) that prevents the
// move from ever completing. Remark is required: unlike Cancel (which
// can mean simply "no longer needed"), ISSUE always means something
// went wrong and should say what.
type IssueRequest struct {
	Remark string `json:"remark" binding:"required" example:"Pallet damaged, cannot move"`
} //	@name	ReportReplenishmentIssueRequest

// RecommendationResponse is one row of GET /replenishment/recommendations
// — see Service.Recommend's own doc for the (deliberately simple, not
// an optimizer) logic behind it. Every field maps 1:1 onto
// CreateRequest so the frontend can turn a recommendation directly into
// a Create call without the user re-typing anything.
type RecommendationResponse struct {
	ProductID        int64   `json:"product_id,string" example:"1"`
	SKU              string  `json:"sku" example:"SKU-0001"`
	ProductName      string  `json:"product_name" example:"Barcode Label Roll"`
	FromLocationID   int64   `json:"from_location_id,string" example:"1"`
	FromLocationCode string  `json:"from_location_code" example:"A-01-01"`
	ToLocationID     int64   `json:"to_location_id,string" example:"2"`
	ToLocationCode   string  `json:"to_location_code" example:"A-02-01"`
	LotNo            string  `json:"lot_no" example:"LOT-0001"`
	SuggestedQty     float64 `json:"suggested_qty" example:"50"`
} //	@name	ReplenishmentRecommendationResponse

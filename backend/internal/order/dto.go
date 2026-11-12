package order

import (
	"time"

	"wms-backend/internal/picking"
	"wms-backend/internal/shipping"
)

// FulfillmentStatus is a read-only, derived-at-query-time richer view
// of an order's real-world progress — PENDING/PICKING/CANCELLED alone
// (Order's own stored Status) can't say "has this been picked yet? has
// it shipped?" without either duplicating internal/picking's and
// internal/shipping's own state (which could drift) or listening for
// their state changes (which would need a messaging component this
// lightweight orchestration layer doesn't need). Computed fresh on
// every GET instead — see deriveFulfillmentStatus.
type FulfillmentStatus string

const (
	FulfillmentPending          FulfillmentStatus = "PENDING"
	FulfillmentPicking          FulfillmentStatus = "PICKING"
	FulfillmentPickingCancelled FulfillmentStatus = "PICKING_CANCELLED"
	FulfillmentPicked           FulfillmentStatus = "PICKED"
	FulfillmentShipping         FulfillmentStatus = "SHIPPING"
	FulfillmentShipIssue        FulfillmentStatus = "SHIP_ISSUE"
	FulfillmentDelivered        FulfillmentStatus = "DELIVERED"
	FulfillmentCancelled        FulfillmentStatus = "CANCELLED"
)

// deriveFulfillmentStatus is the pure core behind FulfillmentStatus —
// extracted so the mapping can be unit tested without a database, the
// same "extract the pure part" precedent every prior phase has already
// established for its own pure-logic piece (Phase 18/20/21's
// applyCancel/pendingAging/buildReminderNotificationText, Phase 24's
// own computeRecommendations).
func deriveFulfillmentStatus(orderStatus Status, pickingStatus *picking.Status, shipmentStatus *shipping.Status) FulfillmentStatus {
	if orderStatus == StatusCancelled {
		return FulfillmentCancelled
	}
	if pickingStatus == nil {
		return FulfillmentPending
	}
	switch *pickingStatus {
	case picking.StatusCancelled:
		return FulfillmentPickingCancelled
	case picking.StatusCompleted, picking.StatusShortPick:
		if shipmentStatus == nil {
			return FulfillmentPicked
		}
		switch *shipmentStatus {
		case shipping.StatusDelivered:
			return FulfillmentDelivered
		case shipping.StatusCancelled:
			return FulfillmentPicked
		case shipping.StatusIssue:
			return FulfillmentShipIssue
		default:
			return FulfillmentShipping
		}
	default:
		return FulfillmentPicking
	}
}

// ItemResponse is one order line.
type ItemResponse struct {
	ID          int64   `json:"id,string" example:"1"`
	ProductID   int64   `json:"product_id,string" example:"1"`
	SKU         string  `json:"sku" example:"SKU-0001"`
	ProductName string  `json:"product_name" example:"Barcode Label Roll"`
	OrderedQty  float64 `json:"ordered_qty" example:"50"`
	UOM         string  `json:"uom" example:"PCS"`
} //	@name	OrderItemResponse

// Response is a full order with its lines — GET /orders/{id}.
type Response struct {
	ID                int64             `json:"id,string" example:"1"`
	OrderNo           string            `json:"order_no" example:"ORD-000001"`
	Customer          *string           `json:"customer"`
	ExternalReference *string           `json:"external_reference"`
	WarehouseID       int64             `json:"warehouse_id,string" example:"1"`
	WarehouseCode     string            `json:"warehouse_code" example:"WH-01"`
	PickingTaskID     *int64            `json:"picking_task_id,string"`
	PickNo            *string           `json:"pick_no"`
	Status            Status            `json:"status" example:"PENDING"`
	FulfillmentStatus FulfillmentStatus `json:"fulfillment_status" example:"PENDING"`
	Remark            *string           `json:"remark"`
	CreatedBy         *int64            `json:"created_by,string"`
	CreatedByName     *string           `json:"created_by_name"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	CancelledAt       *time.Time        `json:"cancelled_at"`
	CancelledBy       *int64            `json:"cancelled_by,string"`
	CancelledByName   *string           `json:"cancelled_by_name"`
	CancelRemark      *string           `json:"cancel_remark"`
	Items             []ItemResponse    `json:"items"`
} //	@name	OrderResponse

// ListResponse is one row of GET /orders.
type ListResponse struct {
	ID                int64             `json:"id,string" example:"1"`
	OrderNo           string            `json:"order_no" example:"ORD-000001"`
	Customer          *string           `json:"customer"`
	ExternalReference *string           `json:"external_reference"`
	WarehouseCode     string            `json:"warehouse_code" example:"WH-01"`
	ItemCount         int               `json:"item_count" example:"3"`
	Status            Status            `json:"status" example:"PENDING"`
	FulfillmentStatus FulfillmentStatus `json:"fulfillment_status" example:"PENDING"`
	CreatedAt         time.Time         `json:"created_at"`
} //	@name	OrderListResponse

// ListParams narrows GET /orders — empty/zero means unfiltered, the
// same convention every other list endpoint in this codebase uses.
// FulfillmentStatus filtering is deliberately not supported: it is
// computed per-row from a live picking/shipping lookup, not a column
// the database can filter on cheaply.
type ListParams struct {
	Search      string
	Status      Status
	WarehouseID int64
	Page        int
	Limit       int
}

// CreateRequest is POST /orders. Never touches inventory — pure header
// + lines persistence.
type CreateRequest struct {
	Customer          *string             `json:"customer"`
	ExternalReference *string             `json:"external_reference"`
	WarehouseID       int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Remark            *string             `json:"remark"`
	Items             []CreateItemRequest `json:"items" binding:"required,min=1"`
} //	@name	CreateOrderRequest

type CreateItemRequest struct {
	ProductID  int64   `json:"product_id,string" binding:"required" example:"1"`
	OrderedQty float64 `json:"ordered_qty" binding:"required" example:"50"`
	UOM        string  `json:"uom" example:"PCS"`
} //	@name	CreateOrderItemRequest

// UpdateRequest is PUT /orders/{id} — full-replace semantics for both
// header and lines (see Service.Update's doc), only while the order is
// still PENDING (Status.CanUpdate).
type UpdateRequest struct {
	Customer          *string             `json:"customer"`
	ExternalReference *string             `json:"external_reference"`
	WarehouseID       int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Remark            *string             `json:"remark"`
	Items             []CreateItemRequest `json:"items" binding:"required,min=1"`
} //	@name	UpdateOrderRequest

// CreatePickingTaskRequest is POST /orders/{id}/create-picking-task —
// the orchestration action. One LocationAssignment per order item,
// matched by order_item_id, supplies the location/lot Picking needs
// but Order itself never tracks (see Item's own doc). Everything else
// is passed straight through to picking.Service.Create, which performs
// its own Phase 12 reservation exactly as it would for a task created
// directly through PickingView.vue — Order Management neither reserves
// nor validates stock itself.
type CreatePickingTaskRequest struct {
	Priority string               `json:"priority" example:"NORMAL"`
	DueDate  *time.Time           `json:"due_date"`
	Items    []LocationAssignment `json:"items" binding:"required,min=1"`
} //	@name	CreateOrderPickingTaskRequest

type LocationAssignment struct {
	OrderItemID int64  `json:"order_item_id,string" binding:"required" example:"1"`
	LocationID  int64  `json:"location_id,string" binding:"required" example:"1"`
	LotNo       string `json:"lot_no" example:"LOT-0001"`
} //	@name	OrderLocationAssignment

// CancelRequest is POST /orders/{id}/cancel.
type CancelRequest struct {
	Remark string `json:"remark" example:"Customer cancelled"`
} //	@name	CancelOrderRequest

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
	FulfillmentReleased         FulfillmentStatus = "RELEASED"
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
		if orderStatus == StatusReleased {
			return FulfillmentReleased
		}
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
	// StorageType is read-only, resolved from the product master
	// (product.StorageType) at display time — never stored on the order
	// line itself, so it can never drift from the product's own current
	// setting.
	StorageType string `json:"storage_type" example:"AMBIENT"`
	// PreferredLotNo/FEFOPreferred (Phase 33A) are a picker HINT only —
	// see Item's own doc.
	PreferredLotNo *string `json:"preferred_lot_no"`
	FEFOPreferred  bool    `json:"fefo_preferred" example:"true"`
	// ShortageFlag/ShortfallQty are set once, by the most recent
	// Service.Release call's own availability check — zero/false on a
	// line that has never been through Release yet.
	ShortageFlag bool     `json:"shortage_flag" example:"false"`
	ShortfallQty *float64 `json:"shortfall_qty"`
	LineNotes    *string  `json:"line_notes"`
} //	@name	OrderItemResponse

// Response is a full order with its lines — GET /orders/{id}.
type Response struct {
	ID                   int64      `json:"id,string" example:"1"`
	OrderNo              string     `json:"order_no" example:"ORD-000001"`
	OrderType            string     `json:"order_type" example:"SALES_ORDER"`
	Priority             string     `json:"priority" example:"NORMAL"`
	Customer             *string    `json:"customer"`
	CustomerID           *int64     `json:"customer_id,string"`
	ShipToID             *int64     `json:"ship_to_id,string"`
	ShipToName           *string    `json:"ship_to_name"`
	ShipToAddress        *string    `json:"ship_to_address"`
	ShipToProvince       *string    `json:"ship_to_province"`
	ShipToDistrict       *string    `json:"ship_to_district"`
	ShipToPostalCode     *string    `json:"ship_to_postal_code"`
	ShipToContactPerson  *string    `json:"ship_to_contact_person"`
	ShipToContactPhone   *string    `json:"ship_to_contact_phone"`
	ExternalReference    *string    `json:"external_reference"`
	RequiredShippingDate *time.Time `json:"required_shipping_date"`
	RequiredDeliveryDate *time.Time `json:"required_delivery_date"`
	Carrier              *string    `json:"carrier"`
	ShippingMethod       *string    `json:"shipping_method"`
	DeliveryInstructions *string    `json:"delivery_instructions"`
	PreparedBy           *string    `json:"prepared_by"`
	ApprovedBy           *string    `json:"approved_by"`
	WarehouseID          int64      `json:"warehouse_id,string" example:"1"`
	WarehouseCode        string     `json:"warehouse_code" example:"WH-01"`
	PickingTaskID        *int64     `json:"picking_task_id,string"`
	PickNo               *string    `json:"pick_no"`
	// PickingMode/PickingBatchID (Phase 33C.1) — see Order.PickingMode's
	// own doc. PickingBatchID is nil until internal/pickingbatch's own
	// Service.Confirm has claimed this order into a consolidated task.
	PickingMode       PickingMode       `json:"picking_mode" example:"PICK_BY_ORDER"`
	PickingBatchID    *int64            `json:"picking_batch_id,string"`
	Status            Status            `json:"status" example:"PENDING"`
	FulfillmentStatus FulfillmentStatus `json:"fulfillment_status" example:"PENDING"`
	Remark            *string           `json:"remark"`
	ReleasedAt        *time.Time        `json:"released_at"`
	ReleasedBy        *int64            `json:"released_by,string"`
	ReleasedByName    *string           `json:"released_by_name"`
	ShortageApproved  bool              `json:"shortage_approved" example:"false"`
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
	OrderType         string            `json:"order_type" example:"SALES_ORDER"`
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
// + lines persistence, and must stay idempotent-safe against duplicate
// submission (a double-click never reserves anything twice — only
// CreatePickingTask's own UNIQUE picking_task_id guard matters for that,
// unchanged by this phase).
type CreateRequest struct {
	Customer *string `json:"customer"`
	// CustomerID selects a tenant-scoped internal/customer master row —
	// when set, Service.Create overwrites Customer above with that
	// customer's own display name (the authoritative source once a real
	// customer is linked) rather than trusting a possibly-stale free-text
	// value in the same request. ShipToID optionally selects one of that
	// customer's own internal/customerdeliverylocation rows; when
	// omitted, Service.Create auto-selects that customer's own ACTIVE
	// default ship-to, if one exists — see Service.resolveShipTo's own
	// doc. ShipToID requires CustomerID in the same request (a ship-to
	// always belongs to a specific customer, never resolved from the
	// ship-to id alone).
	CustomerID *int64 `json:"customer_id,string" example:"1"`
	ShipToID   *int64 `json:"ship_to_id,string" example:"1"`
	// OrderType defaults to SALES_ORDER when omitted. INTERNAL_TRANSFER
	// is also accepted (CK_orders_order_type) but Phase 33A builds no
	// transfer-specific behavior around it — see model.go's own
	// Order.OrderType doc.
	OrderType string `json:"order_type" example:"SALES_ORDER"`
	Priority  string `json:"priority" example:"NORMAL"`
	// PickingMode (Phase 33C.1) defaults to PICK_BY_ORDER when omitted —
	// see Order.PickingMode's own doc. Setting PICK_BY_PRODUCT here does
	// not do anything by itself; it only makes the order eligible for a
	// future internal/pickingbatch consolidation once RELEASED.
	PickingMode          string              `json:"picking_mode" example:"PICK_BY_ORDER"`
	RequiredShippingDate *time.Time          `json:"required_shipping_date"`
	RequiredDeliveryDate *time.Time          `json:"required_delivery_date"`
	Carrier              *string             `json:"carrier"`
	ShippingMethod       *string             `json:"shipping_method"`
	DeliveryInstructions *string             `json:"delivery_instructions"`
	PreparedBy           *string             `json:"prepared_by"`
	ApprovedBy           *string             `json:"approved_by"`
	ExternalReference    *string             `json:"external_reference"`
	WarehouseID          int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Remark               *string             `json:"remark"`
	Items                []CreateItemRequest `json:"items" binding:"required,min=1"`
} //	@name	CreateOrderRequest

type CreateItemRequest struct {
	ProductID  int64   `json:"product_id,string" binding:"required" example:"1"`
	OrderedQty float64 `json:"ordered_qty" binding:"required" example:"50"`
	UOM        string  `json:"uom" example:"PCS"`
	// PreferredLotNo/FEFOPreferred/LineNotes: see Item's own doc — a
	// picker hint and free-text note only, never consulted by any
	// inventory call this package makes.
	PreferredLotNo *string `json:"preferred_lot_no"`
	FEFOPreferred  *bool   `json:"fefo_preferred" example:"true"`
	LineNotes      *string `json:"line_notes"`
} //	@name	CreateOrderItemRequest

// UpdateRequest is PUT /orders/{id} — full-replace semantics for both
// header and lines (see Service.Update's doc), only while the order is
// still PENDING (Status.CanUpdate).
type UpdateRequest struct {
	Customer *string `json:"customer"`
	// CustomerID/ShipToID: see CreateRequest's own doc — identical
	// semantics, re-resolved and re-snapshotted on every Update
	// (full-replace, matching every other header field here: omitting
	// them clears the order's existing customer/ship-to link, the same
	// convention Carrier/TrackingNo already follow in
	// internal/shipping's own UpdateRequest).
	CustomerID *int64 `json:"customer_id,string" example:"1"`
	ShipToID   *int64 `json:"ship_to_id,string" example:"1"`
	OrderType  string `json:"order_type" example:"SALES_ORDER"`
	Priority   string `json:"priority" example:"NORMAL"`
	// PickingMode: see CreateRequest's own doc — identical semantics,
	// re-set on every Update (full-replace, matching every other header
	// field here).
	PickingMode          string              `json:"picking_mode" example:"PICK_BY_ORDER"`
	RequiredShippingDate *time.Time          `json:"required_shipping_date"`
	RequiredDeliveryDate *time.Time          `json:"required_delivery_date"`
	Carrier              *string             `json:"carrier"`
	ShippingMethod       *string             `json:"shipping_method"`
	DeliveryInstructions *string             `json:"delivery_instructions"`
	PreparedBy           *string             `json:"prepared_by"`
	ApprovedBy           *string             `json:"approved_by"`
	ExternalReference    *string             `json:"external_reference"`
	WarehouseID          int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	Remark               *string             `json:"remark"`
	Items                []CreateItemRequest `json:"items" binding:"required,min=1"`
} //	@name	UpdateOrderRequest

// AvailabilityLine is one order line's live, read-only inventory
// availability — never a reservation, just a query result at the
// instant it was computed (see inventory.Service.
// AvailabilityByProductWarehouse's own doc). ShortfallQty is 0 when
// AvailableQty already covers OrderedQty.
type AvailabilityLine struct {
	OrderItemID  int64   `json:"order_item_id,string" example:"1"`
	ProductID    int64   `json:"product_id,string" example:"1"`
	SKU          string  `json:"sku" example:"SKU-0001"`
	ProductName  string  `json:"product_name" example:"Barcode Label Roll"`
	OrderedQty   float64 `json:"ordered_qty" example:"50"`
	AvailableQty float64 `json:"available_qty" example:"30"`
	ReservedQty  float64 `json:"reserved_qty" example:"10"`
	OnHandQty    float64 `json:"on_hand_qty" example:"40"`
	HasShortage  bool    `json:"has_shortage" example:"true"`
	ShortfallQty float64 `json:"shortfall_qty" example:"20"`
} //	@name	OrderAvailabilityLine

// AvailabilityCheckResponse is GET /orders/{id}/availability — the
// Review & Release screen's own read-only preview, and the exact same
// computation Service.Release itself runs before deciding whether a
// PENDING -> RELEASED transition may proceed.
type AvailabilityCheckResponse struct {
	OrderID     int64              `json:"order_id,string" example:"1"`
	HasShortage bool               `json:"has_shortage" example:"true"`
	Lines       []AvailabilityLine `json:"lines"`
} //	@name	OrderAvailabilityCheckResponse

// ReleaseRequest is POST /orders/{id}/release. ApproveShortage is the
// "Controlled shortage approval" override: if the availability check
// finds any line short, Release is rejected with 422 unless this is
// true — and even then, the caller must additionally hold order.confirm
// (enforced by the route itself), never inferred from this flag alone.
type ReleaseRequest struct {
	ApproveShortage bool    `json:"approve_shortage" example:"false"`
	Remark          *string `json:"remark"`
} //	@name	ReleaseOrderRequest

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

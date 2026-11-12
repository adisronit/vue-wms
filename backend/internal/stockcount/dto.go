package stockcount

import "time"

// ItemResponse is one stock count line, matching CheckStockView.vue's
// items table (SKU/Product/Location/Lot/System Qty/Count Qty/Variance/
// UOM/Condition/Status/Remark).
type ItemResponse struct {
	ID           int64      `json:"id,string" example:"1"`
	ProductID    int64      `json:"product_id,string" example:"1"`
	SKU          string     `json:"sku" example:"SKU-0001"`
	ProductName  string     `json:"product_name" example:"Barcode Label Roll"`
	LocationID   int64      `json:"location_id,string" example:"1"`
	LocationCode string     `json:"location_code" example:"A-01-01"`
	LotNo        string     `json:"lot_no"`
	UOM          string     `json:"uom" example:"ROLL"`
	SystemQty    float64    `json:"system_qty" example:"120"`
	CountQty     *float64   `json:"count_qty"`
	VarianceQty  *float64   `json:"variance_qty"`
	Condition    Condition  `json:"condition" example:"GOOD"`
	Status       ItemStatus `json:"status" example:"PENDING"`
	Remark       *string    `json:"remark"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
} //	@name	StockCountItemResponse

// GeneratedAdjustmentResponse is one Stock Adjustment document generated
// from this count via Create Adjustment — Phase 17's traceability
// addition. Deliberately narrow (only what the History detail view
// needs to display and let the user find the real document in Stock
// Adjustment's own list): id/adjustment_no to identify it,
// status/reason to show its state without a second API call,
// created_at for ordering. Always an empty array, never omitted or
// null, when no adjustment was generated (i.e. every count confirmed
// directly via Confirm Count, or not yet acted on at all) — see
// Service.Get's doc.
type GeneratedAdjustmentResponse struct {
	ID           int64     `json:"id,string" example:"1"`
	AdjustmentNo string    `json:"adjustment_no" example:"ADJ-000001"`
	Status       string    `json:"status" example:"PENDING"`
	Reason       string    `json:"reason" example:"Cycle Count Variance"`
	CreatedAt    time.Time `json:"created_at"`
} //	@name	GeneratedStockAdjustmentResponse

// Response is a full stock count with its lines — GET /stock-counts/{id}.
// CreatedBy/CreatedByName were added in Phase 14 (Stock Count History) —
// the id was already stored on the Count model since Phase 11, just
// never surfaced in this response; CreatedByName resolves it to a
// display name via a LEFT JOIN to users (nil if the creating user was
// since deleted, or the column is legitimately null).
// GeneratedAdjustments was added in Phase 17 — see its own doc.
type Response struct {
	ID                   int64                         `json:"id,string" example:"1"`
	CountNo              string                        `json:"count_no" example:"CNT-000001"`
	CountType            CountType                     `json:"count_type" example:"CYCLE_COUNT"`
	WarehouseID          int64                         `json:"warehouse_id,string" example:"1"`
	WarehouseCode        string                        `json:"warehouse_code" example:"WH-01"`
	ZoneID               *int64                        `json:"zone_id,string"`
	ZoneCode             *string                       `json:"zone_code"`
	CountDate            *time.Time                    `json:"count_date"`
	Status               Status                        `json:"status" example:"DRAFT"`
	CreatedBy            *int64                        `json:"created_by,string"`
	CreatedByName        *string                       `json:"created_by_name" example:"Administrator"`
	CreatedAt            time.Time                     `json:"created_at"`
	UpdatedAt            time.Time                     `json:"updated_at"`
	ConfirmedAt          *time.Time                    `json:"confirmed_at"`
	Items                []ItemResponse                `json:"items"`
	GeneratedAdjustments []GeneratedAdjustmentResponse `json:"generated_adjustments"`
} //	@name	StockCountResponse

// ListResponse is one row of GET /stock-counts — the Stock Count History
// list Phase 14 added a real frontend consumer for (see package doc:
// this endpoint existed since Phase 11 with no screen to populate; it
// still doesn't need any new columns or tables, just these two extra
// display fields plumbed through the existing query). ZoneCode/
// CreatedAt/CreatedBy/CreatedByName were added this phase; every other
// field already existed.
type ListResponse struct {
	ID            int64      `json:"id,string" example:"1"`
	CountNo       string     `json:"count_no" example:"CNT-000001"`
	CountType     CountType  `json:"count_type" example:"CYCLE_COUNT"`
	WarehouseCode string     `json:"warehouse_code" example:"WH-01"`
	ZoneCode      *string    `json:"zone_code"`
	CountDate     *time.Time `json:"count_date"`
	ItemCount     int        `json:"item_count" example:"3"`
	VarianceLines int        `json:"variance_lines" example:"1"`
	Status        Status     `json:"status" example:"DRAFT"`
	CreatedBy     *int64     `json:"created_by,string"`
	CreatedByName *string    `json:"created_by_name" example:"Administrator"`
	CreatedAt     time.Time  `json:"created_at"`
} //	@name	StockCountListResponse

// CreateRequest is POST /stock-counts — makes "+ New Count Task" real
// (it was a bare alert() in the mock). Items may optionally be included
// up front, or added one at a time afterward via POST
// /stock-counts/{id}/items (the "+ Add Item" button's real backing).
type CreateRequest struct {
	CountType   CountType           `json:"count_type" binding:"required" example:"CYCLE_COUNT"`
	WarehouseID int64               `json:"warehouse_id,string" binding:"required" example:"1"`
	ZoneID      *int64              `json:"zone_id,string"`
	CountDate   *time.Time          `json:"count_date"`
	Items       []CreateItemRequest `json:"items"`
} //	@name	CreateStockCountRequest

// CreateItemRequest is one line within CreateRequest.Items or the body
// of POST /stock-counts/{id}/items. SystemQty is never accepted from
// the client — it is always snapshotted server-side from Inventory
// Core's live balance at the moment the line is added (see
// Service.addItem's doc).
type CreateItemRequest struct {
	ProductID  int64  `json:"product_id,string" binding:"required" example:"1"`
	LocationID int64  `json:"location_id,string" binding:"required" example:"1"`
	LotNo      string `json:"lot_no" example:"LOT-0001"`
	UOM        string `json:"uom" example:"ROLL"`
} //	@name	CreateStockCountItemRequest

// RecordCountRequest is PUT /stock-counts/{id}/items/{itemId} — records
// the operator's counted quantity for one line (the scan bar's
// "Confirm Count" action and the table's own editable Count Qty/
// Condition/Remark cells both call this). VarianceQty is computed and
// stored server-side from CountQty and the line's frozen SystemQty —
// never accepted from the client.
type RecordCountRequest struct {
	CountQty  float64    `json:"count_qty" binding:"required" example:"118"`
	Condition Condition  `json:"condition" example:"GOOD"`
	Status    ItemStatus `json:"status" example:"COUNTED"`
	Remark    *string    `json:"remark"`
} //	@name	RecordStockCountRequest

// ConfirmRequest is POST /stock-counts/{id}/confirm — "Confirm Count".
// IdempotencyKey is required — see stock_count_confirmations' migration
// doc for why.
type ConfirmRequest struct {
	IdempotencyKey string `json:"idempotency_key" binding:"required" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6"`
} //	@name	ConfirmStockCountRequest

// CreateAdjustmentsResponse is the result of POST
// /stock-counts/{id}/create-adjustments — "Create Adjustment", the
// mock's alternate path to Confirm Count: instead of applying variance
// immediately, generate real internal/stockadjustment DRAFT-equivalent
// documents (one per variance line) that go through that package's own
// separate approval workflow. Neither path mutates inventory itself
// through this response — only internal/stockadjustment's own Confirm
// (APPROVE) does, exactly like a manually-created adjustment would.
type CreateAdjustmentsResponse struct {
	AdjustmentIDs []int64 `json:"adjustment_ids,string" example:"1,2"`
} //	@name	CreateStockCountAdjustmentsResponse

// ListParams narrows GET /stock-counts to the History screen's actual
// search box (matches count_no) plus warehouse/status/date filters, with
// pagination. Search/Date were added in Phase 14 for the History list;
// WarehouseID/Status/Page/Limit already existed since Phase 11.
type ListParams struct {
	Search      string
	WarehouseID int64
	Status      Status
	Date        *time.Time
	Page        int
	Limit       int
}

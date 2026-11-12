package inventory

import "time"

// Status is derived at query time from on_hold/reserved/available vs.
// the product's min_stock — never stored, so it can never drift from
// the numbers it's computed from. Precedence (documented since the
// frontend mock never articulated an exact rule, only examples):
// on_hold wins outright; otherwise any reservation makes a row
// "RESERVED" even if it's also running low; otherwise low available
// stock (<= the product's min_stock) makes it "LOW_STOCK"; otherwise
// "AVAILABLE".
type Status string

const (
	StatusAvailable Status = "AVAILABLE"
	StatusReserved  Status = "RESERVED"
	StatusHold      Status = "HOLD"
	StatusLowStock  Status = "LOW_STOCK"
)

// Response is one row of GET /api/v1/inventory (and the shape of GET
// /api/v1/inventory/{id}) — matches StockInquiryView.vue's stockList
// item fields exactly, with warehouse/zone/location as human-readable
// codes (resolved via the location hierarchy) rather than raw IDs, the
// same way the frontend already displays them.
type Response struct {
	ID            int64      `json:"id,string" example:"1"`
	ProductID     int64      `json:"product_id,string" example:"1"`
	SKU           string     `json:"sku" example:"SKU-0001"`
	ProductName   string     `json:"product_name" example:"Barcode Label Roll"`
	Barcode       *string    `json:"barcode"`
	WarehouseID   int64      `json:"warehouse_id,string" example:"1"`
	WarehouseCode string     `json:"warehouse_code" example:"WH-01"`
	ZoneID        int64      `json:"zone_id,string" example:"1"`
	ZoneCode      string     `json:"zone_code" example:"A"`
	LocationID    int64      `json:"location_id,string" example:"1"`
	LocationCode  string     `json:"location_code" example:"A-01-01"`
	LotNo         string     `json:"lot_no" example:"LOT-0001"`
	ExpiryDate    *time.Time `json:"expiry_date"`
	AvailableQty  float64    `json:"available_qty" example:"120"`
	ReservedQty   float64    `json:"reserved_qty" example:"20"`
	OnHandQty     float64    `json:"on_hand_qty" example:"140"`
	UOM           string     `json:"uom" example:"ROLL"`
	Status        Status     `json:"status" example:"AVAILABLE"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
} //	@name	InventoryResponse

// SummaryResponse matches StockInquiryView.vue's four summary cards.
type SummaryResponse struct {
	TotalSKUs     int64   `json:"total_skus" example:"1248"`
	AvailableQty  float64 `json:"available_qty" example:"85420"`
	ReservedQty   float64 `json:"reserved_qty" example:"6310"`
	LowStockCount int64   `json:"low_stock_count" example:"18"`
} //	@name	InventorySummaryResponse

// TransactionResponse is one row of GET /api/v1/inventory/{id}/transactions
// — powers StockInquiryView.vue's "Recent Movement" panel with real
// (likely empty, this phase) ledger data instead of the mock array.
type TransactionResponse struct {
	ID              int64           `json:"id,string" example:"1"`
	TransactionType TransactionType `json:"transaction_type" example:"ADJUST"`
	Quantity        float64         `json:"quantity" example:"-20"`
	ReferenceType   *string         `json:"reference_type"`
	ReferenceID     *string         `json:"reference_id"`
	Reason          *string         `json:"reason"`
	CreatedBy       *int64          `json:"created_by,string"`
	CreatedAt       time.Time       `json:"created_at"`
} //	@name	InventoryTransactionResponse

// TransactionExportRow is Phase 22's flattened row for the async
// Inventory Transaction History export — the one report in this phase
// expected to genuinely outgrow a synchronous request (an append-only,
// ever-growing ledger), so it is not scoped to a single inventory row
// the way TransactionResponse/Transactions already are.
type TransactionExportRow struct {
	ID              int64
	TransactionType TransactionType
	SKU             string
	ProductName     string
	WarehouseCode   string
	LocationCode    string
	LotNo           string
	Quantity        float64
	ReferenceType   *string
	ReferenceID     *string
	Reason          *string
	CreatedByName   *string
	CreatedAt       time.Time
}

// TransactionExportFilters narrows the global transaction export —
// independent of ListParams (that one scopes current balances, this one
// scopes ledger rows) but follows the identical empty/zero-means-
// unfiltered convention every other filter set in this codebase uses.
// JSON tags matter here specifically: internal/report persists this
// struct as export_jobs.filters_json and round-trips it back through
// the same tags when the worker processes the job.
type TransactionExportFilters struct {
	ProductID       int64           `json:"product_id"`
	WarehouseID     int64           `json:"warehouse_id"`
	TransactionType TransactionType `json:"transaction_type"`
	DateFrom        *time.Time      `json:"date_from"`
	DateTo          *time.Time      `json:"date_to"`
}

// ListParams narrows GET /api/v1/inventory to the Stock Inquiry
// screen's actual search box plus warehouse/zone/status/lot filters.
// warehouse_id/zone_id/location_id are real IDs (from the Warehouse
// Setup APIs built in Phase 3), not the mock's hardcoded WH-01/A
// strings — Stock Inquiry's filter dropdowns are now populated from
// those real APIs.
type ListParams struct {
	Search      string
	ProductID   int64
	WarehouseID int64
	ZoneID      int64
	LocationID  int64
	Status      Status
	LotNo       string
	Page        int
	Limit       int
}

package dashboard

import "wms-backend/internal/receiving"

// SummaryResponse matches DashboardView.vue's existing 4 summary cards
// plus its "Recent Inbound Activity" table — a pure read-side
// aggregation with no table of its own (see service.go's doc for why).
type SummaryResponse struct {
	PendingReceivingCount int64                    `json:"pending_receiving_count" example:"24"`
	AvailableStockQty     float64                  `json:"available_stock_qty" example:"12450"`
	PendingPickingCount   int64                    `json:"pending_picking_count" example:"38"`
	LowStockCount         int64                    `json:"low_stock_count" example:"7"`
	RecentInbound         []receiving.ListResponse `json:"recent_inbound"`
} //	@name	DashboardSummaryResponse

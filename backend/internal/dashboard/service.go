// Package dashboard powers DashboardView.vue's Overview page — Phase
// 23. It owns no table and no migration: every number and row it
// returns is read live from Receiving/Picking/Inventory through their
// own existing Service methods (CountOpen/Summary/List), the same
// "narrow interface satisfied by another package's Service" pattern
// internal/shipping already established for pickingChecker. A pure
// aggregation layer like this is exactly what CLAUDE.md's own dashboard
// guidance calls for: cheap, indexed aggregation endpoints, never a
// full-dataset load for the frontend to summarize itself.
package dashboard

import (
	"context"

	"wms-backend/internal/inventory"
	"wms-backend/internal/receiving"
)

// recentInboundLimit bounds the "Recent Inbound Activity" panel to the
// same small, fixed page DashboardView.vue's original mock always
// showed (3 rows) — rounded up slightly to 5 so the panel isn't
// suspiciously sparse the moment more than 3 receipts exist, but still
// nowhere near a full-table load.
const recentInboundLimit = 5

// receivingSummarizer is the slice of receiving.Service the dashboard
// needs: an open-task count for the "Pending Receiving" card and a
// small recent-activity page for the panel below it.
type receivingSummarizer interface {
	CountOpen(ctx context.Context) (int64, error)
	List(ctx context.Context, params receiving.ListParams) ([]receiving.ListResponse, int64, error)
}

// pickingSummarizer is the slice of picking.Service the dashboard
// needs: just the open-task count for the "Pending Picking" card.
type pickingSummarizer interface {
	CountOpen(ctx context.Context) (int64, error)
}

// inventorySummarizer is the slice of inventory.Service the dashboard
// needs — the exact same Summary() call StockInquiryView.vue's own
// summary cards already use, so "Available Stock"/"Low Stock" can never
// disagree between the two screens.
type inventorySummarizer interface {
	Summary(ctx context.Context) (inventory.SummaryResponse, error)
}

// Service composes the three existing domains above into one read-only
// response. No transaction, no shared consistency guarantee across the
// four numbers is needed or attempted — this is an operational overview
// refreshed on page load, not a financial reconciliation.
type Service struct {
	receiving receivingSummarizer
	picking   pickingSummarizer
	inventory inventorySummarizer
}

func NewService(receivingSvc receivingSummarizer, pickingSvc pickingSummarizer, inventorySvc inventorySummarizer) *Service {
	return &Service{receiving: receivingSvc, picking: pickingSvc, inventory: inventorySvc}
}

// Summary answers GET /dashboard/summary.
func (s *Service) Summary(ctx context.Context) (SummaryResponse, error) {
	pendingReceiving, err := s.receiving.CountOpen(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}

	pendingPicking, err := s.picking.CountOpen(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}

	invSummary, err := s.inventory.Summary(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}

	recent, _, err := s.receiving.List(ctx, receiving.ListParams{Page: 1, Limit: recentInboundLimit})
	if err != nil {
		return SummaryResponse{}, err
	}

	return SummaryResponse{
		PendingReceivingCount: pendingReceiving,
		AvailableStockQty:     invSummary.AvailableQty,
		PendingPickingCount:   pendingPicking,
		LowStockCount:         invSummary.LowStockCount,
		RecentInbound:         recent,
	}, nil
}

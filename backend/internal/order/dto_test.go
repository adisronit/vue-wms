package order

import (
	"testing"

	"wms-backend/internal/picking"
	"wms-backend/internal/shipping"
)

func TestDeriveFulfillmentStatus(t *testing.T) {
	ps := func(s picking.Status) *picking.Status { return &s }
	ss := func(s shipping.Status) *shipping.Status { return &s }

	cases := []struct {
		name           string
		orderStatus    Status
		pickingStatus  *picking.Status
		shipmentStatus *shipping.Status
		want           FulfillmentStatus
	}{
		{"cancelled order is always CANCELLED regardless of any linked task", StatusCancelled, ps(picking.StatusInProgress), nil, FulfillmentCancelled},
		{"no picking task yet", StatusPending, nil, nil, FulfillmentPending},
		{"picking task pending", StatusPicking, ps(picking.StatusPending), nil, FulfillmentPicking},
		{"picking task in progress", StatusPicking, ps(picking.StatusInProgress), nil, FulfillmentPicking},
		{"picking task cancelled", StatusPicking, ps(picking.StatusCancelled), nil, FulfillmentPickingCancelled},
		{"picking completed, no shipment yet", StatusPicking, ps(picking.StatusCompleted), nil, FulfillmentPicked},
		{"short pick, no shipment yet", StatusPicking, ps(picking.StatusShortPick), nil, FulfillmentPicked},
		{"picked and shipment ready/dispatched", StatusPicking, ps(picking.StatusCompleted), ss(shipping.StatusReady), FulfillmentShipping},
		{"picked and shipment dispatched", StatusPicking, ps(picking.StatusCompleted), ss(shipping.StatusDispatched), FulfillmentShipping},
		{"picked and shipment delivered", StatusPicking, ps(picking.StatusCompleted), ss(shipping.StatusDelivered), FulfillmentDelivered},
		{"picked and shipment issue", StatusPicking, ps(picking.StatusCompleted), ss(shipping.StatusIssue), FulfillmentShipIssue},
		{"picked and shipment cancelled falls back to picked", StatusPicking, ps(picking.StatusCompleted), ss(shipping.StatusCancelled), FulfillmentPicked},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveFulfillmentStatus(tc.orderStatus, tc.pickingStatus, tc.shipmentStatus)
			if got != tc.want {
				t.Errorf("deriveFulfillmentStatus(%v, %v, %v) = %v, want %v",
					tc.orderStatus, tc.pickingStatus, tc.shipmentStatus, got, tc.want)
			}
		})
	}
}

package replenishment

import "testing"

func TestStatusCanStart(t *testing.T) {
	expected := map[Status]bool{
		StatusPending:    true,
		StatusInProgress: false, StatusCompleted: false, StatusCancelled: false, StatusIssue: false,
	}
	for status, want := range expected {
		if got := status.CanStart(); got != want {
			t.Errorf("Status(%s).CanStart() = %v, want %v", status, got, want)
		}
	}
}

func TestStatusCanConfirm(t *testing.T) {
	expected := map[Status]bool{
		StatusPending: true, StatusInProgress: true,
		StatusCompleted: false, StatusCancelled: false, StatusIssue: false,
	}
	for status, want := range expected {
		if got := status.CanConfirm(); got != want {
			t.Errorf("Status(%s).CanConfirm() = %v, want %v", status, got, want)
		}
	}
}

func TestStatusCanCancel(t *testing.T) {
	expected := map[Status]bool{
		StatusPending: true, StatusInProgress: true,
		StatusCompleted: false, StatusCancelled: false, StatusIssue: false,
	}
	for status, want := range expected {
		if got := status.CanCancel(); got != want {
			t.Errorf("Status(%s).CanCancel() = %v, want %v", status, got, want)
		}
	}
}

func TestStatusCanReportIssue(t *testing.T) {
	expected := map[Status]bool{
		StatusPending: true, StatusInProgress: true,
		StatusCompleted: false, StatusCancelled: false, StatusIssue: false,
	}
	for status, want := range expected {
		if got := status.CanReportIssue(); got != want {
			t.Errorf("Status(%s).CanReportIssue() = %v, want %v", status, got, want)
		}
	}
}

func TestComputeRecommendations_LowLocationGetsSurplusSource(t *testing.T) {
	// Product 1 has two locations: LOC-A (low, 5 <= min_stock 10) and
	// LOC-B (surplus, 100 available). Rows must already be sorted desc
	// by available_qty per product, matching the repository's own ORDER BY.
	rows := []recommendRow{
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 50,
			LocationID: 2, LocationCode: "LOC-B", LotNo: "LOT-1", AvailableQty: 100},
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 50,
			LocationID: 1, LocationCode: "LOC-A", LotNo: "LOT-1", AvailableQty: 5},
	}

	recs := computeRecommendations(rows)
	if len(recs) != 1 {
		t.Fatalf("len(recs) = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.FromLocationID != 2 || r.ToLocationID != 1 {
		t.Errorf("recommendation moved %d -> %d, want 2 -> 1", r.FromLocationID, r.ToLocationID)
	}
	// suggested = min(max_stock - available, surplus available) = min(50-5, 100) = 45
	if r.SuggestedQty != 45 {
		t.Errorf("SuggestedQty = %v, want 45", r.SuggestedQty)
	}
}

func TestComputeRecommendations_NoSurplusMeansNoRecommendation(t *testing.T) {
	// Only one location for this product, and it's low — no other
	// location exists to draw from, so nothing should be recommended.
	rows := []recommendRow{
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 50,
			LocationID: 1, LocationCode: "LOC-A", LotNo: "LOT-1", AvailableQty: 5},
	}
	recs := computeRecommendations(rows)
	if len(recs) != 0 {
		t.Fatalf("len(recs) = %d, want 0", len(recs))
	}
}

func TestComputeRecommendations_NotLowMeansNoRecommendation(t *testing.T) {
	rows := []recommendRow{
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 50,
			LocationID: 2, LocationCode: "LOC-B", LotNo: "LOT-1", AvailableQty: 100},
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 50,
			LocationID: 1, LocationCode: "LOC-A", LotNo: "LOT-1", AvailableQty: 20},
	}
	recs := computeRecommendations(rows)
	if len(recs) != 0 {
		t.Fatalf("len(recs) = %d, want 0 (20 > min_stock 10, not low)", len(recs))
	}
}

func TestComputeRecommendations_SuggestedQtyCappedBySurplusAvailability(t *testing.T) {
	// Destination has room up to max_stock 100 (needs 95), but the
	// surplus location only has 30 available — suggestion must be
	// capped at 30, never invented beyond what the source actually has.
	rows := []recommendRow{
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 100,
			LocationID: 2, LocationCode: "LOC-B", LotNo: "LOT-1", AvailableQty: 30},
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 100,
			LocationID: 1, LocationCode: "LOC-A", LotNo: "LOT-1", AvailableQty: 5},
	}
	recs := computeRecommendations(rows)
	if len(recs) != 1 {
		t.Fatalf("len(recs) = %d, want 1", len(recs))
	}
	if recs[0].SuggestedQty != 30 {
		t.Errorf("SuggestedQty = %v, want 30 (capped by surplus availability)", recs[0].SuggestedQty)
	}
}

func TestComputeRecommendations_MultipleProductsIndependent(t *testing.T) {
	rows := []recommendRow{
		{ProductID: 2, SKU: "SKU-2", ProductName: "Gadget", MinStock: 5, MaxStock: 20,
			LocationID: 4, LocationCode: "LOC-D", LotNo: "", AvailableQty: 50},
		{ProductID: 2, SKU: "SKU-2", ProductName: "Gadget", MinStock: 5, MaxStock: 20,
			LocationID: 3, LocationCode: "LOC-C", LotNo: "", AvailableQty: 1},
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 50,
			LocationID: 2, LocationCode: "LOC-B", LotNo: "", AvailableQty: 100},
		{ProductID: 1, SKU: "SKU-1", ProductName: "Widget", MinStock: 10, MaxStock: 50,
			LocationID: 1, LocationCode: "LOC-A", LotNo: "", AvailableQty: 5},
	}
	recs := computeRecommendations(rows)
	if len(recs) != 2 {
		t.Fatalf("len(recs) = %d, want 2 (one per product)", len(recs))
	}
}

package receivingreport

import "testing"

func strp(s string) *string { return &s }

// TestCsvHeadersAndRows_FormatsNullableFieldsSafely is a pure-Go test
// (no DB) pinning down the export row shape: nullable fields render as
// empty strings rather than "<nil>", and short_over_qty is the
// already-computed Row field, not recomputed here.
func TestCsvHeadersAndRows_FormatsNullableFieldsSafely(t *testing.T) {
	rows := []Row{
		{
			ReceiptNo: "RCV-000001", ReceivingType: "PO",
			ReferenceNo: strp("PO-001"), SupplierName: strp("Acme Supplier"),
			WarehouseCode: "WH-01", WarehouseName: "Main Warehouse",
			SKU: "SKU-001", ProductName: "Widget",
			ExpectedQty: 100, ReceivedQty: 90, ShortOverQty: -10, UOM: "PCS",
			LotNo: strp("LOT-001"), PalletID: strp("PLT-001"), TemperatureC: ptrF(4.5),
			ReceivingStatus: "PARTIALLY_RECEIVED", QCStatus: "PASSED", PutawayStatus: "PENDING",
			OperatorName: strp("Jane Doe"),
		},
		{
			// A line not yet received: every split-level field nil.
			ReceiptNo: "RCV-000002", ReceivingType: "PO",
			WarehouseCode: "WH-01", WarehouseName: "Main Warehouse",
			SKU: "SKU-002", ProductName: "Gadget",
			ExpectedQty: 50, ReceivedQty: 0, ShortOverQty: -50, UOM: "PCS",
			ReceivingStatus: "WAITING", QCStatus: "PENDING", PutawayStatus: "NOT_STARTED",
		},
	}

	headers, csvRows := csvHeadersAndRows(rows)

	if len(headers) == 0 {
		t.Fatal("headers is empty")
	}
	if len(csvRows) != 2 {
		t.Fatalf("len(csvRows) = %d, want 2", len(csvRows))
	}
	for _, row := range csvRows {
		if len(row) != len(headers) {
			t.Fatalf("row %v has %d columns, want %d matching headers %v", row, len(row), len(headers), headers)
		}
	}

	// Row 2 (no splits yet): nullable columns must render as "", never "<nil>".
	colIndex := func(name string) int {
		for i, h := range headers {
			if h == name {
				return i
			}
		}
		t.Fatalf("header %q not found in %v", name, headers)
		return -1
	}
	row2 := csvRows[1]
	for _, col := range []string{"reference_no", "supplier_name", "lot_no", "pallet_id", "temperature_c", "operator_name"} {
		if got := row2[colIndex(col)]; got != "" {
			t.Errorf("row2[%s] = %q, want empty string for an unset nullable field", col, got)
		}
	}

	// Row 1: short_over_qty must be the pre-computed Row field (-10), not
	// re-derived here.
	row1 := csvRows[0]
	if got := row1[colIndex("short_over_qty")]; got != "-10" {
		t.Errorf("row1[short_over_qty] = %q, want -10", got)
	}
}

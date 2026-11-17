package xlsxexport

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWriteTo_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"receipt_no", "status"}
	rows := [][]string{
		{"RCV-000001", "COMPLETED"},
		{"RCV-000002", "PARTIALLY_RECEIVED"},
	}

	if err := WriteTo(&buf, "Receiving Report", headers, rows); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer f.Close()

	sheetRows, err := f.GetRows("Receiving Report")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(sheetRows) != 3 {
		t.Fatalf("len(sheetRows) = %d, want 3 (header + 2 data rows)", len(sheetRows))
	}
	if sheetRows[0][0] != "receipt_no" || sheetRows[0][1] != "status" {
		t.Errorf("header row = %v, want [receipt_no status]", sheetRows[0])
	}
	if sheetRows[1][0] != "RCV-000001" || sheetRows[1][1] != "COMPLETED" {
		t.Errorf("row 1 = %v, want [RCV-000001 COMPLETED]", sheetRows[1])
	}
	if sheetRows[2][0] != "RCV-000002" || sheetRows[2][1] != "PARTIALLY_RECEIVED" {
		t.Errorf("row 2 = %v, want [RCV-000002 PARTIALLY_RECEIVED]", sheetRows[2])
	}
}

func TestWriteTo_EmptyRows_StillWritesHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTo(&buf, "", []string{"a", "b"}, nil); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer f.Close()

	sheetRows, err := f.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(sheetRows) != 1 {
		t.Fatalf("len(sheetRows) = %d, want 1 (header only)", len(sheetRows))
	}
}

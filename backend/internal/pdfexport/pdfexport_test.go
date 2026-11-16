package pdfexport

import (
	"bytes"
	"testing"
	"time"
)

// TestDocument_BarcodeAndQRCode_ProduceValidPDF is Phase 33B's own smoke
// test for this file's barcode/QR addition (the first caller is
// internal/picking's Pick List) — proves Barcode/QRCode render without
// error, advance the layout as documented (CursorY/Spacer/QRCaption),
// and the resulting document still finalizes to non-empty, well-formed
// PDF bytes.
func TestDocument_BarcodeAndQRCode_ProduceValidPDF(t *testing.T) {
	doc := NewDocument(Meta{
		CompanyName: "Test Co", DocTitle: "Test Document", DocNo: "DOC-0001",
		GeneratedAt: time.Now(), GeneratedBy: "tester",
	})

	startY := doc.CursorY()
	if err := doc.Barcode("PICK-000001", 12, startY, 70, 16); err != nil {
		t.Fatalf("Barcode: %v", err)
	}
	doc.Spacer(20)
	afterBarcodeY := doc.CursorY()
	if afterBarcodeY <= startY {
		t.Errorf("CursorY after Spacer = %v, want > %v (Spacer must advance the cursor)", afterBarcodeY, startY)
	}

	qrY := doc.CursorY()
	if err := doc.QRCode("A-01-01|SKU-0001|LOT-0001", 12, qrY, 20); err != nil {
		t.Fatalf("QRCode: %v", err)
	}
	doc.QRCaption(20, "SKU-0001 @ A-01-01")
	afterQRY := doc.CursorY()
	if afterQRY <= qrY {
		t.Errorf("CursorY after QRCaption = %v, want > %v (QRCaption must advance the cursor past the QR+caption)", afterQRY, qrY)
	}

	// Rendering the exact same content a second time must not error —
	// RegisterImageOptionsReader's own cache-by-name behavior (see this
	// file's own doc) must tolerate a repeated name within one document.
	if err := doc.Barcode("PICK-000001", 12, doc.CursorY(), 70, 16); err != nil {
		t.Fatalf("Barcode (repeated content): %v", err)
	}

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Bytes() returned empty PDF")
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Errorf("Bytes() does not start with a PDF header, got: %q", data[:minInt(20, len(data))])
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

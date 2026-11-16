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

// TestDocument_MarkDraft_RendersWatermarkWithoutError is Phase 33C.3's
// own smoke test for the DRAFT watermark primitive (the first caller is
// internal/packing's Packing List, for a job not yet checker-confirmed)
// — proves MarkDraft can be called immediately after NewDocument (i.e.
// against the page NewDocument already added, before any caller code
// runs) and after a later AddPage, without erroring, and that IsDraft
// reports the flag back accurately. fpdf's own PDF stream is not
// plaintext-searchable for "DRAFT" the way a plain-text format would
// be (the string renders as glyph-positioning operators, not literal
// bytes) — see internal/packing's own watermark test for how this
// package instead asserts the draft/non-draft code path distinction via
// byte-length and IsDraft rather than string-searching the PDF body.
func TestDocument_MarkDraft_RendersWatermarkWithoutError(t *testing.T) {
	doc := NewDocument(Meta{
		CompanyName: "Test Co", DocTitle: "Test Document", DocNo: "DOC-0002",
		GeneratedAt: time.Now(), GeneratedBy: "tester",
	})
	if doc.IsDraft() {
		t.Fatal("IsDraft() = true before MarkDraft was ever called")
	}

	doc.MarkDraft()
	if !doc.IsDraft() {
		t.Fatal("IsDraft() = false immediately after MarkDraft")
	}

	// A second page must also pick up the watermark via drawHeader's own
	// draft check, not just the page that existed when MarkDraft ran.
	doc.AddPage()
	doc.SectionTitle("Second Page")

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("MarkDraft'd document did not finalize to a valid, non-empty PDF")
	}
}

// TestDocument_MarkDraft_OnLabelDocument proves the same primitive works
// against NewLabelDocument's compact/custom-size layout, not just A4 —
// Carton Label's own small-format page must be able to use this too if
// a future caller needs it, even though Phase 33C.3 itself only applies
// MarkDraft to the A4 Packing List.
func TestDocument_MarkDraft_OnLabelDocument(t *testing.T) {
	doc := NewLabelDocument(Meta{
		CompanyName: "Test Co", DocTitle: "Test Label", DocNo: "LBL-0001",
		GeneratedAt: time.Now(), GeneratedBy: "tester",
	}, 101.6, 152.4)
	doc.MarkDraft()

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if len(data) == 0 || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("MarkDraft'd label document did not finalize to a valid, non-empty PDF")
	}
}

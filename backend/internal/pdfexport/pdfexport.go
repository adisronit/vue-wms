// Package pdfexport is Phase 31D's shared PDF-rendering engine — the
// low-level layout primitives (fonts, page header/footer, tables,
// signature blocks) every inbound document (see internal/inbounddoc)
// builds on. It owns no business data and no DB access: callers hand it
// already-fetched, already-tenant-scoped strings and numbers, and it
// only ever turns those into bytes.
//
// Library choice: github.com/go-pdf/fpdf (an actively maintained fork
// of jung-kurt/gofpdf), confirmed via a real rendering proof-of-concept
// (Thai combining vowels/tone marks + digits, verified visually via
// pdftoppm) before being added as a dependency — pure Go, no Chromium/
// wkhtmltopdf dependency, so the Docker image needs nothing beyond this
// module and the embedded font below.
//
// Font: Noto Sans Thai (SIL Open Font License, see fonts/OFL.txt),
// instanced from Google Fonts' variable font (wght=400/700) via
// fonttools — NOT a Windows system font, which cannot be redistributed
// in the Docker image. Embedded directly into the binary via go:embed
// so the running container needs no separate font volume/mount.
package pdfexport

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"strconv"
	"strings"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/qr"
	"github.com/go-pdf/fpdf"
)

//go:embed fonts/NotoSansThai-Regular.ttf
var fontRegular []byte

//go:embed fonts/NotoSansThai-Bold.ttf
var fontBold []byte

const (
	fontFamily = "Noto"
	pageWidth  = 210.0 // A4, mm — NewDocument's own default page width
	marginside = 12.0
	marginTop  = 38.0 // room for the repeated header block
	marginBot  = 18.0 // room for the footer

	// Label-format constants (Phase 33C's Carton/Pallet Label, the first
	// caller needing anything other than A4) — a compact header (no
	// repeated company-name line, far smaller top margin) and footer
	// sized for a small-format page, not a scaled-down A4 layout.
	labelMarginSide = 4.0
	labelMarginTop  = 14.0
	labelMarginBot  = 8.0
)

// Meta is the document-identifying information every generated PDF's
// header/footer shows — the same fields regardless of document type
// (GRN, QC Inspection Report, Putaway Task Sheet, ...), set once per
// document via NewDocument.
type Meta struct {
	CompanyName string
	DocTitle    string // e.g. "Goods Receiving Note (GRN)"
	DocNo       string // e.g. "RCV-000001"
	GeneratedAt time.Time
	GeneratedBy string // display name of the user who requested the PDF
}

// Document wraps *fpdf.Fpdf with this codebase's own layout
// conventions. Every method panics-free; rendering errors surface only
// from Bytes() via fpdf's own deferred error state, matching fpdf's own
// "check err once at the end" design.
//
// pageWidth/marginSide/marginTop/marginBot (Phase 33C) are per-instance
// fields, not the package-level A4 constants directly, so NewLabelDocument
// can lay out a small-format page using the exact same Table/KeyValueGrid/
// SignatureBlock/Barcode/QRCode primitives — NewDocument still defaults
// them to the original A4 constants, so its own behavior (and every
// pre-33C caller's output) is completely unchanged.
type Document struct {
	pdf        *fpdf.Fpdf
	meta       Meta
	pageWidth  float64
	marginSide float64
	marginTop  float64
	marginBot  float64
	// compact switches drawHeader/drawFooter to the small-format layout
	// NewLabelDocument needs (no repeated company-name line, far smaller
	// margins) — false for every NewDocument caller, unchanged.
	compact bool
	// draft (Phase 33C.3) flags this document as not-yet-final — see
	// MarkDraft's own doc. False (no watermark) for every document that
	// never calls MarkDraft, so every pre-existing caller's output is
	// completely unchanged.
	draft bool
}

// NewDocument starts a fresh A4 portrait document with meta's header
// repeating on every page (including page 1 — fpdf calls headerFunc
// once per AddPage, and the first page is added below) and a page
// number footer ("Page X of Y", via AliasNbPages so the total is known
// even while the first page is being drawn).
func NewDocument(meta Meta) *Document {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(fontFamily, "", fontRegular)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", fontBold)
	pdf.SetMargins(marginside, marginTop, marginside)
	pdf.SetAutoPageBreak(true, marginBot)
	pdf.AliasNbPages("")

	d := &Document{pdf: pdf, meta: meta, pageWidth: pageWidth, marginSide: marginside, marginTop: marginTop, marginBot: marginBot}
	pdf.SetHeaderFunc(d.drawHeader)
	pdf.SetFooterFunc(d.drawFooter)
	pdf.AddPage()
	return d
}

// NewLabelDocument starts a custom-size document — e.g. a 4x6" shipping-
// label format (101.6mm x 152.4mm) — with a compact header/footer sized
// for a small page, added in Phase 33C for Packing's Carton/Pallet Label,
// the first caller needing anything other than A4. Orientation is
// inferred from widthMM/heightMM (landscape when wider than tall).
// AddPage (for a second/third label in the same document — e.g. one per
// carton) reuses the exact same custom size automatically: fpdf's own
// AddPage keeps whatever size NewCustom established as the document
// default.
func NewLabelDocument(meta Meta, widthMM, heightMM float64) *Document {
	orientation := "P"
	if widthMM > heightMM {
		orientation = "L"
	}
	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: orientation,
		UnitStr:        "mm",
		Size:           fpdf.SizeType{Wd: widthMM, Ht: heightMM},
	})
	pdf.AddUTF8FontFromBytes(fontFamily, "", fontRegular)
	pdf.AddUTF8FontFromBytes(fontFamily, "B", fontBold)
	pdf.SetMargins(labelMarginSide, labelMarginTop, labelMarginSide)
	pdf.SetAutoPageBreak(true, labelMarginBot)
	pdf.AliasNbPages("")

	d := &Document{
		pdf: pdf, meta: meta, pageWidth: widthMM,
		marginSide: labelMarginSide, marginTop: labelMarginTop, marginBot: labelMarginBot,
		compact: true,
	}
	pdf.SetHeaderFunc(d.drawHeader)
	pdf.SetFooterFunc(d.drawFooter)
	pdf.AddPage()
	return d
}

func (d *Document) drawHeader() {
	pdf := d.pdf
	if d.compact {
		pdf.SetXY(d.marginSide, 3)
		pdf.SetFont(fontFamily, "B", 9)
		pdf.CellFormat(0, 4, d.meta.CompanyName, "", 1, "L", false, 0, "")
		pdf.SetX(d.marginSide)
		pdf.SetFont(fontFamily, "B", 8)
		pdf.CellFormat(0, 4, d.meta.DocTitle+" — "+d.meta.DocNo, "", 1, "L", false, 0, "")
		pdf.SetDrawColor(120, 120, 120)
		pdf.Line(d.marginSide, 11, d.pageWidth-d.marginSide, 11)
		if d.draft {
			d.drawWatermark()
		}
		return
	}

	pdf.SetXY(d.marginSide, 10)
	pdf.SetFont(fontFamily, "B", 14)
	pdf.CellFormat(0, 7, d.meta.CompanyName, "", 1, "L", false, 0, "")

	pdf.SetX(d.marginSide)
	pdf.SetFont(fontFamily, "B", 12)
	pdf.CellFormat(0, 6, d.meta.DocTitle, "", 1, "L", false, 0, "")

	pdf.SetX(d.marginSide)
	pdf.SetFont(fontFamily, "", 9)
	pdf.CellFormat(0, 5, fmt.Sprintf("Document No.: %s    Generated: %s UTC", d.meta.DocNo, d.meta.GeneratedAt.UTC().Format("2006-01-02 15:04:05")), "", 1, "L", false, 0, "")

	pdf.SetDrawColor(120, 120, 120)
	pdf.Line(d.marginSide, 30, d.pageWidth-d.marginSide, 30)

	if d.draft {
		d.drawWatermark()
	}
}

// MarkDraft (Phase 33C.3) flags this document as not yet final — a
// diagonal "DRAFT" banner is drawn across every page, including the
// page already added by NewDocument/NewLabelDocument before the caller
// gets a chance to call this (drawn immediately, once, right here) and
// every subsequent page (drawn automatically by drawHeader, which fpdf
// already calls once per AddPage — see NewDocument's own doc). Added
// for internal/packing's own Packing List: a job that has not yet
// reached checker-confirmed COMPLETED must never be mistaken for the
// FINAL document. Uses fpdf's own rotation/alpha-blending primitives
// directly (TransformRotate + SetAlpha) rather than a pre-rendered
// watermark image, so no extra asset is needed and the banner always
// matches the page's own current size/orientation exactly.
func (d *Document) MarkDraft() {
	d.draft = true
	d.drawWatermark()
}

// IsDraft reports whether MarkDraft has been called — callers use this
// to decide what to record as document_renders.is_draft (see
// internal/docrender's own doc) without having to track the same
// boolean themselves a second time.
func (d *Document) IsDraft() bool {
	return d.draft
}

// drawWatermark renders one diagonal, semi-transparent "DRAFT" banner
// across the CURRENT page only — called once immediately by MarkDraft
// (for the page that already existed before MarkDraft was called) and
// once per subsequent AddPage via drawHeader's own draft check.
func (d *Document) drawWatermark() {
	pdf := d.pdf
	_, pageHeight := pdf.GetPageSize()
	centerX := d.pageWidth / 2
	centerY := pageHeight / 2

	fontSize := 60.0
	if d.compact {
		fontSize = 22.0
	}

	alpha, blendMode := pdf.GetAlpha()
	r, g, b := pdf.GetTextColor()
	pdf.SetAlpha(0.22, "Normal")
	pdf.SetTextColor(200, 30, 30)
	pdf.SetFont(fontFamily, "B", fontSize)

	const text = "DRAFT"
	strWidth := pdf.GetStringWidth(text)
	pdf.TransformBegin()
	pdf.TransformRotate(45, centerX, centerY)
	pdf.Text(centerX-strWidth/2, centerY, text)
	pdf.TransformEnd()

	pdf.SetAlpha(alpha, blendMode)
	pdf.SetTextColor(r, g, b)
	pdf.SetFont(fontFamily, "", 9)
}

func (d *Document) drawFooter() {
	pdf := d.pdf
	if d.compact {
		pdf.SetY(-7)
		pdf.SetFont(fontFamily, "", 6)
		pdf.CellFormat(0, 4, fmt.Sprintf("%d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
		return
	}

	pdf.SetY(-15)
	pdf.SetFont(fontFamily, "", 8)
	pdf.CellFormat(0, 5, fmt.Sprintf("Generated by %s", d.meta.GeneratedBy), "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 5, fmt.Sprintf("Page %d of {nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
}

// SectionTitle renders a bold section heading (e.g. "Receiving
// Details", "QC Dispositions").
func (d *Document) SectionTitle(text string) {
	d.pdf.Ln(3)
	d.pdf.SetFont(fontFamily, "B", 11)
	d.pdf.SetX(d.marginSide)
	d.pdf.CellFormat(0, 6, text, "", 1, "L", false, 0, "")
	d.pdf.SetFont(fontFamily, "", 9)
}

// KeyValueGrid renders pairs as a label/value grid, cols per row (e.g.
// 2), for a document's header fields (Receipt No., Vendor, Warehouse,
// ...). Empty values are rendered as "-" so a blank optional field
// never reads as a layout glitch.
func (d *Document) KeyValueGrid(pairs [][2]string, cols int) {
	pdf := d.pdf
	fontSize := 9.0
	rowHeight := 5.5
	if d.compact {
		fontSize = 7
		rowHeight = 3.8
	}
	colWidth := (d.pageWidth - 2*d.marginSide) / float64(cols)
	labelWidth := colWidth * 0.38
	valueWidth := colWidth - labelWidth

	pdf.SetFont(fontFamily, "", fontSize)
	for i, pair := range pairs {
		if i%cols == 0 {
			pdf.SetX(d.marginSide)
		}
		value := pair[1]
		if value == "" {
			value = "-"
		}
		pdf.SetFont(fontFamily, "B", fontSize)
		pdf.CellFormat(labelWidth, rowHeight, pair[0]+":", "", 0, "L", false, 0, "")
		pdf.SetFont(fontFamily, "", fontSize)
		ln := 0
		if (i+1)%cols == 0 || i == len(pairs)-1 {
			ln = 1
		}
		pdf.CellFormat(valueWidth, rowHeight, value, "", ln, "L", false, 0, "")
	}
	pdf.Ln(1)
}

// TableColumn describes one column of a Table.
type TableColumn struct {
	Header string
	Width  float64 // mm; the row of widths should sum to <= page content width
	Align  string  // "L", "C", or "R" — defaults to "L"
}

const tableRowHeight = 6.0
const tableHeaderHeight = 7.0

// Table renders a header row (bold, shaded) followed by data rows,
// repeating the column header whenever fpdf's auto page break starts a
// new page partway through — fpdf's own SetAutoPageBreak only repeats
// the document header (drawHeader), not column headers, so this checks
// remaining space itself before every row.
func (d *Document) Table(columns []TableColumn, rows [][]string) {
	pdf := d.pdf
	_, pageHeight := pdf.GetPageSize()
	bottomLimit := pageHeight - d.marginBot

	drawColumnHeader := func() {
		pdf.SetFont(fontFamily, "B", 8.5)
		pdf.SetFillColor(230, 230, 230)
		pdf.SetX(d.marginSide)
		for _, col := range columns {
			align := col.Align
			if align == "" {
				align = "L"
			}
			pdf.CellFormat(col.Width, tableHeaderHeight, col.Header, "1", 0, align, true, 0, "")
		}
		pdf.Ln(tableHeaderHeight)
		pdf.SetFont(fontFamily, "", 8.5)
	}

	drawColumnHeader()
	for _, row := range rows {
		if pdf.GetY()+tableRowHeight > bottomLimit {
			pdf.AddPage()
			drawColumnHeader()
		}
		pdf.SetX(d.marginSide)
		for i, cell := range row {
			if i >= len(columns) {
				break
			}
			align := columns[i].Align
			if align == "" {
				align = "L"
			}
			pdf.CellFormat(columns[i].Width, tableRowHeight, cell, "1", 0, align, false, 0, "")
		}
		pdf.Ln(tableRowHeight)
	}
	if len(rows) == 0 {
		pdf.SetX(d.marginSide)
		pdf.CellFormat(0, tableRowHeight, "(no rows)", "1", 1, "L", false, 0, "")
	}
	pdf.Ln(2)
}

// SignatureBlock renders one row of signature lines (e.g. "Received
// By", "Inspected By", "Approved By") near the end of a document — a
// blank line to sign above a printed label, one column per label.
func (d *Document) SignatureBlock(labels []string) {
	pdf := d.pdf
	_, pageHeight := pdf.GetPageSize()
	needed := 22.0
	if pdf.GetY()+needed > pageHeight-d.marginBot {
		pdf.AddPage()
	}
	pdf.Ln(10)
	colWidth := (d.pageWidth - 2*d.marginSide) / float64(len(labels))
	lineY := pdf.GetY() + 12
	for i, label := range labels {
		x := d.marginSide + float64(i)*colWidth
		pdf.Line(x+4, lineY, x+colWidth-4, lineY)
		pdf.SetXY(x, lineY+1.5)
		pdf.SetFont(fontFamily, "", 8.5)
		pdf.CellFormat(colWidth, 5, label, "", 0, "C", false, 0, "")
	}
	pdf.SetXY(d.marginSide, lineY+8)
}

// Library choice for Barcode/QR (Phase 33B, the first caller — Picking's
// Pick List): github.com/boombuler/barcode, a pure-Go, dependency-light
// 1D/2D barcode encoder with no Cgo and no external renderer, matching
// this package's own "pure Go, no Chromium/wkhtmltopdf" constraint — the
// same reasoning that chose go-pdf/fpdf itself. It encodes directly to
// an image.Image (barcode.Barcode embeds image.Image), which this
// package then PNG-encodes in memory and registers with fpdf via
// RegisterImageOptionsReader/ImageOptions — no temp files, no disk I/O.
// Code128 for the 1D case (any ASCII pick/order number — no fixed-length
// or numeric-only restriction the way Code39/EAN would impose) and
// standard QR (error-correction level M, auto-encoding mode) for the
// 2D case (enough payload for "location|product|lot" style scan
// strings). Registered images are cached by a content-derived name, so
// rendering the exact same code twice within one document (unlikely,
// but possible if two lines truly share location+product+lot) embeds
// the PNG bytes only once.

// Barcode renders data as a Code128 barcode at (x, y), sized wMM x hMM
// (both in mm — this package's own unit throughout), on the current
// page. Generated at a fixed, scanner-friendly pixel resolution
// (300x80px) regardless of the requested mm size — fpdf stretches a
// raster image to whatever box ImageOptions is given, so the mm size
// only controls layout, never scan quality.
func (d *Document) Barcode(data string, x, y, wMM, hMM float64) error {
	bc, err := code128.Encode(data)
	if err != nil {
		return err
	}
	return d.placeBarcodeImage("barcode:"+data, bc, 300, 80, x, y, wMM, hMM)
}

// QRCode renders data as a QR code at (x, y), sized sizeMM x sizeMM, on
// the current page. Generated at a fixed 200x200px resolution.
func (d *Document) QRCode(data string, x, y, sizeMM float64) error {
	bc, err := qr.Encode(data, qr.M, qr.Auto)
	if err != nil {
		return err
	}
	return d.placeBarcodeImage("qr:"+data, bc, 200, 200, x, y, sizeMM, sizeMM)
}

// placeBarcodeImage scales bc to widthPx x heightPx, PNG-encodes it into
// memory, registers it with fpdf under imgName (a content-derived name,
// so repeating the exact same code within one document re-uses the
// already-embedded PNG bytes instead of duplicating them), and places
// it on the current page at (x, y) sized wMM x hMM.
func (d *Document) placeBarcodeImage(imgName string, bc barcode.Barcode, widthPx, heightPx int, x, y, wMM, hMM float64) error {
	scaled, err := barcode.Scale(bc, widthPx, heightPx)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return err
	}
	d.pdf.RegisterImageOptionsReader(imgName, fpdf.ImageOptions{ImageType: "PNG"}, &buf)
	d.pdf.ImageOptions(imgName, x, y, wMM, hMM, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	return nil
}

// hexToRGB parses a "#rrggbb" (or "rrggbb") string into its r/g/b
// components, defaulting to black on any parse failure — callers
// (Rect) always have a safe fallback rather than a rendering error
// over a malformed color string.
func hexToRGB(hex string) (r, g, b int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return int(v >> 16 & 0xFF), int(v >> 8 & 0xFF), int(v & 0xFF)
}

// Rect draws one filled-and-stroked rectangle at absolute page
// coordinates (mm), the core primitive Phase 34I's Interactive 2D
// Warehouse Layout PDF export needs that this package did not
// previously expose (every prior caller only ever needed text/table
// layout, never arbitrary shapes). fillHex/strokeHex are "#rrggbb" —
// pass "" for either to skip that fill/stroke (an unfilled, outline-
// only rect, or vice versa). label, if non-empty, is centered inside
// the rect at fontSize (mm-equivalent pt, same unit every other text
// primitive in this file already uses).
func (d *Document) Rect(x, y, w, h float64, fillHex, strokeHex, label string, fontSize float64) {
	pdf := d.pdf
	style := ""
	if fillHex != "" {
		r, g, b := hexToRGB(fillHex)
		pdf.SetFillColor(r, g, b)
		style += "F"
	}
	if strokeHex != "" {
		r, g, b := hexToRGB(strokeHex)
		pdf.SetDrawColor(r, g, b)
		style += "D"
	}
	if style == "" {
		style = "D"
		pdf.SetDrawColor(120, 120, 120)
	}
	pdf.Rect(x, y, w, h, style)

	if label != "" {
		if fontSize <= 0 {
			fontSize = 7
		}
		pdf.SetFont(fontFamily, "", fontSize)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetXY(x, y+h/2-fontSize/2/2.83)
		pdf.CellFormat(w, fontSize/2.83, label, "", 0, "C", false, 0, "")
		pdf.SetFont(fontFamily, "", 9)
	}
}

// Image places a raster image (PNG or JPEG bytes, content detected
// automatically by fpdf from the data itself) at absolute page
// coordinates (mm), sized wMM x hMM — the generic counterpart to
// Barcode/QRCode's own fixed-content image placement, needed for
// Phase 34I's optional calibrated background image.
func (d *Document) Image(imgName string, data []byte, imageType string, x, y, wMM, hMM float64) {
	reader := bytes.NewReader(data)
	d.pdf.RegisterImageOptionsReader(imgName, fpdf.ImageOptions{ImageType: imageType}, reader)
	d.pdf.ImageOptions(imgName, x, y, wMM, hMM, false, fpdf.ImageOptions{ImageType: imageType}, 0, "")
}

// CursorY returns the document's current Y position (mm from the top of
// the page) — added in Phase 33B for Picking's Pick List, the first
// caller needing to place a non-flowing element (Barcode/QRCode, which
// take an explicit x/y rather than auto-flowing like Table/
// KeyValueGrid/SignatureBlock) at "wherever the cursor currently is"
// rather than a hardcoded position.
func (d *Document) CursorY() float64 {
	return d.pdf.GetY()
}

// Spacer advances the cursor down by mm — used after a non-flowing
// element (Barcode/QRCode) that, unlike every other primitive in this
// file, does not itself move the cursor.
func (d *Document) Spacer(mm float64) {
	d.pdf.Ln(mm)
}

// QRCaption prints a caption line positioned qrSizeMM below the cursor
// position the most recently placed QRCode was drawn at (QRCode's own
// placement is non-flowing — see ImageOptions' own doc — so the cursor
// has not moved since), then advances the cursor past the caption
// itself plus a small gap, ready for the next QRCode/caption pair.
func (d *Document) QRCaption(qrSizeMM float64, text string) {
	pdf := d.pdf
	pdf.SetXY(d.marginSide, pdf.GetY()+qrSizeMM+1)
	pdf.SetFont(fontFamily, "", 7.5)
	pdf.CellFormat(0, 4, text, "", 1, "L", false, 0, "")
	pdf.SetFont(fontFamily, "", 9)
	pdf.Ln(2)
}

// AddPage starts a new page in this document, reusing its own size/
// margins/header/footer — added in Phase 33C for a multi-carton Carton
// Label document (one label per page) built from NewLabelDocument;
// equally usable by any NewDocument-created (A4) document that needs an
// explicit page break beyond fpdf's own automatic one.
func (d *Document) AddPage() {
	d.pdf.AddPage()
}

// MarginX returns this document's own left/right margin (mm) — added in
// Phase 33C so a caller building a custom-size label document (see
// NewLabelDocument) can position Barcode/QRCode calls against the
// document's actual margin instead of a hardcoded A4 value.
func (d *Document) MarginX() float64 {
	return d.marginSide
}

// PageContentWidth returns this document's own usable horizontal space
// between its left and right margins — the per-instance equivalent of
// the package-level ContentWidth constant below, correct for a
// NewLabelDocument's custom page size as well as NewDocument's A4
// default.
func (d *Document) PageContentWidth() float64 {
	return d.pageWidth - 2*d.marginSide
}

// Bytes finalizes the document and returns the PDF content.
func (d *Document) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := d.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ContentWidth is the usable horizontal space between the left and
// right margins for a NewDocument-created (A4) document specifically —
// document-specific table column widths should sum to this. A
// NewLabelDocument-created document must use PageContentWidth() instead
// (its page is not A4-sized).
const ContentWidth = pageWidth - 2*marginside

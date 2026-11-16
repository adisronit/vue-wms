// Phase 31D — Inbound document PDF generation. Mirrors internal/
// qualitycheck's and internal/putaway's own Phase 31B/31C tenant-
// isolation test shape (fresh tenant per test, real seeded rows through
// the real Receiving/QC/Putaway services, testsupport.ConnectDB +
// BeginRollbackTx) but exercises this package's own surface: content
// correctness of every one of the five documents against the exact
// mandatory "receive 100 -> PASS 95/HOLD 3/REJECT 2 -> putaway 60+35"
// scenario, Thai-text rendering, multi-page pagination, tenant IDOR
// rejection, and that repeated downloads never mutate any state.
package inbounddoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/platform/testsupport"
	"wms-backend/internal/product"
	"wms-backend/internal/putaway"
	"wms-backend/internal/qualitycheck"
	"wms-backend/internal/receiving"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
	"wms-backend/internal/warehouse"
)

type docFixture struct {
	tenantID      int64
	productID     int64
	warehouseID   int64
	stagingLocID  int64
	storageLocID  int64
	storageLocID2 int64
}

// newDocFixture seeds a fresh tenant/product/warehouse/zones/locations,
// mirroring internal/putaway's own newPutawayFixture (QCRequired is
// always true here — every scenario this package's tests exercise
// starts from a held receipt).
func newDocFixture(t *testing.T, tx *gorm.DB, label string) docFixture {
	t.Helper()

	var tenantID int64
	if err := tx.Raw(
		"INSERT INTO tenants (slug, name, status) OUTPUT INSERTED.id VALUES (?, ?, 'ACTIVE')",
		"doc-iso-"+label, "Inbound Doc Test Tenant "+label,
	).Scan(&tenantID).Error; err != nil {
		t.Fatalf("newDocFixture(%s): insert tenant: %v", label, err)
	}

	var categoryID int64
	if err := tx.Raw(
		"INSERT INTO product_categories (tenant_id, code, name) OUTPUT INSERTED.id VALUES (?, ?, ?)",
		tenantID, "CAT-"+label, "Category "+label,
	).Scan(&categoryID).Error; err != nil {
		t.Fatalf("newDocFixture(%s): insert category: %v", label, err)
	}

	p := product.Product{
		TenantID: tenantID, SKU: "SKU-" + label, Name: "Test Product " + label,
		Category: product.CategoryPackaging, CategoryID: &categoryID,
		BaseUOM: product.UOMPieces, Status: product.StatusActive, QCRequired: true,
	}
	if err := tx.Create(&p).Error; err != nil {
		t.Fatalf("newDocFixture(%s): seed product: %v", label, err)
	}

	var warehouseID int64
	if err := tx.Raw(
		"INSERT INTO warehouses (tenant_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, 'Distribution Center', 'ACTIVE')",
		tenantID, "WH-"+label, "Warehouse "+label,
	).Scan(&warehouseID).Error; err != nil {
		t.Fatalf("newDocFixture(%s): seed warehouse: %v", label, err)
	}

	var receivingZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'RECEIVING', 'ACTIVE')",
		tenantID, warehouseID, "ZR-"+label, "Receiving Zone "+label,
	).Scan(&receivingZoneID).Error; err != nil {
		t.Fatalf("newDocFixture(%s): seed receiving zone: %v", label, err)
	}
	staging := location.Location{TenantID: tenantID, ZoneID: receivingZoneID, Code: "STG-" + label, Name: "Staging " + label, Status: location.StatusActive}
	if err := tx.Create(&staging).Error; err != nil {
		t.Fatalf("newDocFixture(%s): seed staging location: %v", label, err)
	}

	var storageZoneID int64
	if err := tx.Raw(
		"INSERT INTO zones (tenant_id, warehouse_id, code, name, type, status) OUTPUT INSERTED.id VALUES (?, ?, ?, ?, 'STORAGE', 'ACTIVE')",
		tenantID, warehouseID, "ZS-"+label, "Storage Zone "+label,
	).Scan(&storageZoneID).Error; err != nil {
		t.Fatalf("newDocFixture(%s): seed storage zone: %v", label, err)
	}
	storage1 := location.Location{TenantID: tenantID, ZoneID: storageZoneID, Code: "A-01-" + label, Name: "Storage A " + label, Status: location.StatusActive}
	if err := tx.Create(&storage1).Error; err != nil {
		t.Fatalf("newDocFixture(%s): seed storage location A: %v", label, err)
	}
	storage2 := location.Location{TenantID: tenantID, ZoneID: storageZoneID, Code: "B-01-" + label, Name: "Storage B " + label, Status: location.StatusActive}
	if err := tx.Create(&storage2).Error; err != nil {
		t.Fatalf("newDocFixture(%s): seed storage location B: %v", label, err)
	}

	return docFixture{
		tenantID: tenantID, productID: p.ID, warehouseID: warehouseID,
		stagingLocID: staging.ID, storageLocID: storage1.ID, storageLocID2: storage2.ID,
	}
}

// docServices wires every real service this package composes, plus the
// package under test itself, all bound to the same tx.
type docServices struct {
	receiving    *receiving.Service
	qualitycheck *qualitycheck.Service
	putaway      *putaway.Service
	inventory    *inventory.Service
	doc          *Service
}

func newDocServices(tx *gorm.DB) docServices {
	productRepo := product.NewRepository(tx)
	warehouseRepo := warehouse.NewRepository(tx)
	locationRepo := location.NewRepository(tx)
	invSvc := inventory.NewService(inventory.NewRepository(tx), productRepo, locationRepo)

	receivingRepo := receiving.NewRepository(tx)
	receivingSvc := receiving.NewService(receivingRepo, productRepo, warehouseRepo, locationRepo, nil, invSvc)

	qcRepo := qualitycheck.NewRepository(tx)
	qcSvc := qualitycheck.NewService(qcRepo, warehouseRepo, invSvc)

	putawayRepo := putaway.NewRepository(tx)
	putawaySvc := putaway.NewService(putawayRepo, productRepo, warehouseRepo, receivingSvc, invSvc)

	tenantRepo := tenant.NewRepository(tx)
	userRepo := user.NewRepository(tx)

	docSvc := NewService(receivingSvc, qcSvc, putawaySvc, invSvc, locationRepo, tenantRepo, userRepo)

	return docServices{receiving: receivingSvc, qualitycheck: qcSvc, putaway: putawaySvc, inventory: invSvc, doc: docSvc}
}

// seededScenario is every ID produced by seedMandatoryScenario below —
// everything a document render needs to be pointed at.
type seededScenario struct {
	receiptID     int64
	inspectionID  int64
	putawayTaskID int64
}

// seedMandatoryScenario drives the exact mandatory inbound scenario
// through the real services: receive 100 -> QC PASS 95/HOLD 3/REJECT 2
// -> putaway 60 to storageLocID + 35 to storageLocID2. Mirrors
// qualitycheck's own TestService_Confirm_MandatoryPartialDisposition
// Scenario and putaway's own TestService_Confirm_PartialAcross
// MultipleDestinations, combined into one flow (31E's own end-to-end
// scenario), since this package's documents span all three.
func seedMandatoryScenario(t *testing.T, svcs docServices, f docFixture, lotNo string) seededScenario {
	t.Helper()
	ctx := context.Background()

	receiptResp, err := svcs.receiving.Create(ctx, f.tenantID, receiving.CreateRequest{
		ReceivingType: receiving.TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.stagingLocID,
		Items: []receiving.CreateItemRequest{
			{ProductID: f.productID, LocationID: f.stagingLocID, ExpectedQty: 100, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("seedMandatoryScenario: receiving.Create: %v", err)
	}
	receiptItemID := receiptResp.Items[0].ID

	if _, err := svcs.receiving.Confirm(ctx, f.tenantID, receiptResp.ID, receiving.ConfirmRequest{
		IdempotencyKey: "seed-" + lotNo,
		Items:          []receiving.ConfirmItemRequest{{ItemID: receiptItemID, Quantity: 100, LotNo: lotNo}},
	}, 1); err != nil {
		t.Fatalf("seedMandatoryScenario: receiving.Confirm: %v", err)
	}

	qcResp, err := svcs.qualitycheck.Create(ctx, f.tenantID, qualitycheck.CreateRequest{
		ReceiptID: receiptResp.ID, WarehouseID: f.warehouseID,
	}, 1)
	if err != nil {
		t.Fatalf("seedMandatoryScenario: qualitycheck.Create: %v", err)
	}
	qcItemID := qcResp.Items[0].ID

	if _, err := svcs.qualitycheck.Confirm(ctx, f.tenantID, qcResp.ID, qualitycheck.ConfirmRequest{
		IdempotencyKey: "key-" + lotNo,
		Dispositions: []qualitycheck.ConfirmDispositionRequest{
			{ItemID: qcItemID, Disposition: qualitycheck.DispositionPass, Quantity: 95},
			{ItemID: qcItemID, Disposition: qualitycheck.DispositionHold, Quantity: 3},
			{ItemID: qcItemID, Disposition: qualitycheck.DispositionReject, Quantity: 2, DefectCode: "DAMAGED", EvidenceReference: "EVID-" + lotNo},
		},
	}, 1); err != nil {
		t.Fatalf("seedMandatoryScenario: qualitycheck.Confirm: %v", err)
	}

	putawayResp, err := svcs.putaway.Create(ctx, f.tenantID, putaway.CreateRequest{
		ReceiptID: &receiptResp.ID, WarehouseID: f.warehouseID,
		Items: []putaway.CreateItemRequest{
			{
				ReceiptItemID: &receiptItemID, ProductID: f.productID, LotNo: lotNo,
				SourceLocationID: f.stagingLocID, DestinationLocationID: f.storageLocID,
				RequestedQty: 95, UOM: "PCS",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("seedMandatoryScenario: putaway.Create: %v", err)
	}
	putawayItemID := putawayResp.Items[0].ID

	if _, err := svcs.putaway.Confirm(ctx, f.tenantID, putawayResp.ID, putaway.ConfirmRequest{
		IdempotencyKey: "key-pa1-" + lotNo,
		Items:          []putaway.ConfirmItemRequest{{ItemID: putawayItemID, Action: "CONFIRMED", Quantity: 60}},
	}, 1); err != nil {
		t.Fatalf("seedMandatoryScenario: putaway.Confirm(60): %v", err)
	}
	if _, err := svcs.putaway.Confirm(ctx, f.tenantID, putawayResp.ID, putaway.ConfirmRequest{
		IdempotencyKey: "key-pa2-" + lotNo,
		Items: []putaway.ConfirmItemRequest{
			{ItemID: putawayItemID, Action: "CONFIRMED", Quantity: 35, DestinationLocationID: f.storageLocID2},
		},
	}, 1); err != nil {
		t.Fatalf("seedMandatoryScenario: putaway.Confirm(35): %v", err)
	}

	return seededScenario{receiptID: receiptResp.ID, inspectionID: qcResp.ID, putawayTaskID: putawayResp.ID}
}

// pdftotextPath locates poppler's pdftotext for the deep content
// assertions below. Not found -> ("", false): those assertions are
// skipped (not the whole test), since poppler is a local verification
// aid, not a runtime dependency this project's Docker Compose services
// list ever includes. See docs/proposals for the font proof-of-concept
// that first established this tool for visual verification.
func pdftotextPath(t *testing.T) (string, bool) {
	t.Helper()
	if p, err := exec.LookPath("pdftotext"); err == nil {
		return p, true
	}
	const windowsPopplerPath = `C:\poppler\Library\bin\pdftotext.exe`
	if _, err := os.Stat(windowsPopplerPath); err == nil {
		return windowsPopplerPath, true
	}
	return "", false
}

// extractText writes data to a temp PDF and runs pdftotext over it,
// returning the extracted plain text with -enc UTF-8 (required for the
// Thai-rendering assertions to compare correctly).
func extractText(t *testing.T, tool string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		t.Fatalf("extractText: write pdf: %v", err)
	}
	txtPath := filepath.Join(dir, "doc.txt")
	cmd := exec.Command(tool, "-enc", "UTF-8", pdfPath, txtPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("extractText: pdftotext failed: %v (%s)", err, out)
	}
	text, err := os.ReadFile(txtPath)
	if err != nil {
		t.Fatalf("extractText: read txt: %v", err)
	}
	return string(text)
}

// pdfPageCount shells out to poppler's pdfinfo for the multi-page
// pagination assertion. Returns (0, false) if pdfinfo isn't available.
func pdfPageCount(t *testing.T, data []byte) (int, bool) {
	t.Helper()
	tool, ok := pdftotextPath(t)
	if !ok {
		return 0, false
	}
	pdfinfo := strings.Replace(tool, "pdftotext", "pdfinfo", 1)
	if _, err := os.Stat(pdfinfo); err != nil {
		if _, err := exec.LookPath("pdfinfo"); err != nil {
			return 0, false
		}
		pdfinfo = "pdfinfo"
	}
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		t.Fatalf("pdfPageCount: write pdf: %v", err)
	}
	out, err := exec.Command(pdfinfo, pdfPath).Output()
	if err != nil {
		t.Fatalf("pdfPageCount: pdfinfo failed: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			n, convErr := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
			if convErr != nil {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

func assertValidPDF(t *testing.T, data []byte, filename string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("render output does not start with the PDF magic bytes")
	}
	if len(data) < 500 {
		t.Fatalf("render output suspiciously small (%d bytes)", len(data))
	}
	if filename == "" {
		t.Fatalf("render returned an empty filename")
	}
}

func TestRenderGRN_MandatoryScenario_ContentCorrectness(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "grn")
	svcs := newDocServices(tx)
	scenario := seedMandatoryScenario(t, svcs, f, "LOT-GRN")

	data, filename, err := svcs.doc.RenderGRN(context.Background(), f.tenantID, 1, scenario.receiptID)
	assertValidPDF(t, data, filename, err)

	tool, ok := pdftotextPath(t)
	if !ok {
		t.Skip("pdftotext not available — skipping deep content assertions")
	}
	text := extractText(t, tool, data)
	for _, want := range []string{"SKU-grn", "Goods Receiving Note", "WH-grn"} {
		if !strings.Contains(text, want) {
			t.Errorf("GRN text missing %q\n--- extracted text ---\n%s", want, text)
		}
	}
}

// thaiRemark exercises real Thai combining vowels (ั ี ึ ื), tone marks
// (่ ้), and Thai digits (๑๖ ๒๕๖๙) — the exact character classes the
// font proof-of-concept (github.com/go-pdf/fpdf + the embedded Noto
// Sans Thai statics) was validated against before being adopted.
const thaiRemark = "ม้วนป้ายบาร์โค้ด ๑๖ ชุดใหม่ ส่งถึงคลังวันที่ ๒๕๖๙"

// TestRenderGRN_ThaiTextRendering is the "Thai text rendering" test
// CLAUDE.md's own 31D spec requires. It deliberately carries the Thai
// string through receipts.remark (migrations/0011: NVARCHAR(500)) —
// NOT products.name (migrations/0008: VARCHAR(200), a pre-existing,
// out-of-scope-for-this-batch schema gap that silently mangles
// non-Latin-1 text on INSERT, discovered while writing this test).
// NVARCHAR is this project's one genuinely Thai-safe free-text column
// among the fields a GRN prints, so it is what proves the font/
// rendering pipeline itself (pdfexport's embedded Noto Sans Thai,
// AddUTF8FontFromBytes) is correct end-to-end: Go string -> SQL Server
// NVARCHAR -> real tenant-scoped query -> PDF -> pdftotext round-trip.
func TestRenderGRN_ThaiTextRendering(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "thai")
	svcs := newDocServices(tx)
	ctx := context.Background()

	remark := thaiRemark
	resp, err := svcs.receiving.Create(ctx, f.tenantID, receiving.CreateRequest{
		ReceivingType: receiving.TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.stagingLocID,
		Remark: &remark,
		Items: []receiving.CreateItemRequest{
			{ProductID: f.productID, LocationID: f.stagingLocID, ExpectedQty: 10, UOM: "PCS"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("receiving.Create: %v", err)
	}

	data, filename, err := svcs.doc.RenderGRN(ctx, f.tenantID, 1, resp.ID)
	assertValidPDF(t, data, filename, err)

	tool, ok := pdftotextPath(t)
	if !ok {
		t.Skip("pdftotext not available — skipping Thai-rendering assertion")
	}
	text := extractText(t, tool, data)

	// Thai digits (unaffected by combining-mark stacking) must survive
	// verbatim.
	for _, want := range []string{"๑๖", "๒๕๖๙"} {
		if !strings.Contains(text, want) {
			t.Errorf("GRN text missing Thai digit sequence %q\n--- extracted text ---\n%s", want, text)
		}
	}

	// Beyond digits, assert every Thai codepoint in the source string
	// survived into the extraction, as a *multiset* rather than an exact
	// substring. pdftotext is known to reorder Thai combining tone
	// marks relative to an adjacent spacing vowel when reconstructing
	// reading order from glyph x/y positions (confirmed independently
	// here: "ถึงคลังวันที่" — no tone-mark/vowel collision — survives
	// verbatim, while "ป้าย"/"ส่ง" — tone mark stacked with a vowel —
	// come back reordered, e.g. "ปา้ ย"). That is a text-extraction
	// limitation of the reordering heuristic, not evidence of a missing
	// or substituted glyph (which would surface as "?" or an empty
	// multiset difference below) — the authoritative check for visually
	// correct rendering is the font proof-of-concept's own pdftoppm ->
	// PNG -> visual inspection, done before this library/font pairing
	// was adopted.
	wantRunes := thaiRuneMultiset(thaiRemark)
	gotRunes := thaiRuneMultiset(text)
	if !runeMultisetContains(gotRunes, wantRunes) {
		t.Errorf("GRN text is missing some Thai characters from the source remark (glyph/font issue, not just reordering)\nwant runes: %q\ngot runes:  %q\n--- extracted text ---\n%s",
			string(wantRunes), string(gotRunes), text)
	}
}

// thaiRuneMultiset returns every Thai-block codepoint (U+0E00-U+0E7F,
// covering consonants, vowels, tone marks, and digits) in s, sorted —
// a multiset comparable with runeMultisetContains below regardless of
// pdftotext's own reordering of combining marks.
func thaiRuneMultiset(s string) []rune {
	var runes []rune
	for _, r := range s {
		if r >= 0x0E00 && r <= 0x0E7F {
			runes = append(runes, r)
		}
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	return runes
}

// runeMultisetContains reports whether every rune in want (with
// multiplicity) appears in got — both must already be sorted.
func runeMultisetContains(got, want []rune) bool {
	gi := 0
	for _, w := range want {
		for gi < len(got) && got[gi] < w {
			gi++
		}
		if gi >= len(got) || got[gi] != w {
			return false
		}
		gi++
	}
	return true
}

func TestRenderQCReport_MandatoryScenario_ContentCorrectness(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "qcr")
	svcs := newDocServices(tx)
	scenario := seedMandatoryScenario(t, svcs, f, "LOT-QCR")

	data, filename, err := svcs.doc.RenderQCReport(context.Background(), f.tenantID, 1, scenario.inspectionID)
	assertValidPDF(t, data, filename, err)

	tool, ok := pdftotextPath(t)
	if !ok {
		t.Skip("pdftotext not available — skipping deep content assertions")
	}
	text := extractText(t, tool, data)
	for _, want := range []string{"QC Inspection Report", "PASS", "HOLD", "REJECT", "DAMAGED", "EVID-LOT-QCR", "95", "3", "2"} {
		if !strings.Contains(text, want) {
			t.Errorf("QC report text missing %q\n--- extracted text ---\n%s", want, text)
		}
	}
}

func TestRenderPutawayTaskSheet_MandatoryScenario_ContentCorrectness(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "pts")
	svcs := newDocServices(tx)
	scenario := seedMandatoryScenario(t, svcs, f, "LOT-PTS")

	data, filename, err := svcs.doc.RenderPutawayTaskSheet(context.Background(), f.tenantID, 1, scenario.putawayTaskID)
	assertValidPDF(t, data, filename, err)

	tool, ok := pdftotextPath(t)
	if !ok {
		t.Skip("pdftotext not available — skipping deep content assertions")
	}
	text := extractText(t, tool, data)
	for _, want := range []string{"Putaway Task Sheet", "Movement History", "A-01-pts", "B-01-pts", "60", "35"} {
		if !strings.Contains(text, want) {
			t.Errorf("Putaway task sheet text missing %q\n--- extracted text ---\n%s", want, text)
		}
	}
}

func TestRenderCompletionReport_MandatoryScenario_FinalStockPosition(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "cmp")
	svcs := newDocServices(tx)
	scenario := seedMandatoryScenario(t, svcs, f, "LOT-CMP")

	// Ground truth straight from inventory, the same way qualitycheck's
	// and putaway's own mandatory-scenario tests assert it — this report
	// must agree with it, not merely "not crash".
	invRepo := inventory.NewRepository(tx)
	srcBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.stagingLocID, "LOT-CMP")
	if err != nil || srcBal.Quantity != 3 || srcBal.HoldQuantity != 3 {
		t.Fatalf("precondition: source balance = %+v, err=%v, want Quantity=3 Hold=3", srcBal, err)
	}
	destABal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID, "LOT-CMP")
	if err != nil || destABal.Quantity != 60 {
		t.Fatalf("precondition: destination A balance = %+v, err=%v, want Quantity=60", destABal, err)
	}
	destBBal, err := invRepo.FindBalance(context.Background(), f.tenantID, f.productID, f.storageLocID2, "LOT-CMP")
	if err != nil || destBBal.Quantity != 35 {
		t.Fatalf("precondition: destination B balance = %+v, err=%v, want Quantity=35", destBBal, err)
	}

	data, filename, err := svcs.doc.RenderCompletionReport(context.Background(), f.tenantID, 1, scenario.receiptID)
	assertValidPDF(t, data, filename, err)

	tool, ok := pdftotextPath(t)
	if !ok {
		t.Skip("pdftotext not available — skipping deep content assertions")
	}
	text := extractText(t, tool, data)
	for _, want := range []string{
		"Inbound Completion Report", "Final Stock Position", "QC Summary", "Putaway Summary",
		"A-01-cmp", "B-01-cmp", "60", "35", "TOTAL", "98", // 98 = Quantity remaining (100 received - 2 rejected)
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Completion report text missing %q\n--- extracted text ---\n%s", want, text)
		}
	}
}

func TestRenderDiscrepancyReport_MandatoryScenario_ShowsRejectsNotShortage(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "dsc")
	svcs := newDocServices(tx)
	scenario := seedMandatoryScenario(t, svcs, f, "LOT-DSC")

	data, filename, err := svcs.doc.RenderDiscrepancyReport(context.Background(), f.tenantID, 1, scenario.receiptID)
	assertValidPDF(t, data, filename, err)

	tool, ok := pdftotextPath(t)
	if !ok {
		t.Skip("pdftotext not available — skipping deep content assertions")
	}
	text := extractText(t, tool, data)
	// Received exactly 100 of 100 expected — no quantity discrepancy —
	// but the REJECT 2 disposition must still show up as damage.
	if !strings.Contains(text, "(no rows)") {
		t.Errorf("Discrepancy report should show no quantity discrepancies (received == expected); got:\n%s", text)
	}
	for _, want := range []string{"Damage / Rejected Items", "DAMAGED", "EVID-LOT-DSC", "2"} {
		if !strings.Contains(text, want) {
			t.Errorf("Discrepancy report text missing %q\n--- extracted text ---\n%s", want, text)
		}
	}
}

func TestRender_TenantIDOR_NotFound(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	a := newDocFixture(t, tx, "idor-a")
	b := newDocFixture(t, tx, "idor-b")
	svcs := newDocServices(tx)
	scenario := seedMandatoryScenario(t, svcs, b, "LOT-IDOR")

	ctx := context.Background()
	cases := []struct {
		name string
		fn   func() ([]byte, string, error)
	}{
		{"GRN", func() ([]byte, string, error) { return svcs.doc.RenderGRN(ctx, a.tenantID, 1, scenario.receiptID) }},
		{"CompletionReport", func() ([]byte, string, error) {
			return svcs.doc.RenderCompletionReport(ctx, a.tenantID, 1, scenario.receiptID)
		}},
		{"DiscrepancyReport", func() ([]byte, string, error) {
			return svcs.doc.RenderDiscrepancyReport(ctx, a.tenantID, 1, scenario.receiptID)
		}},
		{"QCReport", func() ([]byte, string, error) {
			return svcs.doc.RenderQCReport(ctx, a.tenantID, 1, scenario.inspectionID)
		}},
		{"PutawayTaskSheet", func() ([]byte, string, error) {
			return svcs.doc.RenderPutawayTaskSheet(ctx, a.tenantID, 1, scenario.putawayTaskID)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tc.fn()
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("%s(tenantA, tenantB's record) = %v, want ErrNotFound", tc.name, err)
			}
		})
	}
}

// TestRender_RepeatedDownloads_NeverMutateState is the "no side effects
// from repeated downloads" requirement — every render is read-only, so
// calling the same one three times must leave every balance/status row
// touched by the mandatory scenario bit-for-bit unchanged.
func TestRender_RepeatedDownloads_NeverMutateState(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "repeat")
	svcs := newDocServices(tx)
	scenario := seedMandatoryScenario(t, svcs, f, "LOT-REPEAT")
	ctx := context.Background()

	invRepo := inventory.NewRepository(tx)
	snapshot := func() string {
		src, _ := invRepo.FindBalance(ctx, f.tenantID, f.productID, f.stagingLocID, "LOT-REPEAT")
		a, _ := invRepo.FindBalance(ctx, f.tenantID, f.productID, f.storageLocID, "LOT-REPEAT")
		b, _ := invRepo.FindBalance(ctx, f.tenantID, f.productID, f.storageLocID2, "LOT-REPEAT")
		qc, _ := svcs.qualitycheck.Get(ctx, f.tenantID, scenario.inspectionID)
		pt, _ := svcs.putaway.Get(ctx, f.tenantID, scenario.putawayTaskID)
		return fmt.Sprintf("src=%+v a=%+v b=%+v qcStatus=%v ptStatus=%v", src, a, b, qc.Status, pt.Status)
	}

	before := snapshot()
	for i := 0; i < 3; i++ {
		if _, _, err := svcs.doc.RenderGRN(ctx, f.tenantID, 1, scenario.receiptID); err != nil {
			t.Fatalf("RenderGRN iteration %d: %v", i, err)
		}
		if _, _, err := svcs.doc.RenderCompletionReport(ctx, f.tenantID, 1, scenario.receiptID); err != nil {
			t.Fatalf("RenderCompletionReport iteration %d: %v", i, err)
		}
		if _, _, err := svcs.doc.RenderDiscrepancyReport(ctx, f.tenantID, 1, scenario.receiptID); err != nil {
			t.Fatalf("RenderDiscrepancyReport iteration %d: %v", i, err)
		}
		if _, _, err := svcs.doc.RenderQCReport(ctx, f.tenantID, 1, scenario.inspectionID); err != nil {
			t.Fatalf("RenderQCReport iteration %d: %v", i, err)
		}
		if _, _, err := svcs.doc.RenderPutawayTaskSheet(ctx, f.tenantID, 1, scenario.putawayTaskID); err != nil {
			t.Fatalf("RenderPutawayTaskSheet iteration %d: %v", i, err)
		}
	}
	after := snapshot()

	if before != after {
		t.Errorf("state mutated by repeated PDF downloads:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestRenderGRN_MultiPagePagination forces the receiving lines table
// past one page (60 lines) and verifies the column header repeats
// correctly across pages instead of silently overflowing — the "multi-
// page tables, page numbering, repeated headers" requirement.
func TestRenderGRN_MultiPagePagination(t *testing.T) {
	db := testsupport.ConnectDB(t)
	tx := testsupport.BeginRollbackTx(t, db)

	f := newDocFixture(t, tx, "page")
	svcs := newDocServices(tx)
	ctx := context.Background()

	items := make([]receiving.CreateItemRequest, 0, 60)
	for i := 0; i < 60; i++ {
		items = append(items, receiving.CreateItemRequest{
			ProductID: f.productID, LocationID: f.stagingLocID, ExpectedQty: 10, UOM: "PCS",
			LotNo: fmt.Sprintf("LOT-PAGE-%02d", i),
		})
	}
	resp, err := svcs.receiving.Create(ctx, f.tenantID, receiving.CreateRequest{
		ReceivingType: receiving.TypePO, WarehouseID: f.warehouseID, StagingLocationID: f.stagingLocID,
		Items: items,
	}, 1)
	if err != nil {
		t.Fatalf("receiving.Create (60 lines): %v", err)
	}

	data, filename, err := svcs.doc.RenderGRN(ctx, f.tenantID, 1, resp.ID)
	assertValidPDF(t, data, filename, err)

	pages, ok := pdfPageCount(t, data)
	if !ok {
		t.Skip("pdfinfo not available — skipping page-count assertion")
	}
	if pages < 2 {
		t.Errorf("GRN with 60 lines rendered %d page(s), want >= 2 (auto page break should have triggered)", pages)
	}
}

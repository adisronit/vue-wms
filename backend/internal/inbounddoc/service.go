// Package inbounddoc is Phase 31D's PDF document layer for the inbound
// workflow (Receiving -> QC -> Putaway). It owns no table and no
// migration — every document is rendered live from already-tenant-
// scoped data read through Receiving/Quality Check/Putaway/Inventory's
// own existing Service methods, the same "narrow interface satisfied by
// another package's Service" composition pattern internal/dashboard
// already established, except every call here carries a real, enforced
// tenantID (dashboard's own callers still don't — see that package's
// own doc).
//
// Three of the five required documents (GRN, QC Inspection Report,
// Putaway Task Sheet) are each naturally owned by one domain; the other
// two (Inbound Completion Report, Discrepancy/Damage Report) genuinely
// span all three, which is why they live here instead of inside
// receiving/qualitycheck/putaway themselves (receiving cannot import
// qualitycheck or putaway without a cycle — putaway already imports
// receiving). Centralizing all five in one place, rather than splitting
// three into their owning packages and two here, keeps this one
// genuinely cohesive concern — "render the inbound workflow's
// documents" — in one place instead of scattered across three handlers
// plus a fourth.
//
// Every render is read-only: it only ever calls Get/List on the
// downstream services, never Confirm/Create/Update, so re-downloading
// any document any number of times can never move stock or change a
// workflow state.
package inbounddoc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"wms-backend/internal/inventory"
	"wms-backend/internal/location"
	"wms-backend/internal/pdfexport"
	"wms-backend/internal/platform/csvexport"
	"wms-backend/internal/putaway"
	"wms-backend/internal/qualitycheck"
	"wms-backend/internal/receiving"
	"wms-backend/internal/tenant"
	"wms-backend/internal/user"
)

// ErrNotFound is returned when the requested receipt/inspection/task
// does not exist for the caller's tenant — handler maps this to 404,
// indistinguishable from a cross-tenant ID the same way every other
// domain's own Get already is.
var ErrNotFound = errors.New("inbounddoc: document source not found")

// receivingGetter is the narrow slice of receiving.Service this package
// needs — just the single tenant-scoped Get every GRN/Completion/
// Discrepancy document starts from.
type receivingGetter interface {
	Get(ctx context.Context, tenantID, id int64) (*receiving.Response, error)
}

// qcReader is the narrow slice of qualitycheck.Service this package
// needs: Get for the QC Inspection Report itself, List(ReceiptID=...)
// to discover which inspections exist for a given receipt when
// composing the Completion/Discrepancy reports.
type qcReader interface {
	Get(ctx context.Context, tenantID, id int64) (*qualitycheck.Response, error)
	List(ctx context.Context, tenantID int64, params qualitycheck.ListParams) ([]qualitycheck.ListResponse, int64, error)
}

// putawayReader mirrors qcReader's shape for internal/putaway.
type putawayReader interface {
	Get(ctx context.Context, tenantID, id int64) (*putaway.Response, error)
	List(ctx context.Context, tenantID int64, params putaway.ListParams) ([]putaway.ListResponse, int64, error)
}

// balanceFinder is the one inventory.Service read the Completion Report
// needs for its "Final Stock Position" section — the live, authoritative
// Quantity/ReservedQuantity/HoldQuantity for each (product, location,
// lot) tuple the receipt actually touched, not a value reconstructed
// from the documents above (which could drift from the truth under a
// concurrent mutation between this report's own queries).
type balanceFinder interface {
	FindBalance(ctx context.Context, tenantID, productID, locationID int64, lotNo string) (*inventory.Inventory, error)
	// ListTransactionsByReference recovers every destination a putaway
	// task's line actually moved stock to over its lifetime — see that
	// method's own doc. putaway_task_items.destination_location_id
	// alone only ever holds the MOST RECENT destination for a line
	// confirmed across multiple destinations (Phase 31C's own
	// documented limitation), which would silently make the Completion
	// Report's Final Stock Position miss every earlier destination.
	ListTransactionsByReference(ctx context.Context, tenantID int64, referenceType, referenceID string) ([]inventory.Transaction, error)
}

// locationNamer resolves a location's display code for a ledger-derived
// stockTuple (see balanceFinder's own doc) whose code isn't already
// known from a putaway item response.
type locationNamer interface {
	FindByID(ctx context.Context, tenantID, id int64) (*location.Location, error)
}

// tenantNamer resolves the caller's own tenant display name for the PDF
// header — looking up your own enforced tenantID is always safe, no
// different from any other handler reading its own tenant row.
type tenantNamer interface {
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
}

// userNamer resolves the requesting user's display name for the PDF
// footer ("Generated by ...") — the narrow slice of internal/user.
// Repository this package needs.
type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// Service composes the four domains above into read-only PDF output.
type Service struct {
	receiving    receivingGetter
	qualitycheck qcReader
	putaway      putawayReader
	balances     balanceFinder
	locations    locationNamer
	tenants      tenantNamer
	users        userNamer
}

func NewService(receivingSvc receivingGetter, qualitycheckSvc qcReader, putawaySvc putawayReader, balances balanceFinder, locations locationNamer, tenants tenantNamer, users userNamer) *Service {
	return &Service{receiving: receivingSvc, qualitycheck: qualitycheckSvc, putaway: putawaySvc, balances: balances, locations: locations, tenants: tenants, users: users}
}

// locationCode resolves id's display code, falling back to "#id" if the
// lookup fails — a ledger-derived location should never make an entire
// report fail to render over a display-label nicety.
func (s *Service) locationCode(ctx context.Context, tenantID, id int64) string {
	loc, err := s.locations.FindByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Sprintf("#%d", id)
	}
	return loc.Code
}

func (s *Service) meta(ctx context.Context, tenantID, userID int64, title, docNo string) (pdfexport.Meta, error) {
	t, err := s.tenants.FindByID(ctx, tenantID)
	if err != nil {
		return pdfexport.Meta{}, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return pdfexport.Meta{}, err
	}
	return pdfexport.Meta{
		CompanyName: t.Name,
		DocTitle:    title,
		DocNo:       docNo,
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: u.FullName,
	}, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, receiving.ErrNotFound) || errors.Is(err, qualitycheck.ErrNotFound) || errors.Is(err, putaway.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

const (
	wSKU   = 24.0
	wName  = 42.0
	wLot   = 22.0
	wDate  = 20.0
	wQty   = 18.0
	wUOM   = 14.0
	wLoc   = 24.0
	wStat  = 20.0
	wSmall = 16.0
)

// RenderGRN builds the Goods Receiving Note — the receipt header plus
// its lines, plus a "Lot / Pallet Detail" section listing every
// receipt_item_splits row (Phase 31A) actually posted, since that is
// where lot/pallet/carton/temperature/condition detail lives once a
// line was received across more than one batch.
func (s *Service) RenderGRN(ctx context.Context, tenantID, userID, receiptID int64) ([]byte, string, error) {
	receipt, err := s.receiving.Get(ctx, tenantID, receiptID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}

	meta, err := s.meta(ctx, tenantID, userID, "Goods Receiving Note (GRN)", receipt.ReceiptNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	doc.KeyValueGrid([][2]string{
		{"Receipt No.", receipt.ReceiptNo},
		{"Status", string(receipt.Status)},
		{"Receiving Type", string(receipt.ReceivingType)},
		{"Reference No.", csvexport.FormatString(receipt.ReferenceNo)},
		{"Vendor", csvexport.FormatString(receipt.Vendor)},
		{"Supplier", csvexport.FormatString(receipt.SupplierName)},
		{"Warehouse", receipt.WarehouseCode},
		{"Staging Location", receipt.StagingLocation},
		{"Receiving Date", csvexport.FormatTime(receipt.ReceivingDate)},
		{"Invoice No.", csvexport.FormatString(receipt.InvoiceNo)},
		{"Delivery Note No.", csvexport.FormatString(receipt.DeliveryNoteNo)},
		{"Container No.", csvexport.FormatString(receipt.ContainerNo)},
		{"Seal No.", csvexport.FormatString(receipt.SealNo)},
		{"Transport Company", csvexport.FormatString(receipt.TransportCompany)},
		{"Driver Name", csvexport.FormatString(receipt.DriverName)},
		{"Vehicle Reg. No.", csvexport.FormatString(receipt.VehicleRegNo)},
		{"Remark", csvexport.FormatString(receipt.Remark)},
	}, 2)

	doc.SectionTitle("Receiving Lines")
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: wSKU}, {Header: "Product", Width: wName}, {Header: "Lot No.", Width: wLot},
		{Header: "Expiry", Width: wDate, Align: "C"}, {Header: "Expected", Width: wQty, Align: "R"},
		{Header: "Received", Width: wQty, Align: "R"}, {Header: "UOM", Width: wUOM, Align: "C"},
		{Header: "Status", Width: wStat, Align: "C"},
	}, grnLineRows(receipt.Items))

	var splitRows [][]string
	for _, item := range receipt.Items {
		for _, sp := range item.Splits {
			splitRows = append(splitRows, []string{
				item.SKU, sp.LotNo, csvexport.FormatTime(sp.ExpiryDate), csvexport.FormatFloat(sp.Quantity),
				sp.UOM, csvexport.FormatString(sp.PalletID), sp.LocationCode,
			})
		}
	}
	if len(splitRows) > 0 {
		doc.SectionTitle("Lot / Pallet Detail")
		doc.Table([]pdfexport.TableColumn{
			{Header: "SKU", Width: 26}, {Header: "Lot No.", Width: 28}, {Header: "Expiry", Width: 22, Align: "C"},
			{Header: "Quantity", Width: 20, Align: "R"}, {Header: "UOM", Width: 16, Align: "C"},
			{Header: "Pallet / LPN", Width: 40}, {Header: "Location", Width: 34, Align: "C"},
		}, splitRows)
	}

	doc.SignatureBlock([]string{"Received By", "Checked By", "Warehouse Supervisor"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("GRN_%s.pdf", receipt.ReceiptNo), err
}

func grnLineRows(items []receiving.ItemResponse) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{
			it.SKU, it.ProductName, it.LotNo, csvexport.FormatTime(it.ExpiryDate),
			csvexport.FormatFloat(it.ExpectedQty), csvexport.FormatFloat(it.ReceivedQty), it.UOM, string(it.Status),
		})
	}
	return rows
}

// RenderQCReport builds the QC Inspection Report — the inspection
// header, its checklist/sample items, and the full disposition history
// (PASS/HOLD/REJECT entries) per item.
func (s *Service) RenderQCReport(ctx context.Context, tenantID, userID, inspectionID int64) ([]byte, string, error) {
	insp, err := s.qualitycheck.Get(ctx, tenantID, inspectionID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}

	meta, err := s.meta(ctx, tenantID, userID, "QC Inspection Report", insp.InspectionNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	doc.KeyValueGrid([][2]string{
		{"Inspection No.", insp.InspectionNo},
		{"Status", string(insp.Status)},
		{"Receipt No.", insp.ReceiptNo},
		{"QC Type", string(insp.QCType)},
		{"Warehouse", insp.WarehouseCode},
		{"Inspector", csvexport.FormatString(insp.InspectorName)},
		{"Created At", csvexport.FormatTimeValue(insp.CreatedAt)},
		{"Remark", csvexport.FormatString(insp.Remark)},
	}, 2)

	doc.SectionTitle("Inspection Items")
	itemRows := make([][]string, 0, len(insp.Items))
	for _, item := range insp.Items {
		itemRows = append(itemRows, []string{
			item.SKU, item.LotNo, csvexport.FormatTime(item.ExpiryDate), item.LocationCode,
			csvexport.FormatFloat(item.HeldQtySnapshot), csvexport.FormatFloat(item.RemainingHeldQty),
			csvexport.FormatFloat(item.SampleQty), boolLabel(item.PackagingOK), boolLabel(item.LabelOK), string(item.Status),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 22}, {Header: "Lot No.", Width: 22}, {Header: "Expiry", Width: 18, Align: "C"},
		{Header: "Location", Width: 20, Align: "C"}, {Header: "Held Qty", Width: 18, Align: "R"},
		{Header: "Remaining", Width: 18, Align: "R"}, {Header: "Sample Qty", Width: 18, Align: "R"},
		{Header: "Packaging", Width: 16, Align: "C"}, {Header: "Label", Width: 16, Align: "C"}, {Header: "Status", Width: 18, Align: "C"},
	}, itemRows)

	doc.SectionTitle("Dispositions")
	var dispRows [][]string
	for _, item := range insp.Items {
		for _, d := range item.Dispositions {
			dispRows = append(dispRows, []string{
				item.SKU, item.LotNo, string(d.Disposition), csvexport.FormatFloat(d.Quantity),
				csvexport.FormatString(d.DefectCode), csvexport.FormatString(d.EvidenceReference),
				csvexport.FormatString(d.DecidedByName), csvexport.FormatTimeValue(d.CreatedAt),
			})
		}
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 22}, {Header: "Lot No.", Width: 22}, {Header: "Disposition", Width: 22, Align: "C"},
		{Header: "Qty", Width: 16, Align: "R"}, {Header: "Defect Code", Width: 22}, {Header: "Evidence Ref.", Width: 26},
		{Header: "Decided By", Width: 28}, {Header: "Decided At", Width: 28, Align: "C"},
	}, dispRows)

	doc.SignatureBlock([]string{"Inspector", "QC Supervisor"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("QC_Report_%s.pdf", insp.InspectionNo), err
}

func boolLabel(b *bool) string {
	if b == nil {
		return "-"
	}
	if *b {
		return "OK"
	}
	return "NOT OK"
}

// RenderPutawayTaskSheet builds the Putaway Task Sheet — the task
// header plus its lines (source -> destination, requested vs moved).
func (s *Service) RenderPutawayTaskSheet(ctx context.Context, tenantID, userID, taskID int64) ([]byte, string, error) {
	task, err := s.putaway.Get(ctx, tenantID, taskID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}

	meta, err := s.meta(ctx, tenantID, userID, "Putaway Task Sheet", task.PutawayNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	doc.KeyValueGrid([][2]string{
		{"Putaway No.", task.PutawayNo},
		{"Status", string(task.Status)},
		{"Receipt No.", csvexport.FormatString(task.ReceiptNo)},
		{"Priority", string(task.Priority)},
		{"Warehouse", task.WarehouseCode},
		{"Assigned To", csvexport.FormatString(task.AssignedTo)},
		{"Created At", csvexport.FormatTimeValue(task.CreatedAt)},
		{"Completed At", csvexport.FormatTime(task.CompletedAt)},
		{"Remark", csvexport.FormatString(task.Remark)},
	}, 2)

	doc.SectionTitle("Putaway Lines")
	rows := make([][]string, 0, len(task.Items))
	for _, item := range task.Items {
		rows = append(rows, []string{
			item.SKU, item.ProductName, item.LotNo, item.SourceLocationCode, item.DestinationLocationCode,
			csvexport.FormatFloat(item.RequestedQty), csvexport.FormatFloat(item.MovedQty), item.UOM, string(item.Status),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 20}, {Header: "Product", Width: 34}, {Header: "Lot No.", Width: 18},
		{Header: "Source", Width: 22, Align: "C"}, {Header: "Destination", Width: 24, Align: "C"},
		{Header: "Requested", Width: 18, Align: "R"}, {Header: "Moved", Width: 18, Align: "R"},
		{Header: "UOM", Width: 14, Align: "C"}, {Header: "Status", Width: 18, Align: "C"},
	}, rows)

	// Movement History, from the ledger rather than task.Items — a line
	// confirmed across multiple destinations (the 31E mandatory
	// scenario's own "60 to Location A, 35 to Location B") only ever
	// shows its MOST RECENT destination and cumulative MovedQty in the
	// line above (Phase 31C's own documented limitation); this section
	// is where every individual move this task actually made is still
	// visible. MoveAvailableStockTx always writes the negative (source)
	// leg immediately before the positive (destination) leg within the
	// same DB transaction, so consecutive pairs in id order are always
	// one move's two legs.
	doc.SectionTitle("Movement History")
	txns, err := s.balances.ListTransactionsByReference(ctx, tenantID, "PUTAWAY", task.PutawayNo)
	if err != nil {
		return nil, "", err
	}
	var moveRows [][]string
	for i := 0; i+1 < len(txns); i += 2 {
		from, to := txns[i], txns[i+1]
		sku := ""
		for _, item := range task.Items {
			if item.ProductID == to.ProductID && item.LotNo == to.LotNo {
				sku = item.SKU
				break
			}
		}
		moveRows = append(moveRows, []string{
			sku, to.LotNo, s.locationCode(ctx, tenantID, from.LocationID), s.locationCode(ctx, tenantID, to.LocationID),
			csvexport.FormatFloat(to.Quantity), csvexport.FormatTimeValue(to.CreatedAt),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 26}, {Header: "Lot No.", Width: 24}, {Header: "From", Width: 30, Align: "C"},
		{Header: "To", Width: 30, Align: "C"}, {Header: "Quantity", Width: 24, Align: "R"}, {Header: "When", Width: 52, Align: "C"},
	}, moveRows)

	doc.SignatureBlock([]string{"Putaway Operator", "Warehouse Supervisor"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("Putaway_%s.pdf", task.PutawayNo), err
}

// qcInspectionsForReceipt returns every inspection for receiptID, each
// resolved to full detail (with items/dispositions) — used by both the
// Completion and Discrepancy reports below.
func (s *Service) qcInspectionsForReceipt(ctx context.Context, tenantID, receiptID int64) ([]qualitycheck.Response, error) {
	list, _, err := s.qualitycheck.List(ctx, tenantID, qualitycheck.ListParams{ReceiptID: receiptID, Page: 1, Limit: 200})
	if err != nil {
		return nil, err
	}
	out := make([]qualitycheck.Response, 0, len(list))
	for _, row := range list {
		detail, err := s.qualitycheck.Get(ctx, tenantID, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, *detail)
	}
	return out, nil
}

// putawayTasksForReceipt mirrors qcInspectionsForReceipt for
// internal/putaway. List is item-level, so task IDs are deduplicated
// before resolving each one to full header+line detail.
func (s *Service) putawayTasksForReceipt(ctx context.Context, tenantID, receiptID int64) ([]putaway.Response, error) {
	list, _, err := s.putaway.List(ctx, tenantID, putaway.ListParams{ReceiptID: receiptID, Page: 1, Limit: 500})
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var taskIDs []int64
	for _, row := range list {
		if !seen[row.TaskID] {
			seen[row.TaskID] = true
			taskIDs = append(taskIDs, row.TaskID)
		}
	}
	sort.Slice(taskIDs, func(i, j int) bool { return taskIDs[i] < taskIDs[j] })

	out := make([]putaway.Response, 0, len(taskIDs))
	for _, id := range taskIDs {
		detail, err := s.putaway.Get(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *detail)
	}
	return out, nil
}

// stockTuple identifies one (product, location, lot) balance the
// Completion Report's Final Stock Position section should show.
type stockTuple struct {
	productID  int64
	locationID int64
	lotNo      string
	sku        string
	locCode    string
}

// RenderCompletionReport builds the Inbound Completion Report for one
// receipt — Receiving/QC/Putaway all summarized together, plus a Final
// Stock Position section reading live inventory balances (never
// reconstructed from the documents above, which could have drifted
// from the truth under a concurrent mutation between queries).
func (s *Service) RenderCompletionReport(ctx context.Context, tenantID, userID, receiptID int64) ([]byte, string, error) {
	receipt, err := s.receiving.Get(ctx, tenantID, receiptID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}
	inspections, err := s.qcInspectionsForReceipt(ctx, tenantID, receiptID)
	if err != nil {
		return nil, "", err
	}
	tasks, err := s.putawayTasksForReceipt(ctx, tenantID, receiptID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Inbound Completion Report", receipt.ReceiptNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	doc.KeyValueGrid([][2]string{
		{"Receipt No.", receipt.ReceiptNo},
		{"Status", string(receipt.Status)},
		{"Vendor", csvexport.FormatString(receipt.Vendor)},
		{"Supplier", csvexport.FormatString(receipt.SupplierName)},
		{"Warehouse", receipt.WarehouseCode},
		{"Receiving Date", csvexport.FormatTime(receipt.ReceivingDate)},
	}, 2)

	doc.SectionTitle("Receiving Lines")
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: wSKU}, {Header: "Product", Width: wName}, {Header: "Lot No.", Width: wLot},
		{Header: "Expiry", Width: wDate, Align: "C"}, {Header: "Expected", Width: wQty, Align: "R"},
		{Header: "Received", Width: wQty, Align: "R"}, {Header: "UOM", Width: wUOM, Align: "C"},
		{Header: "Status", Width: wStat, Align: "C"},
	}, grnLineRows(receipt.Items))

	doc.SectionTitle("QC Summary")
	var qcRows [][]string
	for _, insp := range inspections {
		for _, item := range insp.Items {
			var pass, hold, reject float64
			for _, d := range item.Dispositions {
				switch d.Disposition {
				case qualitycheck.DispositionPass:
					pass += d.Quantity
				case qualitycheck.DispositionHold:
					hold += d.Quantity
				case qualitycheck.DispositionReject:
					reject += d.Quantity
				}
			}
			qcRows = append(qcRows, []string{
				insp.InspectionNo, item.SKU, item.LotNo,
				csvexport.FormatFloat(pass), csvexport.FormatFloat(hold), csvexport.FormatFloat(reject), string(item.Status),
			})
		}
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "Inspection No.", Width: 32}, {Header: "SKU", Width: 26}, {Header: "Lot No.", Width: 24},
		{Header: "Passed", Width: 24, Align: "R"}, {Header: "Held", Width: 22, Align: "R"},
		{Header: "Rejected", Width: 24, Align: "R"}, {Header: "Status", Width: 34, Align: "C"},
	}, qcRows)

	doc.SectionTitle("Putaway Summary")
	var paRows [][]string
	tuples := map[string]stockTuple{}
	for _, task := range tasks {
		for _, item := range task.Items {
			paRows = append(paRows, []string{
				task.PutawayNo, item.SKU, item.LotNo, item.DestinationLocationCode,
				csvexport.FormatFloat(item.RequestedQty), csvexport.FormatFloat(item.MovedQty), string(item.Status),
			})
		}

		// Final Stock Position must know every destination this task's
		// lines actually moved stock to, not just each item's current
		// (possibly overwritten) DestinationLocationID — see
		// balanceFinder.ListTransactionsByReference's own doc for why a
		// line confirmed across multiple destinations loses the earlier
		// ones at the putaway_task_items row level. The ledger, tagged
		// ReferenceType="PUTAWAY"/ReferenceID=task.PutawayNo by
		// putaway.Service.Confirm itself, is the one place that history
		// survives; only the positive (arrival) leg of each move is
		// counted here, never the matching negative source leg.
		txns, err := s.balances.ListTransactionsByReference(ctx, tenantID, "PUTAWAY", task.PutawayNo)
		if err != nil {
			return nil, "", err
		}
		for _, txn := range txns {
			if txn.Quantity <= 0 {
				continue
			}
			key := fmt.Sprintf("%d|%d|%s", txn.ProductID, txn.LocationID, txn.LotNo)
			if _, exists := tuples[key]; exists {
				continue
			}
			sku := ""
			for _, item := range task.Items {
				if item.ProductID == txn.ProductID && item.LotNo == txn.LotNo {
					sku = item.SKU
					break
				}
			}
			tuples[key] = stockTuple{productID: txn.ProductID, locationID: txn.LocationID, lotNo: txn.LotNo, sku: sku, locCode: s.locationCode(ctx, tenantID, txn.LocationID)}
		}
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "Putaway No.", Width: 28}, {Header: "SKU", Width: 24}, {Header: "Lot No.", Width: 22},
		{Header: "Destination", Width: 26, Align: "C"}, {Header: "Requested", Width: 22, Align: "R"},
		{Header: "Moved", Width: 22, Align: "R"}, {Header: "Status", Width: 22, Align: "C"},
	}, paRows)

	// Final Stock Position also covers every receiving line's own
	// resting location, in case some quantity never moved on to
	// putaway at all (still sitting wherever Receiving Confirm placed
	// it) — not just what Putaway's own tasks touched.
	for _, item := range receipt.Items {
		if item.ReceivedQty <= 0 {
			continue
		}
		key := fmt.Sprintf("%d|%d|%s", item.ProductID, item.LocationID, item.LotNo)
		if _, exists := tuples[key]; !exists {
			tuples[key] = stockTuple{productID: item.ProductID, locationID: item.LocationID, lotNo: item.LotNo, sku: item.SKU}
		}
	}

	var tupleKeys []string
	for k := range tuples {
		tupleKeys = append(tupleKeys, k)
	}
	sort.Strings(tupleKeys)

	doc.SectionTitle("Final Stock Position")
	var stockRows [][]string
	var totalQty, totalReserved, totalHold, totalAvailable float64
	for _, k := range tupleKeys {
		t := tuples[k]
		bal, err := s.balances.FindBalance(ctx, tenantID, t.productID, t.locationID, t.lotNo)
		if err != nil {
			if errors.Is(err, inventory.ErrNotFound) {
				continue
			}
			return nil, "", err
		}
		available := bal.Quantity - bal.ReservedQuantity - bal.HoldQuantity
		locLabel := t.locCode
		if locLabel == "" {
			locLabel = fmt.Sprintf("#%d", t.locationID)
		}
		stockRows = append(stockRows, []string{
			t.sku, t.lotNo, locLabel,
			csvexport.FormatFloat(bal.Quantity), csvexport.FormatFloat(bal.ReservedQuantity),
			csvexport.FormatFloat(bal.HoldQuantity), csvexport.FormatFloat(available),
		})
		totalQty += bal.Quantity
		totalReserved += bal.ReservedQuantity
		totalHold += bal.HoldQuantity
		totalAvailable += available
	}
	if len(stockRows) > 0 {
		stockRows = append(stockRows, []string{
			"TOTAL", "", "",
			csvexport.FormatFloat(totalQty), csvexport.FormatFloat(totalReserved),
			csvexport.FormatFloat(totalHold), csvexport.FormatFloat(totalAvailable),
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 26}, {Header: "Lot No.", Width: 26}, {Header: "Location", Width: 26, Align: "C"},
		{Header: "Quantity", Width: 26, Align: "R"}, {Header: "Reserved", Width: 26, Align: "R"},
		{Header: "Hold", Width: 26, Align: "R"}, {Header: "Available", Width: 30, Align: "R"},
	}, stockRows)

	doc.SignatureBlock([]string{"Warehouse Supervisor", "Operations Manager"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("Inbound_Completion_%s.pdf", receipt.ReceiptNo), err
}

// RenderDiscrepancyReport builds the Receiving Discrepancy/Damage
// Report for one receipt — expected-vs-received quantity variances on
// the receiving lines themselves, plus every QC REJECT disposition
// (the "damage" side) recorded against that receipt's own inspections.
func (s *Service) RenderDiscrepancyReport(ctx context.Context, tenantID, userID, receiptID int64) ([]byte, string, error) {
	receipt, err := s.receiving.Get(ctx, tenantID, receiptID)
	if err != nil {
		return nil, "", mapNotFound(err)
	}
	inspections, err := s.qcInspectionsForReceipt(ctx, tenantID, receiptID)
	if err != nil {
		return nil, "", err
	}

	meta, err := s.meta(ctx, tenantID, userID, "Receiving Discrepancy / Damage Report", receipt.ReceiptNo)
	if err != nil {
		return nil, "", err
	}
	doc := pdfexport.NewDocument(meta)

	doc.KeyValueGrid([][2]string{
		{"Receipt No.", receipt.ReceiptNo},
		{"Status", string(receipt.Status)},
		{"Vendor", csvexport.FormatString(receipt.Vendor)},
		{"Warehouse", receipt.WarehouseCode},
		{"Receiving Date", csvexport.FormatTime(receipt.ReceivingDate)},
	}, 2)

	doc.SectionTitle("Quantity Discrepancies")
	var discRows [][]string
	for _, item := range receipt.Items {
		variance := item.ReceivedQty - item.ExpectedQty
		if variance == 0 {
			continue
		}
		kind := "SHORTAGE"
		if variance > 0 {
			kind = "OVERAGE"
		}
		discRows = append(discRows, []string{
			item.SKU, item.ProductName, item.LotNo,
			csvexport.FormatFloat(item.ExpectedQty), csvexport.FormatFloat(item.ReceivedQty),
			csvexport.FormatFloat(variance), kind,
		})
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "SKU", Width: 22}, {Header: "Product", Width: 40}, {Header: "Lot No.", Width: 20},
		{Header: "Expected", Width: 20, Align: "R"}, {Header: "Received", Width: 20, Align: "R"},
		{Header: "Variance", Width: 20, Align: "R"}, {Header: "Type", Width: 20, Align: "C"},
	}, discRows)

	doc.SectionTitle("Damage / Rejected Items (QC)")
	var damageRows [][]string
	for _, insp := range inspections {
		for _, item := range insp.Items {
			for _, d := range item.Dispositions {
				if d.Disposition != qualitycheck.DispositionReject {
					continue
				}
				damageRows = append(damageRows, []string{
					insp.InspectionNo, item.SKU, item.LotNo, csvexport.FormatFloat(d.Quantity),
					csvexport.FormatString(d.DefectCode), csvexport.FormatString(d.EvidenceReference),
					csvexport.FormatString(d.Remark), csvexport.FormatString(d.DecidedByName),
				})
			}
		}
	}
	doc.Table([]pdfexport.TableColumn{
		{Header: "Inspection No.", Width: 28}, {Header: "SKU", Width: 20}, {Header: "Lot No.", Width: 18},
		{Header: "Qty", Width: 16, Align: "R"}, {Header: "Defect Code", Width: 24}, {Header: "Evidence Ref.", Width: 26},
		{Header: "Remark", Width: 28}, {Header: "Decided By", Width: 26},
	}, damageRows)

	doc.SignatureBlock([]string{"Warehouse Supervisor", "QC Supervisor"})

	data, err := doc.Bytes()
	return data, fmt.Sprintf("Discrepancy_%s.pdf", receipt.ReceiptNo), err
}

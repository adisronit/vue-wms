package pickingbatch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/picking"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/warehouse"
)

// ErrValidation carries a client-facing message for a 422 response —
// the same shape every other workflow package's own ErrValidation uses.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrWarehouseNotFound = errors.New("pickingbatch: warehouse not found")
	// ErrNotConfirmable covers "not DRAFT" (already CONFIRMED/COMPLETED/
	// CANCELLED) — Repository.FindBatchByID's own status check, before
	// ClaimConfirm's own atomic guard is ever reached.
	ErrNotConfirmable = errors.New("pickingbatch: batch is not in a status that can be confirmed")
	// ErrConfirmConflict covers a genuine concurrent race lost at either
	// ClaimConfirm (another Confirm call already won) or ClaimOrders (one
	// of this batch's own orders was claimed by something else between
	// DRAFT creation and this Confirm call) — Service.Confirm never
	// distinguishes the two in the response, the same "uniform 409, no
	// partial detail" convention internal/order's own ErrNotReleasable
	// follows for its single-order equivalent.
	ErrConfirmConflict = errors.New("pickingbatch: batch or one of its orders could not be claimed (concurrent change)")
	// ErrNotCancellable covers "not DRAFT" — a CONFIRMED batch must be
	// cancelled via internal/picking's own POST /picking/{id}/cancel
	// instead, see Service.Cancel's own doc.
	ErrNotCancellable = errors.New("pickingbatch: batch is not in a status that can be cancelled directly (cancel the underlying picking task instead)")

	// Phase 33C.2 -- Sortation.
	//
	// ErrSortationNotReady covers both "batch never confirmed" (no
	// picking_task_id yet) and "confirmed but the underlying picking
	// task has not yet reached COMPLETED/SHORT_PICK" — Sortation has
	// nothing to sort until the physical pick is actually done.
	ErrSortationNotReady = errors.New("pickingbatch: sortation is not available until this batch's picking task is COMPLETED or SHORT_PICK")
	// ErrSortationScanMismatch covers every "this scan does not resolve
	// to a real target" case uniformly (order_no not a member of this
	// batch, or no allocation row for that order matches the scanned
	// SKU/lot) — never distinguished in the response, the same uniform-
	// rejection convention ErrConfirmConflict already established for
	// this package's own atomic order claim.
	ErrSortationScanMismatch = errors.New("pickingbatch: scanned order/product/lot does not match an allocation for this batch")
	// ErrSortationOverAllocation covers a scan whose quantity would sort
	// more than this order's own remaining allocated quantity — rejected
	// outright (never silently clamped), the same policy packing.
	// ErrOverpack already enforces for packed_qty vs picked_qty.
	ErrSortationOverAllocation = errors.New("pickingbatch: sorted quantity would exceed this order's own remaining allocated quantity")
	// ErrAllocationNotFound covers an allocation_id that either does not
	// exist or does not belong to the batch_id in the URL — both treated
	// identically, closing the same IDOR class every other FindByID in
	// this codebase already closes.
	ErrAllocationNotFound = errors.New("pickingbatch: allocation not found")
	// ErrSortationAlreadyFlagged covers re-flagging an allocation row
	// that already has an exception_reason recorded.
	ErrSortationAlreadyFlagged = errors.New("pickingbatch: allocation has already been flagged with a sortation exception")
)

// warehouseChecker is the narrow slice of internal/warehouse's own
// Repository this package needs — existence only, matching internal/
// picking's and internal/order's own warehouseChecker precedent
// (warehouse ownership is not yet verified anywhere in this codebase).
type warehouseChecker interface {
	FindByID(ctx context.Context, id int64) (*warehouse.Warehouse, error)
}

// pickingOrchestrator is the narrow slice of internal/picking's own
// Service this package calls — SuggestLots (Phase 33B's read-only
// FEFO/FIFO primitive, reused here rather than duplicated) for the DRAFT
// consolidation's own lot/location suggestion, CreateTx (Phase 25's
// addition, already used by internal/order's own CreatePickingTask) so
// Confirm's own atomic order claim and the real picking_tasks row commit
// or roll back together, and Get so Service.Get can resolve a CONFIRMED
// batch's own linked task status for the lazy COMPLETED/CANCELLED
// mirror (see Service.Get's own doc). Nothing in this package calls any
// other picking.Service method, and nothing in this package calls
// internal/inventory at all — every stock reservation this package ever
// causes happens exclusively inside picking.Service.CreateTx's own
// existing, completely unmodified createCore/createItem/ReserveStockTx
// path.
type pickingOrchestrator interface {
	SuggestLots(ctx context.Context, tenantID, productID, warehouseID int64) ([]inventory.LotAvailability, error)
	CreateTx(ctx context.Context, tenantID int64, tx *gorm.DB, req picking.CreateRequest, userID int64) (int64, error)
	Get(ctx context.Context, tenantID, id int64) (*picking.Response, error)
}

// Service holds Batch Picking's own business rules — consolidation,
// the atomic multi-order claim, and the thin status machine wrapped
// around internal/picking's own unmodified execution engine.
type Service struct {
	repo       *Repository
	warehouses warehouseChecker
	picking    pickingOrchestrator
}

func NewService(repo *Repository, warehouses warehouseChecker, pickingSvc pickingOrchestrator) *Service {
	return &Service{repo: repo, warehouses: warehouses, picking: pickingSvc}
}

// List resolves a page of batch headers.
func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	params.Page, params.Limit = page, limit

	rows, total, err := s.repo.List(ctx, tenantID, params)
	if err != nil {
		return nil, 0, err
	}
	responses := make([]ListResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, ListResponse{
			ID: row.ID, BatchNo: row.BatchNo, WarehouseCode: row.WarehouseCode,
			OrderCount: row.OrderCount, LineCount: row.LineCount,
			Status: Status(row.Status), PickNo: row.PickNo, CreatedAt: row.CreatedAt,
		})
	}
	return responses, total, nil
}

// ListAvailableOrders is the multi-select source for the Batch Picking
// screen — see Repository.ListAvailableOrders's own doc.
func (s *Service) ListAvailableOrders(ctx context.Context, tenantID, warehouseID int64) ([]AvailableOrderResponse, error) {
	rows, err := s.repo.ListAvailableOrders(ctx, tenantID, warehouseID)
	if err != nil {
		return nil, err
	}
	out := make([]AvailableOrderResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, AvailableOrderResponse{
			OrderID: row.OrderID, OrderNo: row.OrderNo, Customer: row.Customer,
			ShipToName: row.ShipToName, ItemCount: row.ItemCount,
		})
	}
	return out, nil
}

// Get resolves a batch's full detail — orders, consolidated lines, and
// each line's own order-share allocation breakdown. If the batch is
// CONFIRMED and its linked picking task has since reached a terminal
// status (COMPLETED/SHORT_PICK -> COMPLETED, CANCELLED -> CANCELLED),
// this lazily writes that mirror back via Repository.SyncTerminalStatus
// before building the response — a read-time derivation, never a
// cross-package DB trigger, per this package's own Status doc.
func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	row, err := s.repo.FindBatchDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	status := Status(row.Status)
	if status == StatusConfirmed && row.PickingTaskID != nil {
		taskResp, err := s.picking.Get(ctx, tenantID, *row.PickingTaskID)
		if err != nil {
			return nil, err
		}
		var newStatus Status
		switch taskResp.Status {
		case picking.StatusCompleted, picking.StatusShortPick:
			newStatus = StatusCompleted
		case picking.StatusCancelled:
			newStatus = StatusCancelled
		}
		if newStatus != "" {
			if err := s.repo.SyncTerminalStatus(ctx, tenantID, id, newStatus); err != nil {
				return nil, err
			}
			status = newStatus
		}
	}

	orderRows, err := s.repo.ListBatchOrders(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	orders := make([]OrderSummaryResponse, 0, len(orderRows))
	for _, o := range orderRows {
		orders = append(orders, OrderSummaryResponse{
			OrderID: o.OrderID, OrderNo: o.OrderNo, Customer: o.Customer, ShipToName: o.ShipToName, Status: o.Status,
		})
	}

	allocRows, err := s.repo.ListAllocationsDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	lines := groupAllocationsIntoLines(allocRows)

	return &Response{
		ID: row.ID, BatchNo: row.BatchNo, WarehouseID: row.WarehouseID, WarehouseCode: row.WarehouseCode,
		Status: status, PickingTaskID: row.PickingTaskID, PickNo: row.PickNo,
		Orders: orders, Lines: lines,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		ConfirmedAt: row.ConfirmedAt, CancelledAt: row.CancelledAt, CancelledBy: row.CancelledBy,
		CancelRemark:    row.CancelRemark,
		SortationStatus: SortationStatus(row.SortationStatus), SortationStation: row.SortationStation,
		SortationAssignedTo: row.SortationAssignedTo, SortationStartedAt: row.SortationStartedAt,
		SortationCompletedAt: row.SortationCompletedAt,
	}, nil
}

// groupAllocationsIntoLines is the pure core behind Get/Confirm's own
// display shape — extracted so the grouping can be unit tested without
// a database, the same "extract the pure part" precedent every prior
// phase's own dto.go establishes. Groups allocationDetailRow (one per
// order-share slice) by (product_id, lot_no, location_id) into one
// ConsolidatedLineResponse each, summing AllocatedQty into Qty.
func groupAllocationsIntoLines(rows []allocationDetailRow) []ConsolidatedLineResponse {
	type key struct {
		productID  int64
		lotNo      string
		locationID int64
	}
	order := make([]key, 0)
	byKey := make(map[key]*ConsolidatedLineResponse)
	for _, r := range rows {
		k := key{r.ProductID, r.LotNo, r.LocationID}
		line, ok := byKey[k]
		if !ok {
			line = &ConsolidatedLineResponse{
				ProductID: r.ProductID, SKU: r.SKU, ProductName: r.ProductName,
				LotNo: r.LotNo, LocationID: r.LocationID, LocationCode: r.LocationCode, UOM: r.UOM,
			}
			byKey[k] = line
			order = append(order, k)
		}
		line.Qty += r.AllocatedQty
		line.Allocations = append(line.Allocations, AllocationResponse{
			OrderID: r.OrderID, OrderNo: r.OrderNo, OrderItemID: r.OrderItemID,
			PickingTaskItemID: r.PickingTaskItemID, AllocatedQty: r.AllocatedQty,
		})
	}
	out := make([]ConsolidatedLineResponse, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// demandLine is one order line's own remaining demand for a product —
// consumed (qty decremented, removed once exhausted) as allocateProduct
// walks it against FEFO lot capacity.
type demandLine struct {
	orderID     int64
	orderItemID int64
	qty         float64
}

// productDemand is one product's consolidated cross-order demand.
type productDemand struct {
	sku, productName string
	uom              string
	totalQty         float64
	lines            []demandLine
}

// consolidateDemand groups raw order-item rows by product_id, summing
// ordered_qty and validating UOM consistency — the pure core of
// Service.Create's own step 1, unit-testable without a database (the
// same "extract the pure part" precedent every prior phase's own dto.go
// establishes, e.g. deriveFulfillmentStatus/buildPickingItems).
// Returns productIDs in a stable (ascending) order so downstream lot
// suggestion/allocation runs deterministically.
func consolidateDemand(items []orderItemRow) (map[int64]*productDemand, []int64, error) {
	byProduct := make(map[int64]*productDemand)
	var productIDs []int64
	for _, it := range items {
		d, ok := byProduct[it.ProductID]
		if !ok {
			d = &productDemand{sku: it.SKU, productName: it.ProductName, uom: it.UOM}
			byProduct[it.ProductID] = d
			productIDs = append(productIDs, it.ProductID)
		}
		if d.uom != it.UOM {
			return nil, nil, ErrValidation{Message: fmt.Sprintf(
				"product %s has inconsistent UOM across the selected orders (%s vs %s) — cannot consolidate", it.SKU, d.uom, it.UOM,
			)}
		}
		d.totalQty += it.OrderedQty
		d.lines = append(d.lines, demandLine{orderID: it.OrderID, orderItemID: it.OrderItemID, qty: it.OrderedQty})
	}
	sort.Slice(productIDs, func(i, j int) bool { return productIDs[i] < productIDs[j] })
	return byProduct, productIDs, nil
}

// lotLine is one FEFO-suggested (lot, location) slice of a product's
// consolidated demand — becomes exactly one picking_task_items row at
// Confirm.
type lotLine struct {
	lotNo        string
	locationID   int64
	locationCode string
	qty          float64
}

// allocateProduct is the pure core of Service.Create's own step 2 —
// unit-testable without a database. Walks lots in the FEFO order
// SuggestLots already returned, consuming capacity until demand.
// totalQty is covered (erroring if it never is), then splits demand.
// lines across the resulting lotLines in the same FEFO sequence via a
// two-pointer merge: a single order line's own share may legitimately
// be split across more than one lotLine if one lot can't cover it all.
// Returns the lotLines (for the consolidated picking_task_items side)
// and the full flat allocation slice (one entry per order-share slice,
// for picking_batch_allocations) together, since both are produced by
// the same single walk.
func allocateProduct(demand *productDemand, lots []inventory.LotAvailability) ([]lotLine, []Allocation, error) {
	var lotLines []lotLine
	remaining := demand.totalQty
	for _, lot := range lots {
		if remaining <= 0 {
			break
		}
		take := lot.AvailableQty
		if take > remaining {
			take = remaining
		}
		if take <= 0 {
			continue
		}
		lotLines = append(lotLines, lotLine{lotNo: lot.LotNo, locationID: lot.LocationID, locationCode: lot.LocationCode, qty: take})
		remaining -= take
	}
	if remaining > 0 {
		return nil, nil, ErrValidation{Message: fmt.Sprintf(
			"insufficient available stock for product %s: need %.4f %s, available %.4f", demand.sku, demand.totalQty, demand.uom, demand.totalQty-remaining,
		)}
	}

	// demandQueue is a working copy — demand.lines itself is never
	// mutated, so this function stays safe to call again (e.g. in a
	// test) against the same *productDemand.
	demandQueue := make([]demandLine, len(demand.lines))
	copy(demandQueue, demand.lines)

	var allocations []Allocation
	qi := 0
	for _, ll := range lotLines {
		lotRemaining := ll.qty
		for lotRemaining > 0 {
			if qi >= len(demandQueue) {
				// Cannot happen if remaining==0 above was reached
				// correctly (sum of lotLines == demand.totalQty ==
				// sum of demandQueue) — defensive only.
				return nil, nil, ErrValidation{Message: fmt.Sprintf("internal allocation mismatch for product %s", demand.sku)}
			}
			d := &demandQueue[qi]
			take := d.qty
			if take > lotRemaining {
				take = lotRemaining
			}
			allocations = append(allocations, Allocation{
				OrderID: d.orderID, OrderItemID: d.orderItemID,
				LotNo: ll.lotNo, LocationID: ll.locationID, AllocatedQty: take, UOM: demand.uom,
			})
			d.qty -= take
			lotRemaining -= take
			if d.qty <= 0 {
				qi++
			}
		}
	}
	return lotLines, allocations, nil
}

// eligibilityError formats one order's own reason it cannot be included
// in a new batch — Service.Create reports every failing order
// individually (never silently drops one), matching this package's own
// "no order is left half-claimed" convention for Confirm.
func eligibilityError(orderID int64, reason string) string {
	return fmt.Sprintf("order %d: %s", orderID, reason)
}

// Create is the DRAFT consolidation step — entirely read-only against
// inventory (SuggestLots is a plain SELECT) and against the included
// orders themselves (no order is claimed/mutated here, only inserted
// into picking_batch_orders — see that table's own migration doc for
// why that alone is enough to reserve the order against a second
// concurrent DRAFT). No picking_tasks row exists yet; Confirm is what
// actually commits to a real task and claims the orders.
func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
	if _, err := s.warehouses.FindByID(ctx, req.WarehouseID); err != nil {
		return nil, ErrWarehouseNotFound
	}

	// Dedupe while preserving order — a caller submitting the same
	// order_id twice should not see it double-counted in either the
	// eligibility check or the consolidated demand.
	seen := make(map[int64]bool, len(req.OrderIDs))
	orderIDs := make([]int64, 0, len(req.OrderIDs))
	for _, id := range req.OrderIDs {
		if !seen[id] {
			seen[id] = true
			orderIDs = append(orderIDs, id)
		}
	}
	if len(orderIDs) < 2 {
		return nil, ErrValidation{Message: "at least two distinct orders are required to create a picking batch"}
	}

	candidates, err := s.repo.LoadOrdersForConsolidation(ctx, tenantID, orderIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]orderConsolidationRow, len(candidates))
	for _, c := range candidates {
		byID[c.ID] = c
	}

	var problems []string
	for _, id := range orderIDs {
		c, ok := byID[id]
		if !ok {
			problems = append(problems, eligibilityError(id, "order not found"))
			continue
		}
		if c.Status != "RELEASED" {
			problems = append(problems, eligibilityError(id, fmt.Sprintf("order is %s, not RELEASED", c.Status)))
			continue
		}
		if c.PickingMode != "PICK_BY_PRODUCT" {
			problems = append(problems, eligibilityError(id, "order's picking_mode is not PICK_BY_PRODUCT"))
			continue
		}
		if c.WarehouseID != req.WarehouseID {
			problems = append(problems, eligibilityError(id, "order belongs to a different warehouse than this batch"))
			continue
		}
		if c.ActiveBatchCount > 0 {
			problems = append(problems, eligibilityError(id, "order is already a member of another active picking batch"))
			continue
		}
	}
	if len(problems) > 0 {
		return nil, ErrValidation{Message: "cannot create picking batch: " + joinSemicolon(problems)}
	}

	items, err := s.repo.LoadOrderItems(ctx, tenantID, orderIDs)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrValidation{Message: "the selected orders have no items to consolidate"}
	}

	demandByProduct, productIDs, err := consolidateDemand(items)
	if err != nil {
		return nil, err
	}

	var allAllocations []Allocation
	for _, productID := range productIDs {
		demand := demandByProduct[productID]
		lots, err := s.picking.SuggestLots(ctx, tenantID, productID, req.WarehouseID)
		if err != nil {
			return nil, err
		}
		_, allocations, err := allocateProduct(demand, lots)
		if err != nil {
			return nil, err
		}
		for i := range allocations {
			allocations[i].ProductID = productID
		}
		allAllocations = append(allAllocations, allocations...)
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var batchID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		b := &Batch{WarehouseID: req.WarehouseID, Status: StatusDraft, CreatedBy: &userID, SortationStatus: SortationNotStarted}
		if err := txRepo.CreateBatch(ctx, tenantID, b); err != nil {
			return err
		}
		batchID = b.ID
		if err := txRepo.CreateBatchOrders(ctx, tenantID, batchID, orderIDs); err != nil {
			return err
		}
		for i := range allAllocations {
			allAllocations[i].BatchID = batchID
		}
		return txRepo.CreateAllocations(ctx, tenantID, allAllocations)
	})
	dbctx.LogTransaction(ctx, "pickingbatch.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, batchID)
}

func joinSemicolon(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

// Confirm is the real commit point — inside one transaction: atomically
// claim the batch itself (DRAFT -> CONFIRMED, ClaimConfirm), atomically
// claim every included order (N-row ClaimOrders — if ANY order can no
// longer be claimed, e.g. a concurrent process already moved it,
// RowsAffected < len(orderIDs) and the WHOLE transaction rolls back:
// nothing is claimed, no task is created, no stock is reserved), create
// ONE real picking_tasks row via picking.Service.CreateTx's own
// completely unmodified Create logic with the consolidated items this
// batch's own DRAFT stage already computed, then backfill every
// allocation row's picking_task_item_id now that the real task items
// exist. Any failure at any step rolls back everything, including the
// batch/order claims already made earlier in this same call.
func (s *Service) Confirm(ctx context.Context, tenantID, batchID, userID int64) (*Response, error) {
	batch, err := s.repo.FindBatchByID(ctx, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	if !batch.Status.CanConfirm() {
		return nil, ErrNotConfirmable
	}

	orderRows, err := s.repo.ListBatchOrders(ctx, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	orderIDs := make([]int64, 0, len(orderRows))
	for _, o := range orderRows {
		orderIDs = append(orderIDs, o.OrderID)
	}
	if len(orderIDs) == 0 {
		return nil, ErrValidation{Message: "batch has no orders to confirm"}
	}

	allocRows, err := s.repo.ListAllocationsDetail(ctx, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	if len(allocRows) == 0 {
		return nil, ErrValidation{Message: "batch has no consolidated lines to confirm"}
	}
	items := buildConsolidatedItems(allocRows)

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		rows, err := txRepo.ClaimConfirm(ctx, tenantID, batchID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return ErrConfirmConflict
		}

		claimed, err := txRepo.ClaimOrders(ctx, tenantID, orderIDs, batchID)
		if err != nil {
			return err
		}
		if claimed != int64(len(orderIDs)) {
			// Rolling back this whole transaction also rolls back the
			// batch claim above — the batch is left exactly DRAFT again,
			// and any order among orderIDs that WAS matched by
			// ClaimOrders' own WHERE clause is rolled back too (SQL
			// Server's own transactional guarantee — ClaimOrders issues
			// one statement, but this surrounding transaction is what
			// makes a partial match on a correctly-matching subset not
			// durable either).
			return ErrConfirmConflict
		}

		taskID, err := s.picking.CreateTx(ctx, tenantID, tx, picking.CreateRequest{
			OrderNo: batch.BatchNo, WarehouseID: batch.WarehouseID, Priority: picking.PriorityNormal, Items: items,
		}, userID)
		if err != nil {
			return err
		}
		if err := txRepo.SetPickingTaskID(ctx, tenantID, batchID, taskID); err != nil {
			return err
		}
		return txRepo.BackfillAllocationTaskItems(ctx, tenantID, batchID, taskID)
	})
	dbctx.LogTransaction(ctx, "pickingbatch.confirm", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, batchID)
}

// buildConsolidatedItems re-derives the consolidated picking.
// CreateItemRequest lines from the batch's own already-persisted
// allocation rows (grouped by product_id/lot_no/location_id, summing
// allocated_qty) — the exact same grouping groupAllocationsIntoLines
// uses for display, reused here as Confirm's own source of truth so the
// task that gets created always matches what the DRAFT preview showed.
func buildConsolidatedItems(rows []allocationDetailRow) []picking.CreateItemRequest {
	lines := groupAllocationsIntoLines(rows)
	items := make([]picking.CreateItemRequest, 0, len(lines))
	for _, l := range lines {
		items = append(items, picking.CreateItemRequest{
			ProductID: l.ProductID, LotNo: l.LotNo, LocationID: l.LocationID, OrderedQty: l.Qty, UOM: l.UOM,
		})
	}
	return items
}

// Cancel is DRAFT-only — see Status.CanCancel's own doc. A CONFIRMED
// batch's real picking task must be cancelled via internal/picking's
// own POST /picking/{id}/cancel instead, which already releases its
// own reservation; this package never duplicates that logic.
func (s *Service) Cancel(ctx context.Context, tenantID, batchID, userID int64, req CancelRequest) (*Response, error) {
	batch, err := s.repo.FindBatchByID(ctx, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	if !batch.Status.CanCancel() {
		return nil, ErrNotCancellable
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	rows, err := s.repo.ClaimCancel(ctx, tenantID, batchID, userID, req.Remark)
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrNotCancellable
	}
	return s.Get(ctx, tenantID, batchID)
}

// ---------------------------------------------------------------------
// Phase 33C.2 -- Sortation
// ---------------------------------------------------------------------

// sortationGate loads the batch and verifies Sortation may run against
// it: the batch must have been Confirmed (picking_task_id set) and the
// resulting picking task must have reached a terminal state (COMPLETED
// or SHORT_PICK — both mean the physical pick is done, there is nothing
// left to sort until then). Resolves the task's own live status via
// s.picking.Get rather than trusting batch.Status, since that column
// only mirrors COMPLETED/CANCELLED lazily on Service.Get's own read path
// (see that method's own doc) and sortation may legitimately need to
// run before anything else has triggered that lazy sync.
func (s *Service) sortationGate(ctx context.Context, tenantID, batchID int64) (*Batch, error) {
	batch, err := s.repo.FindBatchByID(ctx, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	if batch.PickingTaskID == nil {
		return nil, ErrSortationNotReady
	}
	taskResp, err := s.picking.Get(ctx, tenantID, *batch.PickingTaskID)
	if err != nil {
		return nil, err
	}
	if taskResp.Status != picking.StatusCompleted && taskResp.Status != picking.StatusShortPick {
		return nil, ErrSortationNotReady
	}
	return batch, nil
}

// sortationLineResolved mirrors the pure allocationResolved predicate
// against a display row, for GetSortation's own per-line Resolved flag.
func sortationLineResolved(allocated, sorted float64, shortage *float64) bool {
	total := sorted
	if shortage != nil {
		total += *shortage
	}
	return total >= allocated
}

// buildSortationResponse assembles SortationResponse from the batch
// header plus its own allocation rows — the single place both
// GetSortation and every mutating sortation action re-read their result
// from, so every caller always sees the true post-write state.
func (s *Service) buildSortationResponse(ctx context.Context, tenantID, batchID int64) (*SortationResponse, error) {
	detail, err := s.repo.FindBatchDetail(ctx, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListAllocationsForSortation(ctx, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	lines := make([]SortationLineResponse, 0, len(rows))
	for _, r := range rows {
		remaining := r.AllocatedQty - r.SortedQty
		if r.ShortageQty != nil {
			remaining -= *r.ShortageQty
		}
		if remaining < 0 {
			remaining = 0
		}
		lines = append(lines, SortationLineResponse{
			AllocationID: r.ID, OrderID: r.OrderID, OrderNo: r.OrderNo, ShipToName: r.ShipToName,
			OrderItemID: r.OrderItemID, ProductID: r.ProductID, SKU: r.SKU, ProductName: r.ProductName,
			LotNo: r.LotNo, LocationID: r.LocationID, LocationCode: r.LocationCode,
			PickingTaskItemID: r.PickingTaskItemID, AllocatedQty: r.AllocatedQty, SortedQty: r.SortedQty,
			ShortageQty: r.ShortageQty, ExceptionReason: r.ExceptionReason, Remaining: remaining, UOM: r.UOM,
			Resolved: sortationLineResolved(r.AllocatedQty, r.SortedQty, r.ShortageQty),
		})
	}
	return &SortationResponse{
		BatchID: detail.ID, BatchNo: detail.BatchNo, PickingTaskID: detail.PickingTaskID, PickNo: detail.PickNo,
		SortationStatus: SortationStatus(detail.SortationStatus), SortationStation: detail.SortationStation,
		SortationAssignedTo: detail.SortationAssignedTo, SortationStartedAt: detail.SortationStartedAt,
		SortationCompletedAt: detail.SortationCompletedAt, Lines: lines,
	}, nil
}

// GetSortation returns the Sortation worklist for batchID — every
// picking_batch_allocations row with its own cumulative sorted_qty/
// shortage progress. Requires the gate (batch Confirmed, underlying
// task terminal); unlike the mutating actions below, a batch whose
// sortation has not yet started is still a perfectly valid thing to
// view (sortation_status NOT_STARTED, every line's remaining == its
// allocated_qty).
func (s *Service) GetSortation(ctx context.Context, tenantID, batchID int64) (*SortationResponse, error) {
	if _, err := s.sortationGate(ctx, tenantID, batchID); err != nil {
		return nil, err
	}
	return s.buildSortationResponse(ctx, tenantID, batchID)
}

// recomputeSortationStatus is the shared tail of every mutating
// sortation action: start the sub-workflow if this is its first
// activity, then check whether every allocation row in the batch has
// become "resolved" (see allocationResolved's own doc for the exact
// rule — fully sorted, or its shortfall fully covered by a recorded
// shortage) and, if so, mark sortation COMPLETED. Both repository calls
// are themselves idempotent-safe guarded UPDATEs (0 rows affected if
// already in the target state), so calling this unconditionally after
// every successful scan/exception is always safe.
func (s *Service) recomputeSortationStatus(ctx context.Context, tenantID, batchID int64, txRepo *Repository) error {
	if err := txRepo.StartSortation(ctx, tenantID, batchID); err != nil {
		return err
	}
	resolved, err := txRepo.AllAllocationsResolved(ctx, tenantID, batchID)
	if err != nil {
		return err
	}
	if resolved {
		return txRepo.CompleteSortation(ctx, tenantID, batchID)
	}
	return nil
}

// ScanSortation is the physical confirmation step: resolve the scanned
// order barcode against this batch's own member orders, resolve the
// scanned SKU/lot against that order's own allocation rows for this
// batch, then atomically increment sorted_qty bounded by allocated_qty.
// A single order/SKU/lot combination can resolve to more than one
// allocation row when that order's own demand was split across more
// than one FEFO lot at Create time — see Repository.
// FindMatchingAllocationsForScan's own doc; this consumes the requested
// quantity across every matching row in id order, inside one
// transaction, and rolls back the WHOLE scan (never a partial apply) if
// the total requested cannot be fully satisfied — the same "reject,
// don't clamp" policy as packing.ErrOverpack.
func (s *Service) ScanSortation(ctx context.Context, tenantID, batchID int64, req SortationScanRequest, userID int64) (*SortationResponse, error) {
	if req.Quantity <= 0 {
		return nil, ErrValidation{Message: "quantity must be greater than zero"}
	}
	if _, err := s.sortationGate(ctx, tenantID, batchID); err != nil {
		return nil, err
	}

	orderID, err := s.repo.ResolveOrderIDByBarcode(ctx, tenantID, batchID, req.OrderBarcode)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrSortationScanMismatch
	}
	if err != nil {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		candidates, err := txRepo.FindMatchingAllocationsForScan(ctx, tenantID, batchID, orderID, req.SKU, req.LotNo)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			return ErrSortationScanMismatch
		}

		remaining := req.Quantity
		for _, row := range candidates {
			if remaining <= 0 {
				break
			}
			room := row.AllocatedQty - row.SortedQty
			if room <= 0 {
				continue
			}
			take := remaining
			if take > room {
				take = room
			}
			affected, err := txRepo.AdjustSortedQty(ctx, tenantID, row.ID, take)
			if err != nil {
				return err
			}
			if affected == 0 {
				// Lost a race against a concurrent scan for this exact
				// row between the SELECT above and this UPDATE — simply
				// does not count toward remaining; the next candidate
				// row (if any) is tried instead.
				continue
			}
			remaining -= take
		}
		if remaining > 0 {
			// Rolling back the whole transaction undoes every partial
			// AdjustSortedQty increment already applied above — this
			// scan either fully succeeds or has no effect at all.
			return ErrSortationOverAllocation
		}

		return s.recomputeSortationStatus(ctx, tenantID, batchID, txRepo)
	})
	dbctx.LogTransaction(ctx, "pickingbatch.sortation_scan", start, err)
	if err != nil {
		return nil, err
	}
	return s.buildSortationResponse(ctx, tenantID, batchID)
}

// FlagSortationException records a sortation-stage shortage/damage
// exception on one specific allocation row, terminating its need for
// further sortation as long as the resulting SortedQty + ShortageQty
// does not exceed AllocatedQty (checked against a fresh read inside the
// same transaction as the write, so this cannot race a concurrent scan
// into over-accounting the row).
func (s *Service) FlagSortationException(ctx context.Context, tenantID, batchID int64, req SortationExceptionRequest, userID int64) (*SortationResponse, error) {
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, ErrValidation{Message: "reason is required"}
	}
	if req.ShortageQty <= 0 {
		return nil, ErrValidation{Message: "shortage_qty must be greater than zero"}
	}
	if _, err := s.sortationGate(ctx, tenantID, batchID); err != nil {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		alloc, err := txRepo.FindAllocationByID(ctx, tenantID, batchID, req.AllocationID)
		if errors.Is(err, ErrNotFound) {
			return ErrAllocationNotFound
		}
		if err != nil {
			return err
		}
		if alloc.ExceptionReason != nil {
			return ErrSortationAlreadyFlagged
		}
		if req.ShortageQty > alloc.AllocatedQty-alloc.SortedQty {
			return ErrValidation{Message: "shortage_qty exceeds this allocation's own remaining (unsorted) quantity"}
		}

		affected, err := txRepo.FlagShortageException(ctx, tenantID, req.AllocationID, req.ShortageQty, reason)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrSortationAlreadyFlagged
		}

		return s.recomputeSortationStatus(ctx, tenantID, batchID, txRepo)
	})
	dbctx.LogTransaction(ctx, "pickingbatch.sortation_exception", start, err)
	if err != nil {
		return nil, err
	}
	return s.buildSortationResponse(ctx, tenantID, batchID)
}

// AssignSortation sets the sortation station code and/or assigned user
// — a planning/ownership annotation at the same authority level as
// AssignPicker/AssignPacker elsewhere, never an authorization gate.
func (s *Service) AssignSortation(ctx context.Context, tenantID, batchID int64, req SortationAssignRequest) (*SortationResponse, error) {
	if _, err := s.sortationGate(ctx, tenantID, batchID); err != nil {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	if err := s.repo.AssignSortation(ctx, tenantID, batchID, req.Station, req.UserID); err != nil {
		return nil, err
	}
	return s.buildSortationResponse(ctx, tenantID, batchID)
}

// ---------------------------------------------------------------------
// Phase 33C.2 -- read-only surface for internal/packing's one-way
// dependency into this package (packing -> pickingbatch; the reverse
// never happens — pickingbatch has no need to ever import packing).
// Lets internal/packing build a per-order batch-sourced job without
// ever importing internal/order or querying picking_batch_allocations
// directly.
// ---------------------------------------------------------------------

// FindBatchByPickingTaskID resolves the batch (if any) that claims
// pickingTaskID. Returns ErrNotFound when the task is NOT batch-sourced
// — the ordinary PICK_BY_ORDER case, which the caller must treat as
// "this task is not a batch task" rather than a genuine failure.
func (s *Service) FindBatchByPickingTaskID(ctx context.Context, tenantID, pickingTaskID int64) (*Batch, error) {
	return s.repo.FindBatchByPickingTaskID(ctx, tenantID, pickingTaskID)
}

// IsOrderInBatch reports whether orderID is one of batchID's own member
// orders.
func (s *Service) IsOrderInBatch(ctx context.Context, tenantID, batchID, orderID int64) (bool, error) {
	return s.repo.IsOrderInBatch(ctx, tenantID, batchID, orderID)
}

// OrderAllocationLine is one of orderID's own allocation rows within a
// batch — exposed read-only for internal/packing's Service.Create to
// build a batch-sourced packing job's lines, using each row's own
// SortedQty (never the shared task's task-wide PickedQty) as the frozen
// packed_qty ceiling.
type OrderAllocationLine struct {
	AllocationID      int64
	PickingTaskItemID int64
	ProductID         int64
	LotNo             string
	AllocatedQty      float64
	SortedQty         float64
	UOM               string
}

// OrderAllocationLines resolves orderID's own allocation rows within
// batchID. Errors if any matching row's own picking_task_item_id is
// still NULL — by the time Sortation/Packing ever run (both gated on
// the underlying task being terminal, long after Confirm's own
// BackfillAllocationTaskItems step), that should never happen; treated
// as an internal invariant violation rather than a client-facing error.
func (s *Service) OrderAllocationLines(ctx context.Context, tenantID, batchID, orderID int64) ([]OrderAllocationLine, error) {
	rows, err := s.repo.ListOrderAllocationLines(ctx, tenantID, batchID, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]OrderAllocationLine, 0, len(rows))
	for _, r := range rows {
		if r.PickingTaskItemID == nil {
			return nil, fmt.Errorf("pickingbatch: allocation %d has no picking_task_item_id (batch not yet confirmed)", r.ID)
		}
		out = append(out, OrderAllocationLine{
			AllocationID: r.ID, PickingTaskItemID: *r.PickingTaskItemID, ProductID: r.ProductID,
			LotNo: r.LotNo, AllocatedQty: r.AllocatedQty, SortedQty: r.SortedQty, UOM: r.UOM,
		})
	}
	return out, nil
}

// OrderPackingSnapshot is orderID's own full customer/ship-to snapshot
// — see Repository.FindOrderPackingSnapshot's own doc for why internal/
// packing needs this instead of the shared picking task's own copied
// fields.
type OrderPackingSnapshot struct {
	OrderID             int64
	OrderNo             string
	Customer            *string
	CustomerID          *int64
	ShipToID            *int64
	ShipToName          *string
	ShipToAddress       *string
	ShipToProvince      *string
	ShipToDistrict      *string
	ShipToPostalCode    *string
	ShipToContactPerson *string
	ShipToContactPhone  *string
	WarehouseID         int64
}

// OrderPackingSnapshot resolves orderID's own full snapshot directly
// from the orders table.
func (s *Service) OrderPackingSnapshot(ctx context.Context, tenantID, orderID int64) (*OrderPackingSnapshot, error) {
	row, err := s.repo.FindOrderPackingSnapshot(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}
	return &OrderPackingSnapshot{
		OrderID: row.OrderID, OrderNo: row.OrderNo, Customer: row.Customer, CustomerID: row.CustomerID,
		ShipToID: row.ShipToID, ShipToName: row.ShipToName, ShipToAddress: row.ShipToAddress,
		ShipToProvince: row.ShipToProvince, ShipToDistrict: row.ShipToDistrict,
		ShipToPostalCode: row.ShipToPostalCode, ShipToContactPerson: row.ShipToContactPerson,
		ShipToContactPhone: row.ShipToContactPhone, WarehouseID: row.WarehouseID,
	}, nil
}

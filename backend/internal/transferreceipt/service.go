package transferreceipt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wms-backend/internal/inventory"
	"wms-backend/internal/order"
	"wms-backend/internal/platform/dbctx"
	"wms-backend/internal/shipping"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

var (
	ErrShipmentNotFound      = errors.New("transferreceipt: shipment not found")
	ErrOrderNotFound         = errors.New("transferreceipt: order not found")
	ErrNotTransferOrder      = errors.New("transferreceipt: shipment's order is not an INTERNAL_TRANSFER order")
	ErrShipmentNotDispatched = errors.New("transferreceipt: shipment has not been dispatched yet")
	ErrReceiptAlreadyExists  = errors.New("transferreceipt: a receipt already exists for this shipment")
	ErrInvalidTransition     = errors.New("transferreceipt: receipt is not in a receivable/confirmable status")
	ErrNotReadyToConfirm     = errors.New("transferreceipt: every line must be fully received or have a recorded discrepancy before confirming")
	ErrNotCancellable        = errors.New("transferreceipt: receipt is not in a cancellable status")
	ErrOverReceipt           = errors.New("transferreceipt: quantity would exceed the line's own dispatched quantity")
	ErrCrossWarehouse        = errors.New("transferreceipt: staging location must belong to the receipt's destination warehouse")
	ErrItemNotInReceipt      = errors.New("transferreceipt: item does not belong to this receipt")
)

// shippingChecker is the narrow slice of shipping.Service this package
// needs — read-only resolution of a shipment's own items/order/status.
// Satisfied structurally by *shipping.Service directly, matching this
// codebase's own "narrow checker interface, satisfied by the concrete
// production type" convention (relocation's productChecker/
// warehouseChecker, receiving's locationChecker, ...).
type shippingChecker interface {
	Get(ctx context.Context, tenantID, id int64) (*shipping.Response, error)
}

// orderChecker is the narrow slice of order.Repository this package
// needs — resolving the shipment's own linked order by its order_no
// snapshot (packing_jobs.order_no is always populated, even for a
// non-batch job; picking_tasks itself carries no order_id column at
// all, so this is the only reliable path from a shipment back to its
// real order row). Satisfied structurally by *order.Repository.
type orderChecker interface {
	FindByOrderNo(ctx context.Context, tenantID int64, orderNo string) (*order.Order, error)
}

// Service holds the Destination Receiving workflow's business rules —
// the status machine, shipment/order validation, and the ReceiveItem/
// Confirm transactions that call inventory.Service.AddStockTx instead
// of mutating stock itself.
type Service struct {
	repo      *Repository
	shipments shippingChecker
	orders    orderChecker
	inventory *inventory.Service
}

func NewService(repo *Repository, shipments shippingChecker, orders orderChecker, inventorySvc *inventory.Service) *Service {
	return &Service{repo: repo, shipments: shipments, orders: orders, inventory: inventorySvc}
}

func (s *Service) List(ctx context.Context, tenantID int64, params ListParams) ([]ListResponse, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	params.Page, params.Limit = page, limit
	return s.repo.List(ctx, tenantID, params)
}

// EligibleShipments backs the Transfer Receiving screen's "+ New
// Receipt" shipment picker.
func (s *Service) EligibleShipments(ctx context.Context, tenantID int64, params EligibleShipmentParams) ([]EligibleShipmentResponse, error) {
	return s.repo.ListEligibleShipments(ctx, tenantID, params)
}

func (s *Service) Get(ctx context.Context, tenantID, id int64) (*Response, error) {
	header, err := s.repo.FindReceiptDetail(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItems(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &Response{
		ID: header.ID, ReceiptNo: header.ReceiptNo,
		ShipmentID: header.ShipmentID, ShipmentNo: header.ShipmentNo,
		OrderID: header.OrderID, OrderNo: header.OrderNo, TransferReference: header.TransferReference,
		DestinationWarehouseID: header.DestinationWarehouseID, DestinationWarehouseCode: header.DestinationWarehouseCode,
		Status:     Status(header.Status),
		ReceivedBy: header.ReceivedBy, ReceivedByName: header.ReceivedByName, ReceivedAt: header.ReceivedAt,
		Remark: header.Remark, CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
		CancelledAt: header.CancelledAt, CancelledBy: header.CancelledBy, CancelledByName: header.CancelledByName,
		CancelRemark: header.CancelRemark,
		Items:        items,
	}, nil
}

// Reconciliation computes GET /transfer-receipts/{id}/reconciliation --
// a read-only summary derived entirely from Get's own already-correct,
// already-joined Response (no second query path duplicating FindReceiptDetail/
// ListItems). Pure arithmetic over already-stored dispatched_qty/
// received_qty/discrepancy_qty: no new table, no mutation, nothing this
// phase's own Create/ReceiveItem/Confirm/Cancel logic doesn't already
// guarantee is internally consistent.
//
// Per-line FullyReconciled is true iff received_qty == dispatched_qty
// AND no (non-zero) discrepancy_qty was recorded -- the same two
// conditions ReceiveItem's own "close short" branch and itemStatus's own
// ItemStatusCompleted path are already mutually exclusive on, so this
// never reinterprets what those already decided.
//
// ReconciliationStatus is PENDING while the receipt itself has not yet
// reached COMPLETED (receiving may still be in progress -- any
// discrepancy recorded so far is provisional until Confirm's own
// "every line settled" gate passes), RECONCILED once COMPLETED with
// zero total discrepancy, or DISCREPANCY once COMPLETED with a non-zero
// total discrepancy.
func (s *Service) Reconciliation(ctx context.Context, tenantID, id int64) (*ReconciliationResponse, error) {
	receipt, err := s.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	lines := make([]ReconciliationLineResponse, 0, len(receipt.Items))
	var totalDispatched, totalReceived, totalDiscrepancy float64
	fullyReconciled := true
	for _, it := range receipt.Items {
		discrepancyQty := 0.0
		if it.DiscrepancyQty != nil {
			discrepancyQty = *it.DiscrepancyQty
		}
		lineReconciled := it.ReceivedQty == it.DispatchedQty && discrepancyQty == 0
		if !lineReconciled {
			fullyReconciled = false
		}
		totalDispatched += it.DispatchedQty
		totalReceived += it.ReceivedQty
		totalDiscrepancy += discrepancyQty

		lines = append(lines, ReconciliationLineResponse{
			ItemID: it.ID, ProductID: it.ProductID, SKU: it.SKU, ProductName: it.ProductName,
			LotNo: it.LotNo, UOM: it.UOM,
			DispatchedQty: it.DispatchedQty, ReceivedQty: it.ReceivedQty,
			DiscrepancyQty: it.DiscrepancyQty, DiscrepancyReason: it.DiscrepancyReason,
			Status: it.Status, FullyReconciled: lineReconciled,
		})
	}
	if len(receipt.Items) == 0 {
		fullyReconciled = false
	}

	reconciliationStatus := "PENDING"
	if receipt.Status == StatusCompleted {
		if totalDiscrepancy != 0 {
			reconciliationStatus = "DISCREPANCY"
		} else {
			reconciliationStatus = "RECONCILED"
		}
	}

	return &ReconciliationResponse{
		ReceiptID: receipt.ID, ReceiptNo: receipt.ReceiptNo, Status: receipt.Status,
		TotalDispatchedQty: totalDispatched, TotalReceivedQty: totalReceived, TotalDiscrepancyQty: totalDiscrepancy,
		FullyReconciled: fullyReconciled, ReconciliationStatus: reconciliationStatus,
		Lines: lines,
	}, nil
}

// Create validates shipmentID is a DISPATCHED-or-later shipment whose
// own order is an INTERNAL_TRANSFER order with no receipt yet, then
// copies one transfer_receipt_item per shipment line with
// dispatched_qty frozen from that shipment item's own Qty (the real,
// packed_qty-sourced shipped quantity — never the original order
// line's ordered_qty, which may have been short-picked). No stock
// mutation happens here — only ReceiveItem ever calls AddStockTx.
func (s *Service) Create(ctx context.Context, tenantID int64, req CreateRequest, userID int64) (*Response, error) {
	shipment, err := s.shipments.Get(ctx, tenantID, req.ShipmentID)
	if err != nil {
		return nil, ErrShipmentNotFound
	}
	if !isDispatchedOrLater(shipment.Status) {
		return nil, ErrShipmentNotDispatched
	}
	if len(shipment.Items) == 0 {
		return nil, ErrValidation{Message: "shipment has no items to receive"}
	}

	ord, err := s.orders.FindByOrderNo(ctx, tenantID, shipment.OrderNo)
	if err != nil {
		return nil, ErrOrderNotFound
	}
	if ord.OrderType != "INTERNAL_TRANSFER" || ord.DestinationWarehouseID == nil {
		return nil, ErrNotTransferOrder
	}

	if _, err := s.repo.FindReceiptByShipmentID(ctx, tenantID, req.ShipmentID); err == nil {
		return nil, ErrReceiptAlreadyExists
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	var receiptID int64
	start := time.Now()
	err = s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		receipt := &Receipt{
			ShipmentID: req.ShipmentID, OrderID: ord.ID, DestinationWarehouseID: *ord.DestinationWarehouseID,
			Status: StatusDraft, Remark: req.Remark, CreatedBy: &userID,
		}
		if err := txRepo.CreateReceipt(ctx, tenantID, receipt); err != nil {
			return err
		}
		receiptID = receipt.ID

		for _, si := range shipment.Items {
			if si.Qty <= 0 {
				continue
			}
			item := &Item{
				TransferReceiptID: receipt.ID, ShipmentItemID: si.ID, ProductID: si.ProductID,
				LotNo: si.LotNo, UOM: si.UOM, DispatchedQty: si.Qty, Status: ItemStatusPending,
			}
			if err := txRepo.CreateItem(ctx, tenantID, item); err != nil {
				return err
			}
		}
		return nil
	})
	dbctx.LogTransaction(ctx, "transferreceipt.create", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
}

// isDispatchedOrLater reports whether a shipment's status is one this
// module may create a receipt against — DISPATCHED, IN_TRANSIT, or
// DELIVERED. Never READY (not yet dispatched), ISSUE, or CANCELLED.
func isDispatchedOrLater(status shipping.Status) bool {
	switch status {
	case shipping.StatusDispatched, shipping.StatusInTransit, shipping.StatusDelivered:
		return true
	default:
		return false
	}
}

// ReceiveItem is the atomic core of this phase: validate -> resolve
// and validate the chosen staging location (belongs to the receipt's
// own destination warehouse, RECEIVING zone type, ACTIVE) -> call
// inventory.AddStockTx for exactly this call's own quantity (same
// transaction, one TRANSFER_RECEIVE ledger row) -> atomically
// increment received_qty, bounded by dispatched_qty (AdjustReceivedQty
// — rejected outright, never clamped, on over-receipt) -> optionally
// close the line with a recorded discrepancy -> commit. Can be called
// any number of times for the same line (partial/incremental
// receiving), the same cumulative shape packing.Service.PackItem
// already establishes for its own AdjustPackedQty.
//
// CloseShort (req.CloseShort) is a deliberate, explicit flag on this
// same call rather than something Confirm infers automatically from a
// shortfall: the receiver must state why a shortfall is being accepted
// (DiscrepancyReason) at the moment they stop expecting more of this
// line, not retroactively when the whole receipt is finalized — and a
// call may combine both in one round trip (receive a final partial
// quantity AND close the line short).
func (s *Service) ReceiveItem(ctx context.Context, tenantID, receiptID, itemID int64, req ReceiveItemRequest, userID int64) (*Response, error) {
	if req.Quantity < 0 {
		return nil, ErrValidation{Message: "quantity must not be negative"}
	}
	if !req.CloseShort && req.Quantity <= 0 {
		return nil, ErrValidation{Message: "quantity must be greater than zero unless closing the line with a discrepancy"}
	}
	if req.CloseShort && req.DiscrepancyReason == "" {
		return nil, ErrValidation{Message: "discrepancy_reason is required to close a line short"}
	}
	if req.Quantity > 0 && req.LocationID == 0 {
		return nil, ErrValidation{Message: "location_id is required to receive a quantity"}
	}

	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		// Confirm the receipt exists BEFORE claiming the idempotency key —
		// transfer_receipt_confirmations.transfer_receipt_id is a real FK
		// (migrations/0118), matching relocation/packing/receiving's own
		// ordering.
		receipt, err := txRepo.FindReceiptByID(ctx, tenantID, receiptID)
		if err != nil {
			return err
		}
		if err := txRepo.InsertConfirmation(ctx, tenantID, req.IdempotencyKey, receiptID); err != nil {
			return err
		}
		if !receipt.Status.CanReceive() {
			return ErrInvalidTransition
		}

		item, err := txRepo.FindItemByID(ctx, tenantID, itemID)
		if err != nil {
			return err
		}
		if item.TransferReceiptID != receiptID {
			return ErrItemNotInReceipt
		}
		if !item.Status.CanReceive() {
			return ErrValidation{Message: fmt.Sprintf("line %d is already fully received or closed with a discrepancy", item.ID)}
		}

		if req.Quantity > 0 {
			locInfo, err := txRepo.resolveLocation(ctx, tenantID, req.LocationID)
			if err != nil {
				return err
			}
			if locInfo.WarehouseID != receipt.DestinationWarehouseID {
				return ErrCrossWarehouse
			}
			if locInfo.ZoneType != "RECEIVING" {
				return ErrValidation{Message: "staging location must be in a RECEIVING zone"}
			}
			if locInfo.Status != "ACTIVE" {
				return ErrValidation{Message: "staging location is not active"}
			}

			invRepo := inventory.NewRepository(tx)
			refType := "TRANSFER_RECEIPT"
			refID := receipt.ReceiptNo
			if _, err := s.inventory.AddStockTx(ctx, invRepo, inventory.StockChangeRequest{
				TenantID: tenantID, ProductID: item.ProductID, LocationID: req.LocationID, LotNo: item.LotNo,
				Quantity: req.Quantity, Type: inventory.TransactionTransferReceive,
				ReferenceType: &refType, ReferenceID: &refID, CreatedBy: &userID,
			}); err != nil {
				return err
			}
		}

		// The concurrency-safe over-receipt guard itself — see
		// AdjustReceivedQty's own doc. affected == 0 means the WHERE
		// clause's own received_qty + delta <= dispatched_qty bound
		// rejected this call; nothing was written (and, critically,
		// nothing above was rolled back by this alone — the whole
		// transaction is, since this handler returns an error here).
		affected, err := txRepo.AdjustReceivedQty(ctx, tenantID, item.ID, req.Quantity)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrOverReceipt
		}

		updated, err := txRepo.FindItemByID(ctx, tenantID, item.ID)
		if err != nil {
			return err
		}
		if req.Quantity > 0 {
			locID := req.LocationID
			updated.LocationID = &locID
		}
		if req.CloseShort {
			if updated.ReceivedQty >= updated.DispatchedQty {
				return ErrValidation{Message: "line is already fully received; there is no shortfall to record as a discrepancy"}
			}
			discrepancyQty := updated.DispatchedQty - updated.ReceivedQty
			reason := req.DiscrepancyReason
			updated.DiscrepancyQty = &discrepancyQty
			updated.DiscrepancyReason = &reason
			updated.Status = ItemStatusDiscrepancy
		} else {
			updated.Status = itemStatus(updated.DispatchedQty, updated.ReceivedQty)
		}
		return txRepo.UpdateItemAfterReceive(ctx, tenantID, updated)
	})
	dbctx.LogTransaction(ctx, "transferreceipt.receive_item", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		// Already processed by an earlier call with this same key —
		// return the current (already-correct) state as a successful,
		// idempotent no-op rather than an error or a double stock add.
		return s.Get(ctx, tenantID, receiptID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
}

// Confirm is the explicit, idempotency-guarded DRAFT -> COMPLETED
// transition — allowed only once every line is "settled" (fully
// received, or closed with a recorded discrepancy). Performs no stock
// mutation of its own; ReceiveItem already did that, incrementally,
// for each line.
func (s *Service) Confirm(ctx context.Context, tenantID, receiptID int64, req ConfirmRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		receipt, err := txRepo.FindReceiptByID(ctx, tenantID, receiptID)
		if err != nil {
			return err
		}
		if err := txRepo.InsertConfirmation(ctx, tenantID, req.IdempotencyKey, receiptID); err != nil {
			return err
		}
		if !receipt.Status.CanConfirm() {
			return ErrInvalidTransition
		}

		items, err := txRepo.FindItemsByReceipt(ctx, tenantID, receiptID)
		if err != nil {
			return err
		}
		if !allItemsSettled(items) {
			return ErrNotReadyToConfirm
		}

		now := time.Now().UTC()
		receipt.Status = StatusCompleted
		receipt.ReceivedAt = &now
		receipt.ReceivedBy = &userID
		return txRepo.UpdateReceipt(ctx, tenantID, receipt)
	})
	dbctx.LogTransaction(ctx, "transferreceipt.confirm", start, err)

	if errors.Is(err, ErrDuplicateIdempotencyKey) {
		return s.Get(ctx, tenantID, receiptID)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
}

// Cancel marks the receipt CANCELLED — DRAFT only. A receipt where any
// line has already received stock can still be cancelled (matching
// relocation's own "abandon what's unconfirmed" precedent, not
// receiving's stricter boundary) since ReceiveItem never reserves
// anything that would need releasing — it only ever adds stock that
// has already, physically, arrived. Whatever was already received
// stays exactly where it landed; only the header moves to CANCELLED,
// which simply means "stop expecting the rest."
func (s *Service) Cancel(ctx context.Context, tenantID, receiptID int64, req CancelRequest, userID int64) (*Response, error) {
	ctx, cancel := dbctx.Detach(ctx)
	defer cancel()

	start := time.Now()
	err := s.repo.Transaction(ctx, func(tx *gorm.DB, txRepo *Repository) error {
		receipt, err := txRepo.FindReceiptByID(ctx, tenantID, receiptID)
		if err != nil {
			return err
		}
		if !receipt.Status.CanCancel() {
			return ErrNotCancellable
		}
		receipt.applyCancel(userID, req.Remark, time.Now().UTC())
		return txRepo.CancelReceipt(ctx, tenantID, receipt)
	})
	dbctx.LogTransaction(ctx, "transferreceipt.cancel", start, err)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, receiptID)
}

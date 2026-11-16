import { apiGet, apiPost } from './api'

// Phase 33E.2 — Destination Receiving. Mirrors services/relocation.js's
// own shape (list/get/create/confirm/cancel) for internal/transferreceipt's
// new, lightweight module — the action that finally credits the
// DESTINATION warehouse once an INTERNAL_TRANSFER order's dispatched
// shipment physically arrives, closing the status-window loop 33E.1
// proved on the backend. No export/pdf endpoints yet (33E.3 scope).

export function listTransferReceipts(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.destinationWarehouseId) query.set('destination_warehouse_id', params.destinationWarehouseId)

  const qs = query.toString()
  return apiGet(`/transfer-receipts${qs ? `?${qs}` : ''}`)
}

// DISPATCHED/IN_TRANSIT/DELIVERED INTERNAL_TRANSFER shipments bound for
// destinationWarehouseId that have no transfer receipt yet — powers the
// "+ New Receipt" shipment picker.
export function listEligibleTransferShipments(destinationWarehouseId, search = '') {
  const query = new URLSearchParams()
  if (destinationWarehouseId) query.set('destination_warehouse_id', destinationWarehouseId)
  if (search) query.set('search', search)

  const qs = query.toString()
  return apiGet(`/transfer-receipts/eligible-shipments${qs ? `?${qs}` : ''}`)
}

export function getTransferReceipt(id) {
  return apiGet(`/transfer-receipts/${id}`)
}

// payload: { shipment_id, remark? }
export function createTransferReceipt(payload) {
  return apiPost('/transfer-receipts', payload)
}

// payload: { idempotency_key, location_id, quantity, close_short?, discrepancy_reason? }
export function receiveTransferReceiptItem(receiptId, itemId, payload) {
  return apiPost(`/transfer-receipts/${receiptId}/items/${itemId}/receive`, payload)
}

// payload: { idempotency_key }
export function confirmTransferReceipt(id, payload) {
  return apiPost(`/transfer-receipts/${id}/confirm`, payload)
}

// payload is optional ({ remark }) — DRAFT only.
export function cancelTransferReceipt(id, payload) {
  return apiPost(`/transfer-receipts/${id}/cancel`, payload || {})
}

export function newIdempotencyKey() {
  return crypto.randomUUID()
}

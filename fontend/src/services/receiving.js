import { apiGet, apiPost, apiPut, apiDownload, triggerDownload } from './api'

export function listReceipts(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  return apiGet(`/receipts${qs ? `?${qs}` : ''}`)
}

// Phase 22 — Receiving History CSV export, honoring the same filters
// as listReceipts (minus pagination).
export async function exportReceipts(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/receipts/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getReceipt(id) {
  return apiGet(`/receipts/${id}`)
}

export function createReceipt(payload) {
  return apiPost('/receipts', payload)
}

export function updateReceipt(id, payload) {
  return apiPut(`/receipts/${id}`, payload)
}

export function addReceiptItem(id, payload) {
  return apiPost(`/receipts/${id}/items`, payload)
}

export function updateReceiptItem(id, itemId, payload) {
  return apiPut(`/receipts/${id}/items/${itemId}`, payload)
}

export function confirmReceipt(id, payload) {
  return apiPost(`/receipts/${id}/confirm`, payload)
}

// Phase 15 — only allowed while the receipt is still DRAFT/WAITING
// (before any stock has been received); payload is optional ({ remark }).
export function cancelReceipt(id, payload) {
  return apiPost(`/receipts/${id}/cancel`, payload || {})
}

// A fresh key per confirm click — see the backend's idempotency design
// (receipt_confirmations). crypto.randomUUID() is available in all
// modern browsers over HTTPS/localhost, which is how this app is always
// served.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

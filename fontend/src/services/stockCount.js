import { apiGet, apiPost, apiPut, apiDownload, triggerDownload } from './api'

// Phase 14 — backs the Stock Count History list. The endpoint itself
// has existed since Phase 11 with no frontend consumer.
export function listStockCounts(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  return apiGet(`/stock-counts${qs ? `?${qs}` : ''}`)
}

// Phase 22 — Stock Count History CSV export, honoring the same filters
// as listStockCounts (minus pagination).
export async function exportStockCounts(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/stock-counts/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getStockCount(id) {
  return apiGet(`/stock-counts/${id}`)
}

export function createStockCount(payload) {
  return apiPost('/stock-counts', payload)
}

export function addStockCountItem(countId, payload) {
  return apiPost(`/stock-counts/${countId}/items`, payload)
}

export function recordStockCount(countId, itemId, payload) {
  return apiPut(`/stock-counts/${countId}/items/${itemId}`, payload)
}

export function confirmStockCount(countId, payload) {
  return apiPost(`/stock-counts/${countId}/confirm`, payload)
}

export function createAdjustmentsFromCount(countId) {
  return apiPost(`/stock-counts/${countId}/create-adjustments`, {})
}

// A fresh key per confirm click — see the backend's idempotency design
// (stock_count_confirmations), the same pattern every prior phase's
// service module uses.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

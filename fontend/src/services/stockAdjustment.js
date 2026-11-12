import { apiGet, apiPost, apiPut, apiDownload, triggerDownload } from './api'

export function listStockAdjustments(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.type) query.set('type', params.type)
  if (params.status) query.set('status', params.status)
  if (params.date) query.set('date', params.date)
  // Phase 20 — pending-adjustment follow-up filters. The backend
  // computes age/overdue server-side (see internal/stockadjustment's
  // pendingAging); overdue=true is the "Overdue" quick-filter.
  if (params.overdue) query.set('overdue', 'true')
  if (params.olderThanDays) query.set('older_than_days', params.olderThanDays)
  if (params.createdBy) query.set('created_by', params.createdBy)
  if (params.reason) query.set('reason', params.reason)

  const qs = query.toString()
  return apiGet(`/stock-adjustments${qs ? `?${qs}` : ''}`)
}

// Phase 22 — Stock Adjustment History CSV export, honoring the same
// filters as listStockAdjustments (minus pagination).
export async function exportStockAdjustments(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.type) query.set('type', params.type)
  if (params.status) query.set('status', params.status)
  if (params.date) query.set('date', params.date)
  if (params.overdue) query.set('overdue', 'true')

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/stock-adjustments/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getStockAdjustment(id) {
  return apiGet(`/stock-adjustments/${id}`)
}

export function createStockAdjustment(payload) {
  return apiPost('/stock-adjustments', payload)
}

export function updateStockAdjustment(id, payload) {
  return apiPut(`/stock-adjustments/${id}`, payload)
}

export function confirmStockAdjustment(id, payload) {
  return apiPost(`/stock-adjustments/${id}/confirm`, payload)
}

// A fresh key per confirm click — see the backend's idempotency design
// (stock_adjustment_confirmations), the same pattern every prior
// phase's service module uses.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

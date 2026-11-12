import { apiGet, apiPost, apiDownload, triggerDownload } from './api'

export function listPicking(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.priority) query.set('priority', params.priority)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  return apiGet(`/picking${qs ? `?${qs}` : ''}`)
}

// Phase 22 — Picking History CSV export, honoring the same filters as
// listPicking (minus pagination).
export async function exportPicking(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.priority) query.set('priority', params.priority)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/picking/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getPickingTask(id) {
  return apiGet(`/picking/${id}`)
}

export function createPickingTask(payload) {
  return apiPost('/picking', payload)
}

export function confirmPickingTask(id, payload) {
  return apiPost(`/picking/${id}/confirm`, payload)
}

// Phase 13 — releases whatever remains reserved for the task's still-
// open lines and marks it CANCELLED; payload is optional ({ remark }).
export function cancelPickingTask(id, payload) {
  return apiPost(`/picking/${id}/cancel`, payload || {})
}

// A fresh key per confirm click — see the backend's idempotency design
// (picking_confirmations), the same pattern services/receiving.js and
// services/putaway.js use.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

import { apiGet, apiPost, apiDownload, triggerDownload } from './api'

export function listPutaway(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.zoneId) query.set('zone_id', params.zoneId)

  const qs = query.toString()
  return apiGet(`/putaway${qs ? `?${qs}` : ''}`)
}

// Phase 22 — Putaway History CSV export, honoring the same filters as
// listPutaway (minus pagination).
export async function exportPutaway(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.zoneId) query.set('zone_id', params.zoneId)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/putaway/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getPutawayTask(id) {
  return apiGet(`/putaway/${id}`)
}

export function getPutawaySources(receiptId) {
  return apiGet(`/putaway/sources/${receiptId}`)
}

export function createPutawayTask(payload) {
  return apiPost('/putaway', payload)
}

export function confirmPutawayTask(id, payload) {
  return apiPost(`/putaway/${id}/confirm`, payload)
}

// Phase 15 — only allowed while no line has moved stock yet; payload is
// optional ({ remark }).
export function cancelPutawayTask(id, payload) {
  return apiPost(`/putaway/${id}/cancel`, payload || {})
}

// A fresh key per confirm click — see the backend's idempotency design
// (putaway_confirmations), the same pattern services/receiving.js uses.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

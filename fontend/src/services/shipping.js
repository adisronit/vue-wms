import { apiGet, apiPost, apiPut, apiDownload, triggerDownload } from './api'

export function listShipments(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.carrier) query.set('carrier', params.carrier)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  return apiGet(`/shipping${qs ? `?${qs}` : ''}`)
}

// Phase 22 — Shipping History CSV export, honoring the same filters as
// listShipments (minus pagination).
export async function exportShipments(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.carrier) query.set('carrier', params.carrier)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/shipping/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getShipment(id) {
  return apiGet(`/shipping/${id}`)
}

// Completed picking tasks with no shipment yet — powers the "+ New
// Shipment" drawer's Source Picking Task selector (the mock itself has
// no way to choose which order to ship at all).
export function listShippableSources() {
  return apiGet('/shipping/sources')
}

export function createShipment(payload) {
  return apiPost('/shipping', payload)
}

export function updateShipment(id, payload) {
  return apiPut(`/shipping/${id}`, payload)
}

export function confirmShipment(id, payload) {
  return apiPost(`/shipping/${id}/confirm`, payload)
}

// Phase 15 — only allowed while the shipment is still READY (before
// dispatch); payload is optional ({ remark }).
export function cancelShipment(id, payload) {
  return apiPost(`/shipping/${id}/cancel`, payload || {})
}

// A fresh key per confirm click — see the backend's idempotency design
// (shipment_confirmations), the same pattern services/picking.js uses.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

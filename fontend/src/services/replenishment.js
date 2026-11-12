import { apiGet, apiPost } from './api'

export function listReplenishment(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.productId) query.set('product_id', params.productId)

  const qs = query.toString()
  return apiGet(`/replenishment${qs ? `?${qs}` : ''}`)
}

// Phase 24's "simple generate/recommend endpoint" — advisory candidates
// only, each one maps directly onto createReplenishmentTask's payload.
export function getReplenishmentRecommendations(warehouseId) {
  return apiGet(`/replenishment/recommendations?warehouse_id=${warehouseId}`)
}

export function getReplenishmentTask(id) {
  return apiGet(`/replenishment/${id}`)
}

export function createReplenishmentTask(payload) {
  return apiPost('/replenishment', payload)
}

export function startReplenishmentTask(id) {
  return apiPost(`/replenishment/${id}/start`, {})
}

export function confirmReplenishmentTask(id, payload) {
  return apiPost(`/replenishment/${id}/confirm`, payload)
}

export function cancelReplenishmentTask(id, payload) {
  return apiPost(`/replenishment/${id}/cancel`, payload || {})
}

export function reportReplenishmentIssue(id, payload) {
  return apiPost(`/replenishment/${id}/issue`, payload)
}

// A fresh key per confirm click — the same idempotency pattern every
// other workflow service in this codebase uses.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

import { apiGet, apiPost, apiPut, apiDownload, triggerDownload, openInNewTab } from './api'

export function listOrders(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)

  const qs = query.toString()
  return apiGet(`/orders${qs ? `?${qs}` : ''}`)
}

export function getOrder(id) {
  return apiGet(`/orders/${id}`)
}

export function createOrder(payload) {
  return apiPost('/orders', payload)
}

export function updateOrder(id, payload) {
  return apiPut(`/orders/${id}`, payload)
}

export function createOrderPickingTask(id, payload) {
  return apiPost(`/orders/${id}/create-picking-task`, payload)
}

export function cancelOrder(id, payload) {
  return apiPost(`/orders/${id}/cancel`, payload || {})
}

// getOrderAvailability backs the Review & Release screen — a pure,
// read-only inventory-availability preview per line (never a
// reservation). The same computation Release itself re-runs server-side
// before deciding whether PENDING -> RELEASED may proceed.
export function getOrderAvailability(id) {
  return apiGet(`/orders/${id}/availability`)
}

// releaseOrder is the new Review & Release step (PENDING -> RELEASED),
// gating createOrderPickingTask — see internal/order's own Status.
// CanCreatePickingTask doc. approveShortage must be explicitly set when
// the availability check found a shortage, matching the backend's own
// "Controlled shortage approval" gate.
export function releaseOrder(id, payload) {
  return apiPost(`/orders/${id}/release`, payload || {})
}

// viewOrderDocument/downloadOrderDocument mirror internal/relocation's
// own inline-vs-attachment PDF convention (?download=true) and the
// frontend's own existing triggerDownload/openInNewTab helpers.
export async function viewOrderDocument(id) {
  const { blob } = await apiDownload(`/orders/${id}/pdf`)
  openInNewTab(blob)
}

export async function downloadOrderDocument(id) {
  const { blob, filename } = await apiDownload(`/orders/${id}/pdf?download=true`)
  triggerDownload(blob, filename)
}

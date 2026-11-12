import { apiGet, apiPost, apiPut } from './api'

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

import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listCustomers mirrors the Customer screen's search box plus status
// filter. No referential blocking exists for customers today, so
// deleteCustomer always succeeds for an existing row (see service docs
// in the CustomerView).
export function listCustomers(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)

  const qs = query.toString()
  return apiGet(`/customers${qs ? `?${qs}` : ''}`)
}

export function getCustomer(id) {
  return apiGet(`/customers/${id}`)
}

export function createCustomer(payload) {
  return apiPost('/customers', payload)
}

export function updateCustomer(id, payload) {
  return apiPut(`/customers/${id}`, payload)
}

export function deleteCustomer(id) {
  return apiDelete(`/customers/${id}`)
}

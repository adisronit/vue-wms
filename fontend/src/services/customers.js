import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listCustomers mirrors the Customer screen's search box plus status
// filter, pagination, and sorting. sortBy/sortDir are passed through
// as-is; the backend is the sole enforcer of the sort-column allowlist
// (400 INVALID_SORT on an unrecognized value) rather than this layer
// silently coercing a bad value to the default. No referential
// blocking exists for customers today, so deleteCustomer always
// succeeds for an existing row (see service docs in the CustomerView).
export function listCustomers(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.sortBy) query.set('sort_by', params.sortBy)
  if (params.sortDir) query.set('sort_dir', params.sortDir)

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

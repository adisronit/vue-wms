import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listSuppliers mirrors the Supplier screen's search box plus status
// filter, pagination, and sorting. sortBy/sortDir are passed through
// as-is; the backend is the sole enforcer of the sort-column allowlist
// (400 INVALID_SORT on an unrecognized value) rather than this layer
// silently coercing a bad value to the default — see
// services/productCategories.js's listProductCategories for the same
// pattern.
export function listSuppliers(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.sortBy) query.set('sort_by', params.sortBy)
  if (params.sortDir) query.set('sort_dir', params.sortDir)

  const qs = query.toString()
  return apiGet(`/suppliers${qs ? `?${qs}` : ''}`)
}

export function getSupplier(id) {
  return apiGet(`/suppliers/${id}`)
}

export function createSupplier(payload) {
  return apiPost('/suppliers', payload)
}

export function updateSupplier(id, payload) {
  return apiPut(`/suppliers/${id}`, payload)
}

export function deleteSupplier(id) {
  return apiDelete(`/suppliers/${id}`)
}

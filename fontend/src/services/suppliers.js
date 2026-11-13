import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listSuppliers mirrors the Supplier screen's search box plus status
// filter. Identical shape to services/customers.js — same field names,
// different endpoint.
export function listSuppliers(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)

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

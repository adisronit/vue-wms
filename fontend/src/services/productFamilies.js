import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listProductFamilies mirrors the Family selector's search box plus
// status filter, with pagination and sorting — identical shape to
// services/brands.js.
export function listProductFamilies(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.sortBy) query.set('sort_by', params.sortBy)
  if (params.sortDir) query.set('sort_dir', params.sortDir)

  const qs = query.toString()
  return apiGet(`/product-families${qs ? `?${qs}` : ''}`)
}

export function getProductFamily(id) {
  return apiGet(`/product-families/${id}`)
}

export function createProductFamily(payload) {
  return apiPost('/product-families', payload)
}

export function updateProductFamily(id, payload) {
  return apiPut(`/product-families/${id}`, payload)
}

export function deleteProductFamily(id) {
  return apiDelete(`/product-families/${id}`)
}

import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listBrands mirrors the Brand selector's search box plus status
// filter, with pagination and sorting — identical shape to
// services/productCategories.js's listProductCategories.
export function listBrands(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.sortBy) query.set('sort_by', params.sortBy)
  if (params.sortDir) query.set('sort_dir', params.sortDir)

  const qs = query.toString()
  return apiGet(`/brands${qs ? `?${qs}` : ''}`)
}

export function getBrand(id) {
  return apiGet(`/brands/${id}`)
}

export function createBrand(payload) {
  return apiPost('/brands', payload)
}

export function updateBrand(id, payload) {
  return apiPut(`/brands/${id}`, payload)
}

export function deleteBrand(id) {
  return apiDelete(`/brands/${id}`)
}

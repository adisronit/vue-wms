import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listProductCategories mirrors the Product Category screen's search box
// plus parent/status filters. parent_id is a string ID (or omitted) —
// the backend returns/accepts category IDs as strings house-wide.
export function listProductCategories(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.parentId) query.set('parent_id', params.parentId)
  if (params.status) query.set('status', params.status)

  const qs = query.toString()
  return apiGet(`/product-categories${qs ? `?${qs}` : ''}`)
}

export function getProductCategory(id) {
  return apiGet(`/product-categories/${id}`)
}

export function createProductCategory(payload) {
  return apiPost('/product-categories', payload)
}

export function updateProductCategory(id, payload) {
  return apiPut(`/product-categories/${id}`, payload)
}

export function deleteProductCategory(id) {
  return apiDelete(`/product-categories/${id}`)
}

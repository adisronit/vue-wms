import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listProductCategories mirrors the Product Category screen's search box
// plus parent/status filters, pagination, and sorting. parent_id is a
// string ID (or omitted) — the backend returns/accepts category IDs as
// strings house-wide. sortBy/sortDir are passed through as-is; the
// backend is the sole enforcer of the sort-column allowlist (400
// INVALID_SORT on an unrecognized value) rather than this layer
// silently coercing a bad value to the default.
export function listProductCategories(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.parentId) query.set('parent_id', params.parentId)
  if (params.status) query.set('status', params.status)
  if (params.sortBy) query.set('sort_by', params.sortBy)
  if (params.sortDir) query.set('sort_dir', params.sortDir)

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

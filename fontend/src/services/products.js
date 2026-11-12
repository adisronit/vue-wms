import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listProducts mirrors the Product Master screen's search box plus
// category/status/lot-rule filters and pagination.
export function listProducts(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.category) query.set('category', params.category)
  if (params.status) query.set('status', params.status)
  if (params.lotRequired) query.set('lot_required', params.lotRequired === 'YES' ? 'true' : 'false')

  const qs = query.toString()
  return apiGet(`/products${qs ? `?${qs}` : ''}`)
}

export function getProduct(id) {
  return apiGet(`/products/${id}`)
}

export function createProduct(payload) {
  return apiPost('/products', payload)
}

export function updateProduct(id, payload) {
  return apiPut(`/products/${id}`, payload)
}

export function deleteProduct(id) {
  return apiDelete(`/products/${id}`)
}

import { apiGet, apiPost, apiPut, apiDelete, apiUpload, apiDownload } from './api'

// listProducts mirrors the Product Master screen's search box plus
// category/brand/status/lot-rule filters and pagination.
export function listProducts(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.category) query.set('category', params.category)
  if (params.categoryId) query.set('category_id', params.categoryId)
  if (params.brandId) query.set('brand_id', params.brandId)
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

// uploadProductImage sends a single JPG/PNG/WebP file (field name
// "image", matching the backend's FormFile("image") call) — the
// backend re-validates the real content (not the filename or
// Content-Type) and the 5MB limit itself; this is just the transport.
export function uploadProductImage(id, file) {
  const formData = new FormData()
  formData.append('image', file)
  return apiUpload(`/products/${id}/image`, formData)
}

export function removeProductImage(id) {
  return apiDelete(`/products/${id}/image`)
}

// fetchProductImageBlob reuses apiDownload's authenticated-fetch-then-
// Blob pattern (Phase 22's CSV export) instead of a plain <img src>,
// since GET /products/:id/image requires the same Bearer token every
// other API call does — a bare <img> tag has no way to attach one.
// Callers turn the Blob into a displayable URL via
// URL.createObjectURL and must URL.revokeObjectURL it when done.
export async function fetchProductImageBlob(id) {
  const { blob } = await apiDownload(`/products/${id}/image`)
  return blob
}

export function listProductBarcodes(productId) {
  return apiGet(`/products/${productId}/barcodes`)
}

export function createProductBarcode(productId, payload) {
  return apiPost(`/products/${productId}/barcodes`, payload)
}

export function updateProductBarcode(productId, barcodeId, payload) {
  return apiPut(`/products/${productId}/barcodes/${barcodeId}`, payload)
}

export function deleteProductBarcode(productId, barcodeId) {
  return apiDelete(`/products/${productId}/barcodes/${barcodeId}`)
}

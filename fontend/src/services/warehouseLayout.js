import { apiGet, apiPut, apiUpload, apiDownload, triggerDownload, openInNewTab } from './api'

export function getLayout(warehouseId) {
  return apiGet(`/warehouses/${warehouseId}/layout`)
}

export function saveLayout(warehouseId, payload) {
  return apiPut(`/warehouses/${warehouseId}/layout`, payload)
}

export function listLayoutVersions(warehouseId) {
  return apiGet(`/warehouses/${warehouseId}/layout/versions`)
}

export function getLayoutVersion(warehouseId, versionId) {
  return apiGet(`/warehouses/${warehouseId}/layout/versions/${versionId}`)
}

export function uploadLayoutBackgroundImage(warehouseId, file) {
  const formData = new FormData()
  formData.append('image', file)
  return apiUpload(`/warehouses/${warehouseId}/layout/images`, formData)
}

export async function previewLayoutPdf(warehouseId) {
  const { blob } = await apiDownload(`/warehouses/${warehouseId}/layout/pdf`)
  return blob
}

export async function downloadLayoutPdf(warehouseId) {
  const { blob, filename } = await apiDownload(`/warehouses/${warehouseId}/layout/pdf?download=true`)
  triggerDownload(blob, filename)
}

export async function openLayoutPdfInNewTab(warehouseId) {
  const { blob } = await apiDownload(`/warehouses/${warehouseId}/layout/pdf`)
  openInNewTab(blob)
}

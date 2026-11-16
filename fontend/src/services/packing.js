import { apiGet, apiPost, apiPut, apiDownload, triggerDownload, openInNewTab } from './api'

// Phase 33C — Packing, mirroring services/picking.js's own shape
// closely (list/export/get/create/cancel/pdf, idempotency key,
// view/download PDF pair). Packing never touches inventory on the
// backend — see backend internal/packing's own package doc — these
// calls are a pure physical-handling/traceability layer over stock
// Picking already removed.

export function listPacking(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)

  const qs = query.toString()
  return apiGet(`/packing-jobs${qs ? `?${qs}` : ''}`)
}

export async function exportPacking(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/packing-jobs/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

// Completed/short-pick picking tasks that have no packing job yet —
// powers the "create packing job from picking task" picker.
export function listPackableSources() {
  return apiGet('/packing-jobs/sources')
}

export function getPackingJob(id) {
  return apiGet(`/packing-jobs/${id}`)
}

export function createPackingJob(payload) {
  return apiPost('/packing-jobs', payload)
}

export function assignPacker(id, packerId) {
  return apiPut(`/packing-jobs/${id}/assign`, { packer_id: packerId })
}

// Scans a product/lot into a carton, incrementing that line's
// packed_qty — overpacking (more than this line's own picked_qty) is
// rejected outright by the backend (422 OVERPACK), never clamped.
export function packItem(jobId, payload) {
  return apiPost(`/packing-jobs/${jobId}/pack`, payload)
}

export function flagPackingException(jobId, itemId, payload) {
  return apiPost(`/packing-jobs/${jobId}/items/${itemId}/exception`, payload)
}

export function createCarton(jobId, payload) {
  return apiPost(`/packing-jobs/${jobId}/cartons`, payload)
}

export function updateCarton(jobId, cartonId, payload) {
  return apiPut(`/packing-jobs/${jobId}/cartons/${cartonId}`, payload)
}

// The second-person checker-verification step — requires every line to
// already be PACKED or have an EXCEPTION flagged; marks the job
// COMPLETED.
export function checkerConfirmPackingJob(jobId, payload) {
  return apiPost(`/packing-jobs/${jobId}/checker-confirm`, payload || {})
}

export function cancelPackingJob(id, payload) {
  return apiPost(`/packing-jobs/${id}/cancel`, payload || {})
}

export function newIdempotencyKey() {
  return crypto.randomUUID()
}

// Packing List PDF — job header + every line + which carton(s) each
// went into.
export async function viewPackingList(id) {
  const { blob } = await apiDownload(`/packing-jobs/${id}/pdf`)
  openInNewTab(blob)
}

export async function downloadPackingList(id) {
  const { blob, filename } = await apiDownload(`/packing-jobs/${id}/pdf?download=true`)
  triggerDownload(blob, filename)
}

// Carton/Pallet Label PDF — omit cartonId to print every carton in the
// job (one compact 4x6" label per page); pass it to print just one.
export async function viewCartonLabels(jobId, cartonId) {
  const suffix = cartonId ? `?carton_id=${cartonId}` : ''
  const { blob } = await apiDownload(`/packing-jobs/${jobId}/cartons/label${suffix}`)
  openInNewTab(blob)
}

export async function downloadCartonLabels(jobId, cartonId) {
  const query = cartonId ? `carton_id=${cartonId}&download=true` : 'download=true'
  const { blob, filename } = await apiDownload(`/packing-jobs/${jobId}/cartons/label?${query}`)
  triggerDownload(blob, filename)
}

// Phase 33C.3 — A4 fallback for the Carton/Pallet Label: the exact same
// label content rendered on a full A4 page instead of the 4x6" media,
// for warehouses without a dedicated label printer.
export async function viewCartonLabelsA4(jobId, cartonId) {
  const params = new URLSearchParams({ format: 'A4' })
  if (cartonId) params.set('carton_id', cartonId)
  const { blob } = await apiDownload(`/packing-jobs/${jobId}/cartons/label?${params.toString()}`)
  openInNewTab(blob)
}

export async function downloadCartonLabelsA4(jobId, cartonId) {
  const params = new URLSearchParams({ format: 'A4', download: 'true' })
  if (cartonId) params.set('carton_id', cartonId)
  const { blob, filename } = await apiDownload(`/packing-jobs/${jobId}/cartons/label?${params.toString()}`)
  triggerDownload(blob, filename)
}

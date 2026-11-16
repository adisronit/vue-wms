import { apiGet, apiPost, apiPut, apiDownload, triggerDownload, openInNewTab } from './api'

export function listInspections(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)

  const qs = query.toString()
  return apiGet(`/qc/inspections${qs ? `?${qs}` : ''}`)
}

export function getInspection(id) {
  return apiGet(`/qc/inspections/${id}`)
}

// Lines on receiptId whose product requires QC and still carry
// inventory hold — powers the "Receiving No." picker's preview.
export function listEligibleSplits(receiptId) {
  const query = new URLSearchParams()
  query.set('receipt_id', receiptId)
  return apiGet(`/qc/eligible-splits?${query.toString()}`)
}

export function createInspection(payload) {
  return apiPost('/qc/inspections', payload)
}

export function updateInspectionItem(inspectionId, itemId, payload) {
  return apiPut(`/qc/inspections/${inspectionId}/items/${itemId}`, payload)
}

export function confirmInspection(inspectionId, payload) {
  return apiPost(`/qc/inspections/${inspectionId}/confirm`, payload)
}

// A fresh key per confirm click — identical convention to
// internal/receiving's own idempotency design.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

// Phase 31D — QC Inspection Report PDF. Read-only on the backend
// (internal/inbounddoc) — viewing/downloading never changes the
// inspection's own status or any disposition.
export async function viewQCReport(inspectionId) {
  const { blob } = await apiDownload(`/qc/inspections/${inspectionId}/pdf`)
  openInNewTab(blob)
}

export async function downloadQCReport(inspectionId) {
  const { blob, filename } = await apiDownload(`/qc/inspections/${inspectionId}/pdf?download=true`)
  triggerDownload(blob, filename)
}

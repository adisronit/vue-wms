import { apiGet, apiPost, apiDownload, triggerDownload, openInNewTab } from './api'

// Phase 32 — Stock Relocation, mirroring services/putaway.js's own
// shape closely (list/export/get/create/confirm/cancel/pdf), with one
// addition (ready) matching the backend's own extra DRAFT -> READY
// transition — see internal/relocation/service.go.

export function listRelocations(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.taskStatus) query.set('task_status', params.taskStatus)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)

  const qs = query.toString()
  return apiGet(`/relocations${qs ? `?${qs}` : ''}`)
}

export async function exportRelocations(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.taskStatus) query.set('task_status', params.taskStatus)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/relocations/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getRelocationTask(id) {
  return apiGet(`/relocations/${id}`)
}

export function createRelocationTask(payload) {
  return apiPost('/relocations', payload)
}

// Locks a DRAFT task's lines and allows Confirm to run — requires at
// least one item (backend: ErrEmptyTask).
export function readyRelocationTask(id) {
  return apiPost(`/relocations/${id}/ready`, {})
}

export function confirmRelocationTask(id, payload) {
  return apiPost(`/relocations/${id}/confirm`, payload)
}

// Allowed from DRAFT, READY, or IN_PROGRESS — including a task with
// lines already partially moved (see internal/relocation/service.go's
// Cancel doc for why this deliberately differs from Putaway's stricter
// rule); payload is optional ({ remark }).
export function cancelRelocationTask(id, payload) {
  return apiPost(`/relocations/${id}/cancel`, payload || {})
}

export function newIdempotencyKey() {
  return crypto.randomUUID()
}

// Relocation Transfer Note PDF. Read-only on the backend — viewing/
// downloading never changes the task's own status or moves any stock.
export async function viewRelocationTransferNote(id) {
  const { blob } = await apiDownload(`/relocations/${id}/pdf`)
  openInNewTab(blob)
}

export async function downloadRelocationTransferNote(id) {
  const { blob, filename } = await apiDownload(`/relocations/${id}/pdf?download=true`)
  triggerDownload(blob, filename)
}

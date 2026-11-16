import { apiGet, apiPost, apiPut, apiDownload, triggerDownload, openInNewTab } from './api'

export function listPicking(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.priority) query.set('priority', params.priority)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  return apiGet(`/picking${qs ? `?${qs}` : ''}`)
}

// Phase 22 — Picking History CSV export, honoring the same filters as
// listPicking (minus pagination).
export async function exportPicking(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.priority) query.set('priority', params.priority)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.date) query.set('date', params.date)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/picking/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export function getPickingTask(id) {
  return apiGet(`/picking/${id}`)
}

export function createPickingTask(payload) {
  return apiPost('/picking', payload)
}

export function confirmPickingTask(id, payload) {
  return apiPost(`/picking/${id}/confirm`, payload)
}

// Phase 13 — releases whatever remains reserved for the task's still-
// open lines and marks it CANCELLED; payload is optional ({ remark }).
export function cancelPickingTask(id, payload) {
  return apiPost(`/picking/${id}/cancel`, payload || {})
}

// A fresh key per confirm click — see the backend's idempotency design
// (picking_confirmations), the same pattern services/receiving.js and
// services/putaway.js use.
export function newIdempotencyKey() {
  return crypto.randomUUID()
}

// Phase 33B — assign (or reassign) a picker to a task. Never touches
// reservation/stock state on the backend; pickerId must be a real
// user id (see backend internal/picking/service.go's AssignPicker doc).
export function assignPicker(id, pickerId) {
  return apiPut(`/picking/${id}/assign`, { picker_id: pickerId })
}

// Phase 33B — validates a scanned location/product/lot against one
// line's own assigned ones. Read-only against inventory on the backend
// (logs the attempt to the scan audit trail either way); returns
// { matched, detail }.
export function scanConfirmPickingItem(taskId, itemId, payload) {
  return apiPost(`/picking/${taskId}/items/${itemId}/scan`, payload)
}

// Phase 33B — reopens a SHORT line for another pick attempt, re-
// reserving its still-outstanding quantity. Fails (422) if the source
// location still has no available stock.
export function repickPickingItem(taskId, itemId) {
  return apiPost(`/picking/${taskId}/items/${itemId}/repick`, {})
}

// Phase 33B — read-only FEFO/FIFO-ordered available lots for a
// product at a warehouse, used to suggest/validate lot selection.
export function suggestPickingLots(productId, warehouseId) {
  return apiGet(`/picking/lots/suggest?product_id=${productId}&warehouse_id=${warehouseId}`)
}

// Phase 33B — Pick List / Picking Work Order PDF, matching services/
// relocation.js's own view/download pair for its Transfer Note PDF.
export async function viewPickList(id) {
  const { blob } = await apiDownload(`/picking/${id}/pdf`)
  openInNewTab(blob)
}

export async function downloadPickList(id) {
  const { blob, filename } = await apiDownload(`/picking/${id}/pdf?download=true`)
  triggerDownload(blob, filename)
}

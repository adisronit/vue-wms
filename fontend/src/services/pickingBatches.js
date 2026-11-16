import { apiGet, apiPost, apiPut, apiDownload, triggerDownload, openInNewTab } from './api'

// Phase 33C.1 — Batch Picking / Wave Management. Mirrors services/
// relocation.js's own list/get/create/confirm/cancel shape — a batch is
// its own header/workflow object (DRAFT consolidation preview ->
// CONFIRMED, via POST .../confirm, which creates one real picking task
// under the hood) rather than a picking.js extension; see internal/
// pickingbatch's own package doc.

export function listPickingBatches(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.status) query.set('status', params.status)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)

  const qs = query.toString()
  return apiGet(`/picking-batches${qs ? `?${qs}` : ''}`)
}

// listAvailableOrdersForBatch backs the multi-select source — every
// RELEASED, PICK_BY_PRODUCT order in warehouseId not already a member
// of another active (DRAFT/CONFIRMED) batch.
export function listAvailableOrdersForBatch(warehouseId) {
  const query = new URLSearchParams()
  query.set('warehouse_id', warehouseId)
  return apiGet(`/picking-batches/available-orders?${query.toString()}`)
}

export function getPickingBatch(id) {
  return apiGet(`/picking-batches/${id}`)
}

// createPickingBatch is the DRAFT consolidation step — read-only
// against inventory, no picking task exists yet. payload:
// { warehouseId, orderIds: [] }.
export function createPickingBatch(payload) {
  return apiPost('/picking-batches', {
    warehouse_id: payload.warehouseId,
    order_ids: payload.orderIds,
  })
}

// confirmPickingBatch is the real commit point — atomically claims
// every included order and creates one real picking task from the
// consolidated lines.
export function confirmPickingBatch(id) {
  return apiPost(`/picking-batches/${id}/confirm`, {})
}

// cancelPickingBatch is DRAFT-only — a CONFIRMED batch's own picking
// task must be cancelled via picking.js's own cancelPickingTask
// instead (see internal/pickingbatch's own Service.Cancel doc).
export function cancelPickingBatch(id, payload) {
  return apiPost(`/picking-batches/${id}/cancel`, payload || {})
}

// ---------------------------------------------------------------------
// Phase 33C.2 — Sortation. Only available once the batch's underlying
// picking task has reached COMPLETED or SHORT_PICK — see internal/
// pickingbatch's own sortationGate doc.
// ---------------------------------------------------------------------

// getSortation returns the per-order-share worklist for batchId.
export function getSortation(batchId) {
  return apiGet(`/picking-batches/${batchId}/sortation`)
}

// scanSortation is the physical confirmation step — payload:
// { orderBarcode, sku, lotNo, quantity }.
export function scanSortation(batchId, payload) {
  return apiPost(`/picking-batches/${batchId}/sortation/scan`, {
    order_barcode: payload.orderBarcode,
    sku: payload.sku,
    lot_no: payload.lotNo || undefined,
    quantity: payload.quantity,
  })
}

// flagSortationException records a shortage/damage exception on one
// specific allocation line — payload: { allocationId, shortageQty, reason }.
export function flagSortationException(batchId, payload) {
  return apiPost(`/picking-batches/${batchId}/sortation/exception`, {
    allocation_id: payload.allocationId,
    shortage_qty: payload.shortageQty,
    reason: payload.reason,
  })
}

// assignSortation sets the station code and/or assigned operator —
// a planning annotation, never an authorization gate.
export function assignSortation(batchId, payload) {
  return apiPut(`/picking-batches/${batchId}/sortation/assign`, {
    station: payload.station || undefined,
    user_id: payload.userId || undefined,
  })
}

// Phase 33C.3 — Consolidated Batch Picking List PDF. Only valid once
// the batch is CONFIRMED (or COMPLETED, mirroring its own task's
// terminal status) — a DRAFT batch has no real picking task yet (see
// backend internal/pickingbatch/pdf.go's own RenderConsolidatedPickList
// doc).
export async function viewConsolidatedPickList(batchId) {
  const { blob } = await apiDownload(`/picking-batches/${batchId}/pdf`)
  openInNewTab(blob)
}

export async function downloadConsolidatedPickList(batchId) {
  const { blob, filename } = await apiDownload(`/picking-batches/${batchId}/pdf?download=true`)
  triggerDownload(blob, filename)
}

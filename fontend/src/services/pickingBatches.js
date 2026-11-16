import { apiGet, apiPost } from './api'

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

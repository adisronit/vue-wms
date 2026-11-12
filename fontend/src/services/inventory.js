import { apiGet, apiPost, apiDownload, triggerDownload } from './api'

// Inventory Core is read-only from the frontend this phase — no
// create/update/delete here. Stock-changing operations only exist as an
// internal Go service (AddStock/RemoveStock/MoveStock) with no HTTP
// surface yet; Receiving/Putaway/Picking/Shipping/Adjustment/Count will
// call it in later phases.

export function listInventory(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.productId) query.set('product_id', params.productId)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.zoneId) query.set('zone_id', params.zoneId)
  if (params.locationId) query.set('location_id', params.locationId)
  if (params.status) query.set('status', params.status)
  if (params.lotNo) query.set('lot_no', params.lotNo)

  const qs = query.toString()
  return apiGet(`/inventory${qs ? `?${qs}` : ''}`)
}

export function getInventory(id) {
  return apiGet(`/inventory/${id}`)
}

export function getInventorySummary() {
  return apiGet('/inventory/summary')
}

export function getInventoryTransactions(id) {
  return apiGet(`/inventory/${id}/transactions`)
}

// Phase 22 — Inventory Stock Report CSV export (synchronous), honoring
// the same filters as listInventory (minus pagination). Requires
// stock_inquiry.export in addition to stock_inquiry.view.
export async function exportInventory(params = {}) {
  const query = new URLSearchParams()
  if (params.search) query.set('search', params.search)
  if (params.warehouseId) query.set('warehouse_id', params.warehouseId)
  if (params.zoneId) query.set('zone_id', params.zoneId)
  if (params.locationId) query.set('location_id', params.locationId)
  if (params.status) query.set('status', params.status)
  if (params.lotNo) query.set('lot_no', params.lotNo)

  const qs = query.toString()
  const { blob, filename } = await apiDownload(`/inventory/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

// Phase 22 — Inventory Transaction History, the one async export
// (the ledger is append-only and can outgrow a synchronous request).
// Queues a job and returns its ID; the caller polls getExportJob until
// status is COMPLETED (or FAILED), then calls downloadExportJob.
export function createInventoryTransactionsExport(filters = {}) {
  return apiPost('/reports/inventory-transactions/export', {
    product_id: filters.productId || undefined,
    warehouse_id: filters.warehouseId || undefined,
    transaction_type: filters.transactionType || undefined,
    date_from: filters.dateFrom || undefined,
    date_to: filters.dateTo || undefined
  })
}

export function getExportJob(jobId) {
  return apiGet(`/reports/export-jobs/${jobId}`)
}

export async function downloadExportJob(jobId) {
  const { blob, filename } = await apiDownload(`/reports/export-jobs/${jobId}/download`)
  triggerDownload(blob, filename)
}

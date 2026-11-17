import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'
import { buildReportQuery } from '../utils/reportFilters'

function reportFilterParams(filters = {}) {
  return {
    warehouse_id: filters.warehouseId,
    zone_id: filters.zoneId,
    location_id: filters.locationId,
    storage_type: filters.storageType,
    search: filters.search,
    lot_no: filters.lotNo,
    below_qty: filters.belowQty,
    expiring_within_days: filters.expiringWithinDays,
    expired_only: filters.expiredOnly ? 'true' : undefined,
    inactive_days: filters.inactiveDays,
  }
}

export function listInventoryReport(filters = {}, { page, limit, sortBy, sortDir } = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), page, limit, sort_by: sortBy, sort_dir: sortDir })
  return apiGet(`/reports/inventory${qs ? `?${qs}` : ''}`)
}

export function getInventoryReportSummary(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  return apiGet(`/reports/inventory/summary${qs ? `?${qs}` : ''}`)
}

export async function exportInventoryReportCsv(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/inventory/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function exportInventoryReportXlsx(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/inventory/export/xlsx${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function previewInventoryReportPdf(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/inventory/pdf${qs ? `?${qs}` : ''}`)
  return blob
}

export async function downloadInventoryReportPdf(filters = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), download: 'true' })
  const { blob, filename } = await apiDownload(`/reports/inventory/pdf${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function openInventoryReportPdfInNewTab(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/inventory/pdf${qs ? `?${qs}` : ''}`)
  openInNewTab(blob)
}

// --- Stock Movement (separate grain) ---

function movementFilterParams(filters = {}) {
  return {
    from: filters.from,
    to: filters.to,
    warehouse_id: filters.warehouseId,
    location_id: filters.locationId,
    transaction_type: filters.transactionType,
    search: filters.search,
  }
}

export function listInventoryMovements(filters = {}, { page, limit } = {}) {
  const qs = buildReportQuery({ ...movementFilterParams(filters), page, limit })
  return apiGet(`/reports/inventory/movements${qs ? `?${qs}` : ''}`)
}

export async function exportInventoryMovementsCsv(filters = {}) {
  const qs = buildReportQuery(movementFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/inventory/movements/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

// --- Historical As-of Balance ---

export function listInventoryAsOf(asOf, filters = {}, { page, limit } = {}) {
  const qs = buildReportQuery({
    as_of: asOf,
    warehouse_id: filters.warehouseId,
    location_id: filters.locationId,
    search: filters.search,
    page, limit,
  })
  return apiGet(`/reports/inventory/as-of${qs ? `?${qs}` : ''}`)
}

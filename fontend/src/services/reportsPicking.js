import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'
import { buildReportQuery } from '../utils/reportFilters'

function reportFilterParams(filters = {}) {
  return {
    from: filters.from,
    to: filters.to,
    warehouse_id: filters.warehouseId,
    picker_id: filters.pickerId,
    status: filters.status,
    item_status: filters.itemStatus,
    short_only: filters.shortOnly ? 'true' : undefined,
    search: filters.search,
    lot_no: filters.lotNo,
  }
}

export function listPickingReport(filters = {}, { page, limit, sortBy, sortDir } = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), page, limit, sort_by: sortBy, sort_dir: sortDir })
  return apiGet(`/reports/picking${qs ? `?${qs}` : ''}`)
}

export function getPickingReportSummary(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  return apiGet(`/reports/picking/summary${qs ? `?${qs}` : ''}`)
}

export async function exportPickingReportCsv(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/picking/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function exportPickingReportXlsx(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/picking/export/xlsx${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function previewPickingReportPdf(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/picking/pdf${qs ? `?${qs}` : ''}`)
  return blob
}

export async function downloadPickingReportPdf(filters = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), download: 'true' })
  const { blob, filename } = await apiDownload(`/reports/picking/pdf${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function openPickingReportPdfInNewTab(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/picking/pdf${qs ? `?${qs}` : ''}`)
  openInNewTab(blob)
}

// --- Batch/Wave Picking Report (separate grain — see backend package doc) ---

function batchFilterParams(filters = {}) {
  return { from: filters.from, to: filters.to, warehouse_id: filters.warehouseId, search: filters.search }
}

export function listPickingBatchAllocations(filters = {}, { page, limit } = {}) {
  const qs = buildReportQuery({ ...batchFilterParams(filters), page, limit })
  return apiGet(`/reports/picking/batch-allocations${qs ? `?${qs}` : ''}`)
}

export async function exportPickingBatchAllocationsCsv(filters = {}) {
  const qs = buildReportQuery(batchFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/picking/batch-allocations/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

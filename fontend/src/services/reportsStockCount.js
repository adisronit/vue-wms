import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'
import { buildReportQuery } from '../utils/reportFilters'

function reportFilterParams(filters = {}) {
  return {
    from: filters.from,
    to: filters.to,
    warehouse_id: filters.warehouseId,
    zone_id: filters.zoneId,
    location_id: filters.locationId,
    counter_id: filters.counterId,
    status: filters.status,
    item_status: filters.itemStatus,
    search: filters.search,
    variance_only: filters.varianceOnly ? 'true' : undefined,
    has_adjustment: filters.hasAdjustment,
  }
}

export function listStockCountReport(filters = {}, { page, limit, sortBy, sortDir } = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), page, limit, sort_by: sortBy, sort_dir: sortDir })
  return apiGet(`/reports/stock-count${qs ? `?${qs}` : ''}`)
}

export function getStockCountReportSummary(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  return apiGet(`/reports/stock-count/summary${qs ? `?${qs}` : ''}`)
}

export async function exportStockCountReportCsv(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/stock-count/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function exportStockCountReportXlsx(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/stock-count/export/xlsx${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function previewStockCountReportPdf(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/stock-count/pdf${qs ? `?${qs}` : ''}`)
  return blob
}

export async function downloadStockCountReportPdf(filters = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), download: 'true' })
  const { blob, filename } = await apiDownload(`/reports/stock-count/pdf${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function openStockCountReportPdfInNewTab(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/stock-count/pdf${qs ? `?${qs}` : ''}`)
  openInNewTab(blob)
}

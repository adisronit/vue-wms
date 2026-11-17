import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'
import { buildReportQuery } from '../utils/reportFilters'

function filterParams(filters = {}) {
  return {
    from: filters.from,
    to: filters.to,
    warehouse_id: filters.warehouseId,
    picker_id: filters.pickerId,
  }
}

export function getPickingKpiSummary(filters = {}) {
  const qs = buildReportQuery(filterParams(filters))
  return apiGet(`/reports/picking-kpi/summary${qs ? `?${qs}` : ''}`)
}

export async function exportPickingKpiCsv(filters = {}) {
  const qs = buildReportQuery(filterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/picking-kpi/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function exportPickingKpiXlsx(filters = {}) {
  const qs = buildReportQuery(filterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/picking-kpi/export/xlsx${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function previewPickingKpiPdf(filters = {}) {
  const qs = buildReportQuery(filterParams(filters))
  const { blob } = await apiDownload(`/reports/picking-kpi/pdf${qs ? `?${qs}` : ''}`)
  return blob
}

export async function downloadPickingKpiPdf(filters = {}) {
  const qs = buildReportQuery({ ...filterParams(filters), download: 'true' })
  const { blob, filename } = await apiDownload(`/reports/picking-kpi/pdf${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function openPickingKpiPdfInNewTab(filters = {}) {
  const qs = buildReportQuery(filterParams(filters))
  const { blob } = await apiDownload(`/reports/picking-kpi/pdf${qs ? `?${qs}` : ''}`)
  openInNewTab(blob)
}

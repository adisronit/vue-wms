import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'
import { buildReportQuery } from '../utils/reportFilters'

// Shared by list/summary/export so the exact same filters always
// produce the exact same scope — resolution_status/defect_code are
// list-only refinements the backend deliberately excludes from
// Summary's own KPI denominators (see qcreport's own Filters doc).
function reportFilterParams(filters = {}) {
  return {
    from: filters.from,
    to: filters.to,
    warehouse_id: filters.warehouseId,
    supplier_id: filters.supplierId,
    inspector_id: filters.inspectorId,
    qc_type: filters.qcType,
    search: filters.search,
    resolution_status: filters.resolutionStatus,
    defect_code: filters.defectCode,
  }
}

export function listQCReport(filters = {}, { page, limit, sortBy, sortDir } = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), page, limit, sort_by: sortBy, sort_dir: sortDir })
  return apiGet(`/reports/qc${qs ? `?${qs}` : ''}`)
}

export function getQCReportSummary(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  return apiGet(`/reports/qc/summary${qs ? `?${qs}` : ''}`)
}

export async function exportQCReportCsv(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/qc/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function exportQCReportXlsx(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/qc/export/xlsx${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function previewQCReportPdf(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/qc/pdf${qs ? `?${qs}` : ''}`)
  return blob
}

export async function downloadQCReportPdf(filters = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), download: 'true' })
  const { blob, filename } = await apiDownload(`/reports/qc/pdf${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function openQCReportPdfInNewTab(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/qc/pdf${qs ? `?${qs}` : ''}`)
  openInNewTab(blob)
}

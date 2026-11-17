import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'
import { buildReportQuery } from '../utils/reportFilters'

// Shared by list/summary/export so the exact same filters always
// produce the exact same scope — Phase 34's own "exports must respect
// exactly the same filters as the displayed report" rule.
function reportFilterParams(filters = {}) {
  return {
    from: filters.from,
    to: filters.to,
    warehouse_id: filters.warehouseId,
    supplier_id: filters.supplierId,
    operator_id: filters.operatorId,
    status: filters.status,
    search: filters.search,
    lot_no: filters.lotNo,
  }
}

export function listReceivingReport(filters = {}, { page, limit, sortBy, sortDir } = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), page, limit, sort_by: sortBy, sort_dir: sortDir })
  return apiGet(`/reports/receiving${qs ? `?${qs}` : ''}`)
}

export function getReceivingReportSummary(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  return apiGet(`/reports/receiving/summary${qs ? `?${qs}` : ''}`)
}

export async function exportReceivingReportCsv(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/receiving/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function exportReceivingReportXlsx(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/receiving/export/xlsx${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

// previewReceivingReportPdf returns a Blob (caller owns the
// object URL, e.g. for ReportPrintPreview's iframe) — mirrors
// services/receiving.js's own previewGRN/downloadGRN split.
export async function previewReceivingReportPdf(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/receiving/pdf${qs ? `?${qs}` : ''}`)
  return blob
}

export async function downloadReceivingReportPdf(filters = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), download: 'true' })
  const { blob, filename } = await apiDownload(`/reports/receiving/pdf${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function openReceivingReportPdfInNewTab(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/receiving/pdf${qs ? `?${qs}` : ''}`)
  openInNewTab(blob)
}

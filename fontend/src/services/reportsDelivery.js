import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'
import { buildReportQuery } from '../utils/reportFilters'

// Shared by list/summary/export so the exact same filters always
// produce the exact same scope.
function reportFilterParams(filters = {}) {
  return {
    from: filters.from,
    to: filters.to,
    warehouse_id: filters.warehouseId,
    carrier: filters.carrier,
    status: filters.status,
    order_type: filters.orderType,
    search: filters.search,
  }
}

export function listDeliveryReport(filters = {}, { page, limit, sortBy, sortDir } = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), page, limit, sort_by: sortBy, sort_dir: sortDir })
  return apiGet(`/reports/delivery${qs ? `?${qs}` : ''}`)
}

export function getDeliveryReportSummary(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  return apiGet(`/reports/delivery/summary${qs ? `?${qs}` : ''}`)
}

export async function exportDeliveryReportCsv(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/delivery/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function exportDeliveryReportXlsx(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob, filename } = await apiDownload(`/reports/delivery/export/xlsx${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function previewDeliveryReportPdf(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/delivery/pdf${qs ? `?${qs}` : ''}`)
  return blob
}

export async function downloadDeliveryReportPdf(filters = {}) {
  const qs = buildReportQuery({ ...reportFilterParams(filters), download: 'true' })
  const { blob, filename } = await apiDownload(`/reports/delivery/pdf${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

export async function openDeliveryReportPdfInNewTab(filters = {}) {
  const qs = buildReportQuery(reportFilterParams(filters))
  const { blob } = await apiDownload(`/reports/delivery/pdf${qs ? `?${qs}` : ''}`)
  openInNewTab(blob)
}

// Loading & Dispatch Report (trip grain) — CSV export only, a
// deliberate scope decision (see deliveryreport's own TripRow doc).
export async function exportLoadingTripsCsv(filters = {}) {
  const qs = buildReportQuery({ from: filters.from, to: filters.to, carrier: filters.carrier, search: filters.search })
  const { blob, filename } = await apiDownload(`/reports/delivery/trips/export${qs ? `?${qs}` : ''}`)
  triggerDownload(blob, filename)
}

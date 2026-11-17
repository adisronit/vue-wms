import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ReceivingReportView from '../ReceivingReportView.vue'

vi.mock('../../../services/reportsReceiving', () => ({
  listReceivingReport: vi.fn(),
  getReceivingReportSummary: vi.fn(),
  exportReceivingReportCsv: vi.fn(),
  exportReceivingReportXlsx: vi.fn(),
  previewReceivingReportPdf: vi.fn(),
  downloadReceivingReportPdf: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn() }))
vi.mock('../../../services/suppliers', () => ({ listSuppliers: vi.fn() }))

import {
  listReceivingReport,
  getReceivingReportSummary,
  exportReceivingReportCsv,
} from '../../../services/reportsReceiving'
import { listWarehouses } from '../../../services/warehouses'
import { listSuppliers } from '../../../services/suppliers'

const globalMocks = { $t: (key) => key }

const row = {
  receipt_item_id: 1, split_id: 10, receipt_no: 'RCV-000001', receiving_date: '2026-11-01T00:00:00Z',
  supplier_name: 'Acme Supplier', warehouse_name: 'Main Warehouse', sku: 'SKU-001',
  expected_qty: 100, received_qty: 90, short_over_qty: -10, uom: 'PCS', lot_no: 'LOT-001',
  receiving_status: 'PARTIALLY_RECEIVED', qc_status: 'PASSED', putaway_status: 'PENDING',
}

const summary = {
  total_receipts: 5, total_received_qty_by_uom: [{ uom: 'PCS', qty: 450 }],
  completion_rate: 80.0, avg_receiving_lead_time_hours: null,
  pending_qc_qty: 3, pending_putaway_qty: 7,
  charts: { trend: [], by_supplier: [], by_warehouse: [], by_operator: [] },
}

function listResponse(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 20, total: rows.length, total_pages: 1 } }
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  listSuppliers.mockResolvedValue({ data: [{ id: 1, company_name: 'Acme Supplier' }] })
  listReceivingReport.mockResolvedValue(listResponse([row]))
  getReceivingReportSummary.mockResolvedValue({ success: true, data: summary })
})

describe('ReceivingReportView', () => {
  it('fetches rows and summary on mount and renders them', async () => {
    const wrapper = mount(ReceivingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listReceivingReport).toHaveBeenCalled()
    expect(getReceivingReportSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('RCV-000001')
    expect(wrapper.text()).toContain('SKU-001')
  })

  it('shows N/A for a null KPI rather than a fabricated value', async () => {
    const wrapper = mount(ReceivingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const leadTimeCard = wrapper.vm.summaryCards.find((c) => c.key === 'lead')
    expect(leadTimeCard.value).toBe('reportsReceiving.kpi.notAvailable')
  })

  it('re-fetches with the new sort column/direction on header click', async () => {
    const wrapper = mount(ReceivingReportView, { global: { mocks: globalMocks } })
    await flushPromises()
    listReceivingReport.mockClear()

    await wrapper.vm.toggleSort('receipt_no')
    await flushPromises()

    expect(listReceivingReport).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ sortBy: 'receipt_no', sortDir: 'asc', page: 1 })
    )
  })

  it('toggles sort direction on a second click of the same column', async () => {
    const wrapper = mount(ReceivingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.toggleSort('receipt_no')
    await flushPromises()
    listReceivingReport.mockClear()
    await wrapper.vm.toggleSort('receipt_no')
    await flushPromises()

    expect(listReceivingReport).toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ sortDir: 'desc' }))
  })

  it('wires the CSV export button to exportReceivingReportCsv with the current filters', async () => {
    const wrapper = mount(ReceivingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.onExportCsv()
    expect(exportReceivingReportCsv).toHaveBeenCalledTimes(1)
  })

  it('shows the load error and an empty table when the list request fails', async () => {
    listReceivingReport.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(ReceivingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.rows).toEqual([])
    expect(wrapper.vm.loadError).toBe('network error')
  })
})

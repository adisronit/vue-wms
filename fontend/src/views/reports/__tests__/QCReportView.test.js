import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import QCReportView from '../QCReportView.vue'

vi.mock('../../../services/reportsQC', () => ({
  listQCReport: vi.fn(),
  getQCReportSummary: vi.fn(),
  exportQCReportCsv: vi.fn(),
  exportQCReportXlsx: vi.fn(),
  previewQCReportPdf: vi.fn(),
  downloadQCReportPdf: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn() }))
vi.mock('../../../services/suppliers', () => ({ listSuppliers: vi.fn() }))

import { listQCReport, getQCReportSummary, exportQCReportCsv } from '../../../services/reportsQC'
import { listWarehouses } from '../../../services/warehouses'
import { listSuppliers } from '../../../services/suppliers'

const globalMocks = { $t: (key) => key }

const rows = [
  {
    inspection_id: 1, inspection_no: 'QC-0001', receipt_id: 1, receipt_no: 'RCV-0001',
    warehouse_code: 'WH1', warehouse_name: 'Main Warehouse', supplier_name: 'Acme Co',
    inspector_name: 'Jane', qc_type: 'NORMAL', inspection_created_at: '2026-11-01T00:00:00Z',
    inspection_item_id: 10, sku: 'SKU-1', product_name: 'Widget', lot_no: 'LOT-1',
    expiry_date: null, location_code: 'A1', held_qty_snapshot: 100, sample_qty: 10,
    temperature_c: null, packaging_ok: true, label_ok: true,
    pass_qty: 70, reject_qty: 30, remaining_hold_qty: 0, resolution_status: 'MIXED',
    defect_code: 'DAMAGED', evidence_reference: null, remark: null,
    qc_turnaround_hours: 2.5, hold_aging_hours: null,
  },
]

const summary = {
  total_inspection_items: 1,
  pass_rate: { rate_pct: 70, numerator: 70, denominator: 100 },
  rejection_rate: { rate_pct: 30, numerator: 30, denominator: 100 },
  hold_rate: { rate_pct: null, numerator: 0, denominator: 0 },
  pending_qc_not_started_count: 2, pending_qc_not_started_qty: 40,
  pending_qc_awaiting_decision_count: 1, pending_qc_awaiting_decision_qty: 10,
  avg_turnaround_hours: 2.5, avg_hold_aging_hours: null, max_hold_aging_hours: null,
  charts: {
    top_defects: [{ defect_code: 'DAMAGED', count: 1, qty: 30 }],
    supplier_rejection_trend: [{ date: '2026-11-01', supplier: 'Acme Co', reject_qty: 30 }],
    by_qc_type: [{ name: 'NORMAL', count: 1 }],
  },
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  listSuppliers.mockResolvedValue({ data: [{ id: 1, company_name: 'Acme Co' }] })
  listQCReport.mockResolvedValue({ data: rows, meta: { total: 1 } })
  getQCReportSummary.mockResolvedValue({ success: true, data: summary })
})

describe('QCReportView', () => {
  it('fetches rows and summary on mount and renders them', async () => {
    const wrapper = mount(QCReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listQCReport).toHaveBeenCalled()
    expect(getQCReportSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('QC-0001')
    expect(wrapper.text()).toContain('MIXED')
  })

  it('shows N/A for a null hold rate with its denominator as a hint, rather than a fabricated value', async () => {
    const wrapper = mount(QCReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'hold')
    expect(card.value).toBe('reportsQc.kpi.notAvailable')
    expect(card.hint).toContain('0/0')
  })

  it('surfaces the pass rate numerator/denominator, not a bare percentage', async () => {
    const wrapper = mount(QCReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'pass')
    expect(card.value).toBe('70.0%')
    expect(card.hint).toContain('70/100')
  })

  it('distinguishes Pending QC Not Started from Pending QC Awaiting Decision', async () => {
    const wrapper = mount(QCReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const notStarted = wrapper.vm.summaryCards.find((c) => c.key === 'pendingNotStarted')
    const awaiting = wrapper.vm.summaryCards.find((c) => c.key === 'pendingAwaiting')
    expect(notStarted.value).toBe('2 / 40')
    expect(awaiting.value).toBe('1 / 10')
  })

  it('wires the CSV export button to exportQCReportCsv with the current filters', async () => {
    const wrapper = mount(QCReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.onExportCsv()
    expect(exportQCReportCsv).toHaveBeenCalledTimes(1)
  })

  it('shows the load error and clears rows when the request fails', async () => {
    listQCReport.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(QCReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.rows).toEqual([])
    expect(wrapper.vm.loadError).toBe('network error')
  })
})

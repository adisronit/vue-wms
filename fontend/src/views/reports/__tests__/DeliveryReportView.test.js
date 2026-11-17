import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import DeliveryReportView from '../DeliveryReportView.vue'

vi.mock('../../../services/reportsDelivery', () => ({
  listDeliveryReport: vi.fn(),
  getDeliveryReportSummary: vi.fn(),
  exportDeliveryReportCsv: vi.fn(),
  exportDeliveryReportXlsx: vi.fn(),
  previewDeliveryReportPdf: vi.fn(),
  downloadDeliveryReportPdf: vi.fn(),
  exportLoadingTripsCsv: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn() }))

import {
  listDeliveryReport,
  getDeliveryReportSummary,
  exportDeliveryReportCsv,
  exportLoadingTripsCsv,
} from '../../../services/reportsDelivery'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = { $t: (key) => key }

const rows = [
  {
    shipment_id: 1, shipment_item_id: 10, shipment_no: 'SHP-0001', order_no: 'SO-0001', order_type: 'SALES_ORDER',
    customer_name: 'Acme Co', warehouse_code: 'WH1', warehouse_name: 'Main Warehouse',
    carrier: 'DHL', tracking_no: 'TRACK-1', packages: 1, weight: 5, status: 'DISPATCHED',
    ship_date: '2026-11-01', dispatched_at: '2026-11-01T00:00:00Z', delivered_at: null,
    sku: 'SKU-1', product_name: 'Widget', lot_no: 'LOT-1', location_code: 'A1', qty: 10, uom: 'PCS',
    delivery_lead_time_hours: null, is_exception: false, exception_remark: null,
  },
]

const summary = {
  total_shipments: 3,
  dispatch_rate: { rate_pct: 66.7, numerator: 2, denominator: 3 },
  delivery_confirmation_rate: { rate_pct: 50, numerator: 1, denominator: 2 },
  exception_rate: { rate_pct: null, numerator: 0, denominator: 0 },
  avg_delivery_lead_time_hours: 4.5,
  total_trips: 2, completed_trips: 2, avg_loading_duration_hours: 1.2,
  charts: {
    trend: [{ date: '2026-11-01', shipment_count: 3 }],
    by_status: [{ name: 'DISPATCHED', count: 2 }],
    by_carrier: [{ name: 'DHL', count: 2 }],
    by_warehouse: [{ name: 'Main Warehouse', count: 3 }],
  },
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  listDeliveryReport.mockResolvedValue({ data: rows, meta: { total: 1 } })
  getDeliveryReportSummary.mockResolvedValue({ success: true, data: summary })
})

describe('DeliveryReportView', () => {
  it('fetches rows and summary on mount and renders them', async () => {
    const wrapper = mount(DeliveryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listDeliveryReport).toHaveBeenCalled()
    expect(getDeliveryReportSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('SHP-0001')
  })

  it('shows N/A for a null exception rate with its denominator as a hint, rather than a fabricated value', async () => {
    const wrapper = mount(DeliveryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'exception')
    expect(card.value).toBe('reportsDelivery.kpi.notAvailable')
    expect(card.hint).toContain('0/0')
  })

  it('surfaces the dispatch rate numerator/denominator, not a bare percentage', async () => {
    const wrapper = mount(DeliveryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'dispatch')
    expect(card.hint).toContain('2/3')
  })

  it('never shows a delivered timestamp for a merely-dispatched row', async () => {
    const wrapper = mount(DeliveryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.rows[0].delivered_at).toBeNull()
    expect(wrapper.vm.rows[0].delivery_lead_time_hours).toBeNull()
  })

  it('wires the CSV export button to exportDeliveryReportCsv with the current filters', async () => {
    const wrapper = mount(DeliveryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.onExportCsv()
    expect(exportDeliveryReportCsv).toHaveBeenCalledTimes(1)
  })

  it('wires the trips tab CSV export to exportLoadingTripsCsv', async () => {
    const wrapper = mount(DeliveryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.onExportTripsCsv()
    expect(exportLoadingTripsCsv).toHaveBeenCalledTimes(1)
  })

  it('shows the load error and clears rows when the request fails', async () => {
    listDeliveryReport.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(DeliveryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.rows).toEqual([])
    expect(wrapper.vm.loadError).toBe('network error')
  })
})

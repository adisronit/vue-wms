import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import StockCountReportView from '../StockCountReportView.vue'

vi.mock('../../../services/reportsStockCount', () => ({
  listStockCountReport: vi.fn(),
  getStockCountReportSummary: vi.fn(),
  exportStockCountReportCsv: vi.fn(),
  exportStockCountReportXlsx: vi.fn(),
  previewStockCountReportPdf: vi.fn(),
  downloadStockCountReportPdf: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn(), listZones: vi.fn() }))

import { listStockCountReport, getStockCountReportSummary } from '../../../services/reportsStockCount'
import { listWarehouses, listZones } from '../../../services/warehouses'

const globalMocks = { $t: (key) => key }

const row = {
  count_item_id: 1, count_no: 'CNT-000001', count_date_time: '2026-11-01T00:00:00Z',
  warehouse_name: 'Main Warehouse', zone_name: 'Zone A', location_code: 'LOC-01',
  sku: 'SKU-001', system_qty: 100, uom: 'PCS', counted_qty: 95, variance_qty: -5, variance_pct: -5,
  item_status: 'COUNTED', counted_by: 'Jane Doe', approved_by: null, adjustment_no: null,
}

const summary = {
  total_count_sessions: 3, counted_locations: 2, counted_skus: 2,
  stock_count_accuracy: 50.0, quantity_variance: -5, recount_rate: null, pending_approval: 1,
}

function listResponse(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 20, total: rows.length, total_pages: 1 } }
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  listZones.mockResolvedValue({ data: [{ id: 1, name: 'Zone A' }] })
  listStockCountReport.mockResolvedValue(listResponse([row]))
  getStockCountReportSummary.mockResolvedValue({ success: true, data: summary })
})

describe('StockCountReportView', () => {
  it('fetches rows and summary on mount and renders them', async () => {
    const wrapper = mount(StockCountReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listStockCountReport).toHaveBeenCalled()
    expect(getStockCountReportSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('CNT-000001')
  })

  it('shows N/A for a null recount rate rather than a fabricated value', async () => {
    const wrapper = mount(StockCountReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const recountCard = wrapper.vm.summaryCards.find((c) => c.key === 'recount')
    expect(recountCard.value).toBe('reportsReceiving.kpi.notAvailable')
  })

  it('applying the Variance Only preset re-fetches with variance_only set', async () => {
    const wrapper = mount(StockCountReportView, { global: { mocks: globalMocks } })
    await flushPromises()
    listStockCountReport.mockClear()

    await wrapper.vm.applyPreset('varianceOnly')
    await flushPromises()

    expect(listStockCountReport).toHaveBeenCalledWith(
      expect.objectContaining({ varianceOnly: true }),
      expect.anything()
    )
  })

  it('applying the Recount preset filters by item_status=RECOUNT', async () => {
    const wrapper = mount(StockCountReportView, { global: { mocks: globalMocks } })
    await flushPromises()
    listStockCountReport.mockClear()

    await wrapper.vm.applyPreset('recount')
    await flushPromises()

    expect(listStockCountReport).toHaveBeenCalledWith(
      expect.objectContaining({ itemStatus: 'RECOUNT' }),
      expect.anything()
    )
  })

  it('selecting a warehouse fetches that warehouse\'s zones and clears any prior zone selection', async () => {
    const wrapper = mount(StockCountReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.filters.warehouseId = 1
    await wrapper.vm.onWarehouseChange()
    await flushPromises()

    expect(listZones).toHaveBeenCalledWith(1)
    expect(wrapper.vm.zones).toEqual([{ id: 1, name: 'Zone A' }])
    expect(wrapper.vm.filters.zoneId).toBe('')
  })

  it('shows the load error and an empty table when the list request fails', async () => {
    listStockCountReport.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(StockCountReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.rows).toEqual([])
    expect(wrapper.vm.loadError).toBe('network error')
  })
})

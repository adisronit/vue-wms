import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import InventoryReportView from '../InventoryReportView.vue'

vi.mock('../../../services/reportsInventory', () => ({
  listInventoryReport: vi.fn(),
  getInventoryReportSummary: vi.fn(),
  exportInventoryReportCsv: vi.fn(),
  exportInventoryReportXlsx: vi.fn(),
  previewInventoryReportPdf: vi.fn(),
  downloadInventoryReportPdf: vi.fn(),
  listInventoryMovements: vi.fn(),
  exportInventoryMovementsCsv: vi.fn(),
  listInventoryAsOf: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn() }))

import {
  listInventoryReport,
  getInventoryReportSummary,
  listInventoryMovements,
  listInventoryAsOf,
} from '../../../services/reportsInventory'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = { $t: (key) => key }

const row = {
  inventory_id: 1, warehouse_name: 'Main Warehouse', zone_name: 'Zone A', location_code: 'LOC-01',
  storage_type: 'STORAGE', sku: 'SKU-001', lot_no: 'LOT-001', expiry_date: null, uom: 'PCS',
  on_hand: 100, reserved: 20, hold: 10, available: 70, inventory_status: 'AVAILABLE',
}

const summary = {
  total_on_hand_by_uom: [{ uom: 'PCS', qty: 100 }], total_available_by_uom: [{ uom: 'PCS', qty: 70 }],
  low_stock_count: null, near_expiry_count: null, expired_count: 0, non_moving_count: null,
}

function listResponse(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 20, total: rows.length, total_pages: 1 } }
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  listInventoryReport.mockResolvedValue(listResponse([row]))
  getInventoryReportSummary.mockResolvedValue({ success: true, data: summary })
  listInventoryMovements.mockResolvedValue(listResponse([]))
  listInventoryAsOf.mockResolvedValue(listResponse([]))
})

describe('InventoryReportView', () => {
  it('fetches balance rows and summary on mount and renders them', async () => {
    const wrapper = mount(InventoryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listInventoryReport).toHaveBeenCalled()
    expect(getInventoryReportSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('SKU-001')
  })

  it('shows N/A for a null low-stock count rather than a fabricated value', async () => {
    const wrapper = mount(InventoryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'lowStock')
    expect(card.value).toBe('reportsReceiving.kpi.notAvailable')
  })

  it('selecting the Low Stock lens re-fetches with belowQty set', async () => {
    const wrapper = mount(InventoryReportView, { global: { mocks: globalMocks } })
    await flushPromises()
    listInventoryReport.mockClear()

    await wrapper.vm.setLens('lowStock')
    await flushPromises()

    expect(listInventoryReport).toHaveBeenCalledWith(
      expect.objectContaining({ belowQty: wrapper.vm.lensParams.belowQty }),
      expect.anything()
    )
  })

  it('switching to the Movement tab fetches that grain separately', async () => {
    const wrapper = mount(InventoryReportView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(listInventoryMovements).not.toHaveBeenCalled()

    await wrapper.vm.switchView('movement')
    await flushPromises()

    expect(listInventoryMovements).toHaveBeenCalled()
    expect(wrapper.vm.view).toBe('movement')
  })

  it('switching to the As-of tab fetches a reconstructed balance with a caveat note in the UI', async () => {
    const wrapper = mount(InventoryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.switchView('asof')
    await flushPromises()

    expect(listInventoryAsOf).toHaveBeenCalled()
    expect(wrapper.text()).toContain('reportsInventory.asOfCaveat')
  })

  it('shows the load error and an empty table when the list request fails', async () => {
    listInventoryReport.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(InventoryReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.rows).toEqual([])
    expect(wrapper.vm.loadError).toBe('network error')
  })
})

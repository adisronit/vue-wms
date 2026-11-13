import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CheckStockView from '../CheckStockView.vue'

vi.mock('../../../services/stockCount', async () => {
  const actual = await vi.importActual('../../../services/stockCount')
  return {
    ...actual,
    listStockCounts: vi.fn(),
    getStockCount: vi.fn(),
    createStockCount: vi.fn(),
    addStockCountItem: vi.fn(),
    recordStockCount: vi.fn(),
    confirmStockCount: vi.fn(),
    createAdjustmentsFromCount: vi.fn(),
    exportStockCounts: vi.fn()
  }
})
vi.mock('../../../services/warehouses', async () => {
  const actual = await vi.importActual('../../../services/warehouses')
  return { ...actual, listWarehouses: vi.fn(), listZones: vi.fn(), listLocations: vi.fn() }
})
vi.mock('../../../services/products', async () => {
  const actual = await vi.importActual('../../../services/products')
  return { ...actual, listProducts: vi.fn() }
})

import { listStockCounts, getStockCount } from '../../../services/stockCount'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v),
  $router: { push: vi.fn() }
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const rawHistoryRow = {
  id: 1,
  count_no: 'CNT-001',
  count_type: 'CYCLE_COUNT',
  warehouse_code: 'WH-01',
  zone_code: 'Z1',
  count_date: '2026-01-01T00:00:00Z',
  item_count: 3,
  variance_lines: 1,
  status: 'CONFIRMED',
  created_by_name: 'Alice',
  created_at: '2026-01-01T00:00:00Z'
}

describe('CheckStockView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched count history in the table', async () => {
    listStockCounts.mockResolvedValue(page([rawHistoryRow]))
    listWarehouses.mockResolvedValue({ data: [] })

    const wrapper = mount(CheckStockView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('CNT-001')
    expect(wrapper.text()).toContain('WH-01')
  })

  it('maps history and item statuses to the expected badge tone', async () => {
    listStockCounts.mockResolvedValue(page([]))
    listWarehouses.mockResolvedValue({ data: [] })

    const wrapper = mount(CheckStockView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.historyStatusTone('CONFIRMED')).toBe('green')
    expect(wrapper.vm.historyStatusTone('ADJUSTMENT_CREATED')).toBe('orange')
    expect(wrapper.vm.historyStatusTone('DRAFT')).toBe('gray')

    expect(wrapper.vm.itemStatusTone('PENDING')).toBe('gray')
    expect(wrapper.vm.itemStatusTone('COUNTED')).toBe('green')
    expect(wrapper.vm.itemStatusTone('RECOUNT')).toBe('red')
  })

  it('saves a draft via the API without falling back to window.alert', async () => {
    listStockCounts.mockResolvedValue(page([]))
    listWarehouses.mockResolvedValue({ data: [] })
    getStockCount.mockResolvedValue({ data: { id: 5, items: [], generated_adjustments: [] } })

    const wrapper = mount(CheckStockView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.countId = 5
    wrapper.vm.items = []

    const alertSpy = vi.spyOn(window, 'alert')
    await wrapper.vm.saveDraft()
    await flushPromises()

    expect(getStockCount).toHaveBeenCalledWith(5)
    expect(alertSpy).not.toHaveBeenCalled()
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import StockAdjustmentView from '../StockAdjustmentView.vue'

vi.mock('../../../services/stockAdjustment', async () => {
  const actual = await vi.importActual('../../../services/stockAdjustment')
  return {
    ...actual,
    listStockAdjustments: vi.fn(),
    getStockAdjustment: vi.fn(),
    createStockAdjustment: vi.fn(),
    updateStockAdjustment: vi.fn(),
    confirmStockAdjustment: vi.fn(),
    exportStockAdjustments: vi.fn()
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
vi.mock('../../../services/inventory', async () => {
  const actual = await vi.importActual('../../../services/inventory')
  return { ...actual, listInventory: vi.fn() }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import {
  listStockAdjustments, getStockAdjustment, updateStockAdjustment
} from '../../../services/stockAdjustment'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v),
  $route: { query: {} },
  $router: { push: vi.fn() }
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const rawRow = {
  id: 1,
  adjustment_no: 'ADJ-001',
  date: '2026-01-01T00:00:00Z',
  sku: 'SKU-1',
  product_name: 'Widget',
  location_code: 'A-01-01',
  before_qty: 10,
  adjust_type: 'INCREASE',
  adjust_qty: 5,
  after_qty: 15,
  reason: 'Found Goods',
  status: 'PENDING',
  age_hours: 2,
  is_overdue: false
}

describe('StockAdjustmentView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched adjustments in the table', async () => {
    listStockAdjustments.mockResolvedValue(page([rawRow]))
    const wrapper = mount(StockAdjustmentView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('ADJ-001')
    expect(wrapper.text()).toContain('Widget')
  })

  it('maps status to the expected badge tone', async () => {
    listStockAdjustments.mockResolvedValue(page([]))
    const wrapper = mount(StockAdjustmentView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('DRAFT')).toBe('gray')
    expect(wrapper.vm.statusTone('PENDING')).toBe('orange')
    expect(wrapper.vm.statusTone('APPROVED')).toBe('green')
    expect(wrapper.vm.statusTone('REJECTED')).toBe('red')
  })

  it('saves a draft via the API without falling back to window.alert', async () => {
    listStockAdjustments.mockResolvedValue(page([rawRow]))
    getStockAdjustment.mockResolvedValue({
      data: {
        sku: 'SKU-1', product_id: 1, product_name: 'Widget',
        location_id: 1, location_code: 'A-01-01', lot_no: '',
        before_qty: 10, adjust_type: 'INCREASE', adjust_qty: 5, after_qty: 15,
        reason: 'Found Goods', remark: '', status: 'DRAFT',
        created_by_name: 'Alice', created_at: '2026-01-01T00:00:00Z',
        age_hours: null, is_overdue: false, pending_warning_hours: null
      }
    })
    updateStockAdjustment.mockResolvedValue({ data: { id: 1 } })

    const wrapper = mount(StockAdjustmentView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openDetail({ id: 1 })
    await flushPromises()

    const alertSpy = vi.spyOn(window, 'alert')
    await wrapper.vm.saveDraft()
    await flushPromises()

    expect(updateStockAdjustment).toHaveBeenCalledTimes(1)
    expect(alertSpy).not.toHaveBeenCalled()
  })
})

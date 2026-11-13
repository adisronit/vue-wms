import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import StockInquiryView from '../StockInquiryView.vue'

vi.mock('../../../services/inventory', async () => {
  const actual = await vi.importActual('../../../services/inventory')
  return {
    ...actual,
    listInventory: vi.fn(),
    getInventorySummary: vi.fn(),
    getInventoryTransactions: vi.fn(),
    exportInventory: vi.fn(),
    createInventoryTransactionsExport: vi.fn(),
    getExportJob: vi.fn(),
    downloadExportJob: vi.fn()
  }
})
vi.mock('../../../services/warehouses', async () => {
  const actual = await vi.importActual('../../../services/warehouses')
  return { ...actual, listWarehouses: vi.fn(), listZones: vi.fn() }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listInventory, getInventorySummary } from '../../../services/inventory'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const rawItem = {
  id: 1,
  sku: 'SKU-1',
  product_name: 'Widget',
  barcode: '12345',
  warehouse_id: 1,
  warehouse_code: 'WH-01',
  zone_id: 1,
  zone_code: 'Z1',
  location_code: 'A-01-01',
  lot_no: 'LOT1',
  expiry_date: '2026-06-01T00:00:00Z',
  available_qty: 10,
  reserved_qty: 2,
  on_hand_qty: 12,
  uom: 'PCS',
  status: 'AVAILABLE'
}

describe('StockInquiryView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched stock in the table', async () => {
    listInventory.mockResolvedValue(page([rawItem]))
    getInventorySummary.mockResolvedValue({ data: {} })
    listWarehouses.mockResolvedValue({ data: [] })

    const wrapper = mount(StockInquiryView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('SKU-1')
    expect(wrapper.text()).toContain('Widget')
  })

  it('maps stock status to the expected badge tone and label', async () => {
    listInventory.mockResolvedValue(page([]))
    getInventorySummary.mockResolvedValue({ data: {} })
    listWarehouses.mockResolvedValue({ data: [] })

    const wrapper = mount(StockInquiryView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('AVAILABLE')).toBe('green')
    expect(wrapper.vm.statusTone('RESERVED')).toBe('blue')
    expect(wrapper.vm.statusTone('HOLD')).toBe('orange')
    expect(wrapper.vm.statusTone('LOW_STOCK')).toBe('red')

    expect(wrapper.vm.statusLabel('AVAILABLE')).toBe('Available')
    expect(wrapper.vm.statusLabel('LOW_STOCK')).toBe('Low Stock')
  })
})

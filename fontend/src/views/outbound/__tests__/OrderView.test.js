import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OrderView from '../OrderView.vue'

vi.mock('../../../services/orders', async () => {
  const actual = await vi.importActual('../../../services/orders')
  return { ...actual, listOrders: vi.fn() }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [] }),
  listZones: vi.fn().mockResolvedValue({ data: [] }),
  listLocations: vi.fn().mockResolvedValue({ data: [] })
}))
vi.mock('../../../services/products', () => ({ listProducts: vi.fn().mockResolvedValue({ data: [] }) }))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listOrders } from '../../../services/orders'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

describe('OrderView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched orders with a status badge', async () => {
    listOrders.mockResolvedValue(page([{
      id: 1, order_no: 'ORD-001', external_reference: null, customer: 'Acme',
      warehouse_code: 'WH-01', item_count: 3, status: 'PENDING', fulfillment_status: 'PENDING', created_at: '2026-01-01T00:00:00Z'
    }]))
    const wrapper = mount(OrderView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.text()).toContain('ORD-001')
    expect(wrapper.findComponent({ name: 'StatusBadge' }).exists()).toBe(true)
  })
})

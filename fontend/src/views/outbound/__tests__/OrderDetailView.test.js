import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OrderDetailView from '../OrderDetailView.vue'

vi.mock('../../../services/orders', () => ({
  getOrder: vi.fn(),
  cancelOrder: vi.fn(),
  createOrderPickingTask: vi.fn(),
  viewOrderDocument: vi.fn(),
  downloadOrderDocument: vi.fn()
}))
vi.mock('../../../services/warehouses', () => ({
  listZones: vi.fn().mockResolvedValue({ data: [] }),
  listLocations: vi.fn().mockResolvedValue({ data: [] })
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { getOrder } from '../../../services/orders'
import { hasPermission } from '../../../services/auth'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key), $d: (v) => String(v) }

function baseOrder(overrides = {}) {
  return {
    id: 1, order_no: 'ORD-000001', order_type: 'SALES_ORDER', priority: 'NORMAL',
    status: 'PENDING', fulfillment_status: 'PENDING', warehouse_id: 1, warehouse_code: 'WH-01',
    customer: 'Acme', items: [{ id: 1, sku: 'SKU-1', product_name: 'Widget', ordered_qty: 10, uom: 'PCS', shortage_flag: false }],
    ...overrides
  }
}

function mountDetail(props = { id: '1' }) {
  return mount(OrderDetailView, {
    props,
    global: { mocks: { ...globalMocks, $router: { push: vi.fn() } } }
  })
}

describe('OrderDetailView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows Edit and Review & Release actions for a PENDING order', async () => {
    getOrder.mockResolvedValue({ data: baseOrder() })
    const wrapper = mountDetail()
    await flushPromises()

    expect(wrapper.text()).toContain('ORD-000001')
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('common.buttons.edit'))).toBe(true)
    expect(buttons.some((t) => t.includes('orders.detailPage.reviewAndRelease'))).toBe(true)
  })

  it('shows Create Picking Task for a RELEASED order', async () => {
    getOrder.mockResolvedValue({ data: baseOrder({ status: 'RELEASED' }) })
    const wrapper = mountDetail()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('orders.detail.createPickingTask'))).toBe(true)
    expect(buttons.some((t) => t.includes('orders.detailPage.reviewAndRelease'))).toBe(false)
  })

  it('hides Review & Release when the caller lacks order.confirm', async () => {
    hasPermission.mockImplementation((perm) => perm !== 'order.confirm')
    getOrder.mockResolvedValue({ data: baseOrder() })
    const wrapper = mountDetail()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('orders.detailPage.reviewAndRelease'))).toBe(false)
  })

  it('shows a shortage line flag when a line was marked short', async () => {
    getOrder.mockResolvedValue({
      data: baseOrder({ items: [{ id: 1, sku: 'SKU-1', product_name: 'Widget', ordered_qty: 10, uom: 'PCS', shortage_flag: true, shortfall_qty: 4 }] })
    })
    const wrapper = mountDetail()
    await flushPromises()
    expect(wrapper.text()).toContain('orders.review.shortfallOf')
  })
})

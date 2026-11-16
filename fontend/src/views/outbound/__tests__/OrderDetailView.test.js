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
vi.mock('../../../services/documents', async () => {
  const actual = await vi.importActual('../../../services/documents')
  return { ...actual, getDocumentHistory: vi.fn() }
})

import { getOrder } from '../../../services/orders'
import { hasPermission } from '../../../services/auth'
import { getDocumentHistory } from '../../../services/documents'

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

  // Phase 33C.3 — Document History panel.
  it('opens the Document History panel and loads this order\'s ORDER history', async () => {
    getOrder.mockResolvedValue({ data: baseOrder() })
    getDocumentHistory.mockResolvedValue({
      data: [{ id: 1, document_type: 'ORDER', document_no: 'ORD-000001', rendered_at: '2026-01-01T00:00:00Z', is_draft: false, rendered_by_name: 'Bob' }]
    })
    const wrapper = mountDetail({ id: '1' })
    await flushPromises()

    const historyBtn = wrapper.findAll('button').find((b) => b.text().includes('orders.detailPage.historyButton'))
    expect(historyBtn).toBeTruthy()
    await historyBtn.trigger('click')
    await flushPromises()

    expect(getDocumentHistory).toHaveBeenCalledWith('ORDER', '1')
  })

  // --- Phase 33E.1: INTERNAL_TRANSFER display ---

  it('shows the destination warehouse/transfer reference instead of customer for an INTERNAL_TRANSFER order', async () => {
    getOrder.mockResolvedValue({
      data: baseOrder({
        order_type: 'INTERNAL_TRANSFER', customer: null,
        destination_warehouse_code: 'WH-02', transfer_reference: 'TRF-0001'
      })
    })
    const wrapper = mountDetail()
    await flushPromises()

    expect(wrapper.text()).toContain('WH-02')
    expect(wrapper.text()).toContain('TRF-0001')
    expect(wrapper.text()).toContain('orders.detail.destinationWarehouseLabel')
    expect(wrapper.text()).toContain('orders.detail.transferReferenceLabel')
    expect(wrapper.text()).not.toContain('orders.detail.customerLabel')
  })

  it('shows the customer (not destination warehouse) for a SALES_ORDER', async () => {
    getOrder.mockResolvedValue({ data: baseOrder() })
    const wrapper = mountDetail()
    await flushPromises()

    expect(wrapper.text()).toContain('orders.detail.customerLabel')
    expect(wrapper.text()).not.toContain('orders.detail.destinationWarehouseLabel')
  })
})

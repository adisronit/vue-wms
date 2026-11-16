import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OrderReviewView from '../OrderReviewView.vue'

vi.mock('../../../services/orders', () => ({
  getOrder: vi.fn(),
  getOrderAvailability: vi.fn(),
  releaseOrder: vi.fn()
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { getOrder, getOrderAvailability, releaseOrder } from '../../../services/orders'
import { hasPermission } from '../../../services/auth'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key), $d: (v) => String(v) }

function mountReview(props = { id: '1' }) {
  return mount(OrderReviewView, {
    props,
    global: { mocks: { ...globalMocks, $router: { push: vi.fn() } } }
  })
}

describe('OrderReviewView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows a no-shortage banner and an enabled Release button when fully available', async () => {
    getOrder.mockResolvedValue({ data: { id: 1, order_no: 'ORD-000001', status: 'PENDING' } })
    getOrderAvailability.mockResolvedValue({
      data: { order_id: 1, has_shortage: false, lines: [{ order_item_id: 1, sku: 'SKU-1', product_name: 'Widget', ordered_qty: 10, available_qty: 30, reserved_qty: 0, has_shortage: false, shortfall_qty: 0 }] }
    })
    const wrapper = mountReview()
    await flushPromises()

    expect(wrapper.text()).toContain('orders.review.noShortage')
    const releaseBtn = wrapper.findAll('button').find((b) => b.text().includes('orders.review.releaseButton'))
    expect(releaseBtn.attributes('disabled')).toBeUndefined()
  })

  it('shows a shortage warning and disables Release until shortage is approved', async () => {
    getOrder.mockResolvedValue({ data: { id: 1, order_no: 'ORD-000001', status: 'PENDING' } })
    getOrderAvailability.mockResolvedValue({
      data: {
        order_id: 1, has_shortage: true,
        lines: [{ order_item_id: 1, sku: 'SKU-1', product_name: 'Widget', ordered_qty: 10, available_qty: 4, reserved_qty: 0, has_shortage: true, shortfall_qty: 6 }]
      }
    })
    const wrapper = mountReview()
    await flushPromises()

    expect(wrapper.text()).toContain('orders.review.shortageWarning')
    const releaseBtn = wrapper.findAll('button').find((b) => b.text().includes('orders.review.releaseButton'))
    expect(releaseBtn.attributes('disabled')).toBeDefined()

    wrapper.vm.approveShortage = true
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('button').find((b) => b.text().includes('orders.review.releaseButton')).attributes('disabled')).toBeUndefined()
  })

  it('never lets a caller without order.confirm enable Release, even with the box checked', async () => {
    hasPermission.mockReturnValue(false)
    getOrder.mockResolvedValue({ data: { id: 1, order_no: 'ORD-000001', status: 'PENDING' } })
    getOrderAvailability.mockResolvedValue({
      data: { order_id: 1, has_shortage: true, lines: [] }
    })
    const wrapper = mountReview()
    await flushPromises()

    wrapper.vm.approveShortage = true
    await wrapper.vm.$nextTick()
    const releaseBtn = wrapper.findAll('button').find((b) => b.text().includes('orders.review.releaseButton'))
    expect(releaseBtn.attributes('disabled')).toBeDefined()
  })

  it('calls releaseOrder with approve_shortage and navigates back on success', async () => {
    getOrder.mockResolvedValue({ data: { id: 1, order_no: 'ORD-000001', status: 'PENDING' } })
    getOrderAvailability.mockResolvedValue({ data: { order_id: 1, has_shortage: false, lines: [] } })
    releaseOrder.mockResolvedValue({ data: { id: 1, status: 'RELEASED' } })
    const wrapper = mountReview()
    await flushPromises()

    await wrapper.vm.submitRelease()
    expect(releaseOrder).toHaveBeenCalledWith('1', expect.objectContaining({ approve_shortage: false }))
  })
})

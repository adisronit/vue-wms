import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OrderFormView from '../OrderFormView.vue'

vi.mock('../../../services/orders', () => ({
  getOrder: vi.fn(),
  createOrder: vi.fn(),
  updateOrder: vi.fn()
}))
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [{ id: 1, code: 'WH-01', name: 'Main' }] })
}))
vi.mock('../../../services/products', () => ({
  listProducts: vi.fn().mockResolvedValue({
    data: [{ id: 5, sku: 'SKU-1', name: 'Widget', base_uom: 'PCS', storage_type: 'AMBIENT' }]
  })
}))
vi.mock('../../../services/customers', () => ({ listCustomers: vi.fn().mockResolvedValue({ data: [] }) }))
vi.mock('../../../services/customerDeliveryLocations', () => ({ listCustomerDeliveryLocations: vi.fn().mockResolvedValue({ data: [] }) }))
vi.mock('../../../services/inventory', () => ({
  listInventory: vi.fn().mockResolvedValue({ data: [{ available_qty: 30, reserved_qty: 5 }] })
}))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { createOrder, getOrder } from '../../../services/orders'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key), $d: (v) => String(v) }

function mountForm(props = {}) {
  return mount(OrderFormView, {
    props,
    global: { mocks: { ...globalMocks, $router: { push: vi.fn() } } }
  })
}

describe('OrderFormView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the create-mode title and warehouse options', async () => {
    const wrapper = mountForm()
    await flushPromises()
    expect(wrapper.text()).toContain('orders.form.newTitle')
    expect(wrapper.text()).toContain('WH-01')
  })

  it('adds a line via SKU lookup and shows a live available/reserved stock preview', async () => {
    const wrapper = mountForm()
    await flushPromises()

    wrapper.vm.form.warehouseId = 1
    wrapper.vm.itemDraft.sku = 'SKU-1'
    wrapper.vm.itemDraft.orderedQty = 10
    await wrapper.vm.addItem()
    await flushPromises()

    expect(wrapper.vm.items).toHaveLength(1)
    expect(wrapper.vm.items[0].productName).toBe('Widget')
    expect(wrapper.vm.items[0].availableQty).toBe(30)
    expect(wrapper.vm.items[0].reservedQty).toBe(5)
  })

  it('submits a Create request with the new header fields and line hints', async () => {
    createOrder.mockResolvedValue({ data: { id: 99 } })
    const wrapper = mountForm()
    await flushPromises()

    wrapper.vm.form.warehouseId = 1
    wrapper.vm.form.orderType = 'SALES_ORDER'
    wrapper.vm.form.priority = 'HIGH'
    wrapper.vm.items = [{
      productId: 5, sku: 'SKU-1', productName: 'Widget', orderedQty: 10, uom: 'PCS',
      preferredLotNo: 'LOT-1', fefoPreferred: true, lineNotes: 'fragile', availableQty: 30, reservedQty: 0, stockLoading: false
    }]

    await wrapper.vm.submitForm()
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      order_type: 'SALES_ORDER',
      priority: 'HIGH',
      warehouse_id: 1,
      items: [expect.objectContaining({ product_id: 5, ordered_qty: 10, preferred_lot_no: 'LOT-1', line_notes: 'fragile' })]
    }))
  })

  it('blocks submission with no items', async () => {
    const wrapper = mountForm()
    await flushPromises()
    wrapper.vm.form.warehouseId = 1
    await wrapper.vm.submitForm()
    expect(createOrder).not.toHaveBeenCalled()
    expect(wrapper.vm.formError).toBeTruthy()
  })

  it('loads an existing PENDING order in edit mode', async () => {
    getOrder.mockResolvedValue({
      data: {
        id: 7, order_no: 'ORD-000007', status: 'PENDING', order_type: 'SALES_ORDER', priority: 'NORMAL',
        warehouse_id: 1, items: [{ id: 1, product_id: 5, sku: 'SKU-1', product_name: 'Widget', ordered_qty: 10, uom: 'PCS', storage_type: 'AMBIENT' }]
      }
    })
    const wrapper = mountForm({ id: '7' })
    await flushPromises()
    expect(wrapper.text()).toContain('orders.form.editTitle')
    expect(wrapper.vm.items).toHaveLength(1)
  })

  it('refuses to edit an order that is no longer PENDING', async () => {
    getOrder.mockResolvedValue({ data: { id: 8, status: 'RELEASED', items: [] } })
    const wrapper = mountForm({ id: '8' })
    await flushPromises()
    expect(wrapper.vm.loadError).toBeTruthy()
  })
})

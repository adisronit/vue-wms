import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NewPickingBatchView from '../NewPickingBatchView.vue'

vi.mock('../../../services/pickingBatches', async () => {
  const actual = await vi.importActual('../../../services/pickingBatches')
  return { ...actual, createPickingBatch: vi.fn(), listAvailableOrdersForBatch: vi.fn() }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [{ id: 1, code: 'WH-01', name: 'Main' }] })
}))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { createPickingBatch, listAvailableOrdersForBatch } from '../../../services/pickingBatches'

const globalMocks = {
  $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key),
  $router: { push: vi.fn() }
}

describe('NewPickingBatchView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('loads eligible orders once a warehouse is chosen', async () => {
    listAvailableOrdersForBatch.mockResolvedValue({
      data: [
        { order_id: 1, order_no: 'ORD-001', customer: 'Acme', ship_to_name: 'DC1', item_count: 2 },
        { order_id: 2, order_no: 'ORD-002', customer: 'Widgets', ship_to_name: 'DC2', item_count: 1 }
      ]
    })
    const wrapper = mount(NewPickingBatchView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.warehouseId = 1
    await wrapper.vm.onWarehouseChange()
    await flushPromises()

    expect(listAvailableOrdersForBatch).toHaveBeenCalledWith(1)
    expect(wrapper.vm.availableOrders).toHaveLength(2)
    expect(wrapper.text()).toContain('ORD-001')
  })

  it('refuses to submit with fewer than two selected orders', async () => {
    const wrapper = mount(NewPickingBatchView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.selectedOrderIds = [1]
    await wrapper.vm.createBatch()
    expect(createPickingBatch).not.toHaveBeenCalled()
  })

  it('creates the batch and navigates to its detail page', async () => {
    createPickingBatch.mockResolvedValue({ data: { id: 42 } })
    const wrapper = mount(NewPickingBatchView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.warehouseId = 1
    wrapper.vm.selectedOrderIds = [1, 2]
    await wrapper.vm.createBatch()
    await flushPromises()

    expect(createPickingBatch).toHaveBeenCalledWith({ warehouseId: 1, orderIds: [1, 2] })
    expect(globalMocks.$router.push).toHaveBeenCalledWith('/outbound/picking-batches/42')
  })
})

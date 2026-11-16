import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NewRelocationView from '../NewRelocationView.vue'

vi.mock('../../../services/relocation', async () => {
  const actual = await vi.importActual('../../../services/relocation')
  return { ...actual, createRelocationTask: vi.fn(), readyRelocationTask: vi.fn() }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [{ id: 1, code: 'WH-01', name: 'Main' }] }),
  listZones: vi.fn().mockResolvedValue({ data: [{ id: 1, code: 'Z1' }] }),
  listLocations: vi.fn().mockResolvedValue({
    data: [
      { id: 1, code: 'A-01' },
      { id: 2, code: 'B-02' }
    ]
  })
}))
vi.mock('../../../services/products', () => ({
  listProducts: vi.fn().mockResolvedValue({
    data: [{ id: 100, sku: 'SKU-1', name: 'Widget', base_uom: 'PCS', barcode: null }]
  })
}))
vi.mock('../../../services/inventory', () => ({
  listInventory: vi.fn().mockResolvedValue({ data: [{ available_qty: 42 }] })
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { createRelocationTask, readyRelocationTask } from '../../../services/relocation'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

describe('NewRelocationView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('scans a line and adds it to the items table', async () => {
    const wrapper = mount(NewRelocationView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.form.warehouseId = 1
    await flushPromises()

    wrapper.vm.scanForm.sku = 'SKU-1'
    wrapper.vm.scanForm.sourceLocation = 'A-01'
    wrapper.vm.scanForm.destinationLocation = 'B-02'
    wrapper.vm.scanForm.qty = 10

    await wrapper.vm.scanItem()
    await flushPromises()

    expect(wrapper.vm.items).toHaveLength(1)
    expect(wrapper.vm.items[0].sku).toBe('SKU-1')
    expect(wrapper.vm.items[0].sourceLocationCode).toBe('A-01')
    expect(wrapper.vm.items[0].destinationLocationCode).toBe('B-02')
  })

  it('rejects a scan where source and destination are the same bin', async () => {
    const wrapper = mount(NewRelocationView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.scanForm.sku = 'SKU-1'
    wrapper.vm.scanForm.sourceLocation = 'A-01'
    wrapper.vm.scanForm.destinationLocation = 'A-01'

    await wrapper.vm.scanItem()
    expect(wrapper.vm.items).toHaveLength(0)
    expect(wrapper.vm.formError).toBeTruthy()
  })

  it('creates the task and marks it ready via the confirm dialog', async () => {
    createRelocationTask.mockResolvedValue({ data: { id: 55 } })
    const wrapper = mount(NewRelocationView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.form.warehouseId = 1
    wrapper.vm.items.push({
      id: 1, productId: 100, sku: 'SKU-1', productName: 'Widget', lotNo: '', lpn: '',
      sourceLocationId: 1, sourceLocationCode: 'A-01', destinationLocationId: 2, destinationLocationCode: 'B-02',
      requestedQty: 10, uom: 'PCS'
    })

    await wrapper.vm.saveAndMarkReady()
    expect(createRelocationTask).toHaveBeenCalledTimes(1)
    expect(readyRelocationTask).toHaveBeenCalledWith(55)
  })
})

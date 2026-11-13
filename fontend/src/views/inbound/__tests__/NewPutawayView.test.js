import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NewPutawayView from '../NewPutawayView.vue'

vi.mock('../../../services/warehouses', async () => {
  const actual = await vi.importActual('../../../services/warehouses')
  return {
    ...actual,
    listWarehouses: vi.fn(),
    listZones: vi.fn(),
    listLocations: vi.fn()
  }
})
vi.mock('../../../services/products', async () => {
  const actual = await vi.importActual('../../../services/products')
  return { ...actual, listProducts: vi.fn() }
})
vi.mock('../../../services/receiving', async () => {
  const actual = await vi.importActual('../../../services/receiving')
  return { ...actual, listReceipts: vi.fn() }
})
vi.mock('../../../services/putaway', async () => {
  const actual = await vi.importActual('../../../services/putaway')
  return {
    ...actual,
    createPutawayTask: vi.fn(),
    confirmPutawayTask: vi.fn(),
    getPutawaySources: vi.fn(),
    newIdempotencyKey: vi.fn(() => 'idem-key-1')
  }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listWarehouses, listZones, listLocations } from '../../../services/warehouses'
import { listReceipts } from '../../../services/receiving'
import { createPutawayTask, confirmPutawayTask } from '../../../services/putaway'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v),
  $router: { push: vi.fn() }
}

function validItem() {
  return {
    id: 1,
    receiptItemId: null,
    productId: 5,
    sku: 'SKU-1',
    productName: 'Widget',
    lotNo: '',
    sourceLocationId: 10,
    destinationLocationId: 20,
    requestedQty: 5,
    uom: 'PCS',
    status: 'CONFIRMED',
    remark: ''
  }
}

function makeValid(wrapper) {
  wrapper.vm.form.warehouseId = 1
  wrapper.vm.items = [validItem()]
}

describe('NewPutawayView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listWarehouses.mockResolvedValue({ data: [] })
    listZones.mockResolvedValue({ data: [] })
    listLocations.mockResolvedValue({ data: [] })
    listReceipts.mockResolvedValue({ data: [] })
  })

  it('mounts without throwing', async () => {
    const wrapper = mount(NewPutawayView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.vm).toBeTruthy()
  })

  it('does not open the confirm dialog or call the API for an invalid form', async () => {
    const wrapper = mount(NewPutawayView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.confirmPutawayClick()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(false)
    expect(createPutawayTask).not.toHaveBeenCalled()
  })

  it('opens the confirm dialog for a valid form without calling the API yet', async () => {
    const wrapper = mount(NewPutawayView, { global: { mocks: globalMocks } })
    await flushPromises()
    makeValid(wrapper)
    await flushPromises()

    wrapper.vm.confirmPutawayClick()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(true)
    expect(createPutawayTask).not.toHaveBeenCalled()
  })

  it('confirmPutaway() calls the API, closes the dialog, and never uses window.alert on success', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    createPutawayTask.mockResolvedValue({ data: { id: 200, items: [{ id: 900 }] } })
    confirmPutawayTask.mockResolvedValue({ data: {} })

    const wrapper = mount(NewPutawayView, { global: { mocks: globalMocks } })
    await flushPromises()
    makeValid(wrapper)
    await flushPromises()
    wrapper.vm.showSubmitConfirm = true

    await wrapper.vm.confirmPutaway()
    await flushPromises()

    expect(createPutawayTask).toHaveBeenCalledTimes(1)
    expect(confirmPutawayTask).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.showSubmitConfirm).toBe(false)
    expect(wrapper.vm.submitError).toBe('')
    expect(alertSpy).not.toHaveBeenCalled()
  })

  it('confirmPutaway() keeps the dialog open and sets submitError on API rejection', async () => {
    createPutawayTask.mockRejectedValue(new Error('network boom'))

    const wrapper = mount(NewPutawayView, { global: { mocks: globalMocks } })
    await flushPromises()
    makeValid(wrapper)
    await flushPromises()
    wrapper.vm.showSubmitConfirm = true

    await wrapper.vm.confirmPutaway()
    await flushPromises()

    expect(wrapper.vm.submitError).toBe('network boom')
    expect(wrapper.vm.showSubmitConfirm).toBe(true)
    expect(confirmPutawayTask).not.toHaveBeenCalled()
  })
})

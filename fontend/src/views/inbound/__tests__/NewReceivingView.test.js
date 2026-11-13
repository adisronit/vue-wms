import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NewReceivingView from '../NewReceivingView.vue'

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
  return {
    ...actual,
    createReceipt: vi.fn(),
    confirmReceipt: vi.fn(),
    newIdempotencyKey: vi.fn(() => 'idem-key-1')
  }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listWarehouses, listZones, listLocations } from '../../../services/warehouses'
import { createReceipt, confirmReceipt } from '../../../services/receiving'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v),
  $router: { push: vi.fn() }
}

function validItem() {
  return {
    id: 1,
    productId: 5,
    sku: 'SKU-1',
    productName: 'Widget',
    barcode: '',
    expectedQty: 5,
    receiveQty: 5,
    uom: 'PCS',
    lotNo: '',
    expiryDate: '',
    locationId: 10
  }
}

// form.warehouseId has a watcher that resets stagingLocationId to '' (it
// refetches locations for the newly-selected warehouse), so the staging
// location must be assigned in a tick AFTER the watcher has already run.
async function makeValid(wrapper) {
  wrapper.vm.form.referenceNo = 'REF-1'
  wrapper.vm.form.vendor = 'ABC Supplier'
  wrapper.vm.form.warehouseId = 1
  await flushPromises()
  wrapper.vm.form.stagingLocationId = 10
  wrapper.vm.items = [validItem()]
}

describe('NewReceivingView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listWarehouses.mockResolvedValue({ data: [] })
    listZones.mockResolvedValue({ data: [] })
    listLocations.mockResolvedValue({ data: [] })
  })

  it('mounts without throwing', async () => {
    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.vm).toBeTruthy()
  })

  it('does not open the confirm dialog or call the API for an invalid form', async () => {
    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.confirmReceivingClick()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(false)
    expect(createReceipt).not.toHaveBeenCalled()
  })

  it('opens the confirm dialog for a valid form without calling the API yet', async () => {
    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await makeValid(wrapper)
    await flushPromises()

    wrapper.vm.confirmReceivingClick()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(true)
    expect(createReceipt).not.toHaveBeenCalled()
  })

  it('submitReceiving() calls the API, closes the dialog, and never uses window.alert on success', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    createReceipt.mockResolvedValue({ data: { id: 100, items: [{ id: 500 }] } })
    confirmReceipt.mockResolvedValue({ data: {} })

    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await makeValid(wrapper)
    await flushPromises()
    wrapper.vm.showSubmitConfirm = true

    await wrapper.vm.submitReceiving()
    await flushPromises()

    expect(createReceipt).toHaveBeenCalledTimes(1)
    expect(confirmReceipt).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.showSubmitConfirm).toBe(false)
    expect(wrapper.vm.submitError).toBe('')
    expect(alertSpy).not.toHaveBeenCalled()
  })

  it('submitReceiving() keeps the dialog open and sets submitError on API rejection', async () => {
    createReceipt.mockRejectedValue(new Error('network boom'))

    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await makeValid(wrapper)
    await flushPromises()
    wrapper.vm.showSubmitConfirm = true

    await wrapper.vm.submitReceiving()
    await flushPromises()

    expect(wrapper.vm.submitError).toBe('network boom')
    expect(wrapper.vm.showSubmitConfirm).toBe(true)
    expect(confirmReceipt).not.toHaveBeenCalled()
  })
})

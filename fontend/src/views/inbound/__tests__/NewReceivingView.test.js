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
vi.mock('../../../services/brands', () => ({ getBrand: vi.fn() }))

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
    product: null,
    brandName: '',
    expectedQty: 5,
    receiveQty: 5,
    uom: 'PCS',
    lotNo: '',
    expiryDate: '',
    locationId: 10,
    splits: [],
    splitsOpen: false
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

  it('blocks confirmReceivingClick when two splits share the same pallet/LPN, even though the old basic validation would have passed', async () => {
    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await makeValid(wrapper)
    wrapper.vm.items = [
      {
        ...validItem(),
        splits: [
          { quantity: 5, lotNo: 'A', palletId: 'PLT-DUP' },
          { quantity: 0, lotNo: 'B', palletId: 'PLT-DUP' }
        ]
      }
    ]
    wrapper.vm.items[0].receiveQty = 5
    await flushPromises()

    wrapper.vm.confirmReceivingClick()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(false)
    expect(wrapper.vm.formError).not.toBe('')
    expect(createReceipt).not.toHaveBeenCalled()
  })

  it('editDocument/editItems switch steps without clearing form or item state', async () => {
    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await makeValid(wrapper)
    wrapper.vm.step = 3
    await flushPromises()

    wrapper.vm.editDocument()
    expect(wrapper.vm.step).toBe(1)
    expect(wrapper.vm.form.referenceNo).toBe('REF-1')
    expect(wrapper.vm.items).toHaveLength(1)

    wrapper.vm.step = 3
    wrapper.vm.editItems()
    expect(wrapper.vm.step).toBe(2)
    expect(wrapper.vm.items[0].sku).toBe('SKU-1')
  })

  it('saveDraft() never calls confirmReceipt (Save Draft must not mutate inventory)', async () => {
    createReceipt.mockResolvedValue({ data: { id: 100, items: [{ id: 500 }] } })

    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await makeValid(wrapper)
    await flushPromises()

    await wrapper.vm.saveDraft()
    await flushPromises()

    expect(createReceipt).toHaveBeenCalledWith(expect.objectContaining({ status: 'DRAFT' }))
    expect(confirmReceipt).not.toHaveBeenCalled()
  })

  it('submitReceiving() carries mfg date, temperature, condition, and country of origin through to the Confirm payload', async () => {
    createReceipt.mockResolvedValue({ data: { id: 100, items: [{ id: 500 }] } })
    confirmReceipt.mockResolvedValue({ data: {} })

    const wrapper = mount(NewReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await makeValid(wrapper)
    wrapper.vm.items = [
      {
        ...validItem(),
        receiveQty: 10,
        splits: [
          {
            quantity: 10, lotNo: 'LOT-1', mfgDate: '2026-01-01', expiryDate: '2026-12-31',
            palletId: 'PLT-1', cartonCount: 2, locationId: 10,
            temperatureC: 4.5, conditionNote: 'Good', countryOfOrigin: 'TH'
          }
        ]
      }
    ]
    await flushPromises()
    wrapper.vm.showSubmitConfirm = true

    await wrapper.vm.submitReceiving()
    await flushPromises()

    const splitSent = confirmReceipt.mock.calls[0][1].items[0].splits[0]
    expect(splitSent).toMatchObject({
      quantity: 10, lot_no: 'LOT-1', mfg_date: '2026-01-01T00:00:00Z', expiry_date: '2026-12-31T00:00:00Z',
      pallet_id: 'PLT-1', carton_count: 2, temperature_c: 4.5, condition_note: 'Good', country_of_origin: 'TH'
    })
  })
})

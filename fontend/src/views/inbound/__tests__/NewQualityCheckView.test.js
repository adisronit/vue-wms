import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NewQualityCheckView from '../NewQualityCheckView.vue'

vi.mock('../../../services/receiving', async () => {
  const actual = await vi.importActual('../../../services/receiving')
  return { ...actual, listReceipts: vi.fn(), getReceipt: vi.fn() }
})
vi.mock('../../../services/qualitycheck', async () => {
  const actual = await vi.importActual('../../../services/qualitycheck')
  return { ...actual, listEligibleSplits: vi.fn(), createInspection: vi.fn() }
})

import { listReceipts, getReceipt } from '../../../services/receiving'
import { listEligibleSplits, createInspection } from '../../../services/qualitycheck'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v),
  $router: { push: vi.fn() }
}

function okList(data) {
  return Promise.resolve({ data })
}

describe('NewQualityCheckView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listReceipts.mockResolvedValue({ data: [{ id: 1, receipt_no: 'RCV-000001' }] })
  })

  it('mounts without throwing and loads receipts', async () => {
    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.vm).toBeTruthy()
    expect(wrapper.vm.receipts).toHaveLength(1)
  })

  it('loads eligible splits and receipt detail when a receipt is selected', async () => {
    getReceipt.mockResolvedValue({ data: { id: 1, warehouse_id: 9, warehouse_code: 'WH-01' } })
    listEligibleSplits.mockResolvedValue(okList([
      { receipt_item_split_id: 1, sku: 'SKU-1', product_name: 'Widget', lot_no: 'LOT-1', location_code: 'A-01', held_qty: 100 }
    ]))

    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.form.receiptId = 1
    await wrapper.vm.onReceiptChange()

    expect(wrapper.vm.eligibleSplits).toHaveLength(1)
    expect(wrapper.vm.receiptDetail.warehouse_code).toBe('WH-01')
    expect(wrapper.vm.formError).toBe('')
  })

  it('shows a validation message instead of creating when the receipt has no eligible lines', async () => {
    getReceipt.mockResolvedValue({ data: { id: 1, warehouse_id: 9, warehouse_code: 'WH-01' } })
    listEligibleSplits.mockResolvedValue(okList([]))

    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.form.receiptId = 1
    await wrapper.vm.onReceiptChange()

    expect(wrapper.vm.formError).toBeTruthy()
    expect(createInspection).not.toHaveBeenCalled()
  })

  it('createInspectionClick() calls the API with the receipt-derived warehouse_id and navigates away', async () => {
    getReceipt.mockResolvedValue({ data: { id: 1, warehouse_id: 9, warehouse_code: 'WH-01' } })
    listEligibleSplits.mockResolvedValue(okList([
      { receipt_item_split_id: 1, sku: 'SKU-1', product_name: 'Widget', lot_no: 'LOT-1', location_code: 'A-01', held_qty: 100 }
    ]))
    createInspection.mockResolvedValue({ data: { id: 5 } })

    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.form.receiptId = 1
    await wrapper.vm.onReceiptChange()

    await wrapper.vm.createInspectionClick()

    expect(createInspection).toHaveBeenCalledWith(expect.objectContaining({ receipt_id: 1, warehouse_id: 9 }))
    expect(globalMocks.$router.push).toHaveBeenCalledWith('/inbound/quality-check')
  })

  it('createInspectionClick() surfaces the API error instead of navigating on failure', async () => {
    getReceipt.mockResolvedValue({ data: { id: 1, warehouse_id: 9, warehouse_code: 'WH-01' } })
    listEligibleSplits.mockResolvedValue(okList([
      { receipt_item_split_id: 1, sku: 'SKU-1', product_name: 'Widget', lot_no: 'LOT-1', location_code: 'A-01', held_qty: 100 }
    ]))
    createInspection.mockRejectedValue(new Error('boom'))

    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.form.receiptId = 1
    await wrapper.vm.onReceiptChange()
    globalMocks.$router.push.mockClear()

    await wrapper.vm.createInspectionClick()

    expect(wrapper.vm.formError).toBe('boom')
    expect(globalMocks.$router.push).not.toHaveBeenCalled()
  })
})

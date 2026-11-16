import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TransferReceivingView from '../TransferReceivingView.vue'

vi.mock('../../../services/transferReceipts', async () => {
  const actual = await vi.importActual('../../../services/transferReceipts')
  return {
    ...actual,
    listTransferReceipts: vi.fn(),
    listEligibleTransferShipments: vi.fn(),
    getTransferReceipt: vi.fn(),
    createTransferReceipt: vi.fn(),
    receiveTransferReceiptItem: vi.fn(),
    confirmTransferReceipt: vi.fn(),
    cancelTransferReceipt: vi.fn()
  }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [{ id: 2, code: 'WH-02' }] }),
  listZones: vi.fn().mockResolvedValue({ data: [{ id: 5, type: 'RECEIVING' }] }),
  listLocations: vi.fn().mockResolvedValue({ data: [{ id: 50, code: 'DOCK-01' }] })
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import {
  listTransferReceipts,
  getTransferReceipt,
  confirmTransferReceipt,
  cancelTransferReceipt,
  receiveTransferReceiptItem
} from '../../../services/transferReceipts'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const row = {
  id: 10,
  receipt_no: 'TRC-000001',
  shipment_id: 1,
  shipment_no: 'SHP-000001',
  order_no: 'SO-20260608-001',
  transfer_reference: 'TRF-0001',
  destination_warehouse_id: 2,
  destination_warehouse_code: 'WH-02',
  status: 'DRAFT'
}

function receiptDetail(overrides = {}) {
  return {
    id: 10,
    receipt_no: 'TRC-000001',
    shipment_id: 1,
    shipment_no: 'SHP-000001',
    order_id: 1,
    order_no: 'SO-20260608-001',
    transfer_reference: 'TRF-0001',
    destination_warehouse_id: 2,
    destination_warehouse_code: 'WH-02',
    status: 'DRAFT',
    received_by: null,
    received_by_name: null,
    received_at: null,
    remark: null,
    cancelled_at: null,
    cancelled_by_name: null,
    cancel_remark: null,
    items: [
      {
        id: 1,
        shipment_item_id: 1,
        sku: 'SKU-1',
        product_name: 'Widget',
        lot_no: 'LOT-1',
        uom: 'PCS',
        dispatched_qty: 40,
        received_qty: 0,
        location_id: null,
        location_code: null,
        discrepancy_qty: null,
        discrepancy_reason: null,
        status: 'PENDING'
      }
    ],
    ...overrides
  }
}

describe('TransferReceivingView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched transfer receipts once a destination warehouse is selected', async () => {
    listTransferReceipts.mockResolvedValue(page([row]))
    const wrapper = mount(TransferReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.filters.destinationWarehouseId = 2
    await wrapper.vm.fetchReceipts()
    await flushPromises()

    expect(wrapper.text()).toContain('TRC-000001')
    expect(wrapper.text()).toContain('SHP-000001')
  })

  it('does not fetch receipts until a destination warehouse is selected', async () => {
    const wrapper = mount(TransferReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.fetchReceipts()
    expect(listTransferReceipts).not.toHaveBeenCalled()
    expect(wrapper.vm.receiptList).toEqual([])
  })

  it('opens the drawer and shows the receipt detail with its lines', async () => {
    getTransferReceipt.mockResolvedValue({ data: receiptDetail() })
    const wrapper = mount(TransferReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openReceipt({ id: 10 })
    await flushPromises()

    expect(wrapper.vm.selectedReceipt.receipt_no).toBe('TRC-000001')
    expect(wrapper.vm.selectedReceipt.items).toHaveLength(1)
    expect(wrapper.vm.isNewReceipt).toBe(false)
  })

  it('does not allow Confirm until every line is settled', async () => {
    getTransferReceipt.mockResolvedValue({ data: receiptDetail() })
    const wrapper = mount(TransferReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openReceipt({ id: 10 })
    await flushPromises()

    expect(wrapper.vm.allLinesSettled).toBe(false)

    // Settle the only line, then Confirm becomes available.
    wrapper.vm.selectedReceipt.items[0].status = 'COMPLETED'
    expect(wrapper.vm.allLinesSettled).toBe(true)

    await wrapper.vm.confirmReceipt()
    expect(confirmTransferReceipt).toHaveBeenCalledWith(10, expect.objectContaining({ idempotency_key: expect.any(String) }))
  })

  it('requires a staging location before receiving a line', async () => {
    getTransferReceipt.mockResolvedValue({ data: receiptDetail() })
    const wrapper = mount(TransferReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openReceipt({ id: 10 })
    await flushPromises()

    const line = wrapper.vm.selectedReceipt.items[0]
    wrapper.vm.receiveForms[line.id].locationId = ''
    await wrapper.vm.receiveLine(line)

    expect(receiveTransferReceiptItem).not.toHaveBeenCalled()
    expect(wrapper.vm.drawerError).toBe('transferReceiving.messages.locationRequired')
  })

  it('requires explicit confirmation before cancelling a receipt', async () => {
    getTransferReceipt.mockResolvedValue({ data: receiptDetail() })
    const wrapper = mount(TransferReceivingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openReceipt({ id: 10 })
    await flushPromises()

    wrapper.vm.showCancelConfirm = true
    await wrapper.vm.$nextTick()
    expect(cancelTransferReceipt).not.toHaveBeenCalled()

    await wrapper.vm.cancelDrawerReceipt()
    expect(cancelTransferReceipt).toHaveBeenCalledTimes(1)
  })
})

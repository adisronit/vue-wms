import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PickingBatchDetailView from '../PickingBatchDetailView.vue'

vi.mock('../../../services/pickingBatches', async () => {
  const actual = await vi.importActual('../../../services/pickingBatches')
  return { ...actual, getPickingBatch: vi.fn(), confirmPickingBatch: vi.fn(), cancelPickingBatch: vi.fn() }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { getPickingBatch, confirmPickingBatch } from '../../../services/pickingBatches'

const globalMocks = { $t: (key) => key }

function draftBatch() {
  return {
    id: 7, batch_no: 'BATCH-000007', warehouse_id: 1, warehouse_code: 'WH-01', status: 'DRAFT',
    picking_task_id: null, pick_no: null,
    orders: [
      { order_id: 1, order_no: 'ORD-001', customer: 'Acme', ship_to_name: 'DC1', status: 'RELEASED' },
      { order_id: 2, order_no: 'ORD-002', customer: 'Widgets', ship_to_name: 'DC2', status: 'RELEASED' }
    ],
    lines: [
      {
        product_id: 100, sku: 'SKU-A', product_name: 'Widget', lot_no: 'LOT-1', location_id: 10, location_code: 'A-01-01',
        qty: 100, uom: 'PCS',
        allocations: [
          { order_id: 1, order_no: 'ORD-001', order_item_id: 11, picking_task_item_id: null, allocated_qty: 20 },
          { order_id: 2, order_no: 'ORD-002', order_item_id: 12, picking_task_item_id: null, allocated_qty: 80 }
        ]
      }
    ]
  }
}

describe('PickingBatchDetailView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the consolidated lines with their per-order allocation breakdown', async () => {
    getPickingBatch.mockResolvedValue({ data: draftBatch() })
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('BATCH-000007')
    expect(wrapper.text()).toContain('SKU-A')
    expect(wrapper.text()).toContain('ORD-001')
    expect(wrapper.text()).toContain('ORD-002')
    expect(wrapper.text()).toContain('20')
    expect(wrapper.text()).toContain('80')
  })

  it('shows the Confirm button only while the batch is DRAFT', async () => {
    getPickingBatch.mockResolvedValue({ data: draftBatch() })
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.text()).toContain('pickingBatches.detail.confirmButton')
  })

  it('confirms the batch and reloads it', async () => {
    getPickingBatch
      .mockResolvedValueOnce({ data: draftBatch() })
      .mockResolvedValueOnce({ data: { ...draftBatch(), status: 'CONFIRMED' } })
    confirmPickingBatch.mockResolvedValue({ data: { ...draftBatch(), status: 'CONFIRMED' } })
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.confirmBatch()
    await flushPromises()

    expect(confirmPickingBatch).toHaveBeenCalledWith(7)
    expect(wrapper.vm.batch.status).toBe('CONFIRMED')
  })
})

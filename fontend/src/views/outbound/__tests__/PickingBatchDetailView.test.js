import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PickingBatchDetailView from '../PickingBatchDetailView.vue'

vi.mock('../../../services/pickingBatches', async () => {
  const actual = await vi.importActual('../../../services/pickingBatches')
  return {
    ...actual,
    getPickingBatch: vi.fn(), confirmPickingBatch: vi.fn(), cancelPickingBatch: vi.fn(),
    viewConsolidatedPickList: vi.fn()
  }
})
vi.mock('../../../services/documents', async () => {
  const actual = await vi.importActual('../../../services/documents')
  return { ...actual, getDocumentHistory: vi.fn() }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { getPickingBatch, confirmPickingBatch, viewConsolidatedPickList } from '../../../services/pickingBatches'
import { getDocumentHistory } from '../../../services/documents'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key) }

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

  // Phase 33C.2: once a batch is CONFIRMED (its pick is in progress or
  // done), a "Go to Sortation" button must be offered alongside "Go to
  // Picking" — Sortation is a continuation of the same batch, not a
  // separate top-level workflow.
  it('shows the Go to Sortation button once the batch is CONFIRMED or COMPLETED', async () => {
    getPickingBatch.mockResolvedValue({ data: { ...draftBatch(), status: 'CONFIRMED', picking_task_id: 5 } })
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('pickingBatches.detail.goToSortation'))).toBe(true)
  })

  it('hides the Go to Sortation button while the batch is still DRAFT', async () => {
    getPickingBatch.mockResolvedValue({ data: draftBatch() })
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('pickingBatches.detail.goToSortation'))).toBe(false)
  })

  // Phase 33C.3 — Consolidated Batch Picking List PDF + Document History.
  it('offers the Print Consolidated Pick List button only once CONFIRMED/COMPLETED, and wires it to viewConsolidatedPickList', async () => {
    getPickingBatch.mockResolvedValue({ data: { ...draftBatch(), status: 'CONFIRMED', picking_task_id: 5 } })
    viewConsolidatedPickList.mockResolvedValue()
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()

    const printBtn = wrapper.findAll('button').find((b) => b.text().includes('pickingBatches.detail.printPickListButton'))
    expect(printBtn).toBeTruthy()
    await printBtn.trigger('click')
    expect(viewConsolidatedPickList).toHaveBeenCalledWith(7)
  })

  it('hides the Print Consolidated Pick List button while the batch is still DRAFT', async () => {
    getPickingBatch.mockResolvedValue({ data: draftBatch() })
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('pickingBatches.detail.printPickListButton'))).toBe(false)
  })

  it('opens the Document History panel and loads this batch\'s PICKING_BATCH_LIST history', async () => {
    getPickingBatch.mockResolvedValue({ data: draftBatch() })
    getDocumentHistory.mockResolvedValue({
      data: [{ id: 1, document_type: 'PICKING_BATCH_LIST', document_no: 'BATCH-000007', rendered_at: '2026-01-01T00:00:00Z', is_draft: false, rendered_by_name: 'Alice' }]
    })
    const wrapper = mount(PickingBatchDetailView, { props: { id: 7 }, global: { mocks: globalMocks } })
    await flushPromises()

    const historyBtn = wrapper.findAll('button').find((b) => b.text().includes('pickingBatches.detail.historyButton'))
    expect(historyBtn).toBeTruthy()
    await historyBtn.trigger('click')
    await flushPromises()

    expect(getDocumentHistory).toHaveBeenCalledWith('PICKING_BATCH_LIST', 7)
    expect(wrapper.text()).toContain('Alice')
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PickingBatchView from '../PickingBatchView.vue'

vi.mock('../../../services/pickingBatches', async () => {
  const actual = await vi.importActual('../../../services/pickingBatches')
  return { ...actual, listPickingBatches: vi.fn() }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [{ id: 1, code: 'WH-01', name: 'Main' }] })
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listPickingBatches } from '../../../services/pickingBatches'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 20, total: rows.length, total_pages: 1 } }
}

describe('PickingBatchView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched batches with a status badge', async () => {
    listPickingBatches.mockResolvedValue(page([{
      id: 1, batch_no: 'BATCH-000001', warehouse_code: 'WH-01', order_count: 3, line_count: 1,
      status: 'DRAFT', pick_no: null, created_at: '2026-01-01T00:00:00Z'
    }]))
    const wrapper = mount(PickingBatchView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.text()).toContain('BATCH-000001')
    expect(wrapper.findComponent({ name: 'StatusBadge' }).exists()).toBe(true)
  })

  it('shows the new-batch button when the caller can create', async () => {
    listPickingBatches.mockResolvedValue(page([]))
    const wrapper = mount(PickingBatchView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.text()).toContain('pickingBatches.newButton')
  })
})

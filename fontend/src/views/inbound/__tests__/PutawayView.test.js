import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PutawayView from '../PutawayView.vue'

vi.mock('../../../services/putaway', async () => {
  const actual = await vi.importActual('../../../services/putaway')
  return { ...actual, listPutaway: vi.fn(), getPutawayTask: vi.fn(), cancelPutawayTask: vi.fn() }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [] }),
  listZones: vi.fn().mockResolvedValue({ data: [] }),
  listLocations: vi.fn().mockResolvedValue({ data: [] })
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listPutaway, getPutawayTask, cancelPutawayTask } from '../../../services/putaway'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const task = {
  item_id: 1,
  task_id: 10,
  putaway_no: 'PA-001',
  receipt_no: 'RCPT-001',
  sku: 'SKU-1',
  product_name: 'Widget',
  requested_qty: 5,
  source_location_code: 'RCV-01',
  destination_location_code: null,
  status: 'PENDING'
}

describe('PutawayView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched tasks in the table', async () => {
    listPutaway.mockResolvedValue(page([task]))
    const wrapper = mount(PutawayView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.text()).toContain('PA-001')
    expect(wrapper.text()).toContain('Widget')
  })

  it('requires explicit confirmation before cancelling a task', async () => {
    listPutaway.mockResolvedValue(page([task]))
    getPutawayTask.mockResolvedValue({ data: { warehouse_id: null, status: 'PENDING', cancelled_at: null, cancelled_by_name: null, cancel_remark: null } })
    const wrapper = mount(PutawayView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openPutaway({ ...task, id: 1, taskId: 10 })
    await flushPromises()

    wrapper.vm.showCancelConfirm = true
    await wrapper.vm.$nextTick()
    expect(cancelPutawayTask).not.toHaveBeenCalled()

    await wrapper.vm.cancelDrawerTask()
    expect(cancelPutawayTask).toHaveBeenCalledTimes(1)
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import RelocationView from '../RelocationView.vue'

vi.mock('../../../services/relocation', async () => {
  const actual = await vi.importActual('../../../services/relocation')
  return {
    ...actual,
    listRelocations: vi.fn(),
    getRelocationTask: vi.fn(),
    cancelRelocationTask: vi.fn(),
    readyRelocationTask: vi.fn(),
    confirmRelocationTask: vi.fn()
  }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [] }),
  listZones: vi.fn().mockResolvedValue({ data: [] }),
  listLocations: vi.fn().mockResolvedValue({ data: [] })
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listRelocations, getRelocationTask, cancelRelocationTask, readyRelocationTask } from '../../../services/relocation'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const row = {
  item_id: 1,
  task_id: 10,
  relocation_no: 'RL-000001',
  sku: 'SKU-1',
  product_name: 'Widget',
  lot_no: 'LOT-1',
  requested_qty: 50,
  moved_qty: 0,
  source_location_code: 'A-01',
  destination_location_code: 'B-02',
  status: 'PENDING',
  task_status: 'DRAFT'
}

const taskDetail = {
  id: 10,
  relocation_no: 'RL-000001',
  warehouse_id: 1,
  warehouse_code: 'WH-01',
  status: 'DRAFT',
  remark: null,
  created_at: '2026-01-01T00:00:00Z',
  ready_at: null,
  completed_at: null,
  cancelled_at: null,
  cancelled_by_name: null,
  cancel_remark: null,
  items: [
    {
      id: 1,
      sku: 'SKU-1',
      product_name: 'Widget',
      lot_no: 'LOT-1',
      lpn: null,
      source_location_code: 'A-01',
      destination_location_code: 'B-02',
      requested_qty: 50,
      moved_qty: 0,
      uom: 'PCS',
      status: 'PENDING'
    }
  ]
}

describe('RelocationView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched relocation tasks in the table', async () => {
    listRelocations.mockResolvedValue(page([row]))
    const wrapper = mount(RelocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.text()).toContain('RL-000001')
    expect(wrapper.text()).toContain('Widget')
  })

  it('opens the drawer and shows Mark Ready for a DRAFT task', async () => {
    listRelocations.mockResolvedValue(page([row]))
    getRelocationTask.mockResolvedValue({ data: taskDetail })
    const wrapper = mount(RelocationView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openRelocation({ taskId: 10 })
    await flushPromises()

    expect(wrapper.vm.selectedTask.relocation_no).toBe('RL-000001')
    expect(readyRelocationTask).not.toHaveBeenCalled()

    await wrapper.vm.markReady()
    expect(readyRelocationTask).toHaveBeenCalledWith(10)
  })

  it('requires explicit confirmation before cancelling a task', async () => {
    listRelocations.mockResolvedValue(page([row]))
    getRelocationTask.mockResolvedValue({ data: taskDetail })
    const wrapper = mount(RelocationView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openRelocation({ taskId: 10 })
    await flushPromises()

    wrapper.vm.showCancelConfirm = true
    await wrapper.vm.$nextTick()
    expect(cancelRelocationTask).not.toHaveBeenCalled()

    await wrapper.vm.cancelDrawerTask()
    expect(cancelRelocationTask).toHaveBeenCalledTimes(1)
  })
})

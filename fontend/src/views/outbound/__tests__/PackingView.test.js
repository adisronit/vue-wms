import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PackingView from '../PackingView.vue'

vi.mock('../../../services/packing', async () => {
  const actual = await vi.importActual('../../../services/packing')
  return {
    ...actual,
    listPacking: vi.fn(),
    exportPacking: vi.fn(),
    listPackableSources: vi.fn(),
    createPackingJob: vi.fn()
  }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [] })
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { listPacking, listPackableSources, createPackingJob } from '../../../services/packing'
import { hasPermission } from '../../../services/auth'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

function mountView() {
  return mount(PackingView, { global: { mocks: { ...globalMocks, $router: { push: vi.fn() } } } })
}

describe('PackingView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // vi.clearAllMocks() only clears call history, not a prior test's
    // mockImplementation override (that's vi.resetAllMocks()) — restore
    // the default "always allowed" implementation explicitly so one
    // test's permission override (below) never leaks into the next.
    hasPermission.mockImplementation(() => true)
  })

  it('renders fetched packing jobs with a status badge', async () => {
    listPacking.mockResolvedValue(page([{
      id: 1, job_no: 'PACK-000001', picking_task_id: 1, order_no: 'SO-001', customer: 'Acme',
      warehouse_code: 'WH-01', status: 'IN_PROGRESS', item_count: 2, progress_pct: 50,
      assigned_packer_name: 'Alice', created_at: '2026-01-01T00:00:00Z'
    }]))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('PACK-000001')
    expect(wrapper.text()).toContain('Alice')
    expect(wrapper.findComponent({ name: 'StatusBadge' }).exists()).toBe(true)
  })

  it('shows the Create Packing Job button when the caller has packing.create', async () => {
    listPacking.mockResolvedValue(page([]))
    const wrapper = mountView()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('packing.newJobButton'))).toBe(true)
  })

  it('hides the Create Packing Job button without packing.create', async () => {
    hasPermission.mockImplementation((perm) => perm !== 'packing.create')
    listPacking.mockResolvedValue(page([]))
    const wrapper = mountView()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('packing.newJobButton'))).toBe(false)
  })

  it('loads packable sources and creates a packing job from the modal', async () => {
    listPacking.mockResolvedValue(page([]))
    listPackableSources.mockResolvedValue({
      success: true,
      data: [{ picking_task_id: 5, pick_no: 'PICK-000005', order_no: 'SO-005', customer: 'Acme', item_count: 2 }]
    })
    createPackingJob.mockResolvedValue({ success: true, data: { id: 9 } })

    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((b) => b.text().includes('packing.newJobButton'))
    await createButton.trigger('click')
    await flushPromises()

    expect(listPackableSources).toHaveBeenCalled()
    expect(wrapper.text()).toContain('PICK-000005')

    // The list page's own filter bar also renders <select> elements —
    // target the modal's picking-task picker specifically, not just the
    // first select in DOM order.
    const select = wrapper.findAll('select').find((s) => s.html().includes('PICK-000005'))
    await select.setValue('5')

    const submitButton = wrapper.findAll('button').find((b) => b.text().includes('packing.create.createButton'))
    await submitButton.trigger('click')
    await flushPromises()

    expect(createPackingJob).toHaveBeenCalledWith(expect.objectContaining({ picking_task_id: 5 }))
  })

  it('shows an empty-sources message when nothing is packable', async () => {
    listPacking.mockResolvedValue(page([]))
    listPackableSources.mockResolvedValue({ success: true, data: [] })

    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((b) => b.text().includes('packing.newJobButton'))
    await createButton.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('packing.create.noSources')
  })

  it('surfaces a load error banner when the list request fails', async () => {
    listPacking.mockRejectedValue(new Error('boom'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('boom')
  })
})

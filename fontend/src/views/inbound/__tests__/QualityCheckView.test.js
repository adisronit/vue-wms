import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import QualityCheckView from '../QualityCheckView.vue'

vi.mock('../../../services/qualitycheck', async () => {
  const actual = await vi.importActual('../../../services/qualitycheck')
  return {
    ...actual,
    listInspections: vi.fn(),
    getInspection: vi.fn(),
    confirmInspection: vi.fn(),
    newIdempotencyKey: vi.fn(() => 'idem-key-1')
  }
})

import { listInspections, getInspection, confirmInspection } from '../../../services/qualitycheck'

const globalMocks = {
  $t: (key, params) => (params ? key + JSON.stringify(params) : key),
  $d: (v) => String(v)
}

function inspectionRow(overrides = {}) {
  return {
    id: 1, inspection_no: 'QC-000001', receipt_no: 'RCV-000001', warehouse_code: 'WH-01',
    qc_type: 'NORMAL', status: 'PENDING', item_count: 1, ...overrides
  }
}

function inspectionDetail(overrides = {}) {
  return {
    id: 1, inspection_no: 'QC-000001', receipt_no: 'RCV-000001', status: 'PENDING',
    items: [
      {
        id: 10, sku: 'SKU-1', product_name: 'Widget', lot_no: 'LOT-1', location_code: 'A-01',
        remaining_held_qty: 100, status: 'PENDING', dispositions: []
      }
    ],
    ...overrides
  }
}

describe('QualityCheckView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listInspections.mockResolvedValue({ data: [inspectionRow()] })
  })

  it('loads real inspections on mount', async () => {
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listInspections).toHaveBeenCalled()
    expect(wrapper.text()).toContain('QC-000001')
  })

  it('computes status counts from the loaded list', async () => {
    listInspections.mockResolvedValue({
      data: [inspectionRow({ status: 'PENDING' }), inspectionRow({ id: 2, status: 'COMPLETED' })]
    })
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusCounts.PENDING).toBe(1)
    expect(wrapper.vm.statusCounts.COMPLETED).toBe(1)
    expect(wrapper.vm.statusCounts.IN_PROGRESS).toBe(0)
  })

  it('openInspection() fetches and opens the real detail drawer', async () => {
    getInspection.mockResolvedValue({ data: inspectionDetail() })
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openInspection(inspectionRow())

    expect(getInspection).toHaveBeenCalledWith(1)
    expect(wrapper.vm.selected.items).toHaveLength(1)
  })

  it('addPending() requires a positive quantity before queuing a disposition', async () => {
    getInspection.mockResolvedValue({ data: inspectionDetail() })
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openInspection(inspectionRow())

    const item = wrapper.vm.selected.items[0]
    wrapper.vm.draft[item.id].quantity = 0
    wrapper.vm.addPending(item)

    expect(wrapper.vm.drawerError).toBeTruthy()
    expect(wrapper.vm.pendingByItem[item.id]).toHaveLength(0)
  })

  it('addPending() requires a defect code for REJECT entries', async () => {
    getInspection.mockResolvedValue({ data: inspectionDetail() })
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openInspection(inspectionRow())

    const item = wrapper.vm.selected.items[0]
    wrapper.vm.draft[item.id] = { disposition: 'REJECT', quantity: 2, defectCode: '', evidenceReference: '' }
    wrapper.vm.addPending(item)

    expect(wrapper.vm.drawerError).toBeTruthy()
    expect(wrapper.vm.pendingByItem[item.id]).toHaveLength(0)
  })

  it('submitConfirm() sends every queued disposition across all items in one call', async () => {
    getInspection.mockResolvedValue({ data: inspectionDetail() })
    confirmInspection.mockResolvedValue({ data: inspectionDetail({ status: 'COMPLETED' }) })
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openInspection(inspectionRow())

    const item = wrapper.vm.selected.items[0]
    wrapper.vm.draft[item.id] = { disposition: 'PASS', quantity: 95, defectCode: '', evidenceReference: '' }
    wrapper.vm.addPending(item)
    wrapper.vm.draft[item.id] = { disposition: 'HOLD', quantity: 3, defectCode: '', evidenceReference: '' }
    wrapper.vm.addPending(item)
    wrapper.vm.draft[item.id] = { disposition: 'REJECT', quantity: 2, defectCode: 'DAMAGED', evidenceReference: '' }
    wrapper.vm.addPending(item)

    expect(wrapper.vm.totalPendingCount).toBe(3)

    await wrapper.vm.submitConfirm()

    expect(confirmInspection).toHaveBeenCalledWith(1, expect.objectContaining({
      idempotency_key: 'idem-key-1',
      dispositions: expect.arrayContaining([
        expect.objectContaining({ disposition: 'PASS', quantity: 95 }),
        expect.objectContaining({ disposition: 'HOLD', quantity: 3 }),
        expect.objectContaining({ disposition: 'REJECT', quantity: 2, defect_code: 'DAMAGED' })
      ])
    }))
    expect(wrapper.vm.selected).toBeNull()
  })

  it('submitConfirm() surfaces the API error and keeps the drawer open on failure', async () => {
    getInspection.mockResolvedValue({ data: inspectionDetail() })
    confirmInspection.mockRejectedValue(new Error('boom'))
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openInspection(inspectionRow())

    const item = wrapper.vm.selected.items[0]
    wrapper.vm.draft[item.id] = { disposition: 'PASS', quantity: 10, defectCode: '', evidenceReference: '' }
    wrapper.vm.addPending(item)

    await wrapper.vm.submitConfirm()

    expect(wrapper.vm.confirmError).toBe('boom')
    expect(wrapper.vm.selected).not.toBeNull()
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PackingDetailView from '../PackingDetailView.vue'

vi.mock('../../../services/packing', () => ({
  getPackingJob: vi.fn(),
  assignPacker: vi.fn(),
  packItem: vi.fn(),
  flagPackingException: vi.fn(),
  createCarton: vi.fn(),
  updateCarton: vi.fn(),
  checkerConfirmPackingJob: vi.fn(),
  cancelPackingJob: vi.fn(),
  newIdempotencyKey: vi.fn(() => 'idem-key-1'),
  viewPackingList: vi.fn(),
  downloadPackingList: vi.fn(),
  viewCartonLabels: vi.fn()
}))
vi.mock('../../../services/users', () => ({ listUsers: vi.fn().mockResolvedValue({ data: [] }) }))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import {
  getPackingJob, packItem, flagPackingException, checkerConfirmPackingJob, cancelPackingJob
} from '../../../services/packing'
import { hasPermission } from '../../../services/auth'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function baseJob(overrides = {}) {
  return {
    id: 1, job_no: 'PACK-000001', picking_task_id: 1, pick_no: 'PICK-000001', order_no: 'SO-001',
    customer: 'Acme', ship_to_name: null, warehouse_code: 'WH-01', status: 'IN_PROGRESS',
    assigned_packer_name: null, checker_name: null, remark: null, cancel_remark: null,
    items: [{
      id: 10, product_id: 1, sku: 'SKU-1', product_name: 'Widget', lot_no: 'LOT-1',
      ordered_qty: 10, picked_qty: 10, packed_qty: 0, uom: 'PCS', status: 'PENDING',
      exception_flag: false, exception_reason: null, exception_qty: null, remark: null, carton_lpns: []
    }],
    cartons: [{
      id: 100, packing_job_id: 1, lpn: 'CTN-000001', packaging_type: 'BOX',
      gross_weight: null, net_weight: null, weight_unit: 'KG',
      carton_length: null, carton_width: null, carton_height: null, dimension_unit: 'CM',
      status: 'OPEN', remark: null, contents: []
    }],
    ...overrides
  }
}

function mountDetail(props = { id: '1' }) {
  return mount(PackingDetailView, {
    props,
    global: { mocks: { ...globalMocks, $router: { push: vi.fn() } } }
  })
}

describe('PackingDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // See PackingView.test.js's own comment: clearAllMocks() does not
    // undo a prior test's mockImplementation override.
    hasPermission.mockImplementation(() => true)
  })

  it('renders the job header and its one open carton', async () => {
    getPackingJob.mockResolvedValue({ data: baseJob() })
    const wrapper = mountDetail()
    await flushPromises()

    expect(wrapper.text()).toContain('PACK-000001')
    expect(wrapper.text()).toContain('CTN-000001')
    expect(wrapper.text()).toContain('SKU-1')
  })

  it('packs a line into a carton with the scanned SKU and quantity', async () => {
    getPackingJob.mockResolvedValue({ data: baseJob() })
    packItem.mockResolvedValue({ data: baseJob({ items: [{ ...baseJob().items[0], packed_qty: 10, status: 'PACKED' }] }) })

    const wrapper = mountDetail()
    await flushPromises()

    // Several <select> elements exist on the page (assign packer, add-
    // carton packaging type, this line's own carton picker) — target
    // the one offering CTN-000001 as an option, not just the first
    // select in DOM order.
    const cartonSelect = wrapper.findAll('select').find((s) => s.html().includes('CTN-000001'))
    await cartonSelect.setValue('100')

    const skuInput = wrapper.find('input[placeholder="packing.detail.scanSkuPlaceholder"]')
    await skuInput.setValue('SKU-1')
    const qtyInput = wrapper.find('input[placeholder="packing.detail.packQtyPlaceholder"]')
    await qtyInput.setValue('10')

    const packButton = wrapper.findAll('button').find((b) => b.text().includes('packing.detail.packButton'))
    await packButton.trigger('click')
    await flushPromises()

    expect(packItem).toHaveBeenCalledWith(1, expect.objectContaining({
      item_id: 10, carton_id: 100, scanned_sku: 'SKU-1', quantity: 10
    }))
  })

  it('flags an exception on a line with a reason', async () => {
    getPackingJob.mockResolvedValue({ data: baseJob() })
    flagPackingException.mockResolvedValue({ data: baseJob({ items: [{ ...baseJob().items[0], status: 'EXCEPTION', exception_flag: true }] }) })

    const wrapper = mountDetail()
    await flushPromises()

    const flagButton = wrapper.findAll('button').find((b) => b.text().includes('packing.detail.flagExceptionButton'))
    await flagButton.trigger('click')
    await flushPromises()

    const reasonSelect = wrapper.findAll('select').find((s) => s.findAll('option').some((o) => o.attributes('value') === 'DAMAGED'))
    await reasonSelect.setValue('DAMAGED')

    const submitButton = wrapper.findAll('button').find((b) => b.text().includes('packing.detail.submitExceptionButton'))
    await submitButton.trigger('click')
    await flushPromises()

    expect(flagPackingException).toHaveBeenCalledWith(1, 10, expect.objectContaining({ reason: 'DAMAGED' }))
  })

  it('shows the checker confirm button only with packing.confirm, gated on IN_PROGRESS', async () => {
    getPackingJob.mockResolvedValue({ data: baseJob({ status: 'IN_PROGRESS' }) })
    const wrapper = mountDetail()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('packing.detail.checkerConfirmButton'))).toBe(true)
  })

  it('hides the checker confirm button for a DRAFT job', async () => {
    getPackingJob.mockResolvedValue({ data: baseJob({ status: 'DRAFT' }) })
    const wrapper = mountDetail()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('packing.detail.checkerConfirmButton'))).toBe(false)
  })

  it('calls checkerConfirmPackingJob when the checker confirm button is clicked', async () => {
    getPackingJob.mockResolvedValue({ data: baseJob({ status: 'IN_PROGRESS' }) })
    checkerConfirmPackingJob.mockResolvedValue({ data: baseJob({ status: 'COMPLETED' }) })

    const wrapper = mountDetail()
    await flushPromises()

    const confirmButton = wrapper.findAll('button').find((b) => b.text().includes('packing.detail.checkerConfirmButton'))
    await confirmButton.trigger('click')
    await flushPromises()

    expect(checkerConfirmPackingJob).toHaveBeenCalledWith(1, {})
  })

  it('cancels the job through the confirm dialog', async () => {
    getPackingJob.mockResolvedValue({ data: baseJob() })
    cancelPackingJob.mockResolvedValue({ data: baseJob({ status: 'CANCELLED' }) })

    const wrapper = mountDetail()
    await flushPromises()

    const cancelButton = wrapper.findAll('button').find((b) => b.text().includes('packing.detail.cancelJobButton'))
    await cancelButton.trigger('click')
    await flushPromises()

    const confirmDialog = wrapper.findComponent({ name: 'ConfirmDialog' })
    expect(confirmDialog.props('open')).toBe(true)

    await confirmDialog.vm.$emit('confirm')
    await flushPromises()

    expect(cancelPackingJob).toHaveBeenCalledWith(1, {})
  })

  it('hides update actions without packing.update', async () => {
    hasPermission.mockImplementation((perm) => perm === 'packing.confirm' || perm === 'packing.approve')
    getPackingJob.mockResolvedValue({ data: baseJob() })
    const wrapper = mountDetail()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('packing.detail.packButton'))).toBe(false)
    expect(buttons.some((t) => t.includes('packing.detail.addCartonButton'))).toBe(false)
  })
})

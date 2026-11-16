import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SortationView from '../SortationView.vue'

vi.mock('../../../services/pickingBatches', async () => {
  const actual = await vi.importActual('../../../services/pickingBatches')
  return {
    ...actual,
    getSortation: vi.fn(),
    scanSortation: vi.fn(),
    flagSortationException: vi.fn(),
    assignSortation: vi.fn()
  }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { getSortation, scanSortation, flagSortationException } from '../../../services/pickingBatches'
import { hasPermission } from '../../../services/auth'

const globalMocks = { $t: (key) => key, $router: { push: vi.fn() } }

function worklist(overrides = {}) {
  return {
    batch_id: 1, batch_no: 'BATCH-000001', picking_task_id: 5, pick_no: 'PICK-000005',
    sortation_status: 'NOT_STARTED', sortation_station: null, sortation_assigned_to: null,
    sortation_started_at: null, sortation_completed_at: null,
    lines: [
      {
        allocation_id: 1, order_id: 101, order_no: 'SO-001', ship_to_name: 'DC1',
        order_item_id: 11, product_id: 1, sku: 'SKU-A', product_name: 'Widget', lot_no: 'LOT-X',
        location_id: 10, location_code: 'A-01-01', picking_task_item_id: 50,
        allocated_qty: 20, sorted_qty: 0, shortage_qty: null, exception_reason: null,
        remaining: 20, uom: 'PCS', resolved: false
      },
      {
        allocation_id: 2, order_id: 102, order_no: 'SO-002', ship_to_name: 'DC2',
        order_item_id: 12, product_id: 1, sku: 'SKU-A', product_name: 'Widget', lot_no: 'LOT-X',
        location_id: 10, location_code: 'A-01-01', picking_task_item_id: 50,
        allocated_qty: 30, sorted_qty: 0, shortage_qty: null, exception_reason: null,
        remaining: 30, uom: 'PCS', resolved: false
      }
    ],
    ...overrides
  }
}

describe('SortationView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    hasPermission.mockImplementation(() => true)
  })

  it('renders the per-order worklist', async () => {
    getSortation.mockResolvedValue({ data: worklist() })
    const wrapper = mount(SortationView, { props: { id: 1 }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('SO-001')
    expect(wrapper.text()).toContain('SO-002')
    expect(wrapper.text()).toContain('20')
    expect(wrapper.text()).toContain('30')
  })

  it('submits a sortation scan with the entered order/sku/lot/quantity', async () => {
    getSortation.mockResolvedValue({ data: worklist() })
    scanSortation.mockResolvedValue({
      data: worklist({
        sortation_status: 'IN_PROGRESS',
        lines: [
          { ...worklist().lines[0], sorted_qty: 20, remaining: 0, resolved: true },
          worklist().lines[1]
        ]
      })
    })
    const wrapper = mount(SortationView, { props: { id: 1 }, global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.scanForm = { orderBarcode: 'SO-001', sku: 'SKU-A', lotNo: 'LOT-X', quantity: 20 }
    await wrapper.vm.submitScan()
    await flushPromises()

    expect(scanSortation).toHaveBeenCalledWith(1, {
      orderBarcode: 'SO-001', sku: 'SKU-A', lotNo: 'LOT-X', quantity: 20
    })
    expect(wrapper.vm.worklist.sortation_status).toBe('IN_PROGRESS')
  })

  it('shows a scan-mismatch error without crashing', async () => {
    getSortation.mockResolvedValue({ data: worklist() })
    scanSortation.mockRejectedValue(new Error('pickingbatch: scanned order/product/lot does not match an allocation for this batch'))
    const wrapper = mount(SortationView, { props: { id: 1 }, global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.scanForm = { orderBarcode: 'WRONG', sku: 'SKU-A', lotNo: 'LOT-X', quantity: 5 }
    await wrapper.vm.submitScan()
    await flushPromises()

    expect(wrapper.vm.scanError).toContain('does not match')
  })

  it('flags a sortation exception on a specific allocation line', async () => {
    getSortation.mockResolvedValue({ data: worklist() })
    flagSortationException.mockResolvedValue({
      data: worklist({
        lines: [
          { ...worklist().lines[0], shortage_qty: 20, exception_reason: 'Damaged', resolved: true, remaining: 0 },
          worklist().lines[1]
        ]
      })
    })
    const wrapper = mount(SortationView, { props: { id: 1 }, global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.toggleExceptionForm(wrapper.vm.worklist.lines[0])
    wrapper.vm.exceptionForm = { shortageQty: 20, reason: 'Damaged' }
    await wrapper.vm.submitException()
    await flushPromises()

    expect(flagSortationException).toHaveBeenCalledWith(1, { allocationId: 1, shortageQty: 20, reason: 'Damaged' })
    expect(wrapper.vm.worklist.lines[0].exception_reason).toBe('Damaged')
  })

  it('shows the completed banner once sortation_status is COMPLETED', async () => {
    getSortation.mockResolvedValue({ data: worklist({ sortation_status: 'COMPLETED' }) })
    const wrapper = mount(SortationView, { props: { id: 1 }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('sortation.completedBanner')
  })

  it('hides mutating sortation controls without picking_batch.sortation', async () => {
    hasPermission.mockImplementation((perm) => perm !== 'picking_batch.sortation')
    getSortation.mockResolvedValue({ data: worklist() })
    const wrapper = mount(SortationView, { props: { id: 1 }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).not.toContain('sortation.scanButton')
  })
})

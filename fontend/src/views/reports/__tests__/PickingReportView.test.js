import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PickingReportView from '../PickingReportView.vue'

vi.mock('../../../services/reportsPicking', () => ({
  listPickingReport: vi.fn(),
  getPickingReportSummary: vi.fn(),
  exportPickingReportCsv: vi.fn(),
  exportPickingReportXlsx: vi.fn(),
  previewPickingReportPdf: vi.fn(),
  downloadPickingReportPdf: vi.fn(),
  listPickingBatchAllocations: vi.fn(),
  exportPickingBatchAllocationsCsv: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn() }))

import {
  listPickingReport,
  getPickingReportSummary,
  listPickingBatchAllocations,
} from '../../../services/reportsPicking'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = { $t: (key) => key }

const row = {
  task_item_id: 1, pick_no: 'PICK-000001', order_no: 'SO-001', picking_mode: 'PICK_BY_ORDER', batch_no: null,
  customer_name: 'Acme Co', warehouse_name: 'Main Warehouse', sku: 'SKU-001', location_code: 'LOC-01',
  requested_qty: 100, picked_qty: 90, short_qty: -10, dispatched_qty: null, uom: 'PCS',
  item_status: 'PARTIALLY_PICKED', assigned_picker: 'Jane Doe',
}

const summary = {
  total_tasks: 4, completed_tasks: 2, total_requested_qty: 400, total_picked_qty: 350,
  total_short_qty: 50, short_pick_rate: null,
}

function listResponse(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 20, total: rows.length, total_pages: 1 } }
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  listPickingReport.mockResolvedValue(listResponse([row]))
  getPickingReportSummary.mockResolvedValue({ success: true, data: summary })
  listPickingBatchAllocations.mockResolvedValue(listResponse([]))
})

describe('PickingReportView', () => {
  it('fetches rows and summary on mount and renders them', async () => {
    const wrapper = mount(PickingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listPickingReport).toHaveBeenCalled()
    expect(getPickingReportSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('PICK-000001')
  })

  it('shows N/A for a null short pick rate rather than a fabricated value', async () => {
    const wrapper = mount(PickingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'shortRate')
    expect(card.value).toBe('reportsReceiving.kpi.notAvailable')
  })

  it('toggling Short Pick Only re-fetches with shortOnly set', async () => {
    const wrapper = mount(PickingReportView, { global: { mocks: globalMocks } })
    await flushPromises()
    listPickingReport.mockClear()

    await wrapper.vm.toggleShortOnly()
    await flushPromises()

    expect(listPickingReport).toHaveBeenCalledWith(
      expect.objectContaining({ shortOnly: true }),
      expect.anything()
    )
  })

  it('switching to the batch-allocation tab fetches that grain separately', async () => {
    const wrapper = mount(PickingReportView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(listPickingBatchAllocations).not.toHaveBeenCalled()

    await wrapper.vm.switchView('batch')
    await flushPromises()

    expect(listPickingBatchAllocations).toHaveBeenCalled()
    expect(wrapper.vm.view).toBe('batch')
  })

  it('shows the load error and an empty table when the list request fails', async () => {
    listPickingReport.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(PickingReportView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.rows).toEqual([])
    expect(wrapper.vm.loadError).toBe('network error')
  })
})

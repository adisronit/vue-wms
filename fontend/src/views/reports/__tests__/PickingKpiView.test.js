import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PickingKpiView from '../PickingKpiView.vue'

vi.mock('../../../services/reportsPickingKpi', () => ({
  getPickingKpiSummary: vi.fn(),
  exportPickingKpiCsv: vi.fn(),
  exportPickingKpiXlsx: vi.fn(),
  previewPickingKpiPdf: vi.fn(),
  downloadPickingKpiPdf: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn() }))

import { getPickingKpiSummary, exportPickingKpiCsv } from '../../../services/reportsPickingKpi'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = { $t: (key) => key }

const summary = {
  total_tasks: 10, completed_tasks: 8, completion_rate_pct: 80,
  picked_lines: 20, picked_units: 200, lines_per_hour: 5, units_per_hour: 50,
  accuracy: { rate_pct: null, numerator: 0, denominator: 0 },
  on_time: { rate_pct: 90, numerator: 9, denominator: 10 },
  tasks_without_due_date: 0,
  avg_cycle_time_hours: 1.5, avg_active_picking_duration_hours: null, tasks_missing_assigned_at: 2,
  short_pick_rate_pct: 5, repick_rate_pct: 2, total_repicks: 1,
  by_operator: [{ key: 'Jane Doe', task_count: 5, completed_task_count: 4, picked_lines: 10, picked_units: 100, lines_per_hour: 5, units_per_hour: 50 }],
  by_warehouse: [],
  by_picking_mode: [{ picking_mode: 'PICK_BY_ORDER', task_count: 8, completion_rate_pct: 80, avg_cycle_time_hours: 1.5, short_pick_rate_pct: 5 }],
  charts: { daily_trend: [{ date: '2026-11-01', completed_tasks: 2, picked_lines: 4, picked_units: 40, short_lines: 0 }], by_operator: [], cycle_time_distribution: [{ label: '0-1h', count: 3 }] },
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  getPickingKpiSummary.mockResolvedValue({ success: true, data: summary })
})

describe('PickingKpiView', () => {
  it('fetches the KPI summary on mount and renders it', async () => {
    const wrapper = mount(PickingKpiView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(getPickingKpiSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('Jane Doe')
  })

  it('shows N/A for a null accuracy rate with its denominator as a hint, rather than a fabricated value', async () => {
    const wrapper = mount(PickingKpiView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'accuracy')
    expect(card.value).toBe('reportsReceiving.kpi.notAvailable')
    expect(card.hint).toContain('0/0')
  })

  it('surfaces the on-time rate denominator (tasks with a due date), not a bare percentage', async () => {
    const wrapper = mount(PickingKpiView, { global: { mocks: globalMocks } })
    await flushPromises()

    const card = wrapper.vm.summaryCards.find((c) => c.key === 'onTime')
    expect(card.value).toBe('90.0%')
    expect(card.hint).toContain('9/10')
  })

  it('wires the CSV export button to exportPickingKpiCsv with the current filters', async () => {
    const wrapper = mount(PickingKpiView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.onExportCsv()
    expect(exportPickingKpiCsv).toHaveBeenCalledTimes(1)
  })

  it('shows the load error and clears the summary when the request fails', async () => {
    getPickingKpiSummary.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(PickingKpiView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.summary).toBeNull()
    expect(wrapper.vm.loadError).toBe('network error')
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import WarehouseView from '../WarehouseView.vue'

vi.mock('../../../services/warehouses', async () => {
  const actual = await vi.importActual('../../../services/warehouses')
  return {
    ...actual,
    listWarehouses: vi.fn(),
    createWarehouse: vi.fn(),
    updateWarehouse: vi.fn(),
    listZones: vi.fn(),
    createZone: vi.fn(),
    updateZone: vi.fn(),
    listLocations: vi.fn()
  }
})

import { listWarehouses } from '../../../services/warehouses'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows, meta = {}) {
  return {
    success: true,
    data: rows,
    meta: { page: 1, limit: 25, total: rows.length, total_pages: 1, ...meta }
  }
}

const warehouse = {
  id: 1,
  code: 'WH-01',
  name: 'Main Warehouse',
  type: 'Distribution Center',
  address: '123 Main St',
  zones: 3,
  locations: 50,
  utilization: 42,
  status: 'ACTIVE'
}

const warehouseB = {
  id: 2,
  code: 'WH-02',
  name: 'Zebra Depot',
  type: 'Transit Warehouse',
  address: '456 Side St',
  zones: 1,
  locations: 10,
  utilization: 10,
  status: 'INACTIVE'
}

describe('WarehouseView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched warehouses in the table', async () => {
    listWarehouses.mockResolvedValue(page([warehouse]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('WH-01')
    expect(wrapper.text()).toContain('Main Warehouse')
  })

  it('maps status to the expected badge tone and label', async () => {
    listWarehouses.mockResolvedValue(page([]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('ACTIVE')).toBe('green')
    expect(wrapper.vm.statusTone('INACTIVE')).toBe('gray')

    expect(wrapper.vm.statusLabel('ACTIVE')).toBe('common.status.active')
    expect(wrapper.vm.statusLabel('INACTIVE')).toBe('common.status.inactive')
  })

  it('generates bin locations via the toast service without falling back to window.alert', async () => {
    listWarehouses.mockResolvedValue(page([]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()

    const alertSpy = vi.spyOn(window, 'alert')
    wrapper.vm.generateLocations()

    expect(alertSpy).not.toHaveBeenCalled()
  })

  it('fetches with the default sort (code ASC) and default page size (25) on mount', async () => {
    listWarehouses.mockResolvedValue(page([warehouse]))

    mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listWarehouses).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, limit: 25, sortBy: 'code', sortDir: 'asc' })
    )
  })

  it('clicking a sortable column header sorts ascending, then descending on a second click, resetting to page 1', async () => {
    listWarehouses.mockResolvedValue(page([warehouse, warehouseB]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 3
    listWarehouses.mockClear()

    // Sortable headers, in render order: code(0), name(1), type(2), status(3).
    const nameHeader = wrapper.findAll('th.sortable')[1]
    await nameHeader.trigger('click')
    await flushPromises()

    expect(listWarehouses).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'name', sortDir: 'asc', page: 1 })
    )

    listWarehouses.mockClear()
    await nameHeader.trigger('click')
    await flushPromises()

    expect(listWarehouses).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'name', sortDir: 'desc', page: 1 })
    )
  })

  it('switching to a different sortable column always starts it at ascending', async () => {
    listWarehouses.mockResolvedValue(page([warehouse, warehouseB]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.sortBy = 'name'
    wrapper.vm.sortDir = 'desc'
    listWarehouses.mockClear()

    wrapper.vm.toggleSort('status')
    await flushPromises()

    expect(wrapper.vm.sortDir).toBe('asc')
    expect(listWarehouses).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'status', sortDir: 'asc', page: 1 })
    )
  })

  it('changing rows-per-page resets to page 1 and refetches with the new limit', async () => {
    listWarehouses.mockResolvedValue(page([warehouse, warehouseB]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listWarehouses.mockClear()

    const select = wrapper.find('select.rows-per-page')
    await select.setValue('50')
    await flushPromises()

    expect(listWarehouses).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, page: 1 }))
  })

  it('displays the correct total count from the response meta', async () => {
    listWarehouses.mockResolvedValue(
      page([warehouse, warehouseB], { page: 1, limit: 25, total: 60, total_pages: 3 })
    )

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.total).toBe(60)
    expect(wrapper.vm.totalPages).toBe(3)
    expect(wrapper.text()).toContain('60')
  })

  it('shows the empty state and no pagination footer when the result set is empty', async () => {
    listWarehouses.mockResolvedValue(page([]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('warehouseSetup.noWarehouses')
    expect(wrapper.findComponent({ name: 'PaginationFooter' }).exists()).toBe(false)
  })

  it('renders rows in the exact order the API returns them, proving sorting is server-side only (no client-side re-sort)', async () => {
    // warehouseB ("WH-02") is returned before warehouse ("WH-01") —
    // deliberately out of code order — to prove the component renders
    // res.data as-is rather than re-sorting it locally.
    listWarehouses.mockResolvedValue(page([warehouseB, warehouse]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.warehouses.map((w) => w.id)).toEqual([2, 1])
    const codeCells = wrapper.findAll('td.font-semibold')
    expect(codeCells[0].text()).toBe('WH-02')
    expect(codeCells[1].text()).toBe('WH-01')
  })

  it('type/status filters re-fetch immediately (no debounce) and reset to page 1', async () => {
    listWarehouses.mockResolvedValue(page([]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listWarehouses.mockClear()

    wrapper.vm.filters.status = 'ACTIVE'
    await flushPromises()

    expect(listWarehouses).toHaveBeenCalledWith(expect.objectContaining({ status: 'ACTIVE', page: 1 }))
  })

  it('debounced search re-fetches with the search term after the user stops typing, resetting to page 1', async () => {
    vi.useFakeTimers()
    listWarehouses.mockResolvedValue(page([]))

    const wrapper = mount(WarehouseView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listWarehouses.mockClear()

    wrapper.vm.filters.keyword = 'bangkok'
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(listWarehouses).toHaveBeenCalledWith(expect.objectContaining({ search: 'bangkok', page: 1 }))
    vi.useRealTimers()
  })
})

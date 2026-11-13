import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import LocationView from '../LocationView.vue'

vi.mock('../../../services/warehouses', async () => {
  const actual = await vi.importActual('../../../services/warehouses')
  return {
    ...actual,
    listWarehouses: vi.fn(),
    listZones: vi.fn(),
    listLocations: vi.fn(),
    createLocation: vi.fn(),
    updateLocation: vi.fn(),
    deleteLocation: vi.fn()
  }
})

vi.mock('../../../services/toast', () => ({
  showToast: vi.fn()
}))

import {
  listWarehouses,
  listZones,
  listLocations,
  createLocation,
  deleteLocation
} from '../../../services/warehouses'
import { showToast } from '../../../services/toast'

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

const warehouse = { id: '1', code: 'WH1', name: 'Main Warehouse' }
const zone = { id: '3', code: 'Z1', name: 'Zone A' }

const location = {
  id: '10',
  zone_id: '3',
  code: 'A-01-01',
  name: 'Bin A1',
  aisle: 'A',
  rack: '01',
  bin: '01',
  type: 'STORAGE',
  capacity: 50,
  status: 'ACTIVE'
}

async function selectWarehouseAndZone(wrapper) {
  wrapper.vm.selectedWarehouseId = warehouse.id
  await flushPromises()
  wrapper.vm.selectedZoneId = zone.id
  await flushPromises()
}

describe('LocationView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listWarehouses.mockResolvedValue(page([warehouse]))
    listZones.mockResolvedValue(page([zone]))
    listLocations.mockResolvedValue(page([location]))
  })

  it('shows a select-warehouse/zone prompt and does not list locations until both are chosen', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('locationMaster.prompts.selectWarehouse')
    expect(listLocations).not.toHaveBeenCalled()
  })

  it('renders fetched locations once a warehouse and zone are selected', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()

    await selectWarehouseAndZone(wrapper)

    expect(listZones).toHaveBeenCalledWith(warehouse.id)
    expect(listLocations).toHaveBeenCalledWith(zone.id, expect.objectContaining({ status: '' }))
    expect(wrapper.text()).toContain('A-01-01')
    expect(wrapper.text()).toContain('Bin A1')
  })

  it('combines aisle/rack/bin into a single position label', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    expect(wrapper.vm.positionLabel(location)).toBe('A / 01 / 01')
    expect(wrapper.vm.positionLabel({})).toBe('—')
  })

  it('delete requires confirmation before calling the API', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.askDelete(location)
    await flushPromises()

    expect(wrapper.vm.locationToDelete).toEqual(location)
    expect(deleteLocation).not.toHaveBeenCalled()
  })

  it('surfaces a HAS_STOCK 409 error from delete without crashing, and keeps the dialog open', async () => {
    const err = new Error('Location still has on-hand inventory and cannot be deleted')
    err.code = 'HAS_STOCK'
    deleteLocation.mockRejectedValue(err)

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.askDelete(location)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(wrapper.vm.deleteError).toBe('Location still has on-hand inventory and cannot be deleted')
    expect(wrapper.vm.locationToDelete).toEqual(location)
  })

  it('shows a success toast (not window.alert) and refetches after a successful delete', async () => {
    deleteLocation.mockResolvedValue({})
    const alertSpy = vi.spyOn(window, 'alert')

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.askDelete(location)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(alertSpy).not.toHaveBeenCalled()
    expect(showToast).toHaveBeenCalledWith('common.messages.deleteSuccess', 'success')
    expect(wrapper.vm.locationToDelete).toBe(null)
  })

  it('create sends the expected payload shape, including the selected zone_id and null capacity when left blank', async () => {
    createLocation.mockResolvedValue({ success: true, data: { ...location, id: '20' } })

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.openLocation(null)
    wrapper.vm.locationForm.code = 'B-02-02'
    wrapper.vm.locationForm.name = 'New Bin'
    wrapper.vm.locationForm.aisle = 'B'
    wrapper.vm.locationForm.rack = '02'
    wrapper.vm.locationForm.bin = '02'
    wrapper.vm.locationForm.type = 'PICKING'
    wrapper.vm.locationForm.capacity = ''
    wrapper.vm.locationForm.status = 'ACTIVE'

    await wrapper.vm.saveLocation()
    await flushPromises()

    expect(createLocation).toHaveBeenCalledWith({
      zone_id: zone.id,
      code: 'B-02-02',
      name: 'New Bin',
      aisle: 'B',
      rack: '02',
      bin: '02',
      type: 'PICKING',
      capacity: null,
      status: 'ACTIVE'
    })
    expect(showToast).toHaveBeenCalledWith('common.messages.saveSuccess', 'success')
  })

  it('does not offer a zone selector on the edit form (zone cannot be changed via update)', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.openLocation(location)
    await flushPromises()

    expect(wrapper.find('select[name="zone"]').exists()).toBe(false)
    expect(wrapper.vm.zoneDisplayLabel).toContain('WH1')
    expect(wrapper.vm.zoneDisplayLabel).toContain('Z1')
  })

  // --- Server-side sorting + pagination (Location Master Management) ---

  it('fetches with the default sort (code asc) and standard page size when a zone is first selected', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    expect(listLocations).toHaveBeenCalledWith(
      zone.id,
      expect.objectContaining({ page: 1, limit: 25, sortBy: 'code', sortDir: 'asc' })
    )
  })

  it('clicking a sortable header toggles sort and resets to page 1', async () => {
    listLocations.mockResolvedValue(page([location], { total_pages: 5 }))

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.page = 3
    listLocations.mockClear()

    wrapper.vm.toggleSort('name')
    await flushPromises()

    expect(wrapper.vm.sortBy).toBe('name')
    expect(wrapper.vm.sortDir).toBe('asc')
    expect(wrapper.vm.page).toBe(1)
    expect(listLocations).toHaveBeenCalledWith(
      zone.id,
      expect.objectContaining({ page: 1, sortBy: 'name', sortDir: 'asc' })
    )
  })

  it('re-clicking the already-active sort column flips direction', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.toggleSort('code')
    await flushPromises()

    expect(wrapper.vm.sortBy).toBe('code')
    expect(wrapper.vm.sortDir).toBe('desc')
  })

  it('changing rows-per-page resets to page 1 and refetches with the new limit', async () => {
    listLocations.mockResolvedValue(page([location], { total_pages: 5 }))

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.page = 4
    listLocations.mockClear()

    wrapper.vm.limit = 50
    await flushPromises()

    expect(wrapper.vm.page).toBe(1)
    expect(listLocations).toHaveBeenCalledWith(zone.id, expect.objectContaining({ page: 1, limit: 50 }))
  })

  it('switching the selected zone resets to page 1', async () => {
    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    wrapper.vm.page = 3
    listLocations.mockClear()

    const zoneB = { id: '4', code: 'Z2', name: 'Zone B' }
    listZones.mockResolvedValue(page([zone, zoneB]))
    wrapper.vm.selectedZoneId = zoneB.id
    await flushPromises()

    expect(wrapper.vm.page).toBe(1)
    expect(listLocations).toHaveBeenCalledWith(zoneB.id, expect.objectContaining({ page: 1 }))
  })

  it('displays the correct total/result-count text from the response meta', async () => {
    listLocations.mockResolvedValue(page([location], { page: 1, limit: 25, total: 60, total_pages: 3 }))

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    expect(wrapper.vm.total).toBe(60)
    expect(wrapper.vm.totalPages).toBe(3)
    expect(wrapper.text()).toContain('60')
  })

  it('shows the empty state and no pagination footer when the result set is empty', async () => {
    listLocations.mockResolvedValue(page([]))

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    expect(wrapper.text()).toContain('locationMaster.empty')
    expect(wrapper.findComponent({ name: 'PaginationFooter' }).exists()).toBe(false)
  })

  it('renders rows in the exact order the API returns them, proving sorting is server-side only (no client-side re-sort)', async () => {
    const locationB = { ...location, id: '11', code: 'Z-99-99', name: 'Zebra Bin' }
    // locationB ("Z-99-99") is returned before location ("A-01-01") —
    // alphabetically backwards — to prove the component renders res.data
    // as-is rather than re-sorting it locally.
    listLocations.mockResolvedValue(page([locationB, location]))

    const wrapper = mount(LocationView, { global: { mocks: globalMocks } })
    await flushPromises()
    await selectWarehouseAndZone(wrapper)

    expect(wrapper.vm.locations.map((l) => l.id)).toEqual(['11', '10'])
    const codeCells = wrapper.findAll('td.font-semibold')
    expect(codeCells[0].text()).toBe('Z-99-99')
    expect(codeCells[1].text()).toBe('A-01-01')
  })
})

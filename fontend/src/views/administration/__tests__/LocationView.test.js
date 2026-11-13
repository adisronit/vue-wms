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

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 500, total: rows.length, total_pages: 1 } }
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
})

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

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
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
})

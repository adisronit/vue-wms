import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import WarehouseLayoutView from '../WarehouseLayoutView.vue'

vi.mock('../../../services/warehouseLayout', () => ({
  getLayout: vi.fn(),
  saveLayout: vi.fn(),
  listLayoutVersions: vi.fn(),
  getLayoutVersion: vi.fn(),
  uploadLayoutBackgroundImage: vi.fn(),
  previewLayoutPdf: vi.fn(),
  downloadLayoutPdf: vi.fn(),
}))
vi.mock('../../../services/warehouses', () => ({ listWarehouses: vi.fn(), listZones: vi.fn() }))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn() }))

import {
  getLayout, saveLayout, listLayoutVersions,
} from '../../../services/warehouseLayout'
import { listWarehouses, listZones } from '../../../services/warehouses'
import { hasPermission } from '../../../services/auth'

const globalMocks = { $t: (key) => key }

const layoutVersion = {
  id: 1, warehouse_id: 1, version_no: 2,
  background_image_id: null, background_image_url: null,
  background_image_width_px: null, background_image_height_px: null,
  scale_px_per_meter: null, remark: null, created_by_name: 'Jane', created_at: '2026-11-01T00:00:00Z',
  objects: [
    { id: 10, object_type: 'RACK', reference_type: null, reference_id: null, reference_label: null,
      x: 10, y: 20, width: 100, height: 50, rotation_degrees: 0, fill_color: '#e2e8f0', stroke_color: '#64748b',
      label: 'Rack A1', z_index: 0 },
  ],
}

beforeEach(() => {
  vi.clearAllMocks()
  listWarehouses.mockResolvedValue({ data: [{ id: 1, name: 'Main Warehouse' }] })
  listZones.mockResolvedValue({ data: [{ id: 1, code: 'Z1', name: 'Zone 1' }] })
  getLayout.mockResolvedValue({ success: true, data: layoutVersion })
  listLayoutVersions.mockResolvedValue({ success: true, data: [] })
  hasPermission.mockReturnValue(false)
})

describe('WarehouseLayoutView', () => {
  it('fetches warehouses on mount', async () => {
    const wrapper = mount(WarehouseLayoutView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listWarehouses).toHaveBeenCalled()
    expect(wrapper.vm.warehouses).toHaveLength(1)
  })

  it('loads the latest layout and its shapes when a warehouse is selected', async () => {
    const wrapper = mount(WarehouseLayoutView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.selectedWarehouseId = 1
    await wrapper.vm.onWarehouseChange()
    await flushPromises()

    expect(getLayout).toHaveBeenCalledWith(1)
    expect(wrapper.vm.objects).toHaveLength(1)
    expect(wrapper.vm.objects[0].label).toBe('Rack A1')
  })

  it('does not show the Edit Layout button without layout.update permission', async () => {
    hasPermission.mockReturnValue(false)
    const wrapper = mount(WarehouseLayoutView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.canEdit).toBe(false)
  })

  it('allows editing when layout.update is granted', async () => {
    hasPermission.mockImplementation((code) => code === 'layout.update')
    const wrapper = mount(WarehouseLayoutView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.canEdit).toBe(true)
  })

  it('adds a new shape with a temporary negative id, not colliding with real ids', async () => {
    const wrapper = mount(WarehouseLayoutView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.selectedWarehouseId = 1
    await wrapper.vm.onWarehouseChange()
    await flushPromises()

    wrapper.vm.addShape('ZONE')
    const added = wrapper.vm.objects[wrapper.vm.objects.length - 1]
    expect(added.object_type).toBe('ZONE')
    expect(added.id).toBeLessThan(0)
    expect(wrapper.vm.selectedObjectId).toBe(added.id)
  })

  it('saves the full object graph via saveLayout', async () => {
    saveLayout.mockResolvedValue({ success: true, data: layoutVersion })
    const wrapper = mount(WarehouseLayoutView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.selectedWarehouseId = 1
    await wrapper.vm.onWarehouseChange()
    await flushPromises()

    await wrapper.vm.onSave()
    expect(saveLayout).toHaveBeenCalledTimes(1)
    const [warehouseId, payload] = saveLayout.mock.calls[0]
    expect(warehouseId).toBe(1)
    expect(payload.objects).toHaveLength(1)
    expect(payload.objects[0].object_type).toBe('RACK')
  })

  it('shows the load error when getLayout fails', async () => {
    getLayout.mockRejectedValueOnce(new Error('network error'))
    const wrapper = mount(WarehouseLayoutView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.selectedWarehouseId = 1
    await wrapper.vm.onWarehouseChange()
    await flushPromises()

    expect(wrapper.vm.loadError).toBe('network error')
  })
})

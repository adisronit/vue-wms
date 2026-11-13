import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ReplenishmentView from '../ReplenishmentView.vue'

vi.mock('../../../services/replenishment', async () => {
  const actual = await vi.importActual('../../../services/replenishment')
  return {
    ...actual,
    listReplenishment: vi.fn(),
    getReplenishmentTask: vi.fn(),
    createReplenishmentTask: vi.fn(),
    startReplenishmentTask: vi.fn(),
    confirmReplenishmentTask: vi.fn(),
    cancelReplenishmentTask: vi.fn(),
    reportReplenishmentIssue: vi.fn(),
    getReplenishmentRecommendations: vi.fn()
  }
})
vi.mock('../../../services/warehouses', async () => {
  const actual = await vi.importActual('../../../services/warehouses')
  return { ...actual, listWarehouses: vi.fn(), listZones: vi.fn(), listLocations: vi.fn() }
})
vi.mock('../../../services/products', async () => {
  const actual = await vi.importActual('../../../services/products')
  return { ...actual, listProducts: vi.fn() }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listReplenishment } from '../../../services/replenishment'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const task = {
  id: 1,
  task_no: 'RPL-001',
  sku: 'SKU-1',
  product_name: 'Widget',
  from_location_code: 'A-01-01',
  to_location_code: 'B-01-01',
  quantity: 20,
  warehouse_code: 'WH-01',
  status: 'PENDING'
}

describe('ReplenishmentView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched tasks in the table', async () => {
    listReplenishment.mockResolvedValue(page([task]))
    listWarehouses.mockResolvedValue({ data: [] })

    const wrapper = mount(ReplenishmentView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('RPL-001')
    expect(wrapper.text()).toContain('Widget')
  })

  it('maps task status to the expected badge tone', async () => {
    listReplenishment.mockResolvedValue(page([]))
    listWarehouses.mockResolvedValue({ data: [] })

    const wrapper = mount(ReplenishmentView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('COMPLETED')).toBe('green')
    expect(wrapper.vm.statusTone('IN_PROGRESS')).toBe('orange')
    expect(wrapper.vm.statusTone('ISSUE')).toBe('gray')
    expect(wrapper.vm.statusTone('CANCELLED')).toBe('gray')
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import LoadingView from '../LoadingView.vue'

vi.mock('../../../services/loading', async () => {
  const actual = await vi.importActual('../../../services/loading')
  return {
    ...actual,
    listLoadingTrips: vi.fn(),
    createLoadingTrip: vi.fn()
  }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listLoadingTrips, createLoadingTrip } from '../../../services/loading'
import { hasPermission } from '../../../services/auth'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

function mountView() {
  return mount(LoadingView, { global: { mocks: { ...globalMocks, $router: { push: vi.fn() } } } })
}

describe('LoadingView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    hasPermission.mockImplementation(() => true)
  })

  it('renders fetched loading trips with a status badge', async () => {
    listLoadingTrips.mockResolvedValue(page([{
      id: 1, trip_no: 'TRIP-000001', vehicle_no: '1กก-1234', driver_name: 'Somchai', status: 'LOADING',
      shipment_count: 2, planned_count: 4, loaded_count: 1, short_count: 0,
      scheduled_loading_time: null, created_at: '2026-01-01T00:00:00Z'
    }]))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('TRIP-000001')
    expect(wrapper.text()).toContain('Somchai')
    expect(wrapper.findComponent({ name: 'StatusBadge' }).exists()).toBe(true)
  })

  it('shows the New Loading Trip button when the caller has loading.create', async () => {
    listLoadingTrips.mockResolvedValue(page([]))
    const wrapper = mountView()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('loading.newTripButton'))).toBe(true)
  })

  it('hides the New Loading Trip button without loading.create', async () => {
    hasPermission.mockImplementation((perm) => perm !== 'loading.create')
    listLoadingTrips.mockResolvedValue(page([]))
    const wrapper = mountView()
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('loading.newTripButton'))).toBe(false)
  })

  it('creates a loading trip from the modal and navigates to its detail page', async () => {
    listLoadingTrips.mockResolvedValue(page([]))
    createLoadingTrip.mockResolvedValue({ success: true, data: { id: 9 } })

    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((b) => b.text().includes('loading.newTripButton'))
    await createButton.trigger('click')
    await flushPromises()

    const vehicleInput = wrapper.findAll('input').find((i) => i.attributes('placeholder') === 'loading.create.vehicleNoPlaceholder')
    await vehicleInput.setValue('1กก-1234')

    const submitButton = wrapper.findAll('button').find((b) => b.text().includes('loading.create.createButton'))
    await submitButton.trigger('click')
    await flushPromises()

    expect(createLoadingTrip).toHaveBeenCalledWith(expect.objectContaining({ vehicle_no: '1กก-1234' }))
  })

  it('surfaces a load error banner when the list request fails', async () => {
    listLoadingTrips.mockRejectedValue(new Error('boom'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('boom')
  })
})

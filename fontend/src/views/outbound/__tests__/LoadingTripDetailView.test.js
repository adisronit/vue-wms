import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import LoadingTripDetailView from '../LoadingTripDetailView.vue'

vi.mock('../../../services/loading', () => ({
  getLoadingTrip: vi.fn(),
  listAssignableShipments: vi.fn(),
  assignShipmentToTrip: vi.fn(),
  scanCarton: vi.fn(),
  shortCarton: vi.fn(),
  confirmLoadingTrip: vi.fn(),
  cancelLoadingTrip: vi.fn(),
  viewLoadingManifest: vi.fn(),
  downloadLoadingManifest: vi.fn()
}))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))
vi.mock('../../../services/documents', async () => {
  const actual = await vi.importActual('../../../services/documents')
  return { ...actual, getDocumentHistory: vi.fn() }
})

import {
  getLoadingTrip, listAssignableShipments, assignShipmentToTrip, scanCarton, shortCarton,
  confirmLoadingTrip, cancelLoadingTrip, viewLoadingManifest, downloadLoadingManifest
} from '../../../services/loading'
import { hasPermission } from '../../../services/auth'
import { getDocumentHistory } from '../../../services/documents'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function baseTrip(overrides = {}) {
  return {
    id: 1, trip_no: 'TRIP-000001', vehicle_no: '1กก-1234', driver_name: 'Somchai', carrier: 'DHL',
    loading_dock: 'DOCK-01', route: 'Bangkok - Chiang Mai', scheduled_loading_time: null,
    actual_loading_start_at: null, actual_loading_end_at: null, vehicle_seal: null, status: 'PLANNED',
    remark: null, cancel_remark: null, planned_count: 1, loaded_count: 0, short_count: 0,
    shipments: [{
      shipment_id: 5, shipment_no: 'SHP-000005', order_no: 'SO-001', customer: 'Acme',
      packing_job_id: 1, job_no: 'PACK-000001', planned_count: 1, loaded_count: 0, short_count: 0
    }],
    cartons: [{
      id: 10, carton_id: 100, lpn: 'CTN-000001', shipment_id: 5, shipment_no: 'SHP-000005',
      status: 'PLANNED', loaded_at: null, loaded_by: null, loaded_by_name: null,
      short_reason: null, short_remark: null, created_at: '2026-01-01T00:00:00Z'
    }],
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    completed_at: null, cancelled_at: null, cancelled_by: null, cancelled_by_name: null,
    ...overrides
  }
}

function mountDetail(props = { id: '1' }) {
  return mount(LoadingTripDetailView, {
    props,
    global: { mocks: { ...globalMocks, $router: { push: vi.fn() } } }
  })
}

describe('LoadingTripDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    hasPermission.mockImplementation(() => true)
  })

  it('renders the trip header and its one planned carton', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip() })
    const wrapper = mountDetail()
    await flushPromises()

    expect(wrapper.text()).toContain('TRIP-000001')
    expect(wrapper.text()).toContain('CTN-000001')
    expect(wrapper.text()).toContain('SHP-000005')
  })

  it('scans a carton as loaded via the scan input', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip() })
    scanCarton.mockResolvedValue({
      data: baseTrip({ status: 'LOADING', loaded_count: 1, cartons: [{ ...baseTrip().cartons[0], status: 'LOADED' }] })
    })

    const wrapper = mountDetail()
    await flushPromises()

    const scanInput = wrapper.findAll('input').find((i) => i.attributes('placeholder') === 'loading.detail.scanPlaceholder')
    await scanInput.setValue('CTN-000001')
    await scanInput.trigger('keyup.enter')
    await flushPromises()

    expect(scanCarton).toHaveBeenCalledWith(1, 'CTN-000001')
    expect(wrapper.text()).toContain('loading.cartonStatus.loaded')
  })

  it('flags a carton short with a reason', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip() })
    shortCarton.mockResolvedValue({
      data: baseTrip({ status: 'LOADING', short_count: 1, cartons: [{ ...baseTrip().cartons[0], status: 'SHORT', short_reason: 'DAMAGED' }] })
    })

    const wrapper = mountDetail()
    await flushPromises()

    const shortToggle = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.shortButton'))
    await shortToggle.trigger('click')
    await flushPromises()

    const lpnInput = wrapper.findAll('input').filter((i) => i.attributes('placeholder') === 'loading.detail.scanPlaceholder').at(-1)
    await lpnInput.setValue('CTN-000001')
    const reasonSelect = wrapper.findAll('select').find((s) => s.html().includes('loading.shortReason.damaged'))
    await reasonSelect.setValue('DAMAGED')

    const submitButton = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.submitShortButton'))
    await submitButton.trigger('click')
    await flushPromises()

    expect(shortCarton).toHaveBeenCalledWith(1, expect.objectContaining({ lpn: 'CTN-000001', reason: 'DAMAGED' }))
  })

  it('enables Confirm Trip only once every carton is resolved', async () => {
    getLoadingTrip.mockResolvedValue({
      data: baseTrip({ status: 'LOADING', loaded_count: 1, cartons: [{ ...baseTrip().cartons[0], status: 'LOADED' }] })
    })
    const wrapper = mountDetail()
    await flushPromises()

    const confirmButton = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.confirmTripButton'))
    expect(confirmButton.attributes('disabled')).toBeUndefined()
  })

  it('disables Confirm Trip while any carton is still planned', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip({ status: 'LOADING' }) })
    const wrapper = mountDetail()
    await flushPromises()

    const confirmButton = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.confirmTripButton'))
    expect(confirmButton.attributes('disabled')).toBeDefined()
  })

  it('confirms the trip', async () => {
    getLoadingTrip.mockResolvedValue({
      data: baseTrip({ status: 'LOADING', loaded_count: 1, cartons: [{ ...baseTrip().cartons[0], status: 'LOADED' }] })
    })
    confirmLoadingTrip.mockResolvedValue({ data: baseTrip({ status: 'COMPLETED' }) })

    const wrapper = mountDetail()
    await flushPromises()

    const confirmButton = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.confirmTripButton'))
    await confirmButton.trigger('click')
    await flushPromises()

    expect(confirmLoadingTrip).toHaveBeenCalledWith(1, {})
  })

  it('assigns a shipment to the trip from the picker', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip({ shipments: [], cartons: [], planned_count: 0 }) })
    listAssignableShipments.mockResolvedValue({
      data: [{ shipment_id: 7, shipment_no: 'SHP-000007', order_no: 'SO-007', customer: 'Acme', packing_job_id: 2, job_no: 'PACK-000002', carton_count: 1 }]
    })
    assignShipmentToTrip.mockResolvedValue({ data: baseTrip() })

    const wrapper = mountDetail()
    await flushPromises()

    const select = wrapper.findAll('select').find((s) => s.html().includes('SHP-000007') === false) // placeholder select before fetch
    // Trigger focus to populate the assignable-shipment list, matching
    // the component's own lazy-fetch-on-focus convention.
    const assignSelect = wrapper.find('select')
    await assignSelect.trigger('focus')
    await flushPromises()

    const populated = wrapper.findAll('select').find((s) => s.html().includes('SHP-000007'))
    await populated.setValue('7')

    const assignButton = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.assignButton'))
    await assignButton.trigger('click')
    await flushPromises()

    expect(assignShipmentToTrip).toHaveBeenCalledWith(1, 7)
    void select
  })

  it('surfaces a load error banner when the get request fails', async () => {
    getLoadingTrip.mockRejectedValue(new Error('boom'))
    const wrapper = mountDetail()
    await flushPromises()

    expect(wrapper.text()).toContain('boom')
  })

  it('cancels the trip', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip() })
    cancelLoadingTrip.mockResolvedValue({ data: baseTrip({ status: 'CANCELLED' }) })

    const wrapper = mountDetail()
    await flushPromises()

    const cancelOpenButton = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.cancelTripButton'))
    await cancelOpenButton.trigger('click')
    await flushPromises()

    const confirmDialogButtons = wrapper.findAll('button').filter((b) => b.text().includes('loading.detail.cancelTripButton'))
    // The ConfirmDialog's own confirm button also reads this same label
    // — click the last one found (the dialog's, rendered after the
    // page-level trigger button).
    await confirmDialogButtons.at(-1).trigger('click')
    await flushPromises()

    expect(cancelLoadingTrip).toHaveBeenCalledWith(1, {})
  })

  // Phase 33D.3 — Loading Manifest PDF (Preview/Download) + Document History.
  it('previews the loading manifest', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip() })
    viewLoadingManifest.mockResolvedValue()
    const wrapper = mountDetail()
    await flushPromises()

    const previewBtn = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.viewManifest'))
    expect(previewBtn).toBeTruthy()
    await previewBtn.trigger('click')
    await flushPromises()

    expect(viewLoadingManifest).toHaveBeenCalledWith(1)
  })

  it('downloads the loading manifest', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip() })
    downloadLoadingManifest.mockResolvedValue()
    const wrapper = mountDetail()
    await flushPromises()

    const downloadBtn = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.downloadManifest'))
    expect(downloadBtn).toBeTruthy()
    await downloadBtn.trigger('click')
    await flushPromises()

    expect(downloadLoadingManifest).toHaveBeenCalledWith(1)
  })

  it('opens the Document History panel and loads this trip\'s LOADING_MANIFEST history', async () => {
    getLoadingTrip.mockResolvedValue({ data: baseTrip() })
    getDocumentHistory.mockResolvedValue({ data: [] })
    const wrapper = mountDetail({ id: '1' })
    await flushPromises()

    const historyBtn = wrapper.findAll('button').find((b) => b.text().includes('loading.detail.historyButton'))
    expect(historyBtn).toBeTruthy()
    await historyBtn.trigger('click')
    await flushPromises()

    expect(getDocumentHistory).toHaveBeenCalledWith('LOADING_MANIFEST', '1')
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ShippingView from '../ShippingView.vue'

vi.mock('../../../services/shipping', async () => {
  const actual = await vi.importActual('../../../services/shipping')
  return {
    ...actual,
    listShipments: vi.fn(),
    getShipment: vi.fn(),
    cancelShipment: vi.fn(),
    listShippableSources: vi.fn(),
    createShipment: vi.fn(),
    viewDeliveryNote: vi.fn(),
    downloadDeliveryNote: vi.fn()
  }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/documents', async () => {
  const actual = await vi.importActual('../../../services/documents')
  return { ...actual, getDocumentHistory: vi.fn() }
})

import {
  listShipments, getShipment, cancelShipment, listShippableSources, createShipment,
  viewDeliveryNote, downloadDeliveryNote
} from '../../../services/shipping'
import { getDocumentHistory } from '../../../services/documents'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key), $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const shipment = {
  id: 1, shipment_no: 'SHP-001', order_no: 'ORD-001', customer: 'Acme',
  carrier: 'DHL', tracking_no: 'TRK1', packages: 2, weight: 5, ship_date: '2026-01-01', status: 'READY'
}

// Phase 33D.1 — a source row is now a completed packing job, not a raw
// picking task (packing_job_id/job_no replace picking_task_id/pick_no).
const packingJobSource = {
  packing_job_id: 7, job_no: 'PACK-000007', picking_task_id: 3, pick_no: 'PICK-000003',
  order_no: 'SO-001', customer: 'Acme', item_count: 2, is_batch: false, batch_no: null
}

describe('ShippingView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched shipments with a status badge', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()
    expect(wrapper.text()).toContain('SHP-001')
    expect(wrapper.findComponent({ name: 'StatusBadge' }).exists()).toBe(true)
  })

  it('requires an explicit confirmation step before cancelling a shipment', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    getShipment.mockResolvedValue({ data: { ...shipment, items: [] } })
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping({ id: 1 })
    await flushPromises()

    expect(wrapper.vm.showCancelConfirm).toBe(false)
    expect(cancelShipment).not.toHaveBeenCalled()

    await wrapper.vm.cancelSelectedShipment()
    expect(cancelShipment).toHaveBeenCalledTimes(1)
  })

  // Phase 33D.1: the "+ New Shipment" drawer now selects a completed
  // packing job, not a raw picking task.
  it('loads packing-job sources when opening the new-shipment drawer', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    listShippableSources.mockResolvedValue({ success: true, data: [packingJobSource] })
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping(null)
    await flushPromises()

    expect(listShippableSources).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.sources).toEqual([packingJobSource])
    expect(wrapper.vm.isNewShipment).toBe(true)
  })

  it('creates a shipment with packing_job_id, not picking_task_id', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    listShippableSources.mockResolvedValue({ success: true, data: [packingJobSource] })
    createShipment.mockResolvedValue({ success: true, data: { id: 99 } })
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping(null)
    await flushPromises()

    wrapper.vm.form.packingJobId = packingJobSource.packing_job_id
    await wrapper.vm.saveNewShipment()

    expect(createShipment).toHaveBeenCalledTimes(1)
    const payload = createShipment.mock.calls[0][0]
    expect(payload.packing_job_id).toBe(packingJobSource.packing_job_id)
    expect(payload.picking_task_id).toBeUndefined()
  })

  it('requires a source packing job before submitting a new shipment', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    listShippableSources.mockResolvedValue({ success: true, data: [] })
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping(null)
    await flushPromises()

    await wrapper.vm.saveNewShipment()

    expect(createShipment).not.toHaveBeenCalled()
    expect(wrapper.vm.drawerError).toBeTruthy()
  })

  // Phase 33D.3 — Delivery Note PDF (Preview/Download) + Document History.
  it('previews the delivery note for an existing shipment', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    getShipment.mockResolvedValue({ data: { ...shipment, items: [] } })
    viewDeliveryNote.mockResolvedValue()
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping({ id: 1 })
    await flushPromises()

    const previewBtn = wrapper.findAll('button').find((b) => b.text().includes('shipping.drawer.viewDeliveryNote'))
    expect(previewBtn).toBeTruthy()
    await previewBtn.trigger('click')
    await flushPromises()

    expect(viewDeliveryNote).toHaveBeenCalledWith(1)
  })

  it('downloads the delivery note for an existing shipment', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    getShipment.mockResolvedValue({ data: { ...shipment, items: [] } })
    downloadDeliveryNote.mockResolvedValue()
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping({ id: 1 })
    await flushPromises()

    const downloadBtn = wrapper.findAll('button').find((b) => b.text().includes('shipping.drawer.downloadDeliveryNote'))
    expect(downloadBtn).toBeTruthy()
    await downloadBtn.trigger('click')
    await flushPromises()

    expect(downloadDeliveryNote).toHaveBeenCalledWith(1)
  })

  it('opens the Document History panel and loads this shipment\'s DELIVERY_NOTE history', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    getShipment.mockResolvedValue({ data: { ...shipment, items: [] } })
    getDocumentHistory.mockResolvedValue({ data: [] })
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping({ id: 1 })
    await flushPromises()

    const historyBtn = wrapper.findAll('button').find((b) => b.text().includes('shipping.drawer.historyButton'))
    expect(historyBtn).toBeTruthy()
    await historyBtn.trigger('click')
    await flushPromises()

    expect(getDocumentHistory).toHaveBeenCalledWith('DELIVERY_NOTE', 1)
  })

  it('does not show the delivery note / history buttons for a new (unsaved) shipment', async () => {
    listShipments.mockResolvedValue(page([shipment]))
    listShippableSources.mockResolvedValue({ success: true, data: [] })
    const wrapper = mount(ShippingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openShipping(null)
    await flushPromises()

    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons.some((t) => t.includes('shipping.drawer.viewDeliveryNote'))).toBe(false)
    expect(buttons.some((t) => t.includes('shipping.drawer.historyButton'))).toBe(false)
  })
})

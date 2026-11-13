import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ShippingView from '../ShippingView.vue'

vi.mock('../../../services/shipping', async () => {
  const actual = await vi.importActual('../../../services/shipping')
  return { ...actual, listShipments: vi.fn(), getShipment: vi.fn(), cancelShipment: vi.fn() }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listShipments, getShipment, cancelShipment } from '../../../services/shipping'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const shipment = {
  id: 1, shipment_no: 'SHP-001', order_no: 'ORD-001', customer: 'Acme',
  carrier: 'DHL', tracking_no: 'TRK1', packages: 2, weight: 5, ship_date: '2026-01-01', status: 'READY'
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
})

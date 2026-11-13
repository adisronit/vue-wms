import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'

vi.mock('../../../services/dashboard', () => ({ getDashboardSummary: vi.fn() }))

import { getDashboardSummary } from '../../../services/dashboard'

const globalMocks = { $t: (key) => key, $d: (v) => String(v), $n: (v) => String(v), $router: { push: vi.fn() } }

const summary = {
  pending_receiving_count: 3,
  available_stock_qty: 1000,
  pending_picking_count: 2,
  low_stock_count: 1,
  recent_inbound: [
    { id: 1, receipt_no: 'RCPT-001', vendor: 'Acme', status: 'PARTIALLY_RECEIVED', receiving_date: '2026-01-01T00:00:00Z' }
  ]
}

describe('DashboardView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the fetched summary via StatusBadge without throwing', async () => {
    getDashboardSummary.mockResolvedValue({ data: summary })
    const wrapper = mount(DashboardView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('RCPT-001')
    expect(wrapper.findComponent({ name: 'StatusBadge' }).exists()).toBe(true)
  })

  it('maps PARTIALLY_RECEIVED/COMPLETED/other statuses to the expected tones', () => {
    const wrapper = mount(DashboardView, { global: { mocks: globalMocks } })
    expect(wrapper.vm.statusTone('PARTIALLY_RECEIVED')).toBe('orange')
    expect(wrapper.vm.statusTone('COMPLETED')).toBe('green')
    expect(wrapper.vm.statusTone('WAITING')).toBe('gray')
  })

  it('renders quick-action labels with no emoji glyphs', async () => {
    getDashboardSummary.mockResolvedValue({ data: summary })
    const wrapper = mount(DashboardView, { global: { mocks: globalMocks } })
    await flushPromises()

    const buttons = wrapper.findAll('.quick-button').map((b) => b.text())
    const emojiPattern = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]/u
    expect(buttons.some((t) => emojiPattern.test(t))).toBe(false)
  })
})

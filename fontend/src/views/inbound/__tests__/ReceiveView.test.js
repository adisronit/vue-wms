import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ReceiveView from '../ReceiveView.vue'

vi.mock('../../../services/receiving', async () => {
  const actual = await vi.importActual('../../../services/receiving')
  return {
    ...actual,
    listReceipts: vi.fn(),
    cancelReceipt: vi.fn(),
    confirmReceipt: vi.fn(),
    getReceipt: vi.fn()
  }
})
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))

import { listReceipts, cancelReceipt, getReceipt } from '../../../services/receiving'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v),
  $router: { push: vi.fn() }
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const receipt = {
  id: 1,
  receipt_no: 'RCPT-001',
  reference_no: 'ASN-1',
  vendor: 'Acme',
  warehouse_code: 'WH-01',
  receiving_date: '2026-01-01T00:00:00Z',
  item_count: 2,
  progress_pct: 50,
  status: 'WAITING'
}

describe('ReceiveView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows exactly one summary card for partially-received documents (no duplicate card)', async () => {
    listReceipts.mockResolvedValue(page([receipt]))
    const wrapper = mount(ReceiveView, { global: { mocks: globalMocks } })
    await flushPromises()

    const labels = wrapper.findAll('.summary-label').map((el) => el.text())
    const occurrences = labels.filter((l) => l === 'receiving.summary.partialReceived').length
    expect(occurrences).toBe(1)
    expect(labels).not.toContain('receiving.summary.receiving')
  })

  it('offers exactly one PARTIALLY_RECEIVED filter option (no duplicate)', async () => {
    listReceipts.mockResolvedValue(page([]))
    const wrapper = mount(ReceiveView, { global: { mocks: globalMocks } })
    await flushPromises()

    const options = wrapper.findAll('option').filter((o) => o.attributes('value') === 'PARTIALLY_RECEIVED')
    expect(options.length).toBe(1)
  })

  it('renders the fetched receipts in the table', async () => {
    listReceipts.mockResolvedValue(page([receipt]))
    const wrapper = mount(ReceiveView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('RCPT-001')
    expect(wrapper.text()).toContain('Acme')
  })

  it('requires an explicit confirmation step before cancelling a receipt', async () => {
    listReceipts.mockResolvedValue(page([receipt]))
    getReceipt.mockResolvedValue({ data: { items: [], warehouse_id: null, cancelled_at: null, cancelled_by_name: null, cancel_remark: null } })
    const wrapper = mount(ReceiveView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openReceiving({ ...receipt, id: 1, status: 'WAITING' })
    await flushPromises()

    expect(wrapper.vm.showCancelConfirm).toBe(false)
    expect(cancelReceipt).not.toHaveBeenCalled()
  })
})

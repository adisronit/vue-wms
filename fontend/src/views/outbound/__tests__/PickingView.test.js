import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import PickingView from '../PickingView.vue'

// Phase 33B: operator assignment, scan-mismatch rejection, structured
// short-pick reason, and re-pick coverage for PickingView.vue's
// enhanced drawer — mirrors ShippingView.test.js's own mocking shape
// (mock the service module, stub auth/toast, mount + flushPromises).
vi.mock('../../../services/picking', async () => {
  const actual = await vi.importActual('../../../services/picking')
  return {
    ...actual,
    listPicking: vi.fn(),
    getPickingTask: vi.fn(),
    createPickingTask: vi.fn(),
    confirmPickingTask: vi.fn(),
    cancelPickingTask: vi.fn(),
    exportPicking: vi.fn(),
    assignPicker: vi.fn(),
    scanConfirmPickingItem: vi.fn(),
    repickPickingItem: vi.fn(),
    viewPickList: vi.fn(),
    viewConfirmationReport: vi.fn(),
    newIdempotencyKey: () => 'test-key'
  }
})
vi.mock('../../../services/documents', async () => {
  const actual = await vi.importActual('../../../services/documents')
  return { ...actual, getDocumentHistory: vi.fn() }
})
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn().mockResolvedValue({ data: [] }),
  listZones: vi.fn().mockResolvedValue({ data: [] }),
  listLocations: vi.fn().mockResolvedValue({ data: [] })
}))
vi.mock('../../../services/products', () => ({ listProducts: vi.fn().mockResolvedValue({ data: [] }) }))
vi.mock('../../../services/users', () => ({ listUsers: vi.fn() }))
vi.mock('../../../services/auth', () => ({ hasPermission: vi.fn(() => true) }))
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import {
  listPicking, getPickingTask, confirmPickingTask, assignPicker,
  scanConfirmPickingItem, repickPickingItem, viewConfirmationReport
} from '../../../services/picking'
import { getDocumentHistory } from '../../../services/documents'
import { listUsers } from '../../../services/users'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key), $d: (v) => String(v) }

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const taskItem = {
  id: 10, product_id: 1, sku: 'SKU-1', product_name: 'Widget',
  location_id: 1, location_code: 'A-01-01', lot_no: 'LOT-1',
  ordered_qty: 20, picked_qty: 0, uom: 'PCS', status: 'PENDING',
  remark: null, short_qty: null, short_reason: null, repick_count: 0
}

const task = {
  id: 1, pick_no: 'PICK-000001', order_no: 'SO-001', customer: 'Acme',
  warehouse_id: 1, warehouse_code: 'WH-01', status: 'PENDING', priority: 'NORMAL',
  due_date: null, assigned_picker_id: null, assigned_picker_name: null, assigned_at: null,
  items: [{ ...taskItem }]
}

describe('PickingView (Phase 33B)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listPicking.mockResolvedValue(page([]))
  })

  it('assigns a picker to a task', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })
    listUsers.mockResolvedValue({ data: [{ id: 5, username: 'jdoe', full_name: 'Jane Doe' }] })
    assignPicker.mockResolvedValue({ data: { ...task, assigned_picker_id: 5, assigned_picker_name: 'Jane Doe' } })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    wrapper.vm.assignPickerId = 5
    await wrapper.vm.assignPickerToTask()
    await flushPromises()

    expect(assignPicker).toHaveBeenCalledWith(1, 5)
    expect(wrapper.vm.selectedPicking.assigned_picker_name).toBe('Jane Doe')
  })

  it('rejects assigning without selecting a picker first', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })
    listUsers.mockResolvedValue({ data: [] })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    wrapper.vm.assignPickerId = ''
    await wrapper.vm.assignPickerToTask()

    expect(assignPicker).not.toHaveBeenCalled()
    expect(wrapper.vm.drawerError).toBeTruthy()
  })

  it('records a scan match and unlocks the confirm action', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })
    scanConfirmPickingItem.mockResolvedValue({ data: { matched: true, detail: '' } })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    expect(wrapper.vm.displayItems[0].scanVerified).toBe(false)

    wrapper.vm.scanInputs[10] = 'A-01-01|SKU-1|LOT-1'
    await wrapper.vm.scanLine(wrapper.vm.displayItems[0])
    await flushPromises()

    expect(scanConfirmPickingItem).toHaveBeenCalledWith(1, 10, {
      location_code: 'A-01-01', sku: 'SKU-1', lot_no: 'LOT-1'
    })
    expect(wrapper.vm.displayItems[0].scanVerified).toBe(true)
  })

  it('surfaces a scan mismatch without marking the line verified', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })
    scanConfirmPickingItem.mockResolvedValue({ data: { matched: false, detail: 'location mismatch' } })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    wrapper.vm.scanInputs[10] = 'WRONG-LOC|SKU-1|LOT-1'
    await wrapper.vm.scanLine(wrapper.vm.displayItems[0])
    await flushPromises()

    expect(wrapper.vm.displayItems[0].scanVerified).toBe(false)
    expect(wrapper.vm.drawerError).toBeTruthy()
  })

  it('blocks Confirm Pick until the line has been scan-verified', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    const product = wrapper.vm.displayItems[0]
    product.inputQty = 5
    await wrapper.vm.confirmPick(product)

    expect(confirmPickingTask).not.toHaveBeenCalled()
    expect(wrapper.vm.drawerError).toBeTruthy()
  })

  it('requires a structured short_reason (not free-text remark) for Short Pick', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    const product = wrapper.vm.displayItems[0]
    product.remark = 'out of stock'
    product.shortReason = ''
    await wrapper.vm.shortPick(product)
    expect(confirmPickingTask).not.toHaveBeenCalled()
    expect(wrapper.vm.drawerError).toBeTruthy()

    confirmPickingTask.mockResolvedValue({ data: { ...task, status: 'SHORT_PICK' } })
    product.shortReason = 'OUT_OF_STOCK'
    await wrapper.vm.shortPick(product)
    await flushPromises()

    expect(confirmPickingTask).toHaveBeenCalledWith(1, {
      idempotency_key: 'test-key',
      items: [{ item_id: 10, action: 'SHORT', short_reason: 'OUT_OF_STOCK', remark: 'out of stock' }]
    })
  })

  it('re-picks a SHORT line', async () => {
    const shortTask = {
      ...task, status: 'SHORT_PICK',
      items: [{ ...taskItem, status: 'SHORT', short_qty: 20, short_reason: 'OUT_OF_STOCK' }]
    }
    getPickingTask.mockResolvedValue({ data: shortTask })
    repickPickingItem.mockResolvedValue({ data: { ...task, status: 'IN_PROGRESS' } })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    await wrapper.vm.repickLine(wrapper.vm.displayItems[0])
    await flushPromises()

    expect(repickPickingItem).toHaveBeenCalledWith(1, 10)
    expect(wrapper.vm.selectedPicking.status).toBe('IN_PROGRESS')
  })

  // Phase 33C.3 — Picking Confirmation Report + Document History.
  it('prints the Confirmation Report for the selected task', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })
    viewConfirmationReport.mockResolvedValue()

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    await wrapper.vm.printConfirmationReport()

    expect(viewConfirmationReport).toHaveBeenCalledWith(1)
  })

  it('opens the Document History panel for the selected task\'s Pick List', async () => {
    getPickingTask.mockResolvedValue({ data: { ...task } })
    getDocumentHistory.mockResolvedValue({ data: [{ id: 1, document_type: 'PICK_LIST', document_no: 'PICK-000001', rendered_at: '2026-01-01T00:00:00Z', is_draft: false, rendered_by_name: 'Jane' }] })

    const wrapper = mount(PickingView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openPicking({ id: 1 })
    await flushPromises()

    const historyBtn = wrapper.findAll('button').find((b) => b.text().includes('picking.documents.historyButton'))
    expect(historyBtn).toBeTruthy()
    await historyBtn.trigger('click')
    await flushPromises()

    expect(getDocumentHistory).toHaveBeenCalledWith('PICK_LIST', 1)
  })
})

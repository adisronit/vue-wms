import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SupplierView from '../SupplierView.vue'

vi.mock('../../../services/suppliers', async () => {
  const actual = await vi.importActual('../../../services/suppliers')
  return {
    ...actual,
    listSuppliers: vi.fn(),
    createSupplier: vi.fn(),
    updateSupplier: vi.fn(),
    deleteSupplier: vi.fn()
  }
})

vi.mock('../../../services/toast', () => ({
  showToast: vi.fn()
}))

import { listSuppliers, deleteSupplier } from '../../../services/suppliers'
import { showToast } from '../../../services/toast'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const supplier = {
  id: '1',
  code: 'SUP-001',
  company_name: 'Global Supply Co',
  tax_id: '9876543210',
  contact_person: 'John Smith',
  contact_phone: '555-0200',
  address: '2 Industrial Way',
  email: 'john@globalsupply.test',
  phone: '555-0201',
  status: 'ACTIVE'
}

describe('SupplierView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched suppliers in the table', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('SUP-001')
    expect(wrapper.text()).toContain('Global Supply Co')
    expect(wrapper.text()).toContain('John Smith')
  })

  it('maps status to the expected badge tone and label', async () => {
    listSuppliers.mockResolvedValue(page([]))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('ACTIVE')).toBe('green')
    expect(wrapper.vm.statusTone('INACTIVE')).toBe('gray')
    expect(wrapper.vm.statusLabel('INACTIVE')).toBe('common.status.inactive')
  })

  it('opens the drawer pre-filled with the selected supplier, mapping snake_case fields to the form', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openSupplier(supplier)

    expect(wrapper.vm.supplierForm.companyName).toBe('Global Supply Co')
    expect(wrapper.vm.supplierForm.contactPerson).toBe('John Smith')
    expect(wrapper.vm.supplierForm.taxId).toBe('9876543210')
  })

  it('deletes via the toast service (not window.alert) and refetches the list', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))
    deleteSupplier.mockResolvedValue({})

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    const alertSpy = vi.spyOn(window, 'alert')
    wrapper.vm.askDelete(supplier)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(alertSpy).not.toHaveBeenCalled()
    expect(deleteSupplier).toHaveBeenCalledWith('1')
    expect(showToast).toHaveBeenCalledWith('common.messages.deleteSuccess', 'success')
    expect(wrapper.vm.supplierToDelete).toBe(null)
  })

  it('surfaces a duplicate-code save failure inline', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))
    const err = new Error('Supplier code already exists')
    err.code = 'DUPLICATE_CODE'
    const { createSupplier } = await import('../../../services/suppliers')
    createSupplier.mockRejectedValue(err)

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openSupplier(null)
    wrapper.vm.supplierForm.code = 'SUP-001'
    wrapper.vm.supplierForm.companyName = 'Duplicate Co'
    await wrapper.vm.saveSupplier()
    await flushPromises()

    expect(wrapper.vm.formError).toBe('Supplier code already exists')
  })
})

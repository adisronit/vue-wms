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

function page(rows, meta = {}) {
  return {
    success: true,
    data: rows,
    meta: { page: 1, limit: 25, total: rows.length, total_pages: 1, ...meta }
  }
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

  it('fetches with the default sort (code ASC) and default page size (25) on mount', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))

    mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listSuppliers).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, limit: 25, sortBy: 'code', sortDir: 'asc' })
    )
  })

  it('clicking a sortable column header sorts ascending, then descending on a second click, resetting to page 1', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 3
    listSuppliers.mockClear()

    const companyNameHeader = wrapper.findAll('th.sortable')[1]
    await companyNameHeader.trigger('click')
    await flushPromises()

    expect(listSuppliers).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'companyName', sortDir: 'asc', page: 1 })
    )

    listSuppliers.mockClear()
    await companyNameHeader.trigger('click')
    await flushPromises()

    expect(listSuppliers).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'companyName', sortDir: 'desc', page: 1 })
    )
  })

  it('switching to a different sortable column always starts it at ascending', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.sortBy = 'companyName'
    wrapper.vm.sortDir = 'desc'
    listSuppliers.mockClear()

    wrapper.vm.toggleSort('status')
    await flushPromises()

    expect(wrapper.vm.sortDir).toBe('asc')
    expect(listSuppliers).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'status', sortDir: 'asc', page: 1 })
    )
  })

  it('changing rows-per-page resets to page 1 and refetches with the new limit', async () => {
    listSuppliers.mockResolvedValue(page([supplier]))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listSuppliers.mockClear()

    const select = wrapper.find('select.rows-per-page')
    await select.setValue('50')
    await flushPromises()

    expect(listSuppliers).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, page: 1 }))
  })

  it('displays the correct total/page count from the response meta, and pagination next/previous respect the boundary disabled state', async () => {
    listSuppliers.mockResolvedValue(page([supplier], { page: 1, limit: 25, total: 60, total_pages: 3 }))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('60')
    expect(wrapper.vm.total).toBe(60)
    expect(wrapper.vm.totalPages).toBe(3)

    const [prevButton, nextButton] = wrapper.findAll('button').filter(
      (b) => b.text() === 'common.pagination.previous' || b.text() === 'common.pagination.next'
    )
    expect(prevButton.attributes('disabled')).toBeDefined()
    expect(nextButton.attributes('disabled')).toBeUndefined()

    listSuppliers.mockClear()
    await nextButton.trigger('click')
    await flushPromises()

    expect(listSuppliers).toHaveBeenCalledWith(expect.objectContaining({ page: 2 }))
  })

  it('shows the empty-results state and no pagination footer when the list is empty', async () => {
    listSuppliers.mockResolvedValue(page([], { total: 0, total_pages: 1 }))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('suppliers.empty')
    expect(wrapper.find('select.rows-per-page').exists()).toBe(false)
  })

  it('renders rows in the exact server-supplied order without any client-side re-sort', async () => {
    // Deliberately out of alphabetical/code order — if the component
    // ever re-sorted client-side, this would come back reordered.
    const zebra = { ...supplier, id: '2', code: 'SUP-ZZZ', company_name: 'Zebra Co' }
    const apple = { ...supplier, id: '3', code: 'SUP-AAA', company_name: 'Apple Co' }
    listSuppliers.mockResolvedValue(page([zebra, apple]))

    const wrapper = mount(SupplierView, { global: { mocks: globalMocks } })
    await flushPromises()

    const codeCells = wrapper.findAll('td.font-semibold').map((td) => td.text())
    expect(codeCells).toEqual(['SUP-ZZZ', 'SUP-AAA'])
  })
})

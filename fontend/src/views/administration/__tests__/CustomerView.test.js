import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CustomerView from '../CustomerView.vue'

vi.mock('../../../services/customers', async () => {
  const actual = await vi.importActual('../../../services/customers')
  return {
    ...actual,
    listCustomers: vi.fn(),
    createCustomer: vi.fn(),
    updateCustomer: vi.fn(),
    deleteCustomer: vi.fn()
  }
})

vi.mock('../../../services/toast', () => ({
  showToast: vi.fn()
}))

import { listCustomers, deleteCustomer } from '../../../services/customers'
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

const customer = {
  id: '1',
  code: 'CUST-001',
  company_name: 'Acme Corp',
  tax_id: '1234567890',
  contact_person: 'Jane Doe',
  contact_phone: '555-0100',
  address: '1 Market St',
  email: 'jane@acme.test',
  phone: '555-0101',
  status: 'ACTIVE'
}

const customerB = {
  id: '2',
  code: 'CUST-002',
  company_name: 'Zebra Logistics',
  tax_id: '9876543210',
  contact_person: 'John Roe',
  contact_phone: '555-0200',
  address: '2 Market St',
  email: 'john@zebra.test',
  phone: '555-0201',
  status: 'INACTIVE'
}

describe('CustomerView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched customers in the table', async () => {
    listCustomers.mockResolvedValue(page([customer]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('CUST-001')
    expect(wrapper.text()).toContain('Acme Corp')
    expect(wrapper.text()).toContain('Jane Doe')
  })

  it('maps status to the expected badge tone and label', async () => {
    listCustomers.mockResolvedValue(page([]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('ACTIVE')).toBe('green')
    expect(wrapper.vm.statusTone('INACTIVE')).toBe('gray')
    expect(wrapper.vm.statusLabel('INACTIVE')).toBe('common.status.inactive')
  })

  it('opens the drawer pre-filled with the selected customer, mapping snake_case fields to the form', async () => {
    listCustomers.mockResolvedValue(page([customer]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openCustomer(customer)

    expect(wrapper.vm.customerForm.companyName).toBe('Acme Corp')
    expect(wrapper.vm.customerForm.contactPerson).toBe('Jane Doe')
    expect(wrapper.vm.customerForm.taxId).toBe('1234567890')
  })

  it('deletes via the toast service (not window.alert) and refetches the list', async () => {
    listCustomers.mockResolvedValue(page([customer]))
    deleteCustomer.mockResolvedValue({})

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    const alertSpy = vi.spyOn(window, 'alert')
    wrapper.vm.askDelete(customer)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(alertSpy).not.toHaveBeenCalled()
    expect(deleteCustomer).toHaveBeenCalledWith('1')
    expect(showToast).toHaveBeenCalledWith('common.messages.deleteSuccess', 'success')
    expect(wrapper.vm.customerToDelete).toBe(null)
  })

  it('surfaces a delete failure message inline without clearing the pending dialog', async () => {
    listCustomers.mockResolvedValue(page([customer]))
    deleteCustomer.mockRejectedValue(new Error('Request failed with status 404'))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.askDelete(customer)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(wrapper.vm.deleteError).toBe('Request failed with status 404')
    expect(wrapper.vm.customerToDelete).toEqual(customer)
  })

  it('fetches with the default sort (code ASC) and default page size (25) on mount', async () => {
    listCustomers.mockResolvedValue(page([customer]))

    mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listCustomers).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, limit: 25, sortBy: 'code', sortDir: 'asc' })
    )
  })

  it('clicking a sortable column header sorts ascending, then descending on a second click, resetting to page 1', async () => {
    listCustomers.mockResolvedValue(page([customer, customerB]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 3
    listCustomers.mockClear()

    const companyNameHeader = wrapper.findAll('th.sortable')[1]
    await companyNameHeader.trigger('click')
    await flushPromises()

    expect(listCustomers).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'companyName', sortDir: 'asc', page: 1 })
    )

    listCustomers.mockClear()
    await companyNameHeader.trigger('click')
    await flushPromises()

    expect(listCustomers).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'companyName', sortDir: 'desc', page: 1 })
    )
  })

  it('switching to a different sortable column always starts it at ascending', async () => {
    listCustomers.mockResolvedValue(page([customer, customerB]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.sortBy = 'companyName'
    wrapper.vm.sortDir = 'desc'
    listCustomers.mockClear()

    wrapper.vm.toggleSort('status')
    await flushPromises()

    expect(wrapper.vm.sortDir).toBe('asc')
    expect(listCustomers).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'status', sortDir: 'asc', page: 1 })
    )
  })

  it('changing rows-per-page resets to page 1 and refetches with the new limit', async () => {
    listCustomers.mockResolvedValue(page([customer, customerB]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listCustomers.mockClear()

    const select = wrapper.find('select.rows-per-page')
    await select.setValue('50')
    await flushPromises()

    expect(listCustomers).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, page: 1 }))
  })

  it('displays the correct total/result-count text from the response meta', async () => {
    listCustomers.mockResolvedValue(
      page([customer, customerB], { page: 1, limit: 25, total: 60, total_pages: 3 })
    )

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.total).toBe(60)
    expect(wrapper.vm.totalPages).toBe(3)
    expect(wrapper.text()).toContain('60')
  })

  it('shows the empty state and no pagination footer when the result set is empty', async () => {
    listCustomers.mockResolvedValue(page([]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('customers.empty')
    expect(wrapper.findComponent({ name: 'PaginationFooter' }).exists()).toBe(false)
  })

  it('renders rows in the exact order the API returns them, proving sorting is server-side only (no client-side re-sort)', async () => {
    // customerB ("Zebra Logistics") is returned before customer
    // ("Acme Corp") — alphabetically backwards — to prove the component
    // renders res.data as-is rather than re-sorting it locally.
    listCustomers.mockResolvedValue(page([customerB, customer]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.customers.map((c) => c.id)).toEqual(['2', '1'])
    const codeCells = wrapper.findAll('td.font-semibold')
    expect(codeCells[0].text()).toBe('CUST-002')
    expect(codeCells[1].text()).toBe('CUST-001')
  })

  it('status filter re-fetches immediately (no debounce) and resets to page 1', async () => {
    listCustomers.mockResolvedValue(page([]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listCustomers.mockClear()

    wrapper.vm.filters.status = 'ACTIVE'
    await flushPromises()

    expect(listCustomers).toHaveBeenCalledWith(expect.objectContaining({ status: 'ACTIVE', page: 1 }))
  })

  it('debounced search re-fetches with the search term after the user stops typing, resetting to page 1', async () => {
    vi.useFakeTimers()
    listCustomers.mockResolvedValue(page([]))

    const wrapper = mount(CustomerView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listCustomers.mockClear()

    wrapper.vm.filters.keyword = 'acme'
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(listCustomers).toHaveBeenCalledWith(expect.objectContaining({ search: 'acme', page: 1 }))
    vi.useRealTimers()
  })
})

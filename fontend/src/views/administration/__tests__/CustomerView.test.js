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

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
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
})

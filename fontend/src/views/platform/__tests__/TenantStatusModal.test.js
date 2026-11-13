import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TenantStatusModal from '../TenantStatusModal.vue'

vi.mock('../../../services/platformTenants', async () => {
  const actual = await vi.importActual('../../../services/platformTenants')
  return { ...actual, updateTenantStatus: vi.fn() }
})

import { updateTenantStatus } from '../../../services/platformTenants'

const globalMocks = { $t: (key) => key }

function baseTenant(status = 'ACTIVE') {
  return { id: '7', slug: 'acme', name: 'Acme Co', status }
}

describe('TenantStatusModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('TRIAL/ACTIVE transitions apply immediately, with no confirmation step', async () => {
    updateTenantStatus.mockResolvedValueOnce({ success: true, data: { ...baseTenant('ACTIVE'), status: 'ACTIVE' } })
    const wrapper = mount(TenantStatusModal, { props: { tenant: baseTenant('TRIAL') }, global: { mocks: globalMocks } })

    wrapper.vm.newStatus = 'ACTIVE'
    await wrapper.vm.onContinue()
    await flushPromises()

    expect(wrapper.vm.confirming).toBe(false)
    expect(updateTenantStatus).toHaveBeenCalledWith('7', { status: 'ACTIVE' })
    expect(wrapper.emitted('updated')).toBeTruthy()
  })

  it('SUSPENDED requires confirmation before the API is called', async () => {
    const wrapper = mount(TenantStatusModal, { props: { tenant: baseTenant('ACTIVE') }, global: { mocks: globalMocks } })

    wrapper.vm.newStatus = 'SUSPENDED'
    await wrapper.vm.onContinue()

    expect(wrapper.vm.confirming).toBe(true)
    expect(updateTenantStatus).not.toHaveBeenCalled()
  })

  it('CANCELLED requires confirmation, and only calls the API after the confirm button is pressed', async () => {
    updateTenantStatus.mockResolvedValueOnce({ success: true, data: { ...baseTenant('CANCELLED') } })
    const wrapper = mount(TenantStatusModal, { props: { tenant: baseTenant('ACTIVE') }, global: { mocks: globalMocks } })

    wrapper.vm.newStatus = 'CANCELLED'
    await wrapper.vm.onContinue()
    expect(updateTenantStatus).not.toHaveBeenCalled()

    await wrapper.vm.submit()
    await flushPromises()

    expect(updateTenantStatus).toHaveBeenCalledWith('7', { status: 'CANCELLED' })
    expect(wrapper.emitted('updated')).toBeTruthy()
  })

  it('sends the optional reason only when non-empty', async () => {
    updateTenantStatus.mockResolvedValueOnce({ success: true, data: baseTenant('SUSPENDED') })
    const wrapper = mount(TenantStatusModal, { props: { tenant: baseTenant('ACTIVE') }, global: { mocks: globalMocks } })

    wrapper.vm.newStatus = 'SUSPENDED'
    wrapper.vm.reason = 'Non-payment'
    await wrapper.vm.onContinue()
    await wrapper.vm.submit()
    await flushPromises()

    expect(updateTenantStatus).toHaveBeenCalledWith('7', { status: 'SUSPENDED', reason: 'Non-payment' })
  })

  it('a 404 (tenant deleted/moved between page load and submit) surfaces the backend message and does not emit updated', async () => {
    updateTenantStatus.mockRejectedValueOnce(Object.assign(new Error('tenant not found'), { status: 404 }))
    const wrapper = mount(TenantStatusModal, { props: { tenant: baseTenant('ACTIVE') }, global: { mocks: globalMocks } })

    wrapper.vm.newStatus = 'SUSPENDED'
    await wrapper.vm.onContinue()
    await wrapper.vm.submit()
    await flushPromises()

    expect(wrapper.vm.statusError).toBe('tenant not found')
    expect(wrapper.emitted('updated')).toBeUndefined()
    // Drops back out of the confirmation step so the user can see the
    // error against the form rather than being stuck on a dead confirm
    // screen.
    expect(wrapper.vm.confirming).toBe(false)
  })

  it('choosing the same status as the current one leaves Update disabled', async () => {
    const wrapper = mount(TenantStatusModal, { props: { tenant: baseTenant('ACTIVE') }, global: { mocks: globalMocks } })
    const updateButton = wrapper.findAll('button').find((b) => b.text() === 'platformAdmin.status.updateButton')
    expect(updateButton.attributes('disabled')).toBeDefined()
  })
})

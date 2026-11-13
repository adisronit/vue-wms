import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import MembershipStatusDialog from '../MembershipStatusDialog.vue'

vi.mock('../../../services/platformTenantUsers', async () => {
  const actual = await vi.importActual('../../../services/platformTenantUsers')
  return { ...actual, changeTenantUserStatus: vi.fn() }
})

import { changeTenantUserStatus } from '../../../services/platformTenantUsers'

const globalMocks = { $t: (key) => key }

const activeMember = {
  user_id: '42',
  username: 'jane',
  full_name: 'Jane Doe',
  email: 'jane@example.com',
  role: { code: 'RECEIVING', name: 'Receiving Staff' },
  status: 'ACTIVE',
}

const disabledMember = { ...activeMember, status: 'DISABLED' }

describe('MembershipStatusDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('requires an explicit confirmation step before suspending an active member', async () => {
    const wrapper = mount(MembershipStatusDialog, {
      props: { tenantId: '1', member: activeMember },
      global: { mocks: globalMocks },
    })

    expect(wrapper.vm.isSuspending).toBe(true)
    expect(wrapper.vm.confirming).toBe(false)

    wrapper.vm.confirming = true
    await wrapper.vm.submit()

    expect(changeTenantUserStatus).toHaveBeenCalledWith('1', '42', { status: 'DISABLED' })
  })

  it('does not call the API before the confirmation step is reached', () => {
    mount(MembershipStatusDialog, { props: { tenantId: '1', member: activeMember }, global: { mocks: globalMocks } })
    expect(changeTenantUserStatus).not.toHaveBeenCalled()
  })

  it('reactivates a disabled member without requiring the confirmation step', async () => {
    changeTenantUserStatus.mockResolvedValueOnce({ data: activeMember })
    const wrapper = mount(MembershipStatusDialog, {
      props: { tenantId: '1', member: disabledMember },
      global: { mocks: globalMocks },
    })

    expect(wrapper.vm.isSuspending).toBe(false)
    await wrapper.vm.submit()

    expect(changeTenantUserStatus).toHaveBeenCalledWith('1', '42', { status: 'ACTIVE' })
    expect(wrapper.emitted('updated')).toBeTruthy()
  })

  it('includes an optional reason in the request when provided', async () => {
    changeTenantUserStatus.mockResolvedValueOnce({ data: disabledMember })
    const wrapper = mount(MembershipStatusDialog, {
      props: { tenantId: '1', member: activeMember },
      global: { mocks: globalMocks },
    })
    wrapper.vm.reason = '  Left the company  '
    wrapper.vm.confirming = true

    await wrapper.vm.submit()

    expect(changeTenantUserStatus).toHaveBeenCalledWith('1', '42', { status: 'DISABLED', reason: 'Left the company' })
  })

  it('shows the last-active-administrator message on a 409 response and resets the confirm step', async () => {
    changeTenantUserStatus.mockRejectedValueOnce(Object.assign(new Error('conflict'), { status: 409 }))
    const wrapper = mount(MembershipStatusDialog, {
      props: { tenantId: '1', member: activeMember },
      global: { mocks: globalMocks },
    })
    wrapper.vm.confirming = true

    await wrapper.vm.submit()

    expect(wrapper.vm.statusError).toContain('platformAdmin.tenantUsers.changeStatus.messages.lastAdmin')
    expect(wrapper.vm.confirming).toBe(false)
    expect(wrapper.emitted('updated')).toBeFalsy()
  })
})

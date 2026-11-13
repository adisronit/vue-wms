import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ChangeRoleDialog from '../ChangeRoleDialog.vue'

vi.mock('../../../services/platformTenantUsers', async () => {
  const actual = await vi.importActual('../../../services/platformTenantUsers')
  return { ...actual, changeTenantUserRole: vi.fn() }
})

import { changeTenantUserRole } from '../../../services/platformTenantUsers'

const globalMocks = { $t: (key) => key }

const member = {
  user_id: '42',
  username: 'jane',
  full_name: 'Jane Doe',
  email: 'jane@example.com',
  role: { code: 'RECEIVING', name: 'Receiving Staff' },
  status: 'ACTIVE',
}

function makeWrapper() {
  return mount(ChangeRoleDialog, { props: { tenantId: '1', member }, global: { mocks: globalMocks } })
}

describe('ChangeRoleDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('initializes the new-role select to the member current role', () => {
    const wrapper = makeWrapper()
    expect(wrapper.vm.newRoleCode).toBe('RECEIVING')
  })

  it('submits the new role and emits updated on success', async () => {
    changeTenantUserRole.mockResolvedValueOnce({ data: { ...member, role: { code: 'PICKING', name: 'Picking Staff' } } })
    const wrapper = makeWrapper()
    wrapper.vm.newRoleCode = 'PICKING'

    await wrapper.vm.save()

    expect(changeTenantUserRole).toHaveBeenCalledWith('1', '42', { role_code: 'PICKING' })
    expect(wrapper.emitted('updated')).toBeTruthy()
  })

  it('shows the last-active-administrator message on a 409 response', async () => {
    changeTenantUserRole.mockRejectedValueOnce(Object.assign(new Error('conflict'), { status: 409 }))
    const wrapper = makeWrapper()
    wrapper.vm.newRoleCode = 'PICKING'

    await wrapper.vm.save()

    expect(wrapper.vm.roleError).toContain('platformAdmin.tenantUsers.changeRole.messages.lastAdmin')
    expect(wrapper.emitted('updated')).toBeFalsy()
  })
})

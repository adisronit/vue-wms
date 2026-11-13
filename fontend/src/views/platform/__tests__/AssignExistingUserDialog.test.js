import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import AssignExistingUserDialog from '../AssignExistingUserDialog.vue'

vi.mock('../../../services/platformTenantUsers', async () => {
  const actual = await vi.importActual('../../../services/platformTenantUsers')
  return { ...actual, createTenantUser: vi.fn() }
})

import { createTenantUser } from '../../../services/platformTenantUsers'

const globalMocks = { $t: (key) => key }

function makeWrapper() {
  return mount(AssignExistingUserDialog, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
}

describe('AssignExistingUserDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('rejects submission with a blank identifier', async () => {
    const wrapper = makeWrapper()
    await wrapper.vm.save()

    expect(createTenantUser).not.toHaveBeenCalled()
    expect(wrapper.vm.formError).toContain('platformAdmin.tenantUsers.assignExisting.messages.identifierRequired')
  })

  it('submits mode=EXISTING with the exact identifier and role on success', async () => {
    createTenantUser.mockResolvedValueOnce({ data: { id: '2', user_id: '42' } })
    const wrapper = makeWrapper()
    wrapper.vm.identifier = '  existing@example.com  '
    wrapper.vm.roleCode = 'WAREHOUSE_MANAGER'

    await wrapper.vm.save()

    expect(createTenantUser).toHaveBeenCalledWith('1', {
      mode: 'EXISTING',
      role_code: 'WAREHOUSE_MANAGER',
      identifier: 'existing@example.com',
    })
    expect(wrapper.emitted('created')).toBeTruthy()
  })

  it('shows a not-found message on a 404 response — never exposing a broad search result', async () => {
    createTenantUser.mockRejectedValueOnce(Object.assign(new Error('not found'), { status: 404 }))
    const wrapper = makeWrapper()
    wrapper.vm.identifier = 'nobody@example.com'

    await wrapper.vm.save()

    expect(wrapper.vm.formError).toContain('platformAdmin.tenantUsers.assignExisting.messages.userNotFound')
  })

  it('shows an already-a-member message on a 409 response', async () => {
    createTenantUser.mockRejectedValueOnce(Object.assign(new Error('conflict'), { status: 409 }))
    const wrapper = makeWrapper()
    wrapper.vm.identifier = 'existing@example.com'

    await wrapper.vm.save()

    expect(wrapper.vm.formError).toContain('platformAdmin.tenantUsers.assignExisting.messages.alreadyMember')
  })
})

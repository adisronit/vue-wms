import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import TenantUserForm from '../TenantUserForm.vue'

vi.mock('../../../services/platformTenantUsers', async () => {
  const actual = await vi.importActual('../../../services/platformTenantUsers')
  return { ...actual, createTenantUser: vi.fn() }
})

import { createTenantUser } from '../../../services/platformTenantUsers'

const globalMocks = { $t: (key) => key }

function makeWrapper() {
  return mount(TenantUserForm, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
}

describe('TenantUserForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('rejects submission when required fields are missing', async () => {
    const wrapper = makeWrapper()
    wrapper.vm.form.password = 'LongEnough1'

    await wrapper.vm.save()

    expect(createTenantUser).not.toHaveBeenCalled()
    expect(wrapper.vm.formError).toContain('platformAdmin.tenantUsers.form.messages.requiredFields')
  })

  it('rejects a password shorter than 8 characters', async () => {
    const wrapper = makeWrapper()
    wrapper.vm.form.full_name = 'New Staff'
    wrapper.vm.form.username = 'newstaff'
    wrapper.vm.form.email = 'new@example.com'
    wrapper.vm.form.password = 'short'

    await wrapper.vm.save()

    expect(createTenantUser).not.toHaveBeenCalled()
    expect(wrapper.vm.formError).toContain('platformAdmin.tenantUsers.form.messages.passwordTooShort')
  })

  it('submits mode=NEW with every field on success and emits created', async () => {
    createTenantUser.mockResolvedValueOnce({ data: { id: '1', user_id: '42' } })
    const wrapper = makeWrapper()
    wrapper.vm.form.full_name = 'New Staff'
    wrapper.vm.form.username = 'newstaff'
    wrapper.vm.form.email = 'new@example.com'
    wrapper.vm.form.password = 'LongEnough1'
    wrapper.vm.form.role_code = 'RECEIVING'

    await wrapper.vm.save()

    expect(createTenantUser).toHaveBeenCalledWith('1', {
      mode: 'NEW',
      role_code: 'RECEIVING',
      full_name: 'New Staff',
      username: 'newstaff',
      email: 'new@example.com',
      password: 'LongEnough1',
    })
    expect(wrapper.emitted('created')).toBeTruthy()
  })

  it('shows a duplicate-user message on a 409 response', async () => {
    createTenantUser.mockRejectedValueOnce(Object.assign(new Error('conflict'), { status: 409 }))
    const wrapper = makeWrapper()
    wrapper.vm.form.full_name = 'New Staff'
    wrapper.vm.form.username = 'newstaff'
    wrapper.vm.form.email = 'new@example.com'
    wrapper.vm.form.password = 'LongEnough1'

    await wrapper.vm.save()

    expect(wrapper.vm.formError).toContain('platformAdmin.tenantUsers.form.messages.duplicateUser')
    expect(wrapper.emitted('created')).toBeFalsy()
  })
})

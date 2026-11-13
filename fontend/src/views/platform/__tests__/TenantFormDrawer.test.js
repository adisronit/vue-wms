import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TenantFormDrawer from '../TenantFormDrawer.vue'

vi.mock('../../../services/platformTenants', async () => {
  const actual = await vi.importActual('../../../services/platformTenants')
  return { ...actual, createTenant: vi.fn(), updateTenant: vi.fn() }
})

import { createTenant, updateTenant } from '../../../services/platformTenants'

const globalMocks = { $t: (key) => key }

describe('TenantFormDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('create mode: rejects an empty name without calling the API', async () => {
    const wrapper = mount(TenantFormDrawer, { props: { tenant: null }, global: { mocks: globalMocks } })
    await wrapper.vm.save()
    expect(createTenant).not.toHaveBeenCalled()
    expect(wrapper.vm.formError).toBe('platformAdmin.form.messages.nameRequired')
  })

  it('create mode: rejects an empty slug without calling the API', async () => {
    const wrapper = mount(TenantFormDrawer, { props: { tenant: null }, global: { mocks: globalMocks } })
    wrapper.vm.form.name = 'Acme Co'
    await wrapper.vm.save()
    expect(createTenant).not.toHaveBeenCalled()
    expect(wrapper.vm.formError).toBe('platformAdmin.form.messages.slugRequired')
  })

  it('create mode: posts the exact backend DTO shape and emits saved on success', async () => {
    createTenant.mockResolvedValueOnce({
      success: true,
      data: { id: '7', slug: 'acme', name: 'Acme Co', status: 'TRIAL' },
    })
    const wrapper = mount(TenantFormDrawer, { props: { tenant: null }, global: { mocks: globalMocks } })
    wrapper.vm.form.name = 'Acme Co'
    wrapper.vm.form.slug = 'acme'
    wrapper.vm.form.status = 'TRIAL'

    await wrapper.vm.save()
    await flushPromises()

    expect(createTenant).toHaveBeenCalledWith({ name: 'Acme Co', slug: 'acme', status: 'TRIAL' })
    expect(wrapper.emitted('saved')[0][0]).toEqual({ id: '7', slug: 'acme', name: 'Acme Co', status: 'TRIAL' })
  })

  it('create mode: a 409 duplicate-slug response shows a clear message, not the raw backend text', async () => {
    createTenant.mockRejectedValueOnce(Object.assign(new Error('tenant slug already exists'), { status: 409 }))
    const wrapper = mount(TenantFormDrawer, { props: { tenant: null }, global: { mocks: globalMocks } })
    wrapper.vm.form.name = 'Acme Co'
    wrapper.vm.form.slug = 'acme'

    await wrapper.vm.save()
    await flushPromises()

    expect(wrapper.vm.formError).toBe('platformAdmin.form.messages.duplicateSlug')
    expect(wrapper.emitted('saved')).toBeUndefined()
  })

  it('create mode: a 422 validation response surfaces the backend\'s own message', async () => {
    createTenant.mockRejectedValueOnce(
      Object.assign(new Error('slug must be lowercase letters, digits, and single hyphens only'), { status: 422 })
    )
    const wrapper = mount(TenantFormDrawer, { props: { tenant: null }, global: { mocks: globalMocks } })
    wrapper.vm.form.name = 'Acme Co'
    wrapper.vm.form.slug = 'Not Valid!'

    await wrapper.vm.save()
    await flushPromises()

    expect(wrapper.vm.formError).toBe('slug must be lowercase letters, digits, and single hyphens only')
  })

  it('edit mode: pre-fills the form from the given tenant and calls updateTenant (never createTenant)', async () => {
    updateTenant.mockResolvedValueOnce({
      success: true,
      data: { id: '7', slug: 'acme-renamed', name: 'Acme Renamed', status: 'ACTIVE' },
    })
    const tenant = { id: '7', slug: 'acme', name: 'Acme Co', status: 'ACTIVE' }
    const wrapper = mount(TenantFormDrawer, { props: { tenant }, global: { mocks: globalMocks } })

    expect(wrapper.vm.form.name).toBe('Acme Co')
    expect(wrapper.vm.form.slug).toBe('acme')

    wrapper.vm.form.name = 'Acme Renamed'
    wrapper.vm.form.slug = 'acme-renamed'
    await wrapper.vm.save()
    await flushPromises()

    expect(updateTenant).toHaveBeenCalledWith('7', { name: 'Acme Renamed', slug: 'acme-renamed' })
    expect(createTenant).not.toHaveBeenCalled()
    expect(wrapper.emitted('saved')[0][0].name).toBe('Acme Renamed')
  })

  it('close button emits close without calling any API', async () => {
    const wrapper = mount(TenantFormDrawer, { props: { tenant: null }, global: { mocks: globalMocks } })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(createTenant).not.toHaveBeenCalled()
  })
})

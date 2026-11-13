import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TenantDetailView from '../TenantDetailView.vue'

vi.mock('../../../services/platformTenants', async () => {
  const actual = await vi.importActual('../../../services/platformTenants')
  return { ...actual, getTenant: vi.fn() }
})

import { getTenant } from '../../../services/platformTenants'

const globalMocks = {
  $t: (key) => key,
  $d: (value) => String(value),
  $route: { params: { id: '1' } },
}

const tenant = {
  id: '1',
  slug: 'acme',
  name: 'Acme Co',
  status: 'ACTIVE',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-02T00:00:00Z',
}

function makeWrapper() {
  return mount(TenantDetailView, {
    global: {
      mocks: globalMocks,
      stubs: { 'router-link': true, TenantUserList: true },
    },
  })
}

describe('TenantDetailView — Users tab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('defaults to the Overview tab and does not render the Users tab content', async () => {
    getTenant.mockResolvedValueOnce({ data: tenant })
    const wrapper = makeWrapper()
    await flushPromises()

    expect(wrapper.vm.activeTab).toBe('overview')
    expect(wrapper.findComponent({ name: 'TenantUserList' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('platformAdmin.detail.fields.slug')
  })

  it('switching to the Users tab mounts TenantUserList with the tenant id', async () => {
    getTenant.mockResolvedValueOnce({ data: tenant })
    const wrapper = makeWrapper()
    await flushPromises()

    wrapper.vm.activeTab = 'users'
    await flushPromises()

    const list = wrapper.findComponent({ name: 'TenantUserList' })
    expect(list.exists()).toBe(true)
    expect(list.props('tenantId')).toBe('1')
  })
})

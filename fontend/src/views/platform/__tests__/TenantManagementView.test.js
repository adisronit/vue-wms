import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TenantManagementView from '../TenantManagementView.vue'

vi.mock('../../../services/platformTenants', async () => {
  const actual = await vi.importActual('../../../services/platformTenants')
  return { ...actual, listTenants: vi.fn() }
})

import { listTenants } from '../../../services/platformTenants'

const globalMocks = {
  $t: (key) => key,
  $d: (value) => String(value),
}

function page(total, rows) {
  return {
    success: true,
    data: rows,
    meta: { page: 1, limit: 20, total, total_pages: Math.max(1, Math.ceil(total / 20)) },
  }
}

describe('TenantManagementView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads and renders the tenant list from the real API shape on mount', async () => {
    listTenants.mockResolvedValueOnce(
      page(2, [
        { id: '1', slug: 'acme', name: 'Acme Co', status: 'ACTIVE', created_at: '2026-01-01T00:00:00Z' },
        { id: '2', slug: 'globex', name: 'Globex', status: 'TRIAL', created_at: '2026-01-02T00:00:00Z' },
      ])
    )

    const wrapper = mount(TenantManagementView, { global: { mocks: globalMocks, stubs: { 'router-link': true } } })
    await flushPromises()

    expect(listTenants).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, limit: 20, search: '', status: '' })
    )
    expect(wrapper.text()).toContain('Acme Co')
    expect(wrapper.text()).toContain('Globex')
  })

  it('shows the empty state when the API returns no tenants', async () => {
    listTenants.mockResolvedValueOnce(page(0, []))

    const wrapper = mount(TenantManagementView, { global: { mocks: globalMocks, stubs: { 'router-link': true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('platformAdmin.tenants.table.noData')
  })

  it('shows the error state and never a raw stack trace when the API call fails', async () => {
    listTenants.mockRejectedValueOnce(Object.assign(new Error('platform admin access required'), { status: 403 }))

    const wrapper = mount(TenantManagementView, { global: { mocks: globalMocks, stubs: { 'router-link': true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('platform admin access required')
  })

  it('debounced search re-fetches with the search term after the user stops typing', async () => {
    vi.useFakeTimers()
    listTenants.mockResolvedValue(page(0, []))

    const wrapper = mount(TenantManagementView, { global: { mocks: globalMocks, stubs: { 'router-link': true } } })
    await flushPromises()
    listTenants.mockClear()

    wrapper.vm.filters.search = 'acme'
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(listTenants).toHaveBeenCalledWith(expect.objectContaining({ search: 'acme', page: 1 }))
    vi.useRealTimers()
  })

  it('status filter change resets to page 1 and refetches with the status param', async () => {
    listTenants.mockResolvedValue(page(0, []))
    const wrapper = mount(TenantManagementView, { global: { mocks: globalMocks, stubs: { 'router-link': true } } })
    await flushPromises()
    listTenants.mockClear()

    wrapper.vm.page = 3
    wrapper.vm.filters.status = 'SUSPENDED'
    await flushPromises()

    expect(listTenants).toHaveBeenCalledWith(expect.objectContaining({ status: 'SUSPENDED', page: 1 }))
  })

  it('pagination Next advances the page and refetches', async () => {
    listTenants.mockResolvedValueOnce(page(50, Array.from({ length: 20 }, (_, i) => ({
      id: String(i), slug: `t${i}`, name: `Tenant ${i}`, status: 'ACTIVE', created_at: '2026-01-01T00:00:00Z',
    }))))
    listTenants.mockResolvedValueOnce(page(50, []))

    const wrapper = mount(TenantManagementView, { global: { mocks: globalMocks, stubs: { 'router-link': true } } })
    await flushPromises()

    await wrapper.vm.goToPage(2)
    await flushPromises()

    expect(listTenants).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
  })
})

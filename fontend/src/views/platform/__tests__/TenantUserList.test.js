import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TenantUserList from '../TenantUserList.vue'

vi.mock('../../../services/platformTenantUsers', async () => {
  const actual = await vi.importActual('../../../services/platformTenantUsers')
  return { ...actual, listTenantUsers: vi.fn() }
})

import { listTenantUsers } from '../../../services/platformTenantUsers'

const globalMocks = {
  $t: (key) => key,
  $d: (value) => String(value),
}

function page(total, rows) {
  return {
    success: true,
    data: rows,
    meta: { page: 1, limit: 25, total, total_pages: Math.max(1, Math.ceil(total / 25)) },
  }
}

const member = {
  id: '1',
  tenant_id: '1',
  user_id: '42',
  username: 'jane',
  full_name: 'Jane Doe',
  email: 'jane@example.com',
  role: { code: 'RECEIVING', name: 'Receiving Staff' },
  status: 'ACTIVE',
  joined_at: '2026-01-01T00:00:00Z',
}

describe('TenantUserList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads and renders tenant members from the real API shape on mount', async () => {
    listTenantUsers.mockResolvedValueOnce(page(1, [member]))

    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(listTenantUsers).toHaveBeenCalledWith('1', expect.objectContaining({ page: 1, limit: 25, sortBy: 'name', sortDir: 'asc' }))
    expect(wrapper.text()).toContain('Jane Doe')
    expect(wrapper.text()).toContain('jane@example.com')
  })

  it('shows the empty state when the API returns no members', async () => {
    listTenantUsers.mockResolvedValueOnce(page(0, []))

    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('platformAdmin.tenantUsers.table.noData')
  })

  it('shows the error message (not a raw stack trace) when the API call fails', async () => {
    listTenantUsers.mockRejectedValueOnce(Object.assign(new Error('tenant not found'), { status: 404 }))

    const wrapper = mount(TenantUserList, { props: { tenantId: '999' }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('tenant not found')
  })

  it('debounced search re-fetches with the search term after the user stops typing', async () => {
    vi.useFakeTimers()
    listTenantUsers.mockResolvedValue(page(0, []))

    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()
    listTenantUsers.mockClear()

    wrapper.vm.filters.search = 'jane'
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(listTenantUsers).toHaveBeenCalledWith('1', expect.objectContaining({ search: 'jane', page: 1 }))
    vi.useRealTimers()
  })

  it('role filter re-fetches immediately (no debounce) and resets to page 1', async () => {
    listTenantUsers.mockResolvedValue(page(0, []))

    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 3
    listTenantUsers.mockClear()

    wrapper.vm.filters.role = 'ADMIN'
    await flushPromises()

    expect(listTenantUsers).toHaveBeenCalledWith('1', expect.objectContaining({ role: 'ADMIN', page: 1 }))
  })

  it('pagination next/previous calls the API with the updated page', async () => {
    listTenantUsers.mockResolvedValue(page(40, [member]))

    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()
    listTenantUsers.mockClear()

    wrapper.vm.goToPage(2)
    await flushPromises()

    expect(listTenantUsers).toHaveBeenCalledWith('1', expect.objectContaining({ page: 2 }))
  })

  it('opens the Add User form and the Assign Existing User dialog from their buttons', async () => {
    listTenantUsers.mockResolvedValue(page(0, []))

    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.showForm).toBe(false)
    wrapper.vm.showForm = true
    expect(wrapper.vm.showForm).toBe(true)

    expect(wrapper.vm.showAssignExisting).toBe(false)
    wrapper.vm.showAssignExisting = true
    expect(wrapper.vm.showAssignExisting).toBe(true)
  })

  it('re-fetches the list after a membership is created or updated', async () => {
    listTenantUsers.mockResolvedValue(page(1, [member]))

    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()
    listTenantUsers.mockClear()

    wrapper.vm.onCreated()
    await flushPromises()
    expect(listTenantUsers).toHaveBeenCalledTimes(1)

    wrapper.vm.onUpdated()
    await flushPromises()
    expect(listTenantUsers).toHaveBeenCalledTimes(2)
  })

  it('clicking a sortable header toggles sort and resets to page 1', async () => {
    listTenantUsers.mockResolvedValue(page(0, []))
    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 3
    listTenantUsers.mockClear()

    wrapper.vm.toggleSort('joinedAt')
    await flushPromises()

    expect(wrapper.vm.sortBy).toBe('joinedAt')
    expect(wrapper.vm.sortDir).toBe('asc')
    expect(wrapper.vm.page).toBe(1)
    expect(listTenantUsers).toHaveBeenCalledWith('1', expect.objectContaining({ sortBy: 'joinedAt', page: 1 }))
  })

  it('changing rows-per-page resets to page 1 and refetches with the new limit', async () => {
    listTenantUsers.mockResolvedValue(page(0, []))
    const wrapper = mount(TenantUserList, { props: { tenantId: '1' }, global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listTenantUsers.mockClear()

    wrapper.vm.limit = 50
    await flushPromises()

    expect(wrapper.vm.page).toBe(1)
    expect(listTenantUsers).toHaveBeenCalledWith('1', expect.objectContaining({ limit: 50, page: 1 }))
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../auth', () => ({
  hasPermission: vi.fn(() => true),
  isPlatformAdmin: vi.fn(() => false),
  isPlatformAdminStatusKnown: vi.fn(() => true)
}))

import { hasPermission, isPlatformAdmin, isPlatformAdminStatusKnown } from '../auth'
import {
  state, MAX_TABS, tabKeyFor, canOpen, openTab, activateTab,
  closeTab, clearAllTabs, restore, reconcileDeferredTabs
} from '../tabs'

function route(name, params = {}, fullPath = null) {
  return { name, params, fullPath: fullPath || `/${name}`, meta: { titleKey: `nav.${name}` } }
}

describe('tabs service', () => {
  beforeEach(() => {
    clearAllTabs()
    sessionStorage.clear()
    vi.clearAllMocks()
    hasPermission.mockReturnValue(true)
    isPlatformAdmin.mockReturnValue(false)
    isPlatformAdminStatusKnown.mockReturnValue(true)
  })

  it('opens a new tab and makes it active', () => {
    openTab(route('Dashboard'))
    expect(state.tabs).toHaveLength(1)
    expect(state.activeKey).toBe(tabKeyFor(route('Dashboard')))
  })

  it('reuses an existing tab instead of duplicating it (REQ-09)', () => {
    openTab(route('Dashboard'))
    openTab(route('StockAdjustment'))
    openTab(route('Dashboard'))
    expect(state.tabs).toHaveLength(2)
    expect(state.activeKey).toBe(tabKeyFor(route('Dashboard')))
  })

  it('treats different params on the same route name as distinct tabs', () => {
    openTab(route('OrderDetail', { id: '1' }))
    openTab(route('OrderDetail', { id: '2' }))
    expect(state.tabs).toHaveLength(2)
    expect(state.tabs[0].key).not.toBe(state.tabs[1].key)
  })

  it('enforces the 10-tab cap but always allows reactivating an open tab (REQ-07)', () => {
    for (let i = 0; i < MAX_TABS; i++) openTab(route('OrderDetail', { id: String(i) }))
    expect(state.tabs).toHaveLength(MAX_TABS)
    expect(canOpen(route('OrderDetail', { id: '999' }))).toBe(false)
    expect(canOpen(route('OrderDetail', { id: '0' }))).toBe(true)
  })

  it('reactivates the most-recently-used remaining tab when the active tab closes (REQ-11)', () => {
    openTab(route('Dashboard'))
    openTab(route('StockInquiry'))
    openTab(route('Receiving'))
    openTab(route('StockAdjustment'))
    activateTab(tabKeyFor(route('StockInquiry')))
    // activation history: Dashboard -> StockInquiry -> Receiving -> StockAdjustment -> StockInquiry
    const next = closeTab(tabKeyFor(route('StockInquiry')))
    expect(next).toBe(tabKeyFor(route('StockAdjustment')))
  })

  it('does not change the active tab when closing an inactive tab', () => {
    openTab(route('Dashboard'))
    openTab(route('StockInquiry'))
    const activeBefore = state.activeKey
    closeTab(tabKeyFor(route('Dashboard')))
    expect(state.activeKey).toBe(activeBefore)
  })

  it('returns null when the last tab is closed', () => {
    openTab(route('Dashboard'))
    const next = closeTab(tabKeyFor(route('Dashboard')))
    expect(next).toBeNull()
    expect(state.tabs).toHaveLength(0)
  })

  it('clearAllTabs empties state and sessionStorage (logout)', () => {
    openTab(route('Dashboard'))
    clearAllTabs()
    expect(state.tabs).toHaveLength(0)
    expect(state.activeKey).toBeNull()
    expect(sessionStorage.getItem('wms.tabs.v1')).toBeNull()
  })

  it('persists and restores tabs, filtered by current permissions (REQ-05)', () => {
    openTab(route('Dashboard'))
    openTab(route('StockAdjustment'))
    const savedKey = state.activeKey

    const raw = sessionStorage.getItem('wms.tabs.v1')
    expect(raw).not.toBeNull()

    // Simulate a fresh module load (new in-memory state) with the same
    // persisted sessionStorage still present.
    state.tabs.splice(0, state.tabs.length)
    state.history.splice(0, state.history.length)
    state.activeKey = null

    restore((name, params) => route(name, params))

    expect(state.tabs.map((t) => t.name).sort()).toEqual(['Dashboard', 'StockAdjustment'])
    expect(state.activeKey).toBe(savedKey)
  })

  it('drops a persisted tab the caller is no longer authorized for (REQ-05)', () => {
    openTab(route('Dashboard'))
    openTab(route('UserManagement'))
    state.tabs.splice(0, state.tabs.length)
    state.history.splice(0, state.history.length)
    state.activeKey = null

    hasPermission.mockImplementation((code) => code !== 'users.view')
    restore((name, params) => route(name, params))

    expect(state.tabs.map((t) => t.name)).toEqual(['Dashboard'])
  })

  it('drops an unrecognized route name rather than restoring it blindly', () => {
    sessionStorage.setItem('wms.tabs.v1', JSON.stringify({
      tabs: [{ key: 'Nonexistent:{}', name: 'Nonexistent', params: {} }],
      activeKey: 'Nonexistent:{}',
      history: ['Nonexistent:{}']
    }))

    restore((name, params) => route(name, params))

    expect(state.tabs).toHaveLength(0)
  })

  describe('deferred Platform Admin restoration (Task 1)', () => {
    it('defers (neither restores nor drops) a Platform-Admin tab when that status is not yet known', () => {
      openTab(route('Dashboard'))
      openTab(route('PlatformTenants'))
      state.tabs.splice(0, state.tabs.length)
      state.history.splice(0, state.history.length)
      state.activeKey = null

      isPlatformAdminStatusKnown.mockReturnValue(false)
      restore((name, params) => route(name, params))

      // Not restored yet...
      expect(state.tabs.map((t) => t.name)).toEqual(['Dashboard'])
      // ...but not force-dropped as unauthorized either — isPlatformAdmin
      // (the actual authorization check) is never even consulted while
      // its status is unknown, so this can't be mistaken for "checked
      // and denied".
      expect(isPlatformAdmin).not.toHaveBeenCalled()
    })

    it('restores a deferred Platform Admin tab once status resolves true, preserving original order', () => {
      openTab(route('Dashboard'))
      openTab(route('PlatformTenants'))
      openTab(route('StockAdjustment'))
      state.tabs.splice(0, state.tabs.length)
      state.history.splice(0, state.history.length)
      state.activeKey = null

      isPlatformAdminStatusKnown.mockReturnValue(false)
      restore((name, params) => route(name, params))
      expect(state.tabs.map((t) => t.name)).toEqual(['Dashboard', 'StockAdjustment'])

      // The current-route navigation (e.g. the user refreshed while
      // sitting on Dashboard) has already landed — Dashboard stays
      // active throughout; only sibling tabs are affected.
      activateTab(tabKeyFor(route('Dashboard')))

      isPlatformAdminStatusKnown.mockReturnValue(true)
      isPlatformAdmin.mockReturnValue(true)
      reconcileDeferredTabs((name, params) => route(name, params))

      expect(state.tabs.map((t) => t.name)).toEqual(['Dashboard', 'PlatformTenants', 'StockAdjustment'])
      expect(state.activeKey).toBe(tabKeyFor(route('Dashboard')))
    })

    it('never restores a deferred Platform Admin tab once status resolves false (RBAC not bypassed)', () => {
      openTab(route('Dashboard'))
      openTab(route('PlatformTenants'))
      state.tabs.splice(0, state.tabs.length)
      state.history.splice(0, state.history.length)
      state.activeKey = null

      isPlatformAdminStatusKnown.mockReturnValue(false)
      restore((name, params) => route(name, params))

      isPlatformAdminStatusKnown.mockReturnValue(true)
      isPlatformAdmin.mockReturnValue(false)
      reconcileDeferredTabs((name, params) => route(name, params))

      expect(state.tabs.map((t) => t.name)).toEqual(['Dashboard'])
    })

    it('does not duplicate a deferred tab the live navigation already opened before reconciliation ran', () => {
      openTab(route('Dashboard'))
      openTab(route('PlatformTenants'))
      const savedTabs = JSON.parse(sessionStorage.getItem('wms.tabs.v1'))
      state.tabs.splice(0, state.tabs.length)
      state.history.splice(0, state.history.length)
      state.activeKey = null

      isPlatformAdminStatusKnown.mockReturnValue(false)
      sessionStorage.setItem('wms.tabs.v1', JSON.stringify(savedTabs))
      restore((name, params) => route(name, params))

      // Simulates the real navigation into PlatformTenants already
      // having resolved (its own router guard independently awaits
      // checkPlatformAdminStatus) and opened the tab normally, before
      // this component's deferred-reconciliation pass runs.
      openTab(route('PlatformTenants'))
      expect(state.tabs.filter((t) => t.name === 'PlatformTenants')).toHaveLength(1)

      isPlatformAdminStatusKnown.mockReturnValue(true)
      isPlatformAdmin.mockReturnValue(true)
      reconcileDeferredTabs((name, params) => route(name, params))

      expect(state.tabs.filter((t) => t.name === 'PlatformTenants')).toHaveLength(1)
    })
  })
})

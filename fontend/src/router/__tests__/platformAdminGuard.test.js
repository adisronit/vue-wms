import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mocked before importing the router module, since router/index.js
// reads isAuthenticated/checkPlatformAdminStatus at navigation time, not
// at import time — but the import itself must resolve against a mock,
// not a real network-backed implementation (Phase 9: "use mocks only
// inside automated tests").
vi.mock('../../services/auth', () => ({
  isAuthenticated: vi.fn(),
  checkPlatformAdminStatus: vi.fn(),
}))
vi.mock('../../services/api', () => ({
  setUnauthorizedHandler: vi.fn(),
}))

describe('Platform Admin route guard', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/')
  })

  it('redirects an unauthenticated visitor straight to /login, never reaching the platform-admin check', async () => {
    const auth = await import('../../services/auth')
    auth.isAuthenticated.mockReturnValue(false)

    const { default: router } = await import('../index.js')
    await router.push('/platform/tenants')
    await router.isReady()

    expect(router.currentRoute.value.path).toBe('/login')
    expect(auth.checkPlatformAdminStatus).not.toHaveBeenCalled()
  })

  it('denies an authenticated non-platform-admin and sends them to /dashboard — Tenant Admin privileges alone must never grant access', async () => {
    const auth = await import('../../services/auth')
    auth.isAuthenticated.mockReturnValue(true)
    auth.checkPlatformAdminStatus.mockResolvedValue(false)

    const { default: router } = await import('../index.js')
    await router.push('/platform/tenants')
    await router.isReady()

    expect(auth.checkPlatformAdminStatus).toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/dashboard')
  })

  it('admits an authenticated platform admin to the tenant list route', async () => {
    const auth = await import('../../services/auth')
    auth.isAuthenticated.mockReturnValue(true)
    auth.checkPlatformAdminStatus.mockResolvedValue(true)

    const { default: router } = await import('../index.js')
    await router.push('/platform/tenants')
    await router.isReady()

    expect(router.currentRoute.value.path).toBe('/platform/tenants')
  })

  it('admits an authenticated platform admin to a tenant detail route', async () => {
    const auth = await import('../../services/auth')
    auth.isAuthenticated.mockReturnValue(true)
    auth.checkPlatformAdminStatus.mockResolvedValue(true)

    const { default: router } = await import('../index.js')
    await router.push('/platform/tenants/42')
    await router.isReady()

    expect(router.currentRoute.value.path).toBe('/platform/tenants/42')
  })

  it('does not gate an ordinary route behind the platform-admin check', async () => {
    const auth = await import('../../services/auth')
    auth.isAuthenticated.mockReturnValue(true)

    const { default: router } = await import('../index.js')
    await router.push('/dashboard')
    await router.isReady()

    expect(router.currentRoute.value.path).toBe('/dashboard')
    expect(auth.checkPlatformAdminStatus).not.toHaveBeenCalled()
  })
})

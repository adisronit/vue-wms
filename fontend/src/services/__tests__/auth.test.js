import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import * as api from '../api'

// All of auth.js's platform-admin logic is exercised against a mocked
// apiGet — no real network call, matching "use mocks only inside
// automated tests" (Phase 9).
vi.mock('../api', async () => {
  const actual = await vi.importActual('../api')
  return { ...actual, apiGet: vi.fn() }
})

describe('platform admin capability check', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('isPlatformAdmin() fails closed (false) before checkPlatformAdminStatus ever runs', async () => {
    const auth = await import('../auth')
    expect(auth.isPlatformAdmin()).toBe(false)
  })

  it('checkPlatformAdminStatus() returns false immediately when not authenticated, without calling the API', async () => {
    const auth = await import('../auth')
    const result = await auth.checkPlatformAdminStatus()
    expect(result).toBe(false)
    expect(auth.isPlatformAdmin()).toBe(false)
    expect(api.apiGet).not.toHaveBeenCalled()
  })

  it('checkPlatformAdminStatus() caches a true result from GET /platform/me', async () => {
    const auth = await import('../auth')
    localStorage.setItem(
      'wms_auth',
      JSON.stringify({ accessToken: 't', expiresAt: Date.now() + 60000, user: { id: '1', permissions: [] } })
    )
    api.apiGet.mockResolvedValueOnce({ success: true, data: { is_platform_admin: true } })

    const result = await auth.checkPlatformAdminStatus()
    expect(result).toBe(true)
    expect(auth.isPlatformAdmin()).toBe(true)
    expect(api.apiGet).toHaveBeenCalledWith('/platform/me')
  })

  it('checkPlatformAdminStatus() fails closed to false on a network/API error — never treats "could not tell" as true', async () => {
    const auth = await import('../auth')
    localStorage.setItem(
      'wms_auth',
      JSON.stringify({ accessToken: 't', expiresAt: Date.now() + 60000, user: { id: '1', permissions: [] } })
    )
    api.apiGet.mockRejectedValueOnce(new Error('network error'))

    const result = await auth.checkPlatformAdminStatus()
    expect(result).toBe(false)
    expect(auth.isPlatformAdmin()).toBe(false)
  })

  it('logout() resets the cached platform-admin flag so a different user session never inherits a stale true', async () => {
    const auth = await import('../auth')
    localStorage.setItem(
      'wms_auth',
      JSON.stringify({ accessToken: 't', expiresAt: Date.now() + 60000, user: { id: '1', permissions: [] } })
    )
    api.apiGet.mockResolvedValueOnce({ success: true, data: { is_platform_admin: true } })
    await auth.checkPlatformAdminStatus()
    expect(auth.isPlatformAdmin()).toBe(true)

    auth.logout()
    expect(auth.isPlatformAdmin()).toBe(false)
  })
})

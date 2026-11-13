import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { apiPatch, apiGet, setStoredAuth, getStoredAuth, setUnauthorizedHandler } from '../api'

function mockFetchOnce(status, body) {
  global.fetch = vi.fn().mockResolvedValueOnce({
    status,
    ok: status >= 200 && status < 300,
    json: async () => body,
  })
}

describe('api client error handling', () => {
  beforeEach(() => {
    localStorage.clear()
    setUnauthorizedHandler(null)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('apiPatch sends a PATCH request with a JSON body — added for PATCH /platform/tenants/:id/status', async () => {
    mockFetchOnce(200, { success: true, data: { ok: true } })
    await apiPatch('/platform/tenants/7/status', { status: 'SUSPENDED' })

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/platform/tenants/7/status'),
      expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ status: 'SUSPENDED' }) })
    )
  })

  it('a 401 response clears the stored session and invokes the unauthorized handler', async () => {
    setStoredAuth('token', 3600, { id: '1' })
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    mockFetchOnce(401, { success: false, message: 'missing authentication', error: 'UNAUTHORIZED' })

    await expect(apiGet('/platform/tenants')).rejects.toThrow('missing authentication')
    expect(getStoredAuth()).toBeNull()
    expect(handler).toHaveBeenCalled()
  })

  it('a 403 response surfaces the backend message with .status set, without clearing the session', async () => {
    setStoredAuth('token', 3600, { id: '1' })
    mockFetchOnce(403, { success: false, message: 'platform admin access required', error: 'FORBIDDEN' })

    try {
      await apiGet('/platform/tenants')
      throw new Error('expected apiGet to throw')
    } catch (err) {
      expect(err.message).toBe('platform admin access required')
      expect(err.status).toBe(403)
    }
    expect(getStoredAuth()).not.toBeNull()
  })

  it('a 404 response is surfaced with .status 404 and the backend message', async () => {
    mockFetchOnce(404, { success: false, message: 'tenant not found', error: 'NOT_FOUND' })
    try {
      await apiGet('/platform/tenants/999')
      throw new Error('expected apiGet to throw')
    } catch (err) {
      expect(err.status).toBe(404)
      expect(err.message).toBe('tenant not found')
    }
  })

  it('a 409 response (duplicate slug) is surfaced with .status 409', async () => {
    mockFetchOnce(409, { success: false, message: 'tenant slug already exists', error: 'DUPLICATE' })
    try {
      await apiGet('/platform/tenants')
      throw new Error('expected apiGet to throw')
    } catch (err) {
      expect(err.status).toBe(409)
    }
  })

  it('a 422 response (validation) is surfaced with .status 422 and the field-level message', async () => {
    mockFetchOnce(422, { success: false, message: 'slug is required', error: 'VALIDATION_ERROR' })
    try {
      await apiGet('/platform/tenants')
      throw new Error('expected apiGet to throw')
    } catch (err) {
      expect(err.status).toBe(422)
      expect(err.message).toBe('slug is required')
    }
  })
})

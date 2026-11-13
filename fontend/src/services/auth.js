import { apiGet, apiPost, setStoredAuth, clearStoredAuth, getStoredAuth } from './api'

// login calls POST /api/v1/auth/login and persists the session on
// success. Throws (with a .message from the backend's error envelope) on
// invalid credentials or an inactive/locked account — callers show
// err.message directly, matching LoginView's existing errorMessage box.
export async function login(username, password) {
  const res = await apiPost('/auth/login', { username, password })
  const { access_token, expires_in, user } = res.data
  setStoredAuth(access_token, expires_in, user)
  return user
}

export function logout() {
  clearStoredAuth()
  resetPlatformAdminStatus()
}

export function isAuthenticated() {
  return getStoredAuth() !== null
}

// currentUser returns the profile captured at login time (id, username,
// full_name, role, permissions) without a network call — enough for nav
// display and permission checks between logins.
export function currentUser() {
  const auth = getStoredAuth()
  return auth ? auth.user : null
}

export function hasPermission(code) {
  const user = currentUser()
  return !!user && Array.isArray(user.permissions) && user.permissions.includes(code)
}

// fetchMe re-fetches the live profile from GET /api/v1/auth/me — used
// where the UI needs the current database row rather than the
// login-time snapshot.
export async function fetchMe() {
  const res = await apiGet('/auth/me')
  return res.data
}

// platformAdminStatus caches the result of the one question the
// frontend is allowed to ask about Platform Admin eligibility: GET
// /api/v1/platform/me, answered server-side from a live platform_admins
// lookup (never from username/email — see that handler's own doc).
// null means "not yet checked this session" — isPlatformAdmin() treats
// that the same as false (fail closed: a nav item or route guard must
// never show privileged UI while the check is still in flight or failed
// to run). This cache is purely a UI convenience; every actual
// /api/v1/platform/tenants/* call is independently re-verified
// server-side by RequirePlatformAdmin regardless of what this holds.
let platformAdminStatus = null

// checkPlatformAdminStatus calls GET /api/v1/platform/me once and caches
// the answer for the rest of the session. Call this after login and on
// app bootstrap (when a stored session already exists) — never trust a
// stale value across logins, which is why logout() above resets it.
export async function checkPlatformAdminStatus() {
  if (!isAuthenticated()) {
    platformAdminStatus = false
    return false
  }
  try {
    const res = await apiGet('/platform/me')
    platformAdminStatus = res.data?.is_platform_admin === true
  } catch {
    // A network error or an unexpected 401/500 here must never be
    // treated as "is an admin" — fail closed to false, matching the
    // backend middleware's own "could not tell is never allowed" rule.
    platformAdminStatus = false
  }
  return platformAdminStatus
}

// isPlatformAdmin reads the cached result synchronously — safe to use
// directly in a computed property or router guard once
// checkPlatformAdminStatus has resolved at least once this session.
export function isPlatformAdmin() {
  return platformAdminStatus === true
}

export function resetPlatformAdminStatus() {
  platformAdminStatus = null
}

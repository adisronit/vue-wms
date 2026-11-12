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

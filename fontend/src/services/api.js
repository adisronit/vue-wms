// Minimal fetch-based API client. No Axios: the app has no other need for
// interceptors/cancellation beyond what fetch + this thin wrapper already
// give it, so adding a dependency for it isn't justified.
const BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'
const AUTH_STORAGE_KEY = 'wms_auth'

// getStoredAuth reads and validates the persisted session. Expiry is
// checked client-side against the access token's own expires_in (echoed
// back from login) so a normal page load doesn't need a network round
// trip just to know whether the user is still logged in.
export function getStoredAuth() {
  const raw = localStorage.getItem(AUTH_STORAGE_KEY)
  if (!raw) return null

  try {
    const parsed = JSON.parse(raw)
    if (!parsed.accessToken || !parsed.expiresAt) {
      localStorage.removeItem(AUTH_STORAGE_KEY)
      return null
    }
    if (Date.now() >= parsed.expiresAt) {
      localStorage.removeItem(AUTH_STORAGE_KEY)
      return null
    }
    return parsed
  } catch {
    localStorage.removeItem(AUTH_STORAGE_KEY)
    return null
  }
}

// setStoredAuth persists only the access token, its computed expiry, and
// the minimal user/session info the UI needs — never the password.
export function setStoredAuth(accessToken, expiresInSeconds, user) {
  localStorage.setItem(
    AUTH_STORAGE_KEY,
    JSON.stringify({
      accessToken,
      expiresAt: Date.now() + expiresInSeconds * 1000,
      user,
    })
  )
}

export function clearStoredAuth() {
  localStorage.removeItem(AUTH_STORAGE_KEY)
}

// onUnauthorized lets the router register a redirect-to-login callback
// without api.js importing the router module (which would be the
// backwards direction — the router already depends on services/auth.js).
let onUnauthorized = null
export function setUnauthorizedHandler(handler) {
  onUnauthorized = handler
}

async function request(path, options = {}) {
  const auth = getStoredAuth()
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
  if (auth) {
    headers.Authorization = `Bearer ${auth.accessToken}`
  }

  const response = await fetch(`${BASE_URL}${path}`, { ...options, headers })

  let body = null
  try {
    body = await response.json()
  } catch {
    body = null
  }

  if (response.status === 401) {
    clearStoredAuth()
    if (onUnauthorized) onUnauthorized()
  }

  if (!response.ok) {
    const message = body?.message || `Request failed with status ${response.status}`
    const error = new Error(message)
    error.status = response.status
    error.code = body?.error
    throw error
  }

  return body
}

export function apiGet(path) {
  return request(path, { method: 'GET' })
}

// apiUpload is the EWTA product-image counterpart to apiDownload below:
// request() above always sets Content-Type: application/json and
// JSON.stringifies the body, which a multipart/form-data upload can't
// use — the browser must set its own Content-Type (with the multipart
// boundary) when sending a FormData body, so this path omits the header
// entirely rather than hardcoding one. Still sends the same Bearer auth
// header and 401-handling every other request does.
export async function apiUpload(path, formData) {
  const auth = getStoredAuth()
  const headers = {}
  if (auth) headers.Authorization = `Bearer ${auth.accessToken}`

  const response = await fetch(`${BASE_URL}${path}`, { method: 'POST', headers, body: formData })

  let body = null
  try {
    body = await response.json()
  } catch {
    body = null
  }

  if (response.status === 401) {
    clearStoredAuth()
    if (onUnauthorized) onUnauthorized()
  }

  if (!response.ok) {
    const message = body?.message || `Request failed with status ${response.status}`
    const error = new Error(message)
    error.status = response.status
    error.code = body?.error
    throw error
  }

  return body
}

export function apiPost(path, data) {
  return request(path, { method: 'POST', body: JSON.stringify(data) })
}

export function apiPut(path, data) {
  return request(path, { method: 'PUT', body: JSON.stringify(data) })
}

export function apiPatch(path, data) {
  return request(path, { method: 'PATCH', body: JSON.stringify(data) })
}

export function apiDelete(path) {
  return request(path, { method: 'DELETE' })
}

// apiDownload is Phase 22's export helper — request() above always
// parses the response as JSON, which a CSV/file response isn't, so
// exports use this separate path instead. Still sends the same Bearer
// auth header every other request does.
export async function apiDownload(path) {
  const auth = getStoredAuth()
  const headers = {}
  if (auth) headers.Authorization = `Bearer ${auth.accessToken}`

  const response = await fetch(`${BASE_URL}${path}`, { headers })

  if (response.status === 401) {
    clearStoredAuth()
    if (onUnauthorized) onUnauthorized()
  }

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`
    try {
      const body = await response.json()
      message = body?.message || message
    } catch {
      // Response wasn't JSON either — keep the generic message.
    }
    const error = new Error(message)
    error.status = response.status
    throw error
  }

  const blob = await response.blob()
  const disposition = response.headers.get('Content-Disposition') || ''
  const match = disposition.match(/filename="?([^"]+)"?/)
  const filename = match ? match[1] : 'export.csv'
  return { blob, filename }
}

// triggerDownload saves a Blob to the user's downloads via a
// synthetic, immediately-removed <a> click — the standard browser
// pattern for a JS-fetched (rather than plain-link) file download.
export function triggerDownload(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

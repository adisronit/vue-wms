import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listUsers mirrors the User Management screen's search box plus
// role/status filters and pagination — no sort param exists in the UI,
// so none is sent.
export function listUsers(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.role) query.set('role', params.role)
  if (params.status) query.set('status', params.status)

  const qs = query.toString()
  return apiGet(`/users${qs ? `?${qs}` : ''}`)
}

export function getUser(id) {
  return apiGet(`/users/${id}`)
}

export function createUser(payload) {
  return apiPost('/users', payload)
}

export function updateUser(id, payload) {
  return apiPut(`/users/${id}`, payload)
}

export function deleteUser(id) {
  return apiDelete(`/users/${id}`)
}

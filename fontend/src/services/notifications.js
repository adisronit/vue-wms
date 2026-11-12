import { apiGet, apiPost } from './api'

// Phase 21 — minimal notification inbox. There is no dedicated unread-
// count endpoint (requirement: "do not create excessive notification
// CRUD"); the bell badge instead calls listNotifications({ isRead:
// false, limit: 100 }) and uses the response's own meta.total.
export function listNotifications(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.isRead !== undefined && params.isRead !== null) {
    query.set('is_read', params.isRead ? 'true' : 'false')
  }

  const qs = query.toString()
  return apiGet(`/notifications${qs ? `?${qs}` : ''}`)
}

export function markNotificationRead(id) {
  return apiPost(`/notifications/${id}/read`, {})
}

import { apiGet, apiPost, apiPut, apiPatch } from './api'

// Platform Admin tenant-management API (WMS-29C/29D) — /api/v1/platform/*,
// gated server-side by RequirePlatformAdmin (see backend internal/tenant's
// Handler). All five endpoints here already exist on the backend; nothing
// in this file is a fabricated field or route.

export function listTenants(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)

  const qs = query.toString()
  return apiGet(`/platform/tenants${qs ? `?${qs}` : ''}`)
}

export function getTenant(id) {
  return apiGet(`/platform/tenants/${id}`)
}

// payload: { slug, name, status? } — status omitted/empty defaults to
// TRIAL server-side (backend internal/tenant's CreateRequest/Service.Create).
export function createTenant(payload) {
  return apiPost('/platform/tenants', payload)
}

// payload: { slug, name } — status is never sent here; status
// transitions only ever go through updateTenantStatus below (backend
// internal/tenant's UpdateRequest deliberately excludes it).
export function updateTenant(id, payload) {
  return apiPut(`/platform/tenants/${id}`, payload)
}

// payload: { status, reason? } — reason is optional free text recorded
// only in the backend's audit log metadata (backend internal/tenant's
// UpdateStatusRequest).
export function updateTenantStatus(id, payload) {
  return apiPatch(`/platform/tenants/${id}/status`, payload)
}

// The exact four values tenants.status's CHECK constraint accepts
// (migrations/0041) — backend internal/tenant's Service treats every
// transition between them as valid; there is no restricted state-machine
// graph to encode here.
export const TENANT_STATUSES = ['TRIAL', 'ACTIVE', 'SUSPENDED', 'CANCELLED']

import { apiGet, apiPost, apiPatch } from './api'

// Platform Admin tenant-membership-management API (WMS-29E) —
// /api/v1/platform/tenants/{tenantId}/users/*, gated server-side by
// RequirePlatformAdmin (see backend internal/tenant's Handler/
// MembershipService). All five endpoints here already exist on the
// backend; nothing in this file is a fabricated field or route.

export function listTenantUsers(tenantId, params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.role) query.set('role', params.role)
  if (params.status) query.set('status', params.status)

  const qs = query.toString()
  return apiGet(`/platform/tenants/${tenantId}/users${qs ? `?${qs}` : ''}`)
}

export function getTenantUser(tenantId, userId) {
  return apiGet(`/platform/tenants/${tenantId}/users/${userId}`)
}

// payload: { mode: 'NEW', role_code, username, password, full_name, email }
//       or { mode: 'EXISTING', role_code, identifier }
export function createTenantUser(tenantId, payload) {
  return apiPost(`/platform/tenants/${tenantId}/users`, payload)
}

// payload: { role_code }
export function changeTenantUserRole(tenantId, userId, payload) {
  return apiPatch(`/platform/tenants/${tenantId}/users/${userId}/role`, payload)
}

// payload: { status: 'ACTIVE' | 'DISABLED', reason? }
export function changeTenantUserStatus(tenantId, userId, payload) {
  return apiPatch(`/platform/tenants/${tenantId}/users/${userId}/status`, payload)
}

// The fixed 7-role catalog every tenant is seeded with on creation
// (backend internal/rbac.RoleCatalog, seeded per-tenant by
// internal/tenant/repository.go's seedRoleCatalog) — mirrored here the
// same way TENANT_STATUSES already mirrors tenants.status's own CHECK
// constraint, rather than depending on the legacy, not-tenant-scoped
// GET /roles endpoint (which lists every tenant's roles at once with no
// tenant label to disambiguate identically-coded rows).
export const TENANT_ROLE_CODES = [
  'ADMIN',
  'WAREHOUSE_MANAGER',
  'RECEIVING',
  'QC',
  'PUTAWAY',
  'PICKING',
  'SHIPPING'
]

// The exact three values tenant_users.status's CHECK constraint accepts
// (migrations/0042). INVITED is a system-set initial value only — never
// a selectable target of the status-change dialog.
export const MEMBERSHIP_STATUSES = ['INVITED', 'ACTIVE', 'DISABLED']

import { apiGet, apiPut } from './api'

export function listRoles() {
  return apiGet('/roles')
}

export function listPermissions() {
  return apiGet('/permissions')
}

export function getRolePermissions(roleId) {
  return apiGet(`/roles/${roleId}/permissions`)
}

export function setRolePermissions(roleId, permissionCodes) {
  return apiPut(`/roles/${roleId}/permissions`, { permission_codes: permissionCodes })
}

export function getRoleWarehouseAccess(roleId) {
  return apiGet(`/roles/${roleId}/warehouse-access`)
}

export function setRoleWarehouseAccess(roleId, warehouseIds) {
  return apiPut(`/roles/${roleId}/warehouse-access`, { warehouse_ids: warehouseIds })
}

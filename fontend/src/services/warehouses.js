import { apiGet, apiPost, apiPut, apiDelete } from './api'

// Warehouses, zones, and locations are grouped in one file because the
// Warehouse Setup screen consumes all three together (zones and
// locations only ever appear as tabs inside an open warehouse's
// drawer) — mirroring the backend's own tightly-coupled hierarchy.

export function listWarehouses(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.type) query.set('type', params.type)
  if (params.status) query.set('status', params.status)

  const qs = query.toString()
  return apiGet(`/warehouses${qs ? `?${qs}` : ''}`)
}

export function getWarehouse(id) {
  return apiGet(`/warehouses/${id}`)
}

export function createWarehouse(payload) {
  return apiPost('/warehouses', payload)
}

export function updateWarehouse(id, payload) {
  return apiPut(`/warehouses/${id}`, payload)
}

export function deleteWarehouse(id) {
  return apiDelete(`/warehouses/${id}`)
}

export function listZones(warehouseId) {
  return apiGet(`/zones?warehouse_id=${warehouseId}`)
}

export function createZone(payload) {
  return apiPost('/zones', payload)
}

export function updateZone(id, payload) {
  return apiPut(`/zones/${id}`, payload)
}

export function deleteZone(id) {
  return apiDelete(`/zones/${id}`)
}

// listLocations always requires zoneId (the backend's own contract) —
// params is optional and additive (search/status/page/limit), added for
// the standalone Location Management screen; existing call sites that
// only ever passed zoneId (WarehouseView.vue's Location Structure tab)
// are unaffected.
export function listLocations(zoneId, params = {}) {
  const query = new URLSearchParams()
  query.set('zone_id', zoneId)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)

  return apiGet(`/locations?${query.toString()}`)
}

export function getLocation(id) {
  return apiGet(`/locations/${id}`)
}

export function createLocation(payload) {
  return apiPost('/locations', payload)
}

export function updateLocation(id, payload) {
  return apiPut(`/locations/${id}`, payload)
}

export function deleteLocation(id) {
  return apiDelete(`/locations/${id}`)
}

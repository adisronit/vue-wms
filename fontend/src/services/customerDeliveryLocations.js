import { apiGet, apiPost, apiPut, apiDelete } from './api'

// listCustomerDeliveryLocations mirrors the Delivery Locations tab's
// search box and status filter. customerId is required — the tab only
// ever lists one open customer's own locations, the same
// required-parent-id shape services/warehouses.js's listZones already
// uses for its own Zone Setup tab.
export function listCustomerDeliveryLocations(customerId, params = {}) {
  const query = new URLSearchParams()
  query.set('customer_id', customerId)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)

  return apiGet(`/customer-delivery-locations?${query.toString()}`)
}

export function getCustomerDeliveryLocation(id) {
  return apiGet(`/customer-delivery-locations/${id}`)
}

export function createCustomerDeliveryLocation(payload) {
  return apiPost('/customer-delivery-locations', payload)
}

export function updateCustomerDeliveryLocation(id, payload) {
  return apiPut(`/customer-delivery-locations/${id}`, payload)
}

export function deleteCustomerDeliveryLocation(id) {
  return apiDelete(`/customer-delivery-locations/${id}`)
}

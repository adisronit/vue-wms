import { apiGet, apiPost } from './api'

// Phase 33D.2 — Loading, the "load" half of the dispatch re-sequencing
// gate (33D.1 already added the "pack" half on internal/shipping's own
// Create). A loading trip carries one or more READY shipments; every
// carton belonging to those shipments' own packing jobs becomes one
// row on the trip's own scan-to-load worklist. Once every planned
// carton is scanned loaded (or flagged short) and the trip is
// confirmed, internal/shipping's own Confirm(DISPATCH) action is
// finally allowed for each shipment on it.

export function listLoadingTrips(params = {}) {
  const query = new URLSearchParams()
  if (params.page) query.set('page', params.page)
  if (params.limit) query.set('limit', params.limit)
  if (params.search) query.set('search', params.search)
  if (params.status) query.set('status', params.status)

  const qs = query.toString()
  return apiGet(`/loading-trips${qs ? `?${qs}` : ''}`)
}

// READY shipments not yet assigned to any active (non-cancelled) trip
// — powers the "assign shipment to trip" picker.
export function listAssignableShipments() {
  return apiGet('/loading-trips/sources')
}

export function getLoadingTrip(id) {
  return apiGet(`/loading-trips/${id}`)
}

export function createLoadingTrip(payload) {
  return apiPost('/loading-trips', payload || {})
}

export function assignShipmentToTrip(tripId, shipmentId) {
  return apiPost(`/loading-trips/${tripId}/shipments`, { shipment_id: shipmentId })
}

// Scan-to-load — resolves the scanned LPN against this trip's own
// planned worklist; the backend rejects a carton belonging to a
// different trip, one never planned at all, or one already resolved.
export function scanCarton(tripId, lpn) {
  return apiPost(`/loading-trips/${tripId}/scan`, { lpn })
}

// Short-loading exception — flags a planned carton as unable to be
// loaded (damaged/missing) instead of LOADED.
export function shortCarton(tripId, payload) {
  return apiPost(`/loading-trips/${tripId}/short`, payload)
}

// Confirms the trip (requires every planned carton LOADED or SHORT) —
// calls internal/shipping's own MarkLoaded for every shipment on the
// trip, the real dispatch-gate payoff.
export function confirmLoadingTrip(id, payload) {
  return apiPost(`/loading-trips/${id}/confirm`, payload || {})
}

export function cancelLoadingTrip(id, payload) {
  return apiPost(`/loading-trips/${id}/cancel`, payload || {})
}

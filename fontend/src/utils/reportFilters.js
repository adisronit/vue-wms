// buildReportQuery turns a flat filter object into a query string,
// shared by every report service module (Phase 34B+) so each one
// doesn't reimplement the same null/empty/array handling. Matches the
// backend's expectation of repeated params for array values (e.g.
// warehouse_id=1&warehouse_id=2), the same shape existing list
// endpoints (receiving, picking, ...) already accept.
export function buildReportQuery(filters = {}) {
  const params = new URLSearchParams()

  Object.entries(filters).forEach(([key, value]) => {
    if (value === null || value === undefined || value === '') return

    if (Array.isArray(value)) {
      value
        .filter((v) => v !== null && v !== undefined && v !== '')
        .forEach((v) => params.append(key, v))
      return
    }

    params.append(key, value)
  })

  return params.toString()
}

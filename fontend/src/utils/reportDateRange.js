// Shared Phase 34 Reports date-range logic. Asia/Bangkok has a fixed
// UTC+7 offset with no DST, so "business time" boundaries are computed
// with plain offset arithmetic rather than pulling in a timezone
// library (Intl.DateTimeFormat's resolved offset would be the same
// constant anyway) — every exported boundary is a real UTC Date/ISO
// string, safe to send to the backend regardless of the viewer's own
// browser timezone.
export const BANGKOK_OFFSET_MINUTES = 7 * 60

export const DATE_RANGE_PRESETS = [
  'today',
  'yesterday',
  'last7Days',
  'last30Days',
  'thisMonth',
  'lastMonth',
  'custom',
]

function toBangkokFields(date) {
  const shifted = new Date(date.getTime() + BANGKOK_OFFSET_MINUTES * 60000)
  return {
    year: shifted.getUTCFullYear(),
    month: shifted.getUTCMonth(),
    day: shifted.getUTCDate(),
    hours: shifted.getUTCHours(),
    minutes: shifted.getUTCMinutes(),
  }
}

// bangkokFieldsToUTC takes Bangkok wall-clock fields and returns the
// real UTC Date instant they correspond to.
function bangkokFieldsToUTC(year, month, day, hours = 0, minutes = 0) {
  const asIfUTC = Date.UTC(year, month, day, hours, minutes, 0, 0)
  return new Date(asIfUTC - BANGKOK_OFFSET_MINUTES * 60000)
}

// startOfBangkokDay returns the UTC instant of Bangkok midnight on the
// calendar day containing `date`.
export function startOfBangkokDay(date) {
  const { year, month, day } = toBangkokFields(date)
  return bangkokFieldsToUTC(year, month, day)
}

export function startOfBangkokMonth(date) {
  const { year, month } = toBangkokFields(date)
  return bangkokFieldsToUTC(year, month, 1)
}

export function addDays(date, days) {
  return new Date(date.getTime() + days * 86400000)
}

export function addMonthsBangkok(date, months) {
  const { year, month } = toBangkokFields(date)
  const total = month + months
  const targetYear = year + Math.floor(total / 12)
  const targetMonth = ((total % 12) + 12) % 12
  return bangkokFieldsToUTC(targetYear, targetMonth, 1)
}

// parseBangkokLocal interprets a `<input type="datetime-local">` value
// ("YYYY-MM-DDTHH:mm") as Bangkok wall-clock time and returns the real
// UTC Date instant.
export function parseBangkokLocal(value) {
  if (!value) return null
  const [datePart, timePart = '00:00'] = value.split('T')
  const [year, month, day] = datePart.split('-').map(Number)
  const [hours, minutes] = timePart.split(':').map(Number)
  return bangkokFieldsToUTC(year, month - 1, day, hours, minutes)
}

// formatBangkokLocal is parseBangkokLocal's inverse, for pre-filling a
// datetime-local input from a stored UTC Date.
export function formatBangkokLocal(date) {
  const { year, month, day, hours, minutes } = toBangkokFields(date)
  const pad = (n) => String(n).padStart(2, '0')
  return `${year}-${pad(month + 1)}-${pad(day)}T${pad(hours)}:${pad(minutes)}`
}

// resolveDateRange computes the [fromISO, toISO) boundary for a preset
// — inclusive From, exclusive To, both real UTC ISO timestamps — or,
// for 'custom', converts the given Bangkok-local from/to values the
// same way. `now` is injectable for tests; defaults to the real clock.
export function resolveDateRange(preset, custom = {}, now = new Date()) {
  const todayStart = startOfBangkokDay(now)

  switch (preset) {
    case 'today':
      return { preset, from: todayStart, to: addDays(todayStart, 1) }
    case 'yesterday':
      return { preset, from: addDays(todayStart, -1), to: todayStart }
    case 'last7Days':
      return { preset, from: addDays(todayStart, -6), to: addDays(todayStart, 1) }
    case 'last30Days':
      return { preset, from: addDays(todayStart, -29), to: addDays(todayStart, 1) }
    case 'thisMonth':
      return { preset, from: startOfBangkokMonth(now), to: addMonthsBangkok(now, 1) }
    case 'lastMonth':
      return {
        preset,
        from: addMonthsBangkok(now, -1),
        to: startOfBangkokMonth(now),
      }
    case 'custom': {
      const from = custom.from ? parseBangkokLocal(custom.from) : null
      const to = custom.to ? parseBangkokLocal(custom.to) : null
      return { preset, from, to }
    }
    default:
      throw new Error(`Unknown date range preset: ${preset}`)
  }
}

// toRangeISO converts a resolveDateRange() result's Date boundaries to
// the ISO strings the backend query params expect; null boundaries
// (an incomplete custom range) pass through as null.
export function toRangeISO({ from, to }) {
  return {
    from: from ? from.toISOString() : null,
    to: to ? to.toISOString() : null,
  }
}

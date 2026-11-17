import { describe, it, expect } from 'vitest'
import {
  resolveDateRange,
  toRangeISO,
  parseBangkokLocal,
  formatBangkokLocal,
} from '../reportDateRange'

// Fixed instant: 2026-11-17T10:30:00Z = 2026-11-17 17:30 Bangkok (UTC+7).
const NOW = new Date('2026-11-17T10:30:00.000Z')

describe('resolveDateRange', () => {
  it('today is an inclusive-from/exclusive-to Bangkok day window', () => {
    const { from, to } = toRangeISO(resolveDateRange('today', {}, NOW))
    expect(from).toBe('2026-11-16T17:00:00.000Z')
    expect(to).toBe('2026-11-17T17:00:00.000Z')
  })

  it('yesterday is the Bangkok day immediately before today', () => {
    const { from, to } = toRangeISO(resolveDateRange('yesterday', {}, NOW))
    expect(from).toBe('2026-11-15T17:00:00.000Z')
    expect(to).toBe('2026-11-16T17:00:00.000Z')
  })

  it('last7Days spans 7 Bangkok days ending today (inclusive)', () => {
    const { from, to } = toRangeISO(resolveDateRange('last7Days', {}, NOW))
    expect(from).toBe('2026-11-10T17:00:00.000Z')
    expect(to).toBe('2026-11-17T17:00:00.000Z')
  })

  it('last30Days spans 30 Bangkok days ending today (inclusive)', () => {
    const { from, to } = toRangeISO(resolveDateRange('last30Days', {}, NOW))
    expect(from).toBe('2026-10-18T17:00:00.000Z')
    expect(to).toBe('2026-11-17T17:00:00.000Z')
  })

  it('thisMonth spans the current Bangkok calendar month', () => {
    const { from, to } = toRangeISO(resolveDateRange('thisMonth', {}, NOW))
    expect(from).toBe('2026-10-31T17:00:00.000Z')
    expect(to).toBe('2026-11-30T17:00:00.000Z')
  })

  it('lastMonth spans the prior Bangkok calendar month, crossing a year boundary correctly', () => {
    const jan = new Date('2027-01-05T03:00:00.000Z') // 2027-01-05 10:00 Bangkok
    const { from, to } = toRangeISO(resolveDateRange('lastMonth', {}, jan))
    expect(from).toBe('2026-11-30T17:00:00.000Z') // Dec 1 00:00 Bangkok
    expect(to).toBe('2026-12-31T17:00:00.000Z') // Jan 1 00:00 Bangkok
  })

  it('custom converts Bangkok-local datetime-local values to UTC', () => {
    const { from, to } = toRangeISO(
      resolveDateRange('custom', { from: '2026-11-01T08:00', to: '2026-11-02T18:30' }, NOW)
    )
    expect(from).toBe('2026-11-01T01:00:00.000Z')
    expect(to).toBe('2026-11-02T11:30:00.000Z')
  })

  it('custom with a missing boundary returns null for it rather than throwing', () => {
    const { from, to } = toRangeISO(resolveDateRange('custom', { from: '2026-11-01T08:00' }, NOW))
    expect(from).toBe('2026-11-01T01:00:00.000Z')
    expect(to).toBeNull()
  })

  it('rejects an unknown preset', () => {
    expect(() => resolveDateRange('nextDecade', {}, NOW)).toThrow()
  })
})

describe('parseBangkokLocal / formatBangkokLocal round-trip', () => {
  it('formats a UTC instant back to the same Bangkok-local string it was parsed from', () => {
    const value = '2026-11-01T08:00'
    const parsed = parseBangkokLocal(value)
    expect(formatBangkokLocal(parsed)).toBe(value)
  })
})

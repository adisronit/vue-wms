import { describe, it, expect, beforeEach } from 'vitest'
import { getTabComponent, pruneTabComponentCache, tabComponentCacheSize } from '../tabComponentCache'

const OrderDetailView = { name: 'OrderDetailView', data: () => ({ x: 1 }) }

describe('tabComponentCache', () => {
  beforeEach(() => {
    pruneTabComponentCache([])
  })

  it('returns the same reference for the same tab key across calls (required for KeepAlive to recognize a cache hit)', () => {
    const a = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"5"}')
    const b = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"5"}')
    expect(a).toBe(b)
  })

  it('gives two tabs sharing the same underlying component distinct identities', () => {
    const tab5 = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"5"}')
    const tab9 = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"9"}')
    expect(tab5).not.toBe(tab9)
    expect(tab5.name).toBe('OrderDetail:{"id":"5"}')
    expect(tab9.name).toBe('OrderDetail:{"id":"9"}')
  })

  it('preserves the original component behavior (data/methods) in the clone', () => {
    const variant = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"5"}')
    expect(variant.data()).toEqual({ x: 1 })
  })

  it('regression: closing one of two same-component tabs evicts only that tab (Task 2)', () => {
    getTabComponent(OrderDetailView, 'OrderDetail:{"id":"5"}')
    const tab9Before = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"9"}')
    expect(tabComponentCacheSize()).toBe(2)

    // Tab 5 closes — only its key remains live.
    pruneTabComponentCache(['OrderDetail:{"id":"9"}'])
    expect(tabComponentCacheSize()).toBe(1)

    // Tab 9's cached variant is untouched by tab 5's eviction (state
    // preserved for the sibling tab using the same component).
    const tab9After = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"9"}')
    expect(tab9After).toBe(tab9Before)

    // Reopening tab 5 after it was evicted must mint a *fresh* variant —
    // proof there's no stale cache entry left over that could resurface
    // stale form values.
    const tab5Fresh = getTabComponent(OrderDetailView, 'OrderDetail:{"id":"5"}')
    expect(tab5Fresh).not.toBe(tab9Before)
    expect(tabComponentCacheSize()).toBe(2)
  })

  it('prunes every entry when no tabs remain open (logout)', () => {
    getTabComponent(OrderDetailView, 'OrderDetail:{"id":"5"}')
    getTabComponent(OrderDetailView, 'OrderDetail:{"id":"9"}')
    pruneTabComponentCache([])
    expect(tabComponentCacheSize()).toBe(0)
  })
})

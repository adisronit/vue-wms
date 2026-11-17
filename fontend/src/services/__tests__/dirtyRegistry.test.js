import { describe, it, expect, beforeEach } from 'vitest'
import { registerDirtyCheck, unregisterDirtyCheck, isTabDirty, clearAllDirtyChecks } from '../dirtyRegistry'

describe('dirtyRegistry', () => {
  beforeEach(() => {
    clearAllDirtyChecks()
  })

  it('reports not dirty when no check is registered for a tab', () => {
    expect(isTabDirty('some-tab')).toBe(false)
  })

  it('reflects the registered predicate', () => {
    let dirty = false
    registerDirtyCheck('tab-a', () => dirty)
    expect(isTabDirty('tab-a')).toBe(false)
    dirty = true
    expect(isTabDirty('tab-a')).toBe(true)
  })

  it('stops reporting dirty after unregistering (tab closed)', () => {
    registerDirtyCheck('tab-a', () => true)
    unregisterDirtyCheck('tab-a')
    expect(isTabDirty('tab-a')).toBe(false)
  })

  it('treats a throwing predicate as not dirty rather than blocking the close', () => {
    registerDirtyCheck('tab-a', () => { throw new Error('boom') })
    expect(isTabDirty('tab-a')).toBe(false)
  })

  it('keeps checks isolated per tab key', () => {
    registerDirtyCheck('tab-a', () => true)
    registerDirtyCheck('tab-b', () => false)
    expect(isTabDirty('tab-a')).toBe(true)
    expect(isTabDirty('tab-b')).toBe(false)
  })
})

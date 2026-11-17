// Per-tab "do I have unsaved changes" registry (REQ-08). A page component
// registers a zero-arg predicate keyed by its own tab key in mounted()
// and unregisters it in unmounted() — both lifecycle hooks fire exactly
// once per KeepAlive'd instance (first mount, final eviction), which
// scopes the check to that tab's whole lifetime regardless of how many
// times the tab is activated/deactivated in between.
const checks = new Map()

export function registerDirtyCheck(tabKey, predicate) {
  checks.set(tabKey, predicate)
}

export function unregisterDirtyCheck(tabKey) {
  checks.delete(tabKey)
}

export function isTabDirty(tabKey) {
  const predicate = checks.get(tabKey)
  if (!predicate) return false
  try {
    return !!predicate()
  } catch {
    // A throwing predicate must never block the user from closing a tab —
    // treat it as "can't tell" the same as "not dirty", not as a reason
    // to trap them.
    return false
  }
}

export function clearAllDirtyChecks() {
  checks.clear()
}

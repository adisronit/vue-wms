import { reactive } from 'vue'
import { isRouteAllowed, permissionRequirementFor, PLATFORM_ADMIN } from '../router/permissions'
import { isPlatformAdminStatusKnown } from './auth'

export const MAX_TABS = 10
const STORAGE_KEY = 'wms.tabs.v1'

// state.tabs: ordered array of { key, name, params, fullPath, titleKey } —
// display order, stable as the user opens/switches tabs.
// state.history: activation-order stack of keys, most-recent-last. This is
// deliberately NOT the same as tab display order — it drives which tab
// reactivates (REQ-11) when the active one closes, which is not always
// the visually adjacent tab.
export const state = reactive({
  tabs: [],
  activeKey: null,
  history: []
})

function sortedParams(params) {
  const out = {}
  for (const k of Object.keys(params || {}).sort()) out[k] = params[k]
  return out
}

// Tab identity is route name + params, deliberately excluding query —
// e.g. a notification deep link's ?adjustment_id=123 re-opens the same
// Stock Adjustment tab rather than spawning a second one.
export function tabKeyFor(route) {
  return `${route.name}:${JSON.stringify(sortedParams(route.params))}`
}

function touchHistory(key) {
  const i = state.history.indexOf(key)
  if (i !== -1) state.history.splice(i, 1)
  state.history.push(key)
}

function findTab(key) {
  return state.tabs.find((t) => t.key === key)
}

// canOpen answers the 10-tab cap (REQ-07) *before* any navigation commits.
// Reactivating an already-open tab never counts against the cap.
export function canOpen(route) {
  if (findTab(tabKeyFor(route))) return true
  return state.tabs.length < MAX_TABS
}

// openTab creates-or-reuses a tab entry and marks it active (REQ-09).
// Safe to call on every successful in-app navigation.
export function openTab(route) {
  const key = tabKeyFor(route)
  let tab = findTab(key)
  if (!tab) {
    tab = {
      key,
      name: route.name,
      params: { ...route.params },
      fullPath: route.fullPath,
      titleKey: route.meta?.titleKey || null
    }
    state.tabs.push(tab)
  } else {
    // Keep fullPath current so reactivating later (e.g. from the tab bar)
    // lands wherever the user last was, including query string.
    tab.fullPath = route.fullPath
  }
  state.activeKey = key
  touchHistory(key)
  persist()
  return tab
}

export function activateTab(key) {
  if (!findTab(key)) return
  state.activeKey = key
  touchHistory(key)
  persist()
}

// closeTab removes the tab. If it was the active one, activation falls
// back to the most-recently-used *remaining* tab (REQ-11), not the
// visually adjacent one; returns the new active key (or null if no tabs
// remain) so the caller can navigate there.
export function closeTab(key) {
  const idx = state.tabs.findIndex((t) => t.key === key)
  if (idx === -1) return state.activeKey

  const wasActive = state.activeKey === key
  state.tabs.splice(idx, 1)
  const hi = state.history.indexOf(key)
  if (hi !== -1) state.history.splice(hi, 1)

  if (wasActive) {
    let nextKey = null
    for (let i = state.history.length - 1; i >= 0; i--) {
      if (findTab(state.history[i])) { nextKey = state.history[i]; break }
    }
    state.activeKey = nextKey
    if (nextKey) touchHistory(nextKey)
  }

  persist()
  return state.activeKey
}

export function clearAllTabs() {
  state.tabs.splice(0, state.tabs.length)
  state.history.splice(0, state.history.length)
  state.activeKey = null
  try { sessionStorage.removeItem(STORAGE_KEY) } catch { /* nothing to clear */ }
}

function persist() {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
      tabs: state.tabs,
      activeKey: state.activeKey,
      history: state.history
    }))
  } catch {
    // Private mode / storage full: tabs just won't survive a refresh.
  }
}

// Set by restore(), read by reconcileDeferredTabs() — the original saved
// display/activation order, so a tab added back later (see below) can be
// reinserted at its original position instead of just appended.
let lastSavedTabOrder = null
let lastSavedHistoryOrder = null

// Tabs restore() couldn't yet evaluate because they require
// PLATFORM_ADMIN and that status hasn't resolved yet at this synchronous,
// module-load-time point (see services/auth.js). Re-evaluated once by
// reconcileDeferredTabs() after the async check resolves — never assumed
// allowed in the meantime (REQ-05: fail closed, never bypass RBAC).
let pendingDeferredTabs = []

function addRestoredTab(t, resolveRoute) {
  if (findTab(t.key)) return // already present (e.g. the live current-route navigation already added it)
  const resolved = resolveRoute(t.name, t.params || {})
  if (!resolved) return
  state.tabs.push({
    key: t.key,
    name: t.name,
    params: t.params || {},
    fullPath: resolved.fullPath,
    titleKey: t.titleKey || resolved.meta?.titleKey || null
  })
}

function rankedSort(arr, order, keyOf) {
  if (!order) return
  const rank = new Map(order.map((k, i) => [k, i]))
  arr.sort((a, b) => {
    const ra = rank.has(keyOf(a)) ? rank.get(keyOf(a)) : Infinity
    const rb = rank.has(keyOf(b)) ? rank.get(keyOf(b)) : Infinity
    return ra - rb
  })
}

// restore re-populates state from sessionStorage, revalidating every
// persisted tab against the caller's *current* permissions (REQ-05) — a
// tab that was valid when saved but isn't anymore (role changed, route
// removed) is silently dropped, never force-opened. resolveRoute maps a
// stored {name, params} back to a live resolved route (for a fresh
// fullPath/meta), e.g. router.resolve({ name, params }).
//
// A tab gated on PLATFORM_ADMIN is neither restored nor dropped here if
// that status isn't resolved yet — it's held in pendingDeferredTabs for
// reconcileDeferredTabs() to re-evaluate once it is, so a legitimate
// platform admin's tab isn't lost to a timing accident, and a non-admin's
// never gets restored in the first place.
export function restore(resolveRoute) {
  let saved
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return
    saved = JSON.parse(raw)
  } catch {
    return
  }
  if (!saved || !Array.isArray(saved.tabs)) return

  lastSavedTabOrder = saved.tabs.map((t) => t && t.key).filter(Boolean)
  lastSavedHistoryOrder = Array.isArray(saved.history) ? saved.history.slice() : []
  pendingDeferredTabs = []

  for (const t of saved.tabs) {
    if (!t || !t.name) continue
    try {
      if (permissionRequirementFor(t.name) === PLATFORM_ADMIN && !isPlatformAdminStatusKnown()) {
        pendingDeferredTabs.push(t)
        continue
      }
      if (!isRouteAllowed(t.name)) continue
      addRestoredTab(t, resolveRoute)
    } catch {
      // Fail closed per-tab: one bad/unevaluable persisted tab must
      // never take down restoration of the rest.
    }
  }

  for (const key of lastSavedHistoryOrder) {
    if (findTab(key)) state.history.push(key)
  }

  if (saved.activeKey && findTab(saved.activeKey)) {
    state.activeKey = saved.activeKey
  } else if (state.tabs.length > 0) {
    state.activeKey = state.tabs[0].key
  }
}

// reconcileDeferredTabs re-evaluates any PLATFORM_ADMIN-gated tabs restore()
// held back, now that the async platform-admin check has resolved. Call
// once, after that check settles (see MainLayout.vue's mounted()).
// Never touches state.activeKey — the real navigation already in flight
// when the app booted (matching whatever URL the browser actually shows)
// is the authority on what's active; this only affects sibling tabs.
export function reconcileDeferredTabs(resolveRoute) {
  if (pendingDeferredTabs.length === 0) return
  for (const t of pendingDeferredTabs) {
    if (!isRouteAllowed(t.name)) continue
    addRestoredTab(t, resolveRoute)
  }
  pendingDeferredTabs = []

  rankedSort(state.tabs, lastSavedTabOrder, (t) => t.key)
  rankedSort(state.history, lastSavedHistoryOrder, (k) => k)
  persist()
}

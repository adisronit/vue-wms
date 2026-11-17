// Per-tab component-definition cache.
//
// Vue's <keep-alive :include>/:exclude> matches on a component's
// registered *name*, not on the vnode :key — so when two open tabs share
// an underlying component (e.g. two different Order Detail tabs, same
// OrderDetailView component, different :id params), they also share one
// name in `include`. Closing one of those tabs can't remove just its own
// cached instance that way: the name still has to stay in `include`
// because the sibling tab still needs it, so KeepAlive has no basis to
// prune the closed tab's slot specifically.
//
// getTabComponent gives every open tab its own component *identity* — a
// shallow clone of the real component definition with `name` set to the
// tab's own key — while still running the exact same data/computed/
// methods/etc. as the original. With that, <keep-alive :include> can be
// driven directly by the list of open tab keys: removing a closed tab's
// key from that list prunes exactly that tab's cached instance, never a
// sibling's, even when they're "the same page" by route.
const cache = new Map()

// The same tab key must always get back the exact same object reference
// across renders — Vue only recognizes a KeepAlive cache hit by matching
// `vnode.type` (plus key), so a fresh clone every render would defeat
// caching entirely rather than fix it.
export function getTabComponent(component, tabKey) {
  if (!cache.has(tabKey)) {
    cache.set(tabKey, { ...component, name: tabKey })
  }
  return cache.get(tabKey)
}

// Called whenever the open-tab list changes (see MainLayout.vue) so a
// closed tab's variant doesn't linger — closing a tab must both evict its
// KeepAlive cache slot (via :include dropping the key) and drop the
// variant definition itself, so reopening that same route+params later
// mounts fresh rather than risking any confusion with the evicted vnode.
export function pruneTabComponentCache(liveKeys) {
  const live = new Set(liveKeys)
  for (const key of cache.keys()) {
    if (!live.has(key)) cache.delete(key)
  }
}

export function tabComponentCacheSize() {
  return cache.size
}

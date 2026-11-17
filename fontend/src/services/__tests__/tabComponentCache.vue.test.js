import { describe, it, expect, beforeEach } from 'vitest'
import { h, defineComponent, KeepAlive } from 'vue'
import { mount } from '@vue/test-utils'
import { getTabComponent, pruneTabComponentCache } from '../tabComponentCache'

// A real Vue component with its own internal state and lifecycle hooks —
// this file proves the tabComponentCache trick (cloning the component
// with an overridden `name`) doesn't break Vue's actual KeepAlive
// behavior, props, or lifecycle hooks; tabComponentCache.test.js covers
// the plain-JS Map logic in isolation.
//
// Lifecycle calls are recorded into the plain `log` array prop (not
// $emit) because KeepAlive-deactivated children aren't reliably
// reachable via @vue/test-utils' wrapper.emitted()/findComponent once
// they're no longer the active render — a shared array read directly
// sidesteps that rather than fighting the test harness.
const CounterView = defineComponent({
  name: 'CounterView',
  props: {
    label: { type: String, default: '' },
    log: { type: Array, required: true }
  },
  data() {
    return { count: 0 }
  },
  mounted() {
    this.log.push(['mounted', this.label])
  },
  unmounted() {
    this.log.push(['unmounted', this.label])
  },
  activated() {
    this.log.push(['activated', this.label])
  },
  deactivated() {
    this.log.push(['deactivated', this.label])
  },
  template: `<div><span class="label">{{ label }}</span><button class="inc" @click="count++">{{ count }}</button></div>`
})

const Harness = defineComponent({
  props: {
    activeKey: { type: String, required: true },
    include: { type: Array, required: true },
    log: { type: Array, required: true }
  },
  render() {
    const Variant = getTabComponent(CounterView, this.activeKey)
    return h(KeepAlive, { include: this.include }, [
      h(Variant, { key: this.activeKey, label: this.activeKey, log: this.log })
    ])
  }
})

function hooksFor(log, label) {
  return log.filter(([, l]) => l === label).map(([hook]) => hook)
}

describe('tabComponentCache + KeepAlive (Vue integration)', () => {
  beforeEach(() => {
    pruneTabComponentCache([])
  })

  it('preserves each tab\'s independent component state across switches (two Order Detail tabs sharing one component)', async () => {
    const log = []
    const wrapper = mount(Harness, {
      props: { activeKey: 'OrderDetail:{"id":"5"}', include: ['OrderDetail:{"id":"5"}', 'OrderDetail:{"id":"9"}'], log }
    })

    await wrapper.find('.inc').trigger('click')
    await wrapper.find('.inc').trigger('click')
    expect(wrapper.find('.inc').text()).toBe('2')

    // Switch to the second tab — its own, independent counter starts at 0.
    await wrapper.setProps({ activeKey: 'OrderDetail:{"id":"9"}' })
    expect(wrapper.find('.label').text()).toBe('OrderDetail:{"id":"9"}')
    expect(wrapper.find('.inc').text()).toBe('0')
    await wrapper.find('.inc').trigger('click')
    expect(wrapper.find('.inc').text()).toBe('1')

    // Switch back to the first tab — its count of 2 survived being
    // deactivated, proving KeepAlive (not a fresh mount) handled it.
    await wrapper.setProps({ activeKey: 'OrderDetail:{"id":"5"}' })
    expect(wrapper.find('.label').text()).toBe('OrderDetail:{"id":"5"}')
    expect(wrapper.find('.inc').text()).toBe('2')
  })

  it('props still pass through correctly despite the per-tab name override', async () => {
    const log = []
    const wrapper = mount(Harness, {
      props: { activeKey: 'tab-a', include: ['tab-a'], log }
    })
    expect(wrapper.find('.label').text()).toBe('tab-a')
    await wrapper.setProps({ activeKey: 'tab-b', include: ['tab-a', 'tab-b'] })
    expect(wrapper.find('.label').text()).toBe('tab-b')
  })

  it('fires activated/deactivated (not mounted/unmounted) on ordinary tab switches', async () => {
    const log = []
    const wrapper = mount(Harness, {
      props: { activeKey: 'tab-a', include: ['tab-a', 'tab-b'], log }
    })
    // Vue's KeepAlive fires `activated` right after `mounted` even on the
    // very first mount, not only on later reactivation — confirmed here
    // rather than assumed.
    expect(hooksFor(log, 'tab-a')).toEqual(['mounted', 'activated'])

    await wrapper.setProps({ activeKey: 'tab-b' })
    expect(hooksFor(log, 'tab-a')).toEqual(['mounted', 'activated', 'deactivated'])
    expect(hooksFor(log, 'tab-b')).toEqual(['mounted', 'activated'])

    await wrapper.setProps({ activeKey: 'tab-a' })
    expect(hooksFor(log, 'tab-a')).toEqual(['mounted', 'activated', 'deactivated', 'activated'])
    expect(hooksFor(log, 'tab-a')).not.toContain('unmounted') // never destroyed, just deactivated/reactivated
  })

  it('regression: closing one of two same-component tabs evicts and unmounts only that tab, preserving the other\'s state (Task 2)', async () => {
    const log = []
    const keyA = 'OrderDetail:{"id":"5"}'
    const keyB = 'OrderDetail:{"id":"9"}'
    const wrapper = mount(Harness, {
      props: { activeKey: keyA, include: [keyA, keyB], log }
    })
    await wrapper.find('.inc').trigger('click') // tab 5's count = 1

    await wrapper.setProps({ activeKey: keyB })
    await wrapper.find('.inc').trigger('click')
    await wrapper.find('.inc').trigger('click')
    await wrapper.find('.inc').trigger('click') // tab 9's count = 3

    // Tab 5 closes: drop it from `include` (what MainLayout's openTabKeys
    // watcher drives) and prune its cached variant.
    pruneTabComponentCache([keyB])
    await wrapper.setProps({ include: [keyB] })
    expect(hooksFor(log, keyA)).toContain('unmounted')

    // Tab 9 (still open, never touched by tab 5's eviction) keeps its
    // state and was never unmounted.
    expect(wrapper.find('.inc').text()).toBe('3')
    expect(hooksFor(log, keyB)).not.toContain('unmounted')

    // Reopening tab 5 after eviction mounts fresh — no stale count=1,
    // no stale form values left over from before it was closed.
    await wrapper.setProps({ activeKey: keyA, include: [keyA, keyB] })
    expect(hooksFor(log, keyA).filter((h) => h === 'mounted')).toHaveLength(2)
    expect(wrapper.find('.inc').text()).toBe('0')
  })
})

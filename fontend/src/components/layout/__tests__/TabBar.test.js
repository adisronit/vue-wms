import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import TabBar from '../TabBar.vue'
import { clearAllTabs, openTab } from '../../../services/tabs'
import { registerDirtyCheck, clearAllDirtyChecks } from '../../../services/dirtyRegistry'

vi.mock('../../../services/auth', () => ({
  hasPermission: vi.fn(() => true),
  isPlatformAdmin: vi.fn(() => false)
}))

const globalMocks = { $t: (key) => key }

function route(name, params = {}) {
  return { name, params, fullPath: `/${name}`, meta: { titleKey: `nav.${name}` } }
}

describe('TabBar', () => {
  beforeEach(() => {
    clearAllTabs()
    clearAllDirtyChecks()
    sessionStorage.clear()
  })

  it('renders nothing when no tabs are open', () => {
    const wrapper = mount(TabBar, { global: { mocks: globalMocks } })
    expect(wrapper.find('[role="tablist"]').exists()).toBe(false)
  })

  it('renders one entry per open tab and highlights the active one', () => {
    openTab(route('Dashboard'))
    openTab(route('StockAdjustment'))
    const wrapper = mount(TabBar, { global: { mocks: globalMocks } })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs).toHaveLength(2)
    expect(tabs[1].attributes('aria-selected')).toBe('true')
    expect(tabs[0].attributes('aria-selected')).toBe('false')
  })

  it('emits navigate with the fullPath when a non-active tab is clicked', async () => {
    openTab(route('Dashboard'))
    openTab(route('StockAdjustment'))
    const wrapper = mount(TabBar, { global: { mocks: globalMocks } })
    await wrapper.findAll('[role="tab"]')[0].trigger('click')
    expect(wrapper.emitted('navigate')[0]).toEqual(['/Dashboard'])
  })

  it('closes the active, clean tab immediately and emits navigate to the fallback tab', async () => {
    openTab(route('Dashboard'))
    openTab(route('StockAdjustment'))
    const wrapper = mount(TabBar, { global: { mocks: globalMocks } })
    const closeButtons = wrapper.findAll('.tab-close')
    await closeButtons[closeButtons.length - 1].trigger('click')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
    expect(wrapper.emitted('navigate')[0]).toEqual(['/Dashboard'])
  })

  it('closes an inactive, clean tab immediately without emitting navigate', async () => {
    openTab(route('Dashboard'))
    openTab(route('StockAdjustment'))
    const wrapper = mount(TabBar, { global: { mocks: globalMocks } })
    await wrapper.findAll('.tab-close')[0].trigger('click')
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
    expect(wrapper.emitted('navigate')).toBeFalsy()
  })

  it('shows the unsaved-changes confirmation when closing a dirty tab, and Cancel keeps it open (REQ-08)', async () => {
    openTab(route('Dashboard'))
    const dirtyTab = openTab(route('StockAdjustment'))
    registerDirtyCheck(dirtyTab.key, () => true)

    const wrapper = mount(TabBar, { global: { mocks: globalMocks } })
    const closeButtons = wrapper.findAll('.tab-close')
    await closeButtons[closeButtons.length - 1].trigger('click')

    expect(wrapper.find('[role="alertdialog"]').exists()).toBe(true)
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(2)

    // Cancel: dialog closes, tab remains.
    await wrapper.find('[role="alertdialog"] button').trigger('click')
    expect(wrapper.find('[role="alertdialog"]').exists()).toBe(false)
    expect(wrapper.findAll('[role="tab"]')).toHaveLength(2)
  })

  it('closes the tab after confirming "Close Without Saving"', async () => {
    openTab(route('Dashboard'))
    const dirtyKey = openTab(route('StockAdjustment')).key
    registerDirtyCheck(dirtyKey, () => true)

    const wrapper = mount(TabBar, { global: { mocks: globalMocks } })
    const closeButtons = wrapper.findAll('.tab-close')
    await closeButtons[closeButtons.length - 1].trigger('click')

    const dialogButtons = wrapper.findAll('[role="alertdialog"] button')
    await dialogButtons[dialogButtons.length - 1].trigger('click')

    expect(wrapper.findAll('[role="tab"]')).toHaveLength(1)
  })
})

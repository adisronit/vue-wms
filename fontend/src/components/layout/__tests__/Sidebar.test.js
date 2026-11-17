import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Sidebar from '../Sidebar.vue'

const globalMocks = { $t: (key) => key }
const stubs = { 'router-link': { template: '<a><slot /></a>' } }

describe('Sidebar', () => {
  it('always shows Dashboard, Inbound, Inventory, and Outbound sections', () => {
    const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
    expect(wrapper.text()).toContain('nav.dashboard')
    expect(wrapper.text()).toContain('nav.sectionInbound')
    expect(wrapper.text()).toContain('nav.sectionInventory')
    expect(wrapper.text()).toContain('nav.sectionOutbound')
  })

  it('hides Master Data, Administration, and Platform Administration sections with no grants', () => {
    const wrapper = mount(Sidebar, {
      props: {
        canViewUsers: false,
        canViewRoles: false,
        canViewWarehouses: false,
        canViewProducts: false,
        canViewPlatformAdmin: false
      },
      global: { mocks: globalMocks, stubs }
    })
    expect(wrapper.text()).not.toContain('nav.sectionMasterData')
    expect(wrapper.text()).not.toContain('nav.sectionAdministration')
    expect(wrapper.text()).not.toContain('nav.sectionPlatformAdmin')
  })

  it('shows Master Data when the caller can view warehouses or products', () => {
    const wrapper = mount(Sidebar, {
      props: { canViewWarehouses: true },
      global: { mocks: globalMocks, stubs }
    })
    expect(wrapper.text()).toContain('nav.sectionMasterData')
    expect(wrapper.text()).toContain('nav.warehouse')
    expect(wrapper.text()).not.toContain('nav.productMaster')
  })

  it('shows Platform Administration only when canViewPlatformAdmin is true', () => {
    const wrapper = mount(Sidebar, {
      props: { canViewPlatformAdmin: true },
      global: { mocks: globalMocks, stubs }
    })
    expect(wrapper.text()).toContain('nav.sectionPlatformAdmin')
    expect(wrapper.text()).toContain('nav.platformTenants')
  })

  it('emits logout when the logout button is clicked', async () => {
    const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('logout')).toBeTruthy()
  })

  it('renders no emoji glyphs in any nav label (text-only, no decorative icons)', () => {
    const wrapper = mount(Sidebar, {
      props: { canViewUsers: true, canViewRoles: true, canViewWarehouses: true, canViewProducts: true, canViewPlatformAdmin: true },
      global: { mocks: globalMocks, stubs }
    })
    // eslint-disable-next-line no-misleading-character-class
    const emojiPattern = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}]/u
    expect(emojiPattern.test(wrapper.text())).toBe(false)
  })

  describe('menu search', () => {
    it('renders an empty search input with the expected placeholder', () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      const input = wrapper.find('input[role="searchbox"]')
      expect(input.exists()).toBe(true)
      expect(input.element.value).toBe('')
      expect(input.attributes('placeholder')).toBe('nav.searchPlaceholder')
    })

    it('filters items case-insensitively and trims whitespace', async () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      const input = wrapper.find('input[role="searchbox"]')

      await input.setValue('  PUTAWAY  ')

      expect(wrapper.text()).toContain('nav.putaway')
      expect(wrapper.text()).not.toContain('nav.receiving')
      expect(wrapper.text()).not.toContain('nav.qualityCheck')
    })

    it('keeps the section title visible and expanded above a matching child item', async () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      const input = wrapper.find('input[role="searchbox"]')

      await input.setValue('putaway')

      expect(wrapper.text()).toContain('nav.sectionInbound')
      expect(wrapper.text()).toContain('nav.putaway')
      expect(wrapper.text()).not.toContain('nav.receiving')
    })

    it('shows every authorized descendant when the section title itself matches', async () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      const input = wrapper.find('input[role="searchbox"]')

      await input.setValue('sectioninventory')

      expect(wrapper.text()).toContain('nav.stockInquiry')
      expect(wrapper.text()).toContain('nav.stockAdjustment')
      expect(wrapper.text()).toContain('nav.checkStock')
      expect(wrapper.text()).not.toContain('nav.receiving')
    })

    it('shows "No matching menus found" when nothing matches', async () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      const input = wrapper.find('input[role="searchbox"]')

      await input.setValue('zzz-nonexistent-menu')

      expect(wrapper.text()).toContain('nav.searchNoResults')
      expect(wrapper.find('.menu-item').exists()).toBe(false)
    })

    it('restores the full authorized menu when the search is cleared via the clear button', async () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      const input = wrapper.find('input[role="searchbox"]')

      await input.setValue('putaway')
      expect(wrapper.text()).not.toContain('nav.receiving')

      const clearBtn = wrapper.find('.clear-btn')
      expect(clearBtn.exists()).toBe(true)
      await clearBtn.trigger('mousedown')

      expect(input.element.value).toBe('')
      expect(wrapper.text()).toContain('nav.receiving')
      expect(wrapper.text()).toContain('nav.sectionOutbound')
    })

    it('restores the full authorized menu when Escape is pressed in the search input', async () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      const input = wrapper.find('input[role="searchbox"]')

      await input.setValue('putaway')
      await input.trigger('keydown.esc')

      expect(input.element.value).toBe('')
      expect(wrapper.text()).toContain('nav.receiving')
    })

    it('never surfaces an unauthorized section even if its title matches the search term', async () => {
      const wrapper = mount(Sidebar, {
        props: { canViewPlatformAdmin: false },
        global: { mocks: globalMocks, stubs }
      })
      const input = wrapper.find('input[role="searchbox"]')

      await input.setValue('platform')

      expect(wrapper.text()).not.toContain('nav.sectionPlatformAdmin')
      expect(wrapper.text()).not.toContain('nav.platformTenants')
      expect(wrapper.text()).toContain('nav.searchNoResults')
    })

    it('does not hide the clear button behind the no-emoji check (✕ only renders while searching)', async () => {
      const wrapper = mount(Sidebar, { global: { mocks: globalMocks, stubs } })
      expect(wrapper.find('.clear-btn').exists()).toBe(false)

      const input = wrapper.find('input[role="searchbox"]')
      await input.setValue('putaway')
      expect(wrapper.find('.clear-btn').exists()).toBe(true)
    })
  })
})

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
})

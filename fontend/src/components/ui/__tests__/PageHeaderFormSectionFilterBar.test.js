import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PageHeader from '../PageHeader.vue'
import FormSection from '../FormSection.vue'
import FilterBar from '../FilterBar.vue'

describe('PageHeader', () => {
  it('renders title/subtitle props and the actions slot', () => {
    const wrapper = mount(PageHeader, {
      props: { title: 'Receiving', subtitle: 'Manage inbound shipments' },
      slots: { actions: '<button>New</button>' }
    })
    expect(wrapper.text()).toContain('Receiving')
    expect(wrapper.text()).toContain('Manage inbound shipments')
    expect(wrapper.text()).toContain('New')
  })

  it('omits the subtitle paragraph entirely when none is given', () => {
    const wrapper = mount(PageHeader, { props: { title: 'Receiving' } })
    expect(wrapper.find('p').exists()).toBe(false)
  })
})

describe('FormSection', () => {
  it('renders a title and description plus slot content', () => {
    const wrapper = mount(FormSection, {
      props: { title: 'Header Info', description: 'Basic details' },
      slots: { default: '<div>field</div>' }
    })
    expect(wrapper.text()).toContain('Header Info')
    expect(wrapper.text()).toContain('Basic details')
    expect(wrapper.text()).toContain('field')
  })
})

describe('FilterBar', () => {
  it('applies the grid-cols class matching the cols prop', () => {
    const wrapper = mount(FilterBar, { props: { cols: 3 } })
    expect(wrapper.find('.grid').classes()).toContain('md:grid-cols-3')
  })

  it('defaults to 4 columns', () => {
    const wrapper = mount(FilterBar)
    expect(wrapper.find('.grid').classes()).toContain('md:grid-cols-4')
  })
})

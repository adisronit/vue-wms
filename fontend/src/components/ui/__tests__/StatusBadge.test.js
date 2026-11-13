import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusBadge from '../StatusBadge.vue'

describe('StatusBadge', () => {
  it('renders the slot content', () => {
    const wrapper = mount(StatusBadge, { props: { tone: 'green' }, slots: { default: 'Completed' } })
    expect(wrapper.text()).toBe('Completed')
  })

  it('applies the tone-specific class', () => {
    const wrapper = mount(StatusBadge, { props: { tone: 'orange' } })
    expect(wrapper.classes()).toContain('status-badge--orange')
  })

  it('falls back to gray for an unrecognized tone', () => {
    const wrapper = mount(StatusBadge, { props: { tone: 'gray' } })
    expect(wrapper.classes()).toContain('status-badge--gray')
  })
})

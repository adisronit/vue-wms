import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SortableTh from '../SortableTh.vue'

describe('SortableTh', () => {
  it('shows the unsorted icon and aria-sort="none" when not the active column', () => {
    const wrapper = mount(SortableTh, { props: { column: 'code', sortBy: 'name', sortDir: 'asc' } })
    expect(wrapper.text()).toContain('⇅')
    expect(wrapper.attributes('aria-sort')).toBe('none')
  })

  it('shows the ascending icon and aria-sort="ascending" when active and asc', () => {
    const wrapper = mount(SortableTh, { props: { column: 'code', sortBy: 'code', sortDir: 'asc' } })
    expect(wrapper.text()).toContain('▲')
    expect(wrapper.attributes('aria-sort')).toBe('ascending')
  })

  it('shows the descending icon and aria-sort="descending" when active and desc', () => {
    const wrapper = mount(SortableTh, { props: { column: 'code', sortBy: 'code', sortDir: 'desc' } })
    expect(wrapper.text()).toContain('▼')
    expect(wrapper.attributes('aria-sort')).toBe('descending')
  })

  it('emits sort with the column name on click', async () => {
    const wrapper = mount(SortableTh, { props: { column: 'code', sortBy: '', sortDir: 'asc' } })
    await wrapper.trigger('click')
    expect(wrapper.emitted('sort')[0]).toEqual(['code'])
  })

  it('emits sort on Enter and Space for keyboard users', async () => {
    const wrapper = mount(SortableTh, { props: { column: 'code', sortBy: '', sortDir: 'asc' } })
    await wrapper.trigger('keydown.enter')
    await wrapper.trigger('keydown.space')
    expect(wrapper.emitted('sort')).toHaveLength(2)
  })

  it('is keyboard-focusable', () => {
    const wrapper = mount(SortableTh, { props: { column: 'code', sortBy: '', sortDir: 'asc' } })
    expect(wrapper.attributes('tabindex')).toBe('0')
  })
})

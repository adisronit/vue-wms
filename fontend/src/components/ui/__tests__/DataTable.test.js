import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DataTable from '../DataTable.vue'

describe('DataTable', () => {
  it('shows the loading row and not the data slot while loading', () => {
    const wrapper = mount(DataTable, {
      props: { loading: true, isEmpty: false, loadingText: 'Loading...', colspan: 3 },
      slots: { head: '<tr><th>A</th></tr>', default: '<tr><td>should not render</td></tr>' }
    })
    expect(wrapper.text()).toContain('Loading...')
    expect(wrapper.text()).not.toContain('should not render')
  })

  it('shows the empty row when not loading and isEmpty is true', () => {
    const wrapper = mount(DataTable, {
      props: { loading: false, isEmpty: true, emptyText: 'No data found', colspan: 3 },
      slots: { default: '<tr><td>should not render</td></tr>' }
    })
    expect(wrapper.text()).toContain('No data found')
    expect(wrapper.text()).not.toContain('should not render')
  })

  it('renders the default slot rows once loaded and non-empty', () => {
    const wrapper = mount(DataTable, {
      props: { loading: false, isEmpty: false, colspan: 3 },
      slots: { default: '<tr><td>Row 1</td></tr>' }
    })
    expect(wrapper.text()).toContain('Row 1')
  })

  it('applies the configured minWidth to the table element', () => {
    const wrapper = mount(DataTable, { props: { minWidth: '1100px' } })
    expect(wrapper.find('table').attributes('style')).toContain('1100px')
  })
})

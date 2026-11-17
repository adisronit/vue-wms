import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ReportDataTable from '../ReportDataTable.vue'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key) }

describe('ReportDataTable', () => {
  it('shows the Reports loading text while loading, not the row slot', () => {
    const wrapper = mount(ReportDataTable, {
      props: { loading: true, colspan: 2 },
      slots: { head: '<tr><th>A</th></tr>', default: '<tr><td>should not render</td></tr>' },
      global: { mocks: globalMocks },
    })
    expect(wrapper.text()).toContain('reports.table.loading')
    expect(wrapper.text()).not.toContain('should not render')
  })

  it('shows the Reports empty text when isEmpty and not loading', () => {
    const wrapper = mount(ReportDataTable, {
      props: { loading: false, isEmpty: true, colspan: 2 },
      global: { mocks: globalMocks },
    })
    expect(wrapper.text()).toContain('reports.table.noData')
  })

  it('omits the pagination footer while loading or empty, even with a total', () => {
    const wrapper = mount(ReportDataTable, {
      props: { loading: true, total: 47, page: 1, limit: 20 },
      global: { mocks: globalMocks },
    })
    expect(wrapper.findComponent({ name: 'PaginationFooter' }).exists()).toBe(false)
  })

  it('renders the pagination footer once loaded with rows and a total', () => {
    const wrapper = mount(ReportDataTable, {
      props: { loading: false, isEmpty: false, total: 47, page: 2, limit: 20 },
      slots: { default: '<tr><td>Row</td></tr>' },
      global: { mocks: globalMocks },
    })
    const footer = wrapper.findComponent({ name: 'PaginationFooter' })
    expect(footer.exists()).toBe(true)
    expect(footer.props('totalPages')).toBe(3)
  })

  it('does not render a footer at all when total is null', () => {
    const wrapper = mount(ReportDataTable, {
      props: { loading: false, isEmpty: false },
      slots: { default: '<tr><td>Row</td></tr>' },
      global: { mocks: globalMocks },
    })
    expect(wrapper.findComponent({ name: 'PaginationFooter' }).exists()).toBe(false)
  })
})

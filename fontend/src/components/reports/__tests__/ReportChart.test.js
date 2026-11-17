import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ReportChart from '../ReportChart.vue'

const globalMocks = { $t: (key) => key }

// Only the no-data fallback is exercised here — mounting an actual
// chart.js canvas needs a real 2D rendering context jsdom doesn't
// provide, so that path is a browser/UAT concern, not a unit test one.
describe('ReportChart', () => {
  it('shows the no-data state when chartData is null', () => {
    const wrapper = mount(ReportChart, { props: { chartData: null }, global: { mocks: globalMocks } })
    expect(wrapper.text()).toContain('reports.chart.noData')
  })

  it('shows the no-data state when chartData has no labels', () => {
    const wrapper = mount(ReportChart, {
      props: { chartData: { labels: [], datasets: [] } },
      global: { mocks: globalMocks },
    })
    expect(wrapper.vm.hasData).toBe(false)
    expect(wrapper.text()).toContain('reports.chart.noData')
  })

  it('reports hasData true once given non-empty labels and datasets', () => {
    const wrapper = mount(ReportChart, {
      props: { chartData: { labels: ['A', 'B'], datasets: [{ data: [1, 2] }] } },
      global: { mocks: globalMocks },
    })
    expect(wrapper.vm.hasData).toBe(true)
  })
})

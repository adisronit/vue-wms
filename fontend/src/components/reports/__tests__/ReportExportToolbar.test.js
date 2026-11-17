import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ReportExportToolbar from '../ReportExportToolbar.vue'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key) }

describe('ReportExportToolbar', () => {
  it('emits the matching event for each export action when enabled', async () => {
    const wrapper = mount(ReportExportToolbar, { global: { mocks: globalMocks } })
    const byLabel = (label) => wrapper.findAll('button').find((b) => b.text() === label)

    await byLabel('reports.export.previewPdf').trigger('click')
    await byLabel('reports.export.downloadPdf').trigger('click')
    await byLabel('reports.export.print').trigger('click')
    await byLabel('reports.export.exportExcel').trigger('click')
    await byLabel('reports.export.exportCsv').trigger('click')

    expect(wrapper.emitted('preview-pdf')).toBeTruthy()
    expect(wrapper.emitted('download-pdf')).toBeTruthy()
    expect(wrapper.emitted('print')).toBeTruthy()
    expect(wrapper.emitted('export-excel')).toBeTruthy()
    expect(wrapper.emitted('export-csv')).toBeTruthy()
  })

  it('disables every button and never emits when disabled', async () => {
    const wrapper = mount(ReportExportToolbar, { props: { disabled: true }, global: { mocks: globalMocks } })
    const buttons = wrapper.findAll('button')
    buttons.forEach((b) => expect(b.attributes('disabled')).toBeDefined())

    await buttons[0].trigger('click')
    expect(wrapper.emitted('preview-pdf')).toBeFalsy()
  })

  it('shows the generated-at timestamp when provided', () => {
    const wrapper = mount(ReportExportToolbar, {
      props: { generatedAt: '2026-11-17 10:00' },
      global: { mocks: globalMocks },
    })
    expect(wrapper.text()).toContain('reports.export.generatedAt')
  })
})

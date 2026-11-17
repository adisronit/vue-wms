import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ReportDateTimeRangePicker from '../ReportDateTimeRangePicker.vue'

const globalMocks = { $t: (key) => key }

describe('ReportDateTimeRangePicker', () => {
  it('emits update:modelValue with the clicked preset, preserving any custom from/to', async () => {
    const wrapper = mount(ReportDateTimeRangePicker, {
      props: { modelValue: { preset: 'today', from: '2026-11-01T00:00', to: '2026-11-02T00:00' } },
      global: { mocks: globalMocks },
    })

    const buttons = wrapper.findAll('button')
    const last7 = buttons.find((b) => b.text() === 'reports.dateRange.last7Days')
    await last7.trigger('click')

    expect(wrapper.emitted('update:modelValue')[0]).toEqual([
      { preset: 'last7Days', from: '2026-11-01T00:00', to: '2026-11-02T00:00' },
    ])
  })

  it('only shows the custom from/to inputs when preset is custom', async () => {
    const wrapper = mount(ReportDateTimeRangePicker, {
      props: { modelValue: { preset: 'today', from: null, to: null } },
      global: { mocks: globalMocks },
    })
    expect(wrapper.find('input[type="datetime-local"]').exists()).toBe(false)

    await wrapper.setProps({ modelValue: { preset: 'custom', from: null, to: null } })
    expect(wrapper.findAll('input[type="datetime-local"]').length).toBe(2)
  })

  it('emits an updated custom field without clobbering the other one', async () => {
    const wrapper = mount(ReportDateTimeRangePicker, {
      props: { modelValue: { preset: 'custom', from: null, to: '2026-11-02T18:30' } },
      global: { mocks: globalMocks },
    })

    const fromInput = wrapper.findAll('input[type="datetime-local"]')[0]
    await fromInput.setValue('2026-11-01T08:00')

    expect(wrapper.emitted('update:modelValue')[0]).toEqual([
      { preset: 'custom', from: '2026-11-01T08:00', to: '2026-11-02T18:30' },
    ])
  })
})

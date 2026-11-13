import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import QualityCheckView from '../QualityCheckView.vue'

const globalMocks = { $t: (key, params) => (params ? key + JSON.stringify(params) : key) }

describe('QualityCheckView', () => {
  it('renders the existing mock QC rows unchanged', () => {
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    expect(wrapper.text()).toContain('QC-20260608-001')
    expect(wrapper.text()).toContain('Barcode Label Roll')
  })

  it('filters rows by keyword', async () => {
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    wrapper.vm.filters.keyword = 'RFID'
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('RFID Tag')
    expect(wrapper.text()).not.toContain('Barcode Label Roll')
  })

  it('shows a validation error instead of submitting when sample qty is missing', async () => {
    const wrapper = mount(QualityCheckView, { global: { mocks: globalMocks } })
    wrapper.vm.openQc(wrapper.vm.qcList[3])
    wrapper.vm.qcForm.sampleQty = ''
    await wrapper.vm.$nextTick()

    wrapper.vm.submitQc('PASS')
    await wrapper.vm.$nextTick()

    expect(wrapper.vm.formError).toBeTruthy()
    expect(wrapper.vm.selectedQc).not.toBeNull()
  })
})

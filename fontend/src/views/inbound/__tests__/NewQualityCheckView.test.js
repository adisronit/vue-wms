import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import NewQualityCheckView from '../NewQualityCheckView.vue'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v),
  $router: { push: vi.fn() }
}

function makeValid(wrapper) {
  // The default-mounted item (id 1, result PASS, no defectReason) is
  // already valid; only the two header fields are empty by default.
  wrapper.vm.form.receivingNo = 'RCV-20260608-001'
  wrapper.vm.form.warehouse = 'WH-01'
}

describe('NewQualityCheckView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts without throwing', () => {
    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    expect(wrapper.vm).toBeTruthy()
  })

  it('does not open the confirm dialog for an invalid (incomplete header) form', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })

    wrapper.vm.confirmQcClick()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(false)
    expect(alertSpy).not.toHaveBeenCalled()
  })

  it('opens the confirm dialog once the form is valid, without submitting yet', async () => {
    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    makeValid(wrapper)

    wrapper.vm.confirmQcClick()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(true)
    expect(globalMocks.$router.push).not.toHaveBeenCalled()
  })

  it('submitQc() closes the dialog, navigates away, and never uses window.alert', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const wrapper = mount(NewQualityCheckView, { global: { mocks: globalMocks } })
    makeValid(wrapper)
    wrapper.vm.showSubmitConfirm = true

    wrapper.vm.submitQc()
    await flushPromises()

    expect(wrapper.vm.showSubmitConfirm).toBe(false)
    expect(globalMocks.$router.push).toHaveBeenCalledWith('/inbound/quality-check')
    expect(alertSpy).not.toHaveBeenCalled()
  })
})

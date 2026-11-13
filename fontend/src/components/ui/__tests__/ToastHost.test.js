import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ToastHost from '../ToastHost.vue'
import { showToast, dismissToast, toastState } from '../../../services/toast'

afterEach(() => {
  toastState().toasts.splice(0)
})

describe('ToastHost', () => {
  it('renders a toast pushed via showToast', async () => {
    showToast('Receiving confirmed', 'success', 0)
    const wrapper = mount(ToastHost, { global: { mocks: { $t: (k) => k } } })
    expect(wrapper.text()).toContain('Receiving confirmed')
  })

  it('removes a toast when its dismiss button is clicked', async () => {
    const id = showToast('Something failed', 'error', 0)
    const wrapper = mount(ToastHost, { global: { mocks: { $t: (k) => k } } })
    expect(wrapper.text()).toContain('Something failed')

    await wrapper.find('button').trigger('click')
    expect(toastState().toasts.find((t) => t.id === id)).toBeUndefined()
    expect(wrapper.text()).not.toContain('Something failed')
  })

  it('auto-dismisses after the given duration', async () => {
    showToast('Auto dismiss me', 'info', 10)
    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(toastState().toasts.length).toBe(0)
  })
})

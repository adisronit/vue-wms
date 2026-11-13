import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfirmDialog from '../ConfirmDialog.vue'

describe('ConfirmDialog', () => {
  it('renders nothing when closed', () => {
    const wrapper = mount(ConfirmDialog, { props: { open: false, title: 'Cancel this?' } })
    expect(wrapper.html()).toBe('<!--v-if-->')
  })

  it('renders the title and message when open', () => {
    const wrapper = mount(ConfirmDialog, { props: { open: true, title: 'Cancel this?', message: 'This cannot be undone.' } })
    expect(wrapper.text()).toContain('Cancel this?')
    expect(wrapper.text()).toContain('This cannot be undone.')
  })

  it('emits confirm and cancel from their respective buttons', async () => {
    const wrapper = mount(ConfirmDialog, { props: { open: true, title: 'T', confirmLabel: 'Yes', cancelLabel: 'No' } })
    await wrapper.findAll('button')[0].trigger('click')
    expect(wrapper.emitted('cancel')).toBeTruthy()
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.emitted('confirm')).toBeTruthy()
  })

  it('shows the loading label and disables confirm while loading', () => {
    const wrapper = mount(ConfirmDialog, { props: { open: true, title: 'T', loading: true, loadingLabel: 'Working...' } })
    const confirmBtn = wrapper.findAll('button')[1]
    expect(confirmBtn.text()).toBe('Working...')
    expect(confirmBtn.attributes('disabled')).toBeDefined()
  })

  it('shows an error message when provided', () => {
    const wrapper = mount(ConfirmDialog, { props: { open: true, title: 'T', errorMessage: 'Failed to cancel' } })
    expect(wrapper.text()).toContain('Failed to cancel')
  })
})

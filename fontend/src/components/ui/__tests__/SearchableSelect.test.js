import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SearchableSelect from '../SearchableSelect.vue'

describe('SearchableSelect', () => {
  it('calls fetchOptions on focus and shows the resolved options', async () => {
    const fetchOptions = vi.fn().mockResolvedValue([{ value: '1', label: 'Alpha' }, { value: '2', label: 'Beta' }])
    const wrapper = mount(SearchableSelect, { props: { modelValue: null, fetchOptions } })

    await wrapper.find('input').trigger('focus')
    await flushPromises()

    expect(fetchOptions).toHaveBeenCalledWith('')
    expect(wrapper.text()).toContain('Alpha')
    expect(wrapper.text()).toContain('Beta')
  })

  it('debounces typed input before calling fetchOptions again', async () => {
    vi.useFakeTimers()
    const fetchOptions = vi.fn().mockResolvedValue([])
    const wrapper = mount(SearchableSelect, { props: { modelValue: null, fetchOptions, debounceMs: 300 } })

    await wrapper.find('input').trigger('focus')
    await flushPromises()
    fetchOptions.mockClear()

    const input = wrapper.find('input')
    await input.setValue('alp')
    expect(fetchOptions).not.toHaveBeenCalled()

    vi.advanceTimersByTime(300)
    await flushPromises()
    expect(fetchOptions).toHaveBeenCalledWith('alp')

    vi.useRealTimers()
  })

  it('emits update:modelValue and update:displayValue when an option is clicked', async () => {
    const fetchOptions = vi.fn().mockResolvedValue([{ value: '7', label: 'Seven' }])
    const wrapper = mount(SearchableSelect, { props: { modelValue: null, fetchOptions } })

    await wrapper.find('input').trigger('focus')
    await flushPromises()

    await wrapper.find('li.option').trigger('mousedown')

    expect(wrapper.emitted('update:modelValue')[0]).toEqual(['7'])
    expect(wrapper.emitted('update:displayValue')[0]).toEqual(['Seven'])
  })

  it('filters a static options array client-side instead of calling fetchOptions', async () => {
    const options = [{ value: '1', label: 'Apple' }, { value: '2', label: 'Banana' }]
    const wrapper = mount(SearchableSelect, { props: { modelValue: null, options } })

    await wrapper.find('input').trigger('focus')
    await wrapper.find('input').setValue('ban')

    expect(wrapper.text()).toContain('Banana')
    expect(wrapper.text()).not.toContain('Apple')
  })

  it('shows and wires the clear button when a value is selected', async () => {
    const wrapper = mount(SearchableSelect, { props: { modelValue: '1', displayValue: 'Alpha', options: [] } })

    const clearBtn = wrapper.find('.clear-btn')
    expect(clearBtn.exists()).toBe(true)

    await clearBtn.trigger('mousedown')

    expect(wrapper.emitted('update:modelValue')[0]).toEqual([null])
    expect(wrapper.emitted('update:displayValue')[0]).toEqual([''])
  })

  it('disables the input and hides the clear button when disabled', () => {
    const wrapper = mount(SearchableSelect, { props: { modelValue: '1', displayValue: 'Alpha', disabled: true, options: [] } })

    expect(wrapper.find('input').attributes('disabled')).toBeDefined()
    expect(wrapper.find('.clear-btn').exists()).toBe(false)
  })
})

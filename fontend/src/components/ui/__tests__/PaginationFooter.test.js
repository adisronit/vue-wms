import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PaginationFooter from '../PaginationFooter.vue'

const globalMocks = { $t: (key) => key }

function mountFooter(props) {
  return mount(PaginationFooter, {
    props: { page: 2, totalPages: 5, total: 47, limit: 10, ...props },
    global: { mocks: globalMocks }
  })
}

describe('PaginationFooter', () => {
  it('computes the showing range from page/limit/total', () => {
    const wrapper = mountFooter({ page: 2, limit: 10, total: 47 })
    expect(wrapper.vm.rangeStart).toBe(11)
    expect(wrapper.vm.rangeEnd).toBe(20)
  })

  it('clamps the range end to total on the last page', () => {
    const wrapper = mountFooter({ page: 5, limit: 10, total: 47 })
    expect(wrapper.vm.rangeEnd).toBe(47)
  })

  it('emits update:page with 1 when First is clicked', async () => {
    const wrapper = mountFooter({ page: 3 })
    const buttons = wrapper.findAll('button')
    await buttons[0].trigger('click')
    expect(wrapper.emitted('update:page')[0]).toEqual([1])
  })

  it('emits update:page with page-1 when Previous is clicked', async () => {
    const wrapper = mountFooter({ page: 3 })
    const buttons = wrapper.findAll('button')
    await buttons[1].trigger('click')
    expect(wrapper.emitted('update:page')[0]).toEqual([2])
  })

  it('emits update:page with page+1 when Next is clicked', async () => {
    const wrapper = mountFooter({ page: 3, totalPages: 5 })
    const buttons = wrapper.findAll('button')
    await buttons[2].trigger('click')
    expect(wrapper.emitted('update:page')[0]).toEqual([4])
  })

  it('emits update:page with totalPages when Last is clicked', async () => {
    const wrapper = mountFooter({ page: 3, totalPages: 5 })
    const buttons = wrapper.findAll('button')
    await buttons[3].trigger('click')
    expect(wrapper.emitted('update:page')[0]).toEqual([5])
  })

  it('disables First/Previous on the first page and Next/Last on the last page', () => {
    const first = mountFooter({ page: 1, totalPages: 5 })
    const firstButtons = first.findAll('button')
    expect(firstButtons[0].attributes('disabled')).toBeDefined()
    expect(firstButtons[1].attributes('disabled')).toBeDefined()
    expect(firstButtons[2].attributes('disabled')).toBeUndefined()
    expect(firstButtons[3].attributes('disabled')).toBeUndefined()

    const last = mountFooter({ page: 5, totalPages: 5 })
    const lastButtons = last.findAll('button')
    expect(lastButtons[0].attributes('disabled')).toBeUndefined()
    expect(lastButtons[1].attributes('disabled')).toBeUndefined()
    expect(lastButtons[2].attributes('disabled')).toBeDefined()
    expect(lastButtons[3].attributes('disabled')).toBeDefined()
  })

  it('disables all controls when disabled prop is true', () => {
    const wrapper = mountFooter({ disabled: true })
    wrapper.findAll('button').forEach((btn) => {
      expect(btn.attributes('disabled')).toBeDefined()
    })
    expect(wrapper.find('select').attributes('disabled')).toBeDefined()
  })

  it('emits update:limit when the rows-per-page select changes', async () => {
    const wrapper = mountFooter({ limit: 10, pageSizeOptions: [10, 25, 50, 100] })
    await wrapper.find('select').setValue('50')
    expect(wrapper.emitted('update:limit')[0]).toEqual([50])
  })
})

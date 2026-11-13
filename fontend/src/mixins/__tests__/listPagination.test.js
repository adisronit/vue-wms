import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import listPagination, { DEFAULT_PAGE_LIMIT, PAGE_SIZE_OPTIONS } from '../listPagination'

// A minimal host mirroring the real contract: declares sortBy/sortDir
// with view-specific defaults and implements fetchList(). The mock is
// kept as its own reference rather than read back off wrapper.vm —
// Vue 3 binds instance methods to the public proxy, which wraps the
// original vi.fn() in a plain bound function with no mock metadata.
function makeHost(fetchList) {
  return {
    mixins: [listPagination],
    data() {
      return { sortBy: 'code', sortDir: 'asc', totalPages: 5 }
    },
    methods: {
      fetchList
    },
    render() {
      return null
    }
  }
}

describe('listPagination mixin', () => {
  it('defaults to page 1 and the standard limit/options', () => {
    const wrapper = mount(makeHost(vi.fn()))
    expect(wrapper.vm.page).toBe(1)
    expect(wrapper.vm.limit).toBe(DEFAULT_PAGE_LIMIT)
    expect(wrapper.vm.pageSizeOptions).toEqual(PAGE_SIZE_OPTIONS)
  })

  it('goToPage clamps to [1, totalPages] and calls fetchList', async () => {
    const fetchList = vi.fn()
    const wrapper = mount(makeHost(fetchList))

    wrapper.vm.goToPage(3)
    expect(wrapper.vm.page).toBe(3)
    expect(fetchList).toHaveBeenCalledTimes(1)

    wrapper.vm.goToPage(999)
    expect(wrapper.vm.page).toBe(5)

    wrapper.vm.goToPage(-5)
    expect(wrapper.vm.page).toBe(1)
  })

  it('goToPage is a no-op when the target equals the current page', () => {
    const fetchList = vi.fn()
    const wrapper = mount(makeHost(fetchList))
    wrapper.vm.page = 2
    fetchList.mockClear()
    wrapper.vm.goToPage(2)
    expect(fetchList).not.toHaveBeenCalled()
  })

  it('changing limit resets to page 1 and refetches', async () => {
    const fetchList = vi.fn()
    const wrapper = mount(makeHost(fetchList))
    wrapper.vm.page = 3
    fetchList.mockClear()

    wrapper.vm.limit = 50
    await wrapper.vm.$nextTick()

    expect(wrapper.vm.page).toBe(1)
    expect(fetchList).toHaveBeenCalledTimes(1)
  })

  it('toggleSort on a new column sets it with the given default direction and resets to page 1', () => {
    const fetchList = vi.fn()
    const wrapper = mount(makeHost(fetchList))
    wrapper.vm.page = 2
    fetchList.mockClear()

    wrapper.vm.toggleSort('name', 'desc')

    expect(wrapper.vm.sortBy).toBe('name')
    expect(wrapper.vm.sortDir).toBe('desc')
    expect(wrapper.vm.page).toBe(1)
    expect(fetchList).toHaveBeenCalledTimes(1)
  })

  it('toggleSort on the already-active column flips direction', () => {
    const wrapper = mount(makeHost(vi.fn()))
    wrapper.vm.toggleSort('code')
    expect(wrapper.vm.sortDir).toBe('desc')
    wrapper.vm.toggleSort('code')
    expect(wrapper.vm.sortDir).toBe('asc')
  })

  it('resetToFirstPage resets to page 1 and refetches regardless of current page', () => {
    const fetchList = vi.fn()
    const wrapper = mount(makeHost(fetchList))
    wrapper.vm.page = 4
    fetchList.mockClear()
    wrapper.vm.resetToFirstPage()
    expect(wrapper.vm.page).toBe(1)
    expect(fetchList).toHaveBeenCalledTimes(1)
  })
})

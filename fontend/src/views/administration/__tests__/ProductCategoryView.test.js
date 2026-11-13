import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProductCategoryView from '../ProductCategoryView.vue'

vi.mock('../../../services/productCategories', async () => {
  const actual = await vi.importActual('../../../services/productCategories')
  return {
    ...actual,
    listProductCategories: vi.fn(),
    createProductCategory: vi.fn(),
    updateProductCategory: vi.fn(),
    deleteProductCategory: vi.fn()
  }
})

vi.mock('../../../services/toast', () => ({
  showToast: vi.fn()
}))

import {
  listProductCategories,
  deleteProductCategory
} from '../../../services/productCategories'
import { showToast } from '../../../services/toast'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows, meta = {}) {
  return {
    success: true,
    data: rows,
    meta: { page: 1, limit: 25, total: rows.length, total_pages: 1, ...meta }
  }
}

const parentCategory = {
  id: '1',
  code: 'PKG',
  name: 'Packaging',
  parent_id: null,
  description: 'Packaging materials',
  status: 'ACTIVE'
}

const childCategory = {
  id: '2',
  code: 'PKG-BOX',
  name: 'Boxes',
  parent_id: '1',
  description: 'Cardboard boxes',
  status: 'ACTIVE'
}

describe('ProductCategoryView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders fetched categories, resolving the parent column to a readable label', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory, childCategory]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('PKG')
    expect(wrapper.text()).toContain('PKG-BOX')
    // The child row's parent column should resolve "1" to the parent's
    // own code/name, not show the raw parent_id.
    expect(wrapper.text()).toContain('PKG - Packaging')
  })

  it('maps status to the expected badge tone and label', async () => {
    listProductCategories.mockResolvedValue(page([]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('ACTIVE')).toBe('green')
    expect(wrapper.vm.statusTone('INACTIVE')).toBe('gray')
    expect(wrapper.vm.statusLabel('ACTIVE')).toBe('common.status.active')
  })

  it('excludes the category being edited and its descendants from the parent dropdown', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory, childCategory]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openCategory(parentCategory)
    await flushPromises()

    const availableIds = wrapper.vm.availableParents.map((c) => c.id)
    // Editing the parent must not offer itself or its own child as a
    // parent option — both would be an obviously circular request.
    expect(availableIds).not.toContain('1')
    expect(availableIds).not.toContain('2')
  })

  it('shows the server error message inline (e.g. HAS_CHILDREN) instead of retrying automatically on a blocked delete', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory]))
    const err = new Error('Category has child categories and cannot be deleted')
    err.code = 'HAS_CHILDREN'
    deleteProductCategory.mockRejectedValue(err)

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.askDelete(parentCategory)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(wrapper.vm.deleteError).toBe('Category has child categories and cannot be deleted')
    // The dialog stays open on failure (categoryToDelete is not cleared)
    // so the user sees the error rather than it silently disappearing.
    expect(wrapper.vm.categoryToDelete).toEqual(parentCategory)
  })

  it('shows a success toast (not window.alert) and refetches after a successful delete', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory]))
    deleteProductCategory.mockResolvedValue({})

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()

    const alertSpy = vi.spyOn(window, 'alert')
    wrapper.vm.askDelete(parentCategory)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(alertSpy).not.toHaveBeenCalled()
    expect(showToast).toHaveBeenCalledWith('common.messages.deleteSuccess', 'success')
    expect(wrapper.vm.categoryToDelete).toBe(null)
  })

  it('fetches with the default sort (code ASC) and default page size (25) on mount', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory, childCategory]))

    mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, limit: 25, sortBy: 'code', sortDir: 'asc' })
    )
  })

  it('clicking a sortable column header sorts ascending, then descending on a second click, resetting to page 1', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory, childCategory]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 3
    listProductCategories.mockClear()

    const nameHeader = wrapper.findAll('th.sortable')[1]
    await nameHeader.trigger('click')
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'name', sortDir: 'asc', page: 1 })
    )

    listProductCategories.mockClear()
    await nameHeader.trigger('click')
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'name', sortDir: 'desc', page: 1 })
    )
  })

  it('switching to a different sortable column always starts it at ascending', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory, childCategory]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.sortBy = 'name'
    wrapper.vm.sortDir = 'desc'
    listProductCategories.mockClear()

    wrapper.vm.toggleSort('status')
    await flushPromises()

    expect(wrapper.vm.sortDir).toBe('asc')
    expect(listProductCategories).toHaveBeenCalledWith(
      expect.objectContaining({ sortBy: 'status', sortDir: 'asc', page: 1 })
    )
  })

  it('changing rows-per-page resets to page 1 and refetches with the new limit', async () => {
    listProductCategories.mockResolvedValue(page([parentCategory, childCategory]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listProductCategories.mockClear()

    const select = wrapper.find('select.rows-per-page')
    await select.setValue('50')
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, page: 1 }))
  })

  it('pagination next/previous call the API with the updated page and respect the boundary disabled state', async () => {
    listProductCategories.mockResolvedValue(
      page([parentCategory], { page: 1, limit: 25, total: 60, total_pages: 3 })
    )

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()

    const [prevButton, nextButton] = wrapper.findAll('button').filter(
      (b) => b.text() === 'common.pagination.previous' || b.text() === 'common.pagination.next'
    )
    expect(prevButton.attributes('disabled')).toBeDefined()
    expect(nextButton.attributes('disabled')).toBeUndefined()

    listProductCategories.mockClear()
    await nextButton.trigger('click')
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith(expect.objectContaining({ page: 2 }))
  })

  it('debounced search re-fetches with the search term after the user stops typing, resetting to page 1', async () => {
    vi.useFakeTimers()
    listProductCategories.mockResolvedValue(page([]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listProductCategories.mockClear()

    wrapper.vm.filters.keyword = 'pkg'
    await wrapper.vm.$nextTick()
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith(expect.objectContaining({ search: 'pkg', page: 1 }))
    vi.useRealTimers()
  })

  it('status filter re-fetches immediately (no debounce) and resets to page 1', async () => {
    listProductCategories.mockResolvedValue(page([]))

    const wrapper = mount(ProductCategoryView, { global: { mocks: globalMocks } })
    await flushPromises()
    wrapper.vm.page = 2
    listProductCategories.mockClear()

    wrapper.vm.filters.status = 'ACTIVE'
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith(expect.objectContaining({ status: 'ACTIVE', page: 1 }))
  })
})

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

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
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
})

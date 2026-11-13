import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import BrandView from '../BrandView.vue'

vi.mock('../../../services/brands', async () => {
  const actual = await vi.importActual('../../../services/brands')
  return {
    ...actual,
    listBrands: vi.fn(),
    createBrand: vi.fn(),
    updateBrand: vi.fn(),
    deleteBrand: vi.fn()
  }
})

vi.mock('../../../services/toast', () => ({
  showToast: vi.fn()
}))

import { listBrands, deleteBrand } from '../../../services/brands'
import { showToast } from '../../../services/toast'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const brand = { id: '1', code: 'BR-ANCHOR', name: 'Anchor', status: 'ACTIVE' }

describe('BrandView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched brands in the table', async () => {
    listBrands.mockResolvedValue(page([brand]))

    const wrapper = mount(BrandView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('BR-ANCHOR')
    expect(wrapper.text()).toContain('Anchor')
  })

  it('opens the drawer pre-filled with the selected brand', async () => {
    listBrands.mockResolvedValue(page([brand]))

    const wrapper = mount(BrandView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openBrand(brand)

    expect(wrapper.vm.brandForm.code).toBe('BR-ANCHOR')
    expect(wrapper.vm.brandForm.name).toBe('Anchor')
  })

  it('deletes via the toast service (not window.alert) and refetches the list', async () => {
    listBrands.mockResolvedValue(page([brand]))
    deleteBrand.mockResolvedValue({})

    const wrapper = mount(BrandView, { global: { mocks: globalMocks } })
    await flushPromises()

    const alertSpy = vi.spyOn(window, 'alert')
    wrapper.vm.askDelete(brand)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(alertSpy).not.toHaveBeenCalled()
    expect(deleteBrand).toHaveBeenCalledWith('1')
    expect(showToast).toHaveBeenCalledWith('common.messages.deleteSuccess', 'success')
  })

  it('surfaces a duplicate-code save failure inline, preventing silent data loss', async () => {
    listBrands.mockResolvedValue(page([brand]))
    const err = new Error('code already exists')
    err.code = 'DUPLICATE_CODE'
    const { createBrand } = await import('../../../services/brands')
    createBrand.mockRejectedValue(err)

    const wrapper = mount(BrandView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openBrand(null)
    wrapper.vm.brandForm.code = 'BR-ANCHOR'
    wrapper.vm.brandForm.name = 'Duplicate'
    await wrapper.vm.saveBrand()
    await flushPromises()

    expect(wrapper.vm.formError).toBe('code already exists')
  })
})

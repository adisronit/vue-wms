import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProductFamilyView from '../ProductFamilyView.vue'

vi.mock('../../../services/productFamilies', async () => {
  const actual = await vi.importActual('../../../services/productFamilies')
  return {
    ...actual,
    listProductFamilies: vi.fn(),
    createProductFamily: vi.fn(),
    updateProductFamily: vi.fn(),
    deleteProductFamily: vi.fn()
  }
})

vi.mock('../../../services/toast', () => ({
  showToast: vi.fn()
}))

import { listProductFamilies, deleteProductFamily } from '../../../services/productFamilies'
import { showToast } from '../../../services/toast'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const family = { id: '1', code: 'FAM-CHEDDAR', name: 'Cheddar Block', status: 'ACTIVE' }

describe('ProductFamilyView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mounts and renders the fetched families in the table', async () => {
    listProductFamilies.mockResolvedValue(page([family]))

    const wrapper = mount(ProductFamilyView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('FAM-CHEDDAR')
    expect(wrapper.text()).toContain('Cheddar Block')
  })

  it('opens the drawer pre-filled with the selected family', async () => {
    listProductFamilies.mockResolvedValue(page([family]))

    const wrapper = mount(ProductFamilyView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openFamily(family)

    expect(wrapper.vm.familyForm.code).toBe('FAM-CHEDDAR')
    expect(wrapper.vm.familyForm.name).toBe('Cheddar Block')
  })

  it('deletes via the toast service (not window.alert) and refetches the list', async () => {
    listProductFamilies.mockResolvedValue(page([family]))
    deleteProductFamily.mockResolvedValue({})

    const wrapper = mount(ProductFamilyView, { global: { mocks: globalMocks } })
    await flushPromises()

    const alertSpy = vi.spyOn(window, 'alert')
    wrapper.vm.askDelete(family)
    await wrapper.vm.confirmDelete()
    await flushPromises()

    expect(alertSpy).not.toHaveBeenCalled()
    expect(deleteProductFamily).toHaveBeenCalledWith('1')
    expect(showToast).toHaveBeenCalledWith('common.messages.deleteSuccess', 'success')
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProductMasterView from '../ProductMasterView.vue'

vi.mock('../../../services/products', async () => {
  const actual = await vi.importActual('../../../services/products')
  return { ...actual, listProducts: vi.fn(), createProduct: vi.fn(), updateProduct: vi.fn() }
})

vi.mock('../../../services/productCategories', async () => {
  const actual = await vi.importActual('../../../services/productCategories')
  return { ...actual, listProductCategories: vi.fn() }
})

import { listProducts } from '../../../services/products'
import { listProductCategories } from '../../../services/productCategories'

const globalMocks = {
  $t: (key) => key,
  $d: (v) => String(v)
}

function page(rows) {
  return { success: true, data: rows, meta: { page: 1, limit: 100, total: rows.length, total_pages: 1 } }
}

const activeProduct = {
  id: 1,
  sku: 'SKU-001',
  name: 'Widget',
  description: 'A widget',
  category: 'PACKAGING',
  base_uom: 'PCS',
  pack_size: 10,
  barcode: '885000001001',
  lot_required: false,
  serial_required: false,
  expiry_required: false,
  default_zone: 'A',
  min_stock: 0,
  max_stock: 100,
  status: 'ACTIVE'
}

const productCategory = {
  id: '5',
  code: 'PKG',
  name: 'Packaging',
  parent_id: null,
  description: '',
  status: 'ACTIVE'
}

describe('ProductMasterView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Every test mounts the component, which fetches the Product
    // Category dropdown options on mount — default this to an empty,
    // successful page so tests that don't care about categories aren't
    // forced to stub it individually.
    listProductCategories.mockResolvedValue(page([]))
  })

  // Regression test: the template calls statusLabel(product.status) for
  // every row. Before this refactor, no statusLabel() method existed on
  // the component (only getStatusClass/statusTone), so mounting with any
  // product row would throw `TypeError: statusLabel is not a function`
  // at render time. This asserts the mount succeeds and the status cell
  // renders the expected label.
  it('mounts without throwing and renders the correct status label for a product row', async () => {
    listProducts.mockResolvedValue(page([activeProduct]))

    let wrapper
    expect(() => {
      wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    }).not.toThrow()

    await flushPromises()

    expect(wrapper.text()).toContain('SKU-001')
    expect(wrapper.text()).toContain('Widget')

    // $t is mocked as an identity function, so statusLabel('ACTIVE')
    // resolves to the raw translation key under this mock.
    expect(wrapper.vm.statusLabel('ACTIVE')).toBe('common.status.active')
    expect(wrapper.text()).toContain('common.status.active')
  })

  it('renders the inactive status label for an inactive product row', async () => {
    listProducts.mockResolvedValue(page([{ ...activeProduct, id: 2, sku: 'SKU-002', status: 'INACTIVE' }]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusLabel('INACTIVE')).toBe('common.status.inactive')
    expect(wrapper.text()).toContain('common.status.inactive')
  })

  it('maps status to the expected badge tone', async () => {
    listProducts.mockResolvedValue(page([]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.statusTone('ACTIVE')).toBe('green')
    expect(wrapper.vm.statusTone('INACTIVE')).toBe('gray')
  })

  // Regression coverage for the new, additive category_id dropdown: it
  // must coexist with the pre-existing fixed category enum select, be
  // populated from GET /product-categories, and resolve a product's
  // category_id to a readable name in the list column.
  it('fetches product categories on mount and renders them as dropdown options', async () => {
    listProducts.mockResolvedValue(page([]))
    listProductCategories.mockResolvedValue(page([productCategory]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(listProductCategories).toHaveBeenCalledWith({ status: 'ACTIVE', limit: 100 })
    expect(wrapper.vm.productCategories).toEqual([productCategory])

    wrapper.vm.openProduct(null)
    await flushPromises()

    const options = wrapper.findAll('option').map((o) => o.text())
    expect(options).toContain('PKG - Packaging')
  })

  it('resolves a product row\'s category_id to its code/name in the table instead of showing the raw ID', async () => {
    listProducts.mockResolvedValue(page([{ ...activeProduct, category_id: '5' }]))
    listProductCategories.mockResolvedValue(page([productCategory]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.categoryName('5')).toBe('PKG - Packaging')
    expect(wrapper.text()).toContain('PKG - Packaging')
  })

  it('shows a placeholder for products without a category_id', async () => {
    listProducts.mockResolvedValue(page([activeProduct]))
    listProductCategories.mockResolvedValue(page([productCategory]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.vm.categoryName(null)).toBe('—')
    expect(wrapper.vm.categoryName('')).toBe('—')
  })

  it('preserves the legacy fixed category enum field alongside the new category_id in the save payload', async () => {
    listProducts.mockResolvedValue(page([]))
    listProductCategories.mockResolvedValue(page([productCategory]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    wrapper.vm.openProduct(null)
    wrapper.vm.productForm.sku = 'SKU-NEW'
    wrapper.vm.productForm.productName = 'New Widget'
    wrapper.vm.productForm.categoryId = '5'

    const { createProduct } = await import('../../../services/products')
    createProduct.mockResolvedValue({})

    await wrapper.vm.saveProduct()
    await flushPromises()

    expect(createProduct).toHaveBeenCalledWith(
      expect.objectContaining({ category: 'PACKAGING', category_id: '5' })
    )
  })
})

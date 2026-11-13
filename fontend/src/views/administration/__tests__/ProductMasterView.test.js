import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProductMasterView from '../ProductMasterView.vue'

vi.mock('../../../services/products', () => ({
  listProducts: vi.fn(),
  createProduct: vi.fn(),
  updateProduct: vi.fn(),
  uploadProductImage: vi.fn(),
  removeProductImage: vi.fn(),
  fetchProductImageBlob: vi.fn(),
  listProductBarcodes: vi.fn(),
  createProductBarcode: vi.fn(),
  updateProductBarcode: vi.fn(),
  deleteProductBarcode: vi.fn()
}))
vi.mock('../../../services/productCategories', () => ({
  listProductCategories: vi.fn(),
  getProductCategory: vi.fn()
}))
vi.mock('../../../services/brands', () => ({
  listBrands: vi.fn(),
  getBrand: vi.fn()
}))
vi.mock('../../../services/productFamilies', () => ({
  listProductFamilies: vi.fn(),
  getProductFamily: vi.fn()
}))
vi.mock('../../../services/suppliers', () => ({
  listSuppliers: vi.fn(),
  getSupplier: vi.fn()
}))
vi.mock('../../../services/warehouses', () => ({
  listWarehouses: vi.fn(),
  listZones: vi.fn(),
  getWarehouse: vi.fn()
}))

import { listProducts, createProduct, updateProduct, listProductBarcodes } from '../../../services/products'
import { listProductCategories, getProductCategory } from '../../../services/productCategories'
import { listBrands, getBrand } from '../../../services/brands'
import { listProductFamilies } from '../../../services/productFamilies'
import { listSuppliers } from '../../../services/suppliers'
import { listWarehouses } from '../../../services/warehouses'

const globalMocks = { $t: (key) => key, $d: (v) => String(v) }

function page(rows, meta = {}) {
  return { success: true, data: rows, meta: { page: 1, limit: 25, total: rows.length, total_pages: 1, ...meta } }
}

const apiProduct = {
  id: '1', sku: 'SKU-CHEDDAR', name: 'Cheddar Block 150g', name_th: null, description: '',
  category: 'PACKAGING', category_id: '10', brand_id: '20', family_id: null, variant_name: '150g Block',
  base_uom: 'G', pack_size: 150, pack_size_unit: 'G', barcode: null,
  lot_required: false, serial_required: false, expiry_required: true,
  storage_type: 'CHILLED', rotation_rule: 'FEFO',
  default_supplier_id: null, country_of_origin: 'New Zealand',
  preferred_warehouse_id: null, preferred_zone_id: null, default_zone: '',
  manufacturer: '', min_stock: 0, max_stock: 0,
  gross_weight: null, net_weight: null, weight_unit: '', length_mm: null, width_mm: null, height_mm: null,
  status: 'ACTIVE', has_image: false, image_url: null
}

function emptyPage() {
  return page([])
}

describe('ProductMasterView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listProducts.mockResolvedValue(emptyPage())
    listProductCategories.mockResolvedValue(emptyPage())
    listBrands.mockResolvedValue(emptyPage())
    listProductFamilies.mockResolvedValue(emptyPage())
    listSuppliers.mockResolvedValue(emptyPage())
    listWarehouses.mockResolvedValue(emptyPage())
    listProductBarcodes.mockResolvedValue(page([]))
    // Default resolutions for the by-ID label lookups resolveEditLabels
    // calls when opening an existing product — individual tests
    // override these where the resolved label itself is asserted on.
    getProductCategory.mockResolvedValue({ data: { code: 'CAT', name: 'Category' } })
    getBrand.mockResolvedValue({ data: { code: 'BR', name: 'Brand' } })
  })

  it('mounts and renders the fetched products in the table', async () => {
    listProducts.mockResolvedValue(page([
      { ...apiProductResponse(apiProduct) }
    ]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    expect(wrapper.text()).toContain('SKU-CHEDDAR')
    expect(wrapper.text()).toContain('Cheddar Block 150g')
  })

  it('opens the drawer with the Advanced section collapsed by default', async () => {
    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openProduct(null)
    await flushPromises()

    expect(wrapper.vm.advancedExpanded).toBe(false)
  })

  it('rejects Create when sku, name, or category_id is missing', async () => {
    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openProduct(null)

    await wrapper.vm.saveProduct()

    expect(wrapper.vm.fieldErrors.sku).toBeTruthy()
    expect(wrapper.vm.fieldErrors.name).toBeTruthy()
    expect(wrapper.vm.fieldErrors.categoryId).toBeTruthy()
    expect(createProduct).not.toHaveBeenCalled()
  })

  it('does not require category_id when editing a pre-existing product that never had one set', async () => {
    const legacyProduct = apiProductResponse({ ...apiProduct, category_id: null })
    listProducts.mockResolvedValue(page([legacyProduct]))
    updateProduct.mockResolvedValue({ data: legacyProduct })

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openProduct(wrapper.vm.products[0])
    await flushPromises()

    await wrapper.vm.saveProduct()
    await flushPromises()

    expect(wrapper.vm.fieldErrors.categoryId).toBeUndefined()
    expect(updateProduct).toHaveBeenCalled()
  })

  it('creates a new product and, if an image was picked before saving, uploads it once the new ID is known', async () => {
    const created = apiProductResponse({ ...apiProduct, id: '99' })
    createProduct.mockResolvedValue({ data: created })

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openProduct(null)

    wrapper.vm.productForm.sku = 'SKU-NEW'
    wrapper.vm.productForm.name = 'New Product'
    wrapper.vm.productForm.categoryId = '10'
    wrapper.vm.productForm.baseUom = 'PCS'

    const uploadPendingFile = vi.fn().mockResolvedValue()
    wrapper.vm.$refs.imageUpload.uploadPendingFile = uploadPendingFile

    await wrapper.vm.saveProduct()
    await flushPromises()

    expect(createProduct).toHaveBeenCalledWith(expect.objectContaining({ sku: 'SKU-NEW', name: 'New Product', category_id: '10' }))
    expect(uploadPendingFile).toHaveBeenCalledWith('99')
  })

  it('resolves category/brand display labels by ID when opening an existing product for edit', async () => {
    getProductCategory.mockResolvedValue({ data: { code: 'CAT-DAIRY', name: 'Dairy' } })
    getBrand.mockResolvedValue({ data: { code: 'BR-ANCHOR', name: 'Anchor' } })
    const existing = apiProductResponse(apiProduct)
    listProducts.mockResolvedValue(page([existing]))

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.openProduct(wrapper.vm.products[0])
    await flushPromises()

    expect(wrapper.vm.productForm.categoryLabel).toBe('CAT-DAIRY - Dairy')
    expect(wrapper.vm.productForm.brandLabel).toBe('BR-ANCHOR - Anchor')
  })

  it('tracks unsaved changes and shows the discard-confirmation dialog on Cancel', async () => {
    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openProduct(null)

    expect(wrapper.vm.isDirty()).toBe(false)

    wrapper.vm.productForm.name = 'Something typed'
    expect(wrapper.vm.isDirty()).toBe(true)

    wrapper.vm.attemptClose()
    expect(wrapper.vm.showUnsavedConfirm).toBe(true)
    expect(wrapper.vm.selectedProduct).not.toBeNull()
  })

  it('closes immediately on Cancel when nothing has changed', async () => {
    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openProduct(null)

    wrapper.vm.attemptClose()

    expect(wrapper.vm.showUnsavedConfirm).toBe(false)
    expect(wrapper.vm.selectedProduct).toBeNull()
  })

  it('forceClose discards changes and closes the drawer', async () => {
    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openProduct(null)
    wrapper.vm.productForm.name = 'Unsaved'
    wrapper.vm.showUnsavedConfirm = true

    wrapper.vm.forceClose()

    expect(wrapper.vm.selectedProduct).toBeNull()
    expect(wrapper.vm.showUnsavedConfirm).toBe(false)
  })

  it('maps status to the expected badge tone and label', () => {
    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    expect(wrapper.vm.statusTone('ACTIVE')).toBe('green')
    expect(wrapper.vm.statusTone('INACTIVE')).toBe('gray')
    expect(wrapper.vm.statusLabel('ACTIVE')).toBe('common.status.active')
  })

  it('surfaces a duplicate-SKU save failure inline', async () => {
    const err = new Error('sku already exists')
    err.code = 'DUPLICATE_SKU'
    createProduct.mockRejectedValue(err)

    const wrapper = mount(ProductMasterView, { global: { mocks: globalMocks } })
    await flushPromises()
    await wrapper.vm.openProduct(null)
    wrapper.vm.productForm.sku = 'SKU-DUP'
    wrapper.vm.productForm.name = 'Dup'
    wrapper.vm.productForm.categoryId = '10'

    await wrapper.vm.saveProduct()
    await flushPromises()

    expect(wrapper.vm.formError).toBe('sku already exists')
  })
})

// apiProductResponse mirrors what the backend's own has_image/image_url
// derived fields look like on Response — the frontend's mapProductFromApi
// expects exactly this snake_case shape.
function apiProductResponse(p) {
  return { ...p }
}

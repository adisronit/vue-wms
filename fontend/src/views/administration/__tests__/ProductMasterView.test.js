import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProductMasterView from '../ProductMasterView.vue'

vi.mock('../../../services/products', async () => {
  const actual = await vi.importActual('../../../services/products')
  return { ...actual, listProducts: vi.fn(), createProduct: vi.fn(), updateProduct: vi.fn() }
})

import { listProducts } from '../../../services/products'

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

describe('ProductMasterView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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
})

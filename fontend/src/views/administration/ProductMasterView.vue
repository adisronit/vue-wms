<template>
  <div class="space-y-6">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">Product Master</h1>
        <p class="text-slate-500">
          Manage product information, barcode mapping, UOM and storage rules
        </p>
      </div>

      <button
        @click="openProduct(null)"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        + New Product
      </button>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">Total Products</p>
        <h2 class="summary-value text-number">{{ products.length }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Active Products</p>
        <h2 class="summary-value text-green-600 text-number">{{ activeProductCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Lot Required</p>
        <h2 class="summary-value text-orange-600 text-number">{{ lotRequiredCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">Inactive</p>
        <h2 class="summary-value text-red-600 text-number">{{ inactiveProductCount }}</h2>
      </div>
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="filters.keyword"
          type="text"
          class="input"
          placeholder="Search SKU, product, barcode..."
        />

        <select v-model="filters.category" class="input">
          <option value="">All Categories</option>
          <option value="PACKAGING">Packaging</option>
          <option value="LABEL">Label</option>
          <option value="RFID">RFID</option>
          <option value="RAW_MATERIAL">Raw Material</option>
        </select>

        <select v-model="filters.status" class="input">
          <option value="">All Status</option>
          <option value="ACTIVE">Active</option>
          <option value="INACTIVE">Inactive</option>
        </select>

        <select v-model="filters.lotRequired" class="input">
          <option value="">Lot Rule</option>
          <option value="YES">Lot Required</option>
          <option value="NO">No Lot</option>
        </select>

        <button
          @click="clearFilter"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
        >
          Clear Filter
        </button>
      </div>
    </div>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1250px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">SKU</th>
              <th class="table-th">Product Name</th>
              <th class="table-th">Category</th>
              <th class="table-th">Base UOM</th>
              <th class="table-th text-right">Pack Size</th>
              <th class="table-th">Lot</th>
              <th class="table-th">Serial</th>
              <th class="table-th">Expiry</th>
              <th class="table-th">Default Zone</th>
              <th class="table-th">Status</th>
              <th class="table-th text-right">Action</th>
            </tr>
          </thead>

          <tbody>
            <tr v-if="loading">
              <td class="table-td" colspan="11">Loading products...</td>
            </tr>
            <tr v-else-if="products.length === 0">
              <td class="table-td" colspan="11">No products found.</td>
            </tr>
            <tr
              v-for="product in products"
              :key="product.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ product.sku }}
              </td>

              <td class="table-td">
                {{ product.productName }}
              </td>

              <td class="table-td">
                {{ product.category }}
              </td>

              <td class="table-td">
                {{ product.baseUom }}
              </td>

              <td class="table-td text-number">
                {{ product.packSize }}
              </td>

              <td class="table-td">
                <span :class="product.lotRequired ? 'badge-orange' : 'badge-gray'">
                  {{ product.lotRequired ? 'Yes' : 'No' }}
                </span>
              </td>

              <td class="table-td">
                <span :class="product.serialRequired ? 'badge-orange' : 'badge-gray'">
                  {{ product.serialRequired ? 'Yes' : 'No' }}
                </span>
              </td>

              <td class="table-td">
                <span :class="product.expiryRequired ? 'badge-orange' : 'badge-gray'">
                  {{ product.expiryRequired ? 'Yes' : 'No' }}
                </span>
              </td>

              <td class="table-td">
                {{ product.defaultZone }}
              </td>

              <td class="table-td">
                <span :class="getStatusClass(product.status)">
                  {{ product.status }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openProduct(product)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  Manage
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div
      v-if="selectedProduct"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-5xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ selectedProduct.id ? 'Product Detail' : 'New Product' }}
            </h2>
            <p class="text-slate-500">
              Product master, barcode mapping and inventory rules
            </p>
          </div>

          <button
            @click="selectedProduct = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
        </div>

        <div class="flex gap-2 mb-6 overflow-x-auto">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            @click="activeTab = tab.key"
            class="tab-button"
            :class="activeTab === tab.key ? 'tab-active' : ''"
          >
            {{ tab.label }}
          </button>
        </div>

        <div v-if="activeTab === 'INFO'" class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="label">SKU</label>
              <input v-model="productForm.sku" class="input" />
            </div>

            <div class="md:col-span-2">
              <label class="label">Product Name</label>
              <input v-model="productForm.productName" class="input" />
            </div>

            <div>
              <label class="label">Category</label>
              <select v-model="productForm.category" class="input">
                <option value="PACKAGING">Packaging</option>
                <option value="LABEL">Label</option>
                <option value="RFID">RFID</option>
                <option value="RAW_MATERIAL">Raw Material</option>
              </select>
            </div>

            <div>
              <label class="label">Base UOM</label>
              <select v-model="productForm.baseUom" class="input">
                <option value="PCS">PCS</option>
                <option value="BOX">BOX</option>
                <option value="ROLL">ROLL</option>
                <option value="KG">KG</option>
              </select>
            </div>

            <div>
              <label class="label">Status</label>
              <select v-model="productForm.status" class="input">
                <option value="ACTIVE">Active</option>
                <option value="INACTIVE">Inactive</option>
              </select>
            </div>

            <div>
              <label class="label">Barcode</label>
              <input v-model="productForm.barcode" class="input" placeholder="Primary barcode (optional)" />
            </div>
          </div>

          <div>
            <label class="label">Description</label>
            <textarea v-model="productForm.description" rows="3" class="input"></textarea>
          </div>
        </div>

        <div v-if="activeTab === 'BARCODE'" class="space-y-4">
          <div class="flex justify-between items-center">
            <h3 class="font-bold text-slate-800">Barcode Mapping</h3>

            <button
              @click="addBarcode"
              class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold"
            >
              + Add Barcode
            </button>
          </div>

          <div class="border border-slate-200 rounded-2xl overflow-hidden">
            <div class="w-full overflow-x-auto">
              <table class="min-w-[900px] w-full text-sm">
                <thead class="bg-slate-50 text-slate-500">
                  <tr>
                    <th class="table-th">Barcode Value</th>
                    <th class="table-th">Barcode Type</th>
                    <th class="table-th">UOM</th>
                    <th class="table-th text-right">Qty Per Barcode</th>
                    <th class="table-th">Primary</th>
                    <th class="table-th">Status</th>
                    <th class="table-th text-right">Action</th>
                  </tr>
                </thead>

                <tbody>
                  <tr
                    v-for="(barcode, index) in productForm.barcodes"
                    :key="barcode.id"
                    class="border-t hover:bg-orange-50"
                  >
                    <td class="table-td">
                      <input v-model="barcode.barcodeValue" class="table-input min-w-[200px]" />
                    </td>

                    <td class="table-td">
                      <select v-model="barcode.barcodeType" class="table-input min-w-[180px]">
                        <option value="SUPPLIER_BARCODE">Supplier Barcode</option>
                        <option value="INTERNAL_BARCODE">Internal Barcode</option>
                        <option value="BOX_CODE">Box Code</option>
                        <option value="QR_CODE">QR Code</option>
                      </select>
                    </td>

                    <td class="table-td">
                      <select v-model="barcode.uom" class="table-input">
                        <option value="PCS">PCS</option>
                        <option value="BOX">BOX</option>
                        <option value="ROLL">ROLL</option>
                        <option value="KG">KG</option>
                      </select>
                    </td>

                    <td class="table-td">
                      <input
                        v-model.number="barcode.qtyPerBarcode"
                        type="number"
                        class="table-input input-number"
                      />
                    </td>

                    <td class="table-td text-center">
                      <input
                        type="checkbox"
                        v-model="barcode.isPrimary"
                        class="checkbox-orange"
                      />
                    </td>

                    <td class="table-td">
                      <select v-model="barcode.status" class="table-input">
                        <option value="ACTIVE">Active</option>
                        <option value="INACTIVE">Inactive</option>
                      </select>
                    </td>

                    <td class="table-td text-right">
                      <button
                        @click="removeBarcode(index)"
                        class="text-red-600 hover:text-red-700 font-semibold"
                      >
                        Remove
                      </button>
                    </td>
                  </tr>

                  <tr v-if="productForm.barcodes.length === 0">
                    <td colspan="7" class="text-center py-10 text-slate-400">
                      No barcode mapping
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <div v-if="activeTab === 'RULES'" class="space-y-5">
          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <label class="checkbox-card">
              <input type="checkbox" v-model="productForm.lotRequired" />
              <div>
                <p class="font-semibold text-slate-800">Lot Required</p>
                <p class="text-sm text-slate-500">Require lot number for receiving and stock</p>
              </div>
            </label>

            <label class="checkbox-card">
              <input type="checkbox" v-model="productForm.serialRequired" />
              <div>
                <p class="font-semibold text-slate-800">Serial Required</p>
                <p class="text-sm text-slate-500">Require serial number per item</p>
              </div>
            </label>

            <label class="checkbox-card">
              <input type="checkbox" v-model="productForm.expiryRequired" />
              <div>
                <p class="font-semibold text-slate-800">Expiry Required</p>
                <p class="text-sm text-slate-500">Require expiry date control</p>
              </div>
            </label>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label class="label">Default Putaway Zone</label>
              <select v-model="productForm.defaultZone" class="input">
                <option value="A">Zone A</option>
                <option value="B">Zone B</option>
                <option value="C">Zone C</option>
              </select>
            </div>

            <div>
              <label class="label">Min Stock</label>
              <input v-model.number="productForm.minStock" type="number" class="input input-number" />
            </div>

            <div>
              <label class="label">Max Stock</label>
              <input v-model.number="productForm.maxStock" type="number" class="input input-number" />
            </div>
          </div>
        </div>

        <div v-if="activeTab === 'UOM'" class="space-y-4">
          <div class="flex justify-between items-center">
            <h3 class="font-bold text-slate-800">UOM Conversion</h3>

            <button
              @click="addUom"
              class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold"
            >
              + Add UOM
            </button>
          </div>

          <div
            v-for="(uom, index) in productForm.uomConversions"
            :key="uom.id"
            class="grid grid-cols-1 md:grid-cols-4 gap-4 border border-slate-200 rounded-2xl p-4"
          >
            <select v-model="uom.fromUom" class="input">
              <option value="BOX">BOX</option>
              <option value="ROLL">ROLL</option>
              <option value="PCS">PCS</option>
              <option value="KG">KG</option>
            </select>

            <input
              v-model.number="uom.qty"
              type="number"
              class="input input-number"
              placeholder="Qty"
            />

            <select v-model="uom.toUom" class="input">
              <option value="PCS">PCS</option>
              <option value="ROLL">ROLL</option>
              <option value="KG">KG</option>
            </select>

            <button
              @click="removeUom(index)"
              class="bg-red-50 hover:bg-red-100 text-red-600 rounded-xl font-semibold"
            >
              Remove
            </button>
          </div>
        </div>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mt-6">
          {{ formError }}
        </div>

        <div class="bg-white border-t border-slate-200 mt-8 pt-6">
          <div class="flex flex-col md:flex-row justify-end gap-3">
            <button
              @click="selectedProduct = null"
              class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
            >
              Cancel
            </button>

            <button
              @click="saveProduct"
              :disabled="saving"
              class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ saving ? 'Saving...' : 'Save Product' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { listProducts, createProduct, updateProduct } from '../../services/products'

let searchDebounce = null

// The real backend stores camelCase-free snake_case field names; the
// frontend's existing productForm/product objects use camelCase
// throughout (product.productName, product.baseUom, ...). These two
// mapping functions are the only place that boundary is crossed, so the
// rest of the component (and its template) is unchanged.
function mapProductFromApi(p) {
  return {
    id: p.id,
    sku: p.sku,
    productName: p.name,
    description: p.description,
    category: p.category,
    baseUom: p.base_uom,
    packSize: p.pack_size,
    barcode: p.barcode || '',
    lotRequired: p.lot_required,
    serialRequired: p.serial_required,
    expiryRequired: p.expiry_required,
    defaultZone: p.default_zone,
    minStock: p.min_stock,
    maxStock: p.max_stock,
    status: p.status
  }
}

function mapProductToApi(form) {
  return {
    sku: form.sku,
    name: form.productName,
    description: form.description,
    category: form.category,
    base_uom: form.baseUom,
    pack_size: form.packSize,
    barcode: form.barcode || null,
    lot_required: form.lotRequired,
    serial_required: form.serialRequired,
    expiry_required: form.expiryRequired,
    default_zone: form.defaultZone,
    min_stock: form.minStock,
    max_stock: form.maxStock,
    status: form.status
  }
}

export default {
  name: 'ProductMasterView',

  data() {
    return {
      selectedProduct: null,
      activeTab: 'INFO',
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      filters: {
        keyword: '',
        category: '',
        status: '',
        lotRequired: ''
      },
      tabs: [
        { key: 'INFO', label: 'Product Info' },
        { key: 'BARCODE', label: 'Barcode Mapping' },
        { key: 'RULES', label: 'Inventory Rules' },
        { key: 'UOM', label: 'UOM Conversion' }
      ],
      products: [],
      productForm: this.createEmptyProduct()
    }
  },

  computed: {
    activeProductCount() {
      return this.products.filter((p) => p.status === 'ACTIVE').length
    },
    inactiveProductCount() {
      return this.products.filter((p) => p.status === 'INACTIVE').length
    },
    lotRequiredCount() {
      return this.products.filter((p) => p.lotRequired).length
    }
  },

  watch: {
    'filters.category'() {
      this.fetchProducts()
    },
    'filters.status'() {
      this.fetchProducts()
    },
    'filters.lotRequired'() {
      this.fetchProducts()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchProducts(), 300)
    }
  },

  mounted() {
    this.fetchProducts()
  },

  methods: {
    async fetchProducts() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listProducts({
          search: this.filters.keyword,
          category: this.filters.category,
          status: this.filters.status,
          lotRequired: this.filters.lotRequired,
          limit: 100
        })
        this.products = res.data.map(mapProductFromApi)
      } catch (err) {
        this.listError = err.message || 'Failed to load products'
      } finally {
        this.loading = false
      }
    },

    createEmptyProduct() {
      return {
        sku: '',
        productName: '',
        category: 'PACKAGING',
        baseUom: 'PCS',
        status: 'ACTIVE',
        description: '',
        barcode: '',
        lotRequired: false,
        serialRequired: false,
        expiryRequired: false,
        defaultZone: 'A',
        minStock: 0,
        maxStock: 0,
        barcodes: [],
        uomConversions: []
      }
    },

    openProduct(product) {
      this.formError = ''
      this.selectedProduct = product || {}
      this.activeTab = 'INFO'

      // barcodes/uomConversions stay local-only decorative mock data —
      // the real backend only stores one primary barcode per product
      // (the new Barcode field above); the multi-row Barcode Mapping
      // and UOM Conversion tabs are not persisted this phase (see
      // PROJECT_STATUS.md).
      this.productForm = product
        ? {
            ...product,
            minStock: 10,
            maxStock: 1000,
            barcodes: [
              {
                id: 1,
                barcodeValue: product.barcode || '885000001001',
                barcodeType: 'SUPPLIER_BARCODE',
                uom: product.baseUom,
                qtyPerBarcode: 1,
                isPrimary: true,
                status: 'ACTIVE'
              }
            ],
            uomConversions: [
              {
                id: 1,
                fromUom: 'BOX',
                qty: product.packSize,
                toUom: product.baseUom
              }
            ]
          }
        : this.createEmptyProduct()
    },

    addBarcode() {
      this.productForm.barcodes.push({
        id: Date.now(),
        barcodeValue: '',
        barcodeType: 'SUPPLIER_BARCODE',
        uom: this.productForm.baseUom || 'PCS',
        qtyPerBarcode: 1,
        isPrimary: false,
        status: 'ACTIVE'
      })
    },

    removeBarcode(index) {
      this.productForm.barcodes.splice(index, 1)
    },

    addUom() {
      this.productForm.uomConversions.push({
        id: Date.now(),
        fromUom: 'BOX',
        qty: 1,
        toUom: this.productForm.baseUom || 'PCS'
      })
    },

    removeUom(index) {
      this.productForm.uomConversions.splice(index, 1)
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.category = ''
      this.filters.status = ''
      this.filters.lotRequired = ''
    },

    async saveProduct() {
      this.formError = ''

      if (!this.productForm.sku) {
        this.formError = 'SKU is required'
        return
      }

      if (!this.productForm.productName) {
        this.formError = 'Product Name is required'
        return
      }

      this.saving = true
      const payload = mapProductToApi(this.productForm)

      try {
        if (this.selectedProduct.id) {
          await updateProduct(this.selectedProduct.id, payload)
        } else {
          await createProduct(payload)
        }

        this.selectedProduct = null
        await this.fetchProducts()
      } catch (err) {
        this.formError = err.message || 'Failed to save product'
      } finally {
        this.saving = false
      }
    },

    getStatusClass(status) {
      if (status === 'ACTIVE') return 'badge-green'
      if (status === 'INACTIVE') return 'badge-gray'
      return 'badge-gray'
    }
  }
}
</script>

<style scoped>
.input {
  width: 100%;
  border: 1px solid #cbd5e1;
  border-radius: 12px;
  padding: 11px 14px;
  outline: none;
}

.input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: 600;
  font-size: 14px;
}

.summary-card {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 20px;
}

.summary-label {
  color: #64748b;
  font-size: 14px;
}

.summary-value {
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 8px;
}

.table-th {
  padding: 16px;
  font-weight: 700;
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  color: #475569;
}

.table-td {
  padding: 14px 16px;
  border-bottom: 1px solid #f1f5f9;
  vertical-align: middle;
}

.table-input {
  width: 100%;
  min-width: 120px;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 9px 10px;
  outline: none;
}

.input-number,
.text-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.checkbox-orange {
  width: 18px;
  height: 18px;
  accent-color: #ea580c;
}

.checkbox-card {
  display: flex;
  gap: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 16px;
  padding: 16px;
  cursor: pointer;
}

.checkbox-card:hover {
  background: #fff7ed;
  border-color: #fb923c;
}

.tab-button {
  padding: 10px 16px;
  border-radius: 12px;
  background: #f1f5f9;
  color: #475569;
  font-weight: 600;
  white-space: nowrap;
}

.tab-active {
  background: #ea580c;
  color: white;
}

.badge-gray {
  background: #f1f5f9;
  color: #475569;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-green {
  background: #dcfce7;
  color: #16a34a;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}
</style>
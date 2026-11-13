<template>
  <div class="space-y-6">
    <PageHeader :title="$t('productMaster.title')" :subtitle="$t('productMaster.subtitle')">
      <template #actions>
        <button
          @click="openProduct(null)"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('productMaster.newProductButton') }}
        </button>
      </template>
    </PageHeader>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('productMaster.summary.totalProducts') }}</p>
        <h2 class="summary-value text-number">{{ total }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('productMaster.summary.activeProducts') }}</p>
        <h2 class="summary-value text-green-600 text-number">{{ activeProductCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('productMaster.summary.lotRequired') }}</p>
        <h2 class="summary-value text-orange-600 text-number">{{ lotRequiredCount }}</h2>
      </div>
      <div class="summary-card">
        <p class="summary-label">{{ $t('productMaster.summary.inactive') }}</p>
        <h2 class="summary-value text-red-600 text-number">{{ inactiveProductCount }}</h2>
      </div>
    </div>

    <FilterBar :cols="5">
      <input
        v-model="filters.keyword"
        type="text"
        class="input"
        :placeholder="$t('productMaster.filters.searchPlaceholder')"
      />

      <SearchableSelect
        v-model="filters.categoryId"
        v-model:display-value="filters.categoryLabel"
        :fetch-options="fetchCategoryOptions"
        :placeholder="$t('productMaster.filters.categoryPlaceholder')"
      />

      <SearchableSelect
        v-model="filters.brandId"
        v-model:display-value="filters.brandLabel"
        :fetch-options="fetchBrandOptions"
        :placeholder="$t('productMaster.filters.brandPlaceholder')"
      />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('productMaster.filters.allStatus') }}</option>
        <option value="ACTIVE">{{ $t('common.status.active') }}</option>
        <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
      </select>

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <div v-if="listError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ listError }}
    </div>

    <DataTable
      :loading="loading"
      :is-empty="products.length === 0"
      :loading-text="$t('productMaster.loadingProducts')"
      :empty-text="$t('productMaster.noProducts')"
      :colspan="9"
      min-width="1100px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('productMaster.table.thumbnail') }}</th>
          <th class="table-th">{{ $t('productMaster.table.sku') }}</th>
          <th class="table-th">{{ $t('productMaster.table.productName') }}</th>
          <th class="table-th">{{ $t('productMaster.table.brand') }}</th>
          <th class="table-th">{{ $t('productMaster.table.category') }}</th>
          <th class="table-th">{{ $t('productMaster.table.baseUom') }}</th>
          <th class="table-th text-right">{{ $t('productMaster.table.packSize') }}</th>
          <th class="table-th">{{ $t('productMaster.table.status') }}</th>
          <th class="table-th text-right">{{ $t('productMaster.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="product in products" :key="product.id" class="border-t hover:bg-orange-50">
        <td class="table-td">
          <ProductThumbnail :product-id="product.id" :has-image="product.hasImage" :alt-text="product.name" />
        </td>
        <td class="table-td font-semibold text-slate-800">{{ product.sku }}</td>
        <td class="table-td">{{ product.name }}</td>
        <td class="table-td">{{ brandName(product.brandId) }}</td>
        <td class="table-td">{{ categoryName(product.categoryId) }}</td>
        <td class="table-td">{{ product.baseUom }}</td>
        <td class="table-td text-number">{{ product.packSize }}{{ product.packSizeUnit ? ' ' + product.packSizeUnit : '' }}</td>
        <td class="table-td">
          <StatusBadge :tone="statusTone(product.status)">{{ statusLabel(product.status) }}</StatusBadge>
        </td>
        <td class="table-td text-right">
          <button @click="openProduct(product)" class="text-orange-600 hover:text-orange-700 font-semibold">
            {{ $t('productMaster.manageButton') }}
          </button>
        </td>
      </tr>

      <template v-if="!loading && products.length > 0" #footer>
        <div class="flex flex-col md:flex-row items-center justify-between gap-3 px-5 py-4 text-sm text-slate-500">
          <span>
            {{ $t('common.pagination.showing') }} {{ rangeStart }} {{ $t('common.pagination.to') }} {{ rangeEnd }}
            {{ $t('common.pagination.of') }} {{ total }} {{ $t('common.pagination.results') }}
          </span>
          <div class="flex items-center gap-4">
            <label class="flex items-center gap-2">
              {{ $t('common.pagination.rowsPerPage') }}
              <select v-model.number="limit" class="rows-per-page">
                <option v-for="size in pageSizeOptions" :key="size" :value="size">{{ size }}</option>
              </select>
            </label>
            <div class="flex items-center gap-3">
              <button @click="page > 1 && goToPage(page - 1)" :disabled="page <= 1" class="page-btn">
                {{ $t('common.pagination.previous') }}
              </button>
              <span>{{ $t('common.pagination.page') }} {{ page }} {{ $t('common.pagination.of') }} {{ totalPages }}</span>
              <button @click="page < totalPages && goToPage(page + 1)" :disabled="page >= totalPages" class="page-btn">
                {{ $t('common.pagination.next') }}
              </button>
            </div>
          </div>
        </div>
      </template>
    </DataTable>

    <!-- Full-width responsive drawer with a sticky Save/Cancel footer -->
    <div v-if="selectedProduct" class="fixed inset-0 bg-black/40 flex justify-end z-50" @keydown.esc="attemptClose">
      <div class="drawer-panel bg-white h-full flex flex-col">
        <div class="flex items-center justify-between px-6 py-5 border-b border-slate-200">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ productForm.id ? $t('productMaster.drawer.detailTitle') : $t('productMaster.drawer.newTitle') }}
            </h2>
            <p class="text-slate-500 text-sm">{{ $t('productMaster.drawer.subtitle') }}</p>
          </div>
          <button @click="attemptClose" class="text-slate-500 hover:text-red-600 text-xl" :aria-label="$t('productMaster.cancelButton')">✕</button>
        </div>

        <div class="flex-1 overflow-y-auto px-6 py-6 space-y-6">
          <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
            {{ formError }}
          </div>

          <FormSection :title="$t('productMaster.sections.basic')">
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <label class="label">{{ $t('productMaster.basic.sku') }}*</label>
                <input v-model="productForm.sku" class="input" :class="{ 'input--error': fieldErrors.sku }" />
                <p v-if="fieldErrors.sku" class="field-error">{{ fieldErrors.sku }}</p>
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.nameEn') }}*</label>
                <input v-model="productForm.name" class="input" :class="{ 'input--error': fieldErrors.name }" />
                <p v-if="fieldErrors.name" class="field-error">{{ fieldErrors.name }}</p>
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.nameTh') }}</label>
                <input v-model="productForm.nameTh" class="input" />
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.category') }}*</label>
                <SearchableSelect
                  v-model="productForm.categoryId"
                  v-model:display-value="productForm.categoryLabel"
                  :fetch-options="fetchCategoryOptions"
                  :placeholder="$t('productMaster.basic.categoryPlaceholder')"
                />
                <p v-if="fieldErrors.categoryId" class="field-error">{{ fieldErrors.categoryId }}</p>
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.brand') }}</label>
                <SearchableSelect
                  v-model="productForm.brandId"
                  v-model:display-value="productForm.brandLabel"
                  :fetch-options="fetchBrandOptions"
                  :placeholder="$t('productMaster.basic.brandPlaceholder')"
                />
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.barcode') }}</label>
                <input v-model="productForm.barcode" class="input" :placeholder="$t('productMaster.basic.barcodePlaceholder')" />
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.packSize') }}</label>
                <input v-model.number="productForm.packSize" type="number" min="0" class="input input-number" />
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.packSizeUnit') }}</label>
                <select v-model="productForm.packSizeUnit" class="input">
                  <option value="">—</option>
                  <option v-for="u in uomOptions" :key="u" :value="u">{{ u }}</option>
                </select>
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.baseUom') }}*</label>
                <select v-model="productForm.baseUom" class="input" :class="{ 'input--error': fieldErrors.baseUom }">
                  <option v-for="u in uomOptions" :key="u" :value="u">{{ u }}</option>
                </select>
                <p v-if="fieldErrors.baseUom" class="field-error">{{ fieldErrors.baseUom }}</p>
              </div>

              <div>
                <label class="label">{{ $t('productMaster.basic.status') }}</label>
                <select v-model="productForm.status" class="input">
                  <option value="ACTIVE">{{ $t('common.status.active') }}</option>
                  <option value="INACTIVE">{{ $t('common.status.inactive') }}</option>
                </select>
              </div>
            </div>

            <div>
              <label class="label">{{ $t('productMaster.image.placeholder') }}</label>
              <ProductImageUpload
                ref="imageUpload"
                :product-id="productForm.id"
                :has-image="productForm.hasImage"
                :alt-text="productForm.name"
                @uploaded="onImageUploaded"
                @removed="onImageRemoved"
              />
            </div>
          </FormSection>

          <FormSection :title="$t('productMaster.sections.warehouse')">
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <label class="label">{{ $t('productMaster.warehouse.storageType') }}</label>
                <select v-model="productForm.storageType" class="input">
                  <option value="">{{ $t('productMaster.warehouse.noStorageTypeOption') }}</option>
                  <option value="AMBIENT">{{ $t('productMaster.warehouse.storageTypes.AMBIENT') }}</option>
                  <option value="CHILLED">{{ $t('productMaster.warehouse.storageTypes.CHILLED') }}</option>
                  <option value="FROZEN">{{ $t('productMaster.warehouse.storageTypes.FROZEN') }}</option>
                </select>
              </div>

              <div>
                <label class="label">{{ $t('productMaster.warehouse.rotationRule') }}</label>
                <select v-model="productForm.rotationRule" class="input">
                  <option value="">{{ $t('productMaster.warehouse.noRotationRuleOption') }}</option>
                  <option value="FIFO">{{ $t('productMaster.warehouse.rotationRules.FIFO') }}</option>
                  <option value="FEFO">{{ $t('productMaster.warehouse.rotationRules.FEFO') }}</option>
                </select>
              </div>

              <div>
                <label class="label">{{ $t('productMaster.warehouse.countryOfOrigin') }}</label>
                <input v-model="productForm.countryOfOrigin" class="input" :placeholder="$t('productMaster.warehouse.countryOfOriginPlaceholder')" />
              </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <label class="checkbox-card">
                <input type="checkbox" v-model="productForm.lotRequired" />
                <div>
                  <p class="font-semibold text-slate-800">{{ $t('productMaster.warehouse.lotTrackingTitle') }}</p>
                  <p class="text-sm text-slate-500">{{ $t('productMaster.warehouse.lotTrackingDesc') }}</p>
                </div>
              </label>
              <label class="checkbox-card">
                <input type="checkbox" v-model="productForm.expiryRequired" />
                <div>
                  <p class="font-semibold text-slate-800">{{ $t('productMaster.warehouse.expiryTrackingTitle') }}</p>
                  <p class="text-sm text-slate-500">{{ $t('productMaster.warehouse.expiryTrackingDesc') }}</p>
                </div>
              </label>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="label">{{ $t('productMaster.warehouse.defaultSupplier') }}</label>
                <SearchableSelect
                  v-model="productForm.defaultSupplierId"
                  v-model:display-value="productForm.defaultSupplierLabel"
                  :fetch-options="fetchSupplierOptions"
                  :placeholder="$t('productMaster.warehouse.defaultSupplierPlaceholder')"
                />
              </div>

              <div>
                <label class="label">{{ $t('productMaster.warehouse.preferredWarehouse') }}</label>
                <SearchableSelect
                  v-model="productForm.preferredWarehouseId"
                  v-model:display-value="productForm.preferredWarehouseLabel"
                  :fetch-options="fetchWarehouseOptions"
                  :placeholder="$t('productMaster.warehouse.preferredWarehousePlaceholder')"
                  @change="onWarehouseChanged"
                />
              </div>

              <div>
                <label class="label">{{ $t('productMaster.warehouse.preferredZone') }}</label>
                <SearchableSelect
                  v-model="productForm.preferredZoneId"
                  v-model:display-value="productForm.preferredZoneLabel"
                  :options="zoneOptions"
                  :disabled="!productForm.preferredWarehouseId"
                  :placeholder="$t('productMaster.warehouse.preferredZonePlaceholder')"
                />
              </div>
            </div>
          </FormSection>

          <FormSection>
            <template #default>
              <button type="button" class="advanced-toggle" @click="advancedExpanded = !advancedExpanded">
                <span>{{ $t('productMaster.sections.advanced') }}</span>
                <span class="advanced-toggle__chevron" :class="{ 'advanced-toggle__chevron--open': advancedExpanded }">▾</span>
              </button>
              <p class="advanced-hint">{{ $t('productMaster.sections.advancedHint') }}</p>

              <div v-show="advancedExpanded" class="space-y-5 mt-4">
                <div>
                  <label class="label">{{ $t('productMaster.advanced.description') }}</label>
                  <textarea v-model="productForm.description" rows="3" class="input"></textarea>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.manufacturer') }}</label>
                    <input v-model="productForm.manufacturer" class="input" />
                  </div>
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.variantName') }}</label>
                    <input v-model="productForm.variantName" class="input" />
                  </div>
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.family') }}</label>
                    <SearchableSelect
                      v-model="productForm.familyId"
                      v-model:display-value="productForm.familyLabel"
                      :fetch-options="fetchFamilyOptions"
                      :placeholder="$t('productMaster.advanced.familyPlaceholder')"
                    />
                  </div>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.grossWeight') }}</label>
                    <input v-model.number="productForm.grossWeight" type="number" min="0" step="0.001" class="input input-number" />
                  </div>
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.netWeight') }}</label>
                    <input v-model.number="productForm.netWeight" type="number" min="0" step="0.001" class="input input-number" />
                  </div>
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.weightUnit') }}</label>
                    <select v-model="productForm.weightUnit" class="input">
                      <option value="">—</option>
                      <option value="G">G</option>
                      <option value="KG">KG</option>
                    </select>
                  </div>
                </div>

                <div>
                  <label class="label">{{ $t('productMaster.advanced.dimensions') }}</label>
                  <div class="grid grid-cols-3 gap-3">
                    <input v-model.number="productForm.lengthMm" type="number" min="0" step="0.01" class="input input-number" placeholder="L" />
                    <input v-model.number="productForm.widthMm" type="number" min="0" step="0.01" class="input input-number" placeholder="W" />
                    <input v-model.number="productForm.heightMm" type="number" min="0" step="0.01" class="input input-number" placeholder="H" />
                  </div>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <label class="checkbox-card">
                    <input type="checkbox" v-model="productForm.serialRequired" />
                    <div>
                      <p class="font-semibold text-slate-800">{{ $t('productMaster.advanced.serialRequired') }}</p>
                      <p class="text-sm text-slate-500">{{ $t('productMaster.advanced.serialRequiredDesc') }}</p>
                    </div>
                  </label>
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.minStock') }}</label>
                    <input v-model.number="productForm.minStock" type="number" min="0" class="input input-number" />
                  </div>
                  <div>
                    <label class="label">{{ $t('productMaster.advanced.maxStock') }}</label>
                    <input v-model.number="productForm.maxStock" type="number" min="0" class="input input-number" />
                  </div>
                </div>

                <div>
                  <h4 class="font-semibold text-slate-800 mb-2">{{ $t('productMaster.advanced.barcodesTitle') }}</h4>
                  <ProductBarcodeList :product-id="productForm.id" />
                </div>

                <div class="coming-soon">
                  <h4 class="font-semibold text-slate-500 mb-1">{{ $t('productMaster.advanced.packagingConversionTitle') }}</h4>
                  <p class="text-sm text-slate-400">{{ $t('productMaster.advanced.packagingConversionComingSoon') }}</p>
                </div>
              </div>
            </template>
          </FormSection>
        </div>

        <div class="drawer-footer">
          <button @click="attemptClose" class="footer-btn footer-btn--secondary">
            {{ $t('productMaster.cancelButton') }}
          </button>
          <button @click="saveProduct" :disabled="saving" class="footer-btn footer-btn--primary">
            {{ saving ? $t('productMaster.saving') : $t('productMaster.saveProductButton') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="showUnsavedConfirm"
      :title="$t('productMaster.unsavedChanges.title')"
      :message="$t('productMaster.unsavedChanges.message')"
      :confirm-label="$t('productMaster.unsavedChanges.confirmButton')"
      :cancel-label="$t('productMaster.unsavedChanges.cancelButton')"
      danger
      @cancel="showUnsavedConfirm = false"
      @confirm="forceClose"
    />
  </div>
</template>

<script>
import {
  listProducts,
  createProduct,
  updateProduct
} from '../../services/products'
import { listProductCategories, getProductCategory } from '../../services/productCategories'
import { listBrands, getBrand } from '../../services/brands'
import { listProductFamilies, getProductFamily } from '../../services/productFamilies'
import { listSuppliers, getSupplier } from '../../services/suppliers'
import { listWarehouses, listZones, getWarehouse } from '../../services/warehouses'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import FormSection from '../../components/ui/FormSection.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import SearchableSelect from '../../components/ui/SearchableSelect.vue'
import ProductImageUpload from '../../components/products/ProductImageUpload.vue'
import ProductThumbnail from '../../components/products/ProductThumbnail.vue'
import ProductBarcodeList from '../../components/products/ProductBarcodeList.vue'

let searchDebounce = null
const PAGE_SIZE_OPTIONS = [10, 25, 50, 100]
const DEFAULT_LIMIT = 25
const UOM_OPTIONS = ['PCS', 'BOX', 'ROLL', 'KG', 'G', 'L', 'ML']

// The real backend stores snake_case field names; the frontend form
// object uses camelCase throughout. *Label fields (categoryLabel,
// brandLabel, ...) are UI-only — resolved display text for whatever
// *Id is currently set, never sent to the API — see
// resolveEditLabels() for how they're populated when opening an
// existing product.
function mapProductFromApi(p) {
  return {
    id: p.id,
    sku: p.sku,
    name: p.name,
    nameTh: p.name_th || '',
    description: p.description,
    category: p.category,
    categoryId: p.category_id || '',
    categoryLabel: '',
    brandId: p.brand_id || '',
    brandLabel: '',
    familyId: p.family_id || '',
    familyLabel: '',
    variantName: p.variant_name || '',
    baseUom: p.base_uom,
    packSize: p.pack_size,
    packSizeUnit: p.pack_size_unit || '',
    barcode: p.barcode || '',
    lotRequired: p.lot_required,
    serialRequired: p.serial_required,
    expiryRequired: p.expiry_required,
    storageType: p.storage_type || '',
    rotationRule: p.rotation_rule || '',
    defaultSupplierId: p.default_supplier_id || '',
    defaultSupplierLabel: '',
    countryOfOrigin: p.country_of_origin || '',
    preferredWarehouseId: p.preferred_warehouse_id || '',
    preferredWarehouseLabel: '',
    preferredZoneId: p.preferred_zone_id || '',
    preferredZoneLabel: '',
    defaultZone: p.default_zone || '',
    manufacturer: p.manufacturer || '',
    minStock: p.min_stock,
    maxStock: p.max_stock,
    grossWeight: p.gross_weight,
    netWeight: p.net_weight,
    weightUnit: p.weight_unit || '',
    lengthMm: p.length_mm,
    widthMm: p.width_mm,
    heightMm: p.height_mm,
    status: p.status,
    hasImage: p.has_image,
    imageUrl: p.image_url
  }
}

function mapProductToApi(form) {
  return {
    sku: form.sku,
    name: form.name,
    name_th: form.nameTh || null,
    description: form.description || '',
    category: form.category || '',
    category_id: form.categoryId || null,
    brand_id: form.brandId || null,
    family_id: form.familyId || null,
    variant_name: form.variantName || '',
    base_uom: form.baseUom,
    pack_size: form.packSize || 0,
    pack_size_unit: form.packSizeUnit || '',
    barcode: form.barcode || null,
    lot_required: form.lotRequired,
    serial_required: form.serialRequired,
    expiry_required: form.expiryRequired,
    storage_type: form.storageType || '',
    rotation_rule: form.rotationRule || '',
    default_supplier_id: form.defaultSupplierId || null,
    country_of_origin: form.countryOfOrigin || '',
    preferred_warehouse_id: form.preferredWarehouseId || null,
    preferred_zone_id: form.preferredZoneId || null,
    default_zone: form.defaultZone || '',
    manufacturer: form.manufacturer || '',
    min_stock: form.minStock || 0,
    max_stock: form.maxStock || 0,
    gross_weight: form.grossWeight || null,
    net_weight: form.netWeight || null,
    weight_unit: form.weightUnit || '',
    length_mm: form.lengthMm || null,
    width_mm: form.widthMm || null,
    height_mm: form.heightMm || null,
    status: form.status
  }
}

export default {
  name: 'ProductMasterView',

  components: {
    PageHeader, FilterBar, DataTable, StatusBadge, FormSection, ConfirmDialog,
    SearchableSelect, ProductImageUpload, ProductThumbnail, ProductBarcodeList
  },

  data() {
    return {
      selectedProduct: null,
      loading: false,
      listError: '',
      saving: false,
      formError: '',
      fieldErrors: {},
      advancedExpanded: false,
      showUnsavedConfirm: false,
      formSnapshot: '',
      uomOptions: UOM_OPTIONS,
      filters: { keyword: '', categoryId: '', categoryLabel: '', brandId: '', brandLabel: '', status: '' },
      pageSizeOptions: PAGE_SIZE_OPTIONS,
      page: 1,
      limit: DEFAULT_LIMIT,
      total: 0,
      totalPages: 1,
      products: [],
      productForm: this.createEmptyProduct(),
      zoneOptions: [],
      // Cached lookups for the list table's resolved Category/Brand
      // columns — fetched once on mount, same pattern
      // ProductCategoryView.vue's own parent-label resolution uses.
      categoriesCache: [],
      brandsCache: []
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
    },
    rangeStart() {
      return this.total === 0 ? 0 : (this.page - 1) * this.limit + 1
    },
    rangeEnd() {
      return Math.min(this.page * this.limit, this.total)
    }
  },

  watch: {
    'filters.categoryId'() {
      this.page = 1
      this.fetchProducts()
    },
    'filters.brandId'() {
      this.page = 1
      this.fetchProducts()
    },
    'filters.status'() {
      this.page = 1
      this.fetchProducts()
    },
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => {
        this.page = 1
        this.fetchProducts()
      }, 300)
    },
    limit() {
      this.page = 1
      this.fetchProducts()
    }
  },

  mounted() {
    this.fetchProducts()
    this.fetchLookupCaches()
  },

  beforeUnmount() {
    window.removeEventListener('beforeunload', this.handleBeforeUnload)
  },

  methods: {
    createEmptyProduct() {
      return {
        sku: '', name: '', nameTh: '', description: '',
        category: 'PACKAGING', categoryId: '', categoryLabel: '',
        brandId: '', brandLabel: '', familyId: '', familyLabel: '', variantName: '',
        baseUom: 'PCS', packSize: 1, packSizeUnit: '', barcode: '',
        lotRequired: false, serialRequired: false, expiryRequired: false,
        storageType: '', rotationRule: '',
        defaultSupplierId: '', defaultSupplierLabel: '', countryOfOrigin: '',
        preferredWarehouseId: '', preferredWarehouseLabel: '', preferredZoneId: '', preferredZoneLabel: '',
        defaultZone: '', manufacturer: '', minStock: 0, maxStock: 0,
        grossWeight: null, netWeight: null, weightUnit: '',
        lengthMm: null, widthMm: null, heightMm: null,
        status: 'ACTIVE', hasImage: false, imageUrl: null
      }
    },

    async fetchLookupCaches() {
      try {
        const [catRes, brandRes] = await Promise.all([
          listProductCategories({ status: 'ACTIVE', limit: 100 }),
          listBrands({ status: 'ACTIVE', limit: 100 })
        ])
        this.categoriesCache = catRes.data
        this.brandsCache = brandRes.data
      } catch {
        // Non-critical: the list table's resolved Category/Brand columns
        // simply show "—" this cycle — product load itself is unaffected.
      }
    },

    categoryName(categoryId) {
      if (!categoryId) return '—'
      const cat = this.categoriesCache.find((c) => c.id === categoryId)
      return cat ? `${cat.code} - ${cat.name}` : '—'
    },

    brandName(brandId) {
      if (!brandId) return '—'
      const b = this.brandsCache.find((x) => x.id === brandId)
      return b ? b.name : '—'
    },

    async fetchProducts() {
      this.loading = true
      this.listError = ''
      try {
        const res = await listProducts({
          search: this.filters.keyword,
          categoryId: this.filters.categoryId,
          brandId: this.filters.brandId,
          status: this.filters.status,
          page: this.page,
          limit: this.limit
        })
        this.products = res.data.map(mapProductFromApi)
        this.total = res.meta.total
        this.totalPages = res.meta.total_pages
      } catch (err) {
        this.listError = err.message || this.$t('productMaster.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    goToPage(page) {
      this.page = page
      this.fetchProducts()
    },

    clearFilter() {
      this.filters = { keyword: '', categoryId: '', categoryLabel: '', brandId: '', brandLabel: '', status: '' }
    },

    async fetchCategoryOptions(search) {
      const res = await listProductCategories({ search, status: 'ACTIVE', limit: 20 })
      return res.data.map((c) => ({ value: c.id, label: `${c.code} - ${c.name}` }))
    },

    async fetchBrandOptions(search) {
      const res = await listBrands({ search, status: 'ACTIVE', limit: 20 })
      return res.data.map((b) => ({ value: b.id, label: `${b.code} - ${b.name}` }))
    },

    async fetchFamilyOptions(search) {
      const res = await listProductFamilies({ search, status: 'ACTIVE', limit: 20 })
      return res.data.map((f) => ({ value: f.id, label: `${f.code} - ${f.name}` }))
    },

    async fetchSupplierOptions(search) {
      const res = await listSuppliers({ search, status: 'ACTIVE', limit: 20 })
      return res.data.map((s) => ({ value: s.id, label: `${s.code} - ${s.company_name}` }))
    },

    async fetchWarehouseOptions(search) {
      const res = await listWarehouses({ search, status: 'ACTIVE', limit: 20 })
      return res.data.map((w) => ({ value: w.id, label: `${w.code} - ${w.name}` }))
    },

    async onWarehouseChanged(opt) {
      this.productForm.preferredZoneId = ''
      this.productForm.preferredZoneLabel = ''
      await this.loadZoneOptions(opt ? opt.value : '')
    },

    async loadZoneOptions(warehouseId) {
      if (!warehouseId) {
        this.zoneOptions = []
        return
      }
      try {
        const res = await listZones(warehouseId)
        this.zoneOptions = res.data.map((z) => ({ value: z.id, label: `${z.code} - ${z.name}` }))
      } catch {
        this.zoneOptions = []
      }
    },

    // Opening an existing product only has IDs for category/brand/
    // family/supplier/warehouse/zone — this resolves the display text
    // for each via a direct by-ID lookup, rather than requiring every
    // list to already be fully loaded client-side.
    async resolveEditLabels(form) {
      const tasks = []
      if (form.categoryId) {
        tasks.push(
          getProductCategory(form.categoryId)
            .then((r) => { form.categoryLabel = `${r.data.code} - ${r.data.name}` })
            .catch(() => {})
        )
      }
      if (form.brandId) {
        tasks.push(
          getBrand(form.brandId)
            .then((r) => { form.brandLabel = `${r.data.code} - ${r.data.name}` })
            .catch(() => {})
        )
      }
      if (form.preferredWarehouseId) {
        tasks.push(
          getWarehouse(form.preferredWarehouseId)
            .then((r) => {
              form.preferredWarehouseLabel = `${r.data.code} - ${r.data.name}`
              return this.loadZoneOptions(form.preferredWarehouseId)
            })
            .then(() => {
              if (form.preferredZoneId) {
                const z = this.zoneOptions.find((o) => o.value === form.preferredZoneId)
                if (z) form.preferredZoneLabel = z.label
              }
            })
            .catch(() => {})
        )
      }
      if (form.defaultSupplierId) {
        tasks.push(
          getSupplier(form.defaultSupplierId)
            .then((r) => { form.defaultSupplierLabel = `${r.data.code} - ${r.data.company_name}` })
            .catch(() => {})
        )
      }
      if (form.familyId) {
        tasks.push(
          getProductFamily(form.familyId)
            .then((r) => { form.familyLabel = `${r.data.code} - ${r.data.name}` })
            .catch(() => {})
        )
      }
      await Promise.all(tasks)
    },

    async openProduct(product) {
      this.formError = ''
      this.fieldErrors = {}
      this.advancedExpanded = false
      this.zoneOptions = []
      this.selectedProduct = product || {}
      this.productForm = product ? mapProductFromApi(this.rawProductFor(product)) : this.createEmptyProduct()

      if (product) await this.resolveEditLabels(this.productForm)

      this.formSnapshot = JSON.stringify(this.productForm)
      window.addEventListener('beforeunload', this.handleBeforeUnload)
    },

    // The list's own `products` array already went through
    // mapProductFromApi once; re-running it on the same object is a
    // harmless no-op (every field is already in the expected shape), so
    // this just returns the row as-is rather than re-fetching GET
    // /products/:id purely to reopen the same data the list already has.
    rawProductFor(product) {
      return {
        id: product.id, sku: product.sku, name: product.name, name_th: product.nameTh,
        description: product.description, category: product.category, category_id: product.categoryId,
        brand_id: product.brandId, family_id: product.familyId, variant_name: product.variantName,
        base_uom: product.baseUom, pack_size: product.packSize, pack_size_unit: product.packSizeUnit,
        barcode: product.barcode, lot_required: product.lotRequired, serial_required: product.serialRequired,
        expiry_required: product.expiryRequired, storage_type: product.storageType, rotation_rule: product.rotationRule,
        default_supplier_id: product.defaultSupplierId, country_of_origin: product.countryOfOrigin,
        preferred_warehouse_id: product.preferredWarehouseId, preferred_zone_id: product.preferredZoneId,
        default_zone: product.defaultZone, manufacturer: product.manufacturer, min_stock: product.minStock,
        max_stock: product.maxStock, gross_weight: product.grossWeight, net_weight: product.netWeight,
        weight_unit: product.weightUnit, length_mm: product.lengthMm, width_mm: product.widthMm,
        height_mm: product.heightMm, status: product.status, has_image: product.hasImage, image_url: product.imageUrl
      }
    },

    isDirty() {
      return JSON.stringify(this.productForm) !== this.formSnapshot
    },

    attemptClose() {
      if (this.isDirty()) {
        this.showUnsavedConfirm = true
        return
      }
      this.forceClose()
    },

    forceClose() {
      this.showUnsavedConfirm = false
      this.selectedProduct = null
      window.removeEventListener('beforeunload', this.handleBeforeUnload)
    },

    handleBeforeUnload(event) {
      if (!this.isDirty()) return
      event.preventDefault()
      event.returnValue = ''
    },

    onImageUploaded(data) {
      this.productForm.hasImage = data.has_image
      this.productForm.imageUrl = data.image_url
      this.formSnapshot = JSON.stringify(this.productForm) // an image upload is already persisted; it shouldn't count toward "unsaved changes"
    },

    onImageRemoved(data) {
      this.productForm.hasImage = data.has_image
      this.productForm.imageUrl = data.image_url
      this.formSnapshot = JSON.stringify(this.productForm)
    },

    validateForm() {
      const errors = {}
      if (!this.productForm.sku) errors.sku = this.$t('productMaster.messages.skuRequired')
      if (!this.productForm.name) errors.name = this.$t('productMaster.messages.nameRequired')
      if (!this.productForm.baseUom) errors.baseUom = this.$t('productMaster.messages.baseUomRequired')
      // category_id is only strictly required for brand-new products —
      // mirrors the backend's own Create-only enforcement, so editing a
      // product created before this field existed never gets blocked on
      // an unrelated change.
      if (!this.productForm.id && !this.productForm.categoryId) {
        errors.categoryId = this.$t('productMaster.messages.categoryRequired')
      }
      this.fieldErrors = errors
      return Object.keys(errors).length === 0
    },

    async saveProduct() {
      this.formError = ''
      if (!this.validateForm()) {
        this.formError = this.$t('productMaster.messages.saveFailed')
        return
      }

      this.saving = true
      const payload = mapProductToApi(this.productForm)

      try {
        let newId = this.productForm.id
        if (newId) {
          await updateProduct(newId, payload)
        } else {
          const res = await createProduct(payload)
          newId = res.data.id
          if (this.$refs.imageUpload) {
            await this.$refs.imageUpload.uploadPendingFile(newId)
          }
        }

        window.removeEventListener('beforeunload', this.handleBeforeUnload)
        this.selectedProduct = null
        await this.fetchProducts()
      } catch (err) {
        this.formError = err.message || this.$t('productMaster.messages.saveFailed')
      } finally {
        this.saving = false
      }
    },

    statusTone(status) {
      if (status === 'ACTIVE') return 'green'
      if (status === 'INACTIVE') return 'gray'
      return 'gray'
    },

    statusLabel(status) {
      if (status === 'ACTIVE') return this.$t('common.status.active')
      if (status === 'INACTIVE') return this.$t('common.status.inactive')
      return status
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

.input--error {
  border-color: #fca5a5;
}

.field-error {
  color: #dc2626;
  font-size: 0.78rem;
  margin-top: 4px;
}

.label {
  display: block;
  margin-bottom: 8px;
  color: #334155;
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
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
  font-size: var(--fs-kpi);
  font-weight: var(--fw-kpi);
  color: #0f172a;
  margin-top: 8px;
  font-variant-numeric: tabular-nums;
}

.table-th {
  padding: 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  color: #475569;
}

.table-td {
  padding: 14px 16px;
  border-bottom: 1px solid #f1f5f9;
  vertical-align: middle;
}

.input-number,
.text-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
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

.rows-per-page {
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 6px 10px;
  outline: none;
  background: #fff;
  color: #334155;
}

.page-btn {
  padding: 6px 12px;
  border-radius: 8px;
  background: #f1f5f9;
  color: #334155;
  font-weight: 600;
}

.page-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* Wide, responsive drawer (not the previous max-w-5xl) with a sticky
   footer — the middle content area scrolls, the header/footer never
   do, so Save/Cancel stay reachable without scrolling through every
   section first. */
.drawer-panel {
  width: 100%;
  max-width: 1100px;
}

@media (max-width: 768px) {
  .drawer-panel {
    max-width: 100%;
  }
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 24px;
  border-top: 1px solid #e2e8f0;
  background: #fff;
  flex-shrink: 0;
}

.footer-btn {
  padding: 12px 24px;
  border-radius: 12px;
  font-weight: 600;
  cursor: pointer;
}

.footer-btn--secondary {
  background: #fff;
  color: #334155;
  border: 1px solid #cbd5e1;
}

.footer-btn--secondary:hover {
  background: #f1f5f9;
}

.footer-btn--primary {
  background: #ea580c;
  color: #fff;
  border: none;
}

.footer-btn--primary:hover:not(:disabled) {
  background: #c2410c;
}

.footer-btn--primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.advanced-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  background: none;
  border: none;
  cursor: pointer;
  font-weight: 700;
  font-size: 1rem;
  color: #0f172a;
  padding: 0;
}

.advanced-toggle__chevron {
  transition: transform 0.15s ease;
  color: #ea580c;
}

.advanced-toggle__chevron--open {
  transform: rotate(180deg);
}

.advanced-hint {
  color: #94a3b8;
  font-size: 0.8rem;
  margin-top: 4px;
}

.coming-soon {
  border: 1px dashed #e2e8f0;
  border-radius: 14px;
  padding: 14px 16px;
  background: #f8fafc;
}
</style>

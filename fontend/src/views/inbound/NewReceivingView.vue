<template>
  <div class="space-y-6">
    <PageHeader :title="$t('newReceiving.title')" :subtitle="$t('newReceiving.subtitle')">
      <template #actions>
        <button
          @click="$router.push('/inbound/receiving')"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newReceiving.backButton') }}
        </button>
      </template>
    </PageHeader>

    <!-- Wizard step indicator -->
    <div class="flex items-center gap-3">
      <div
        v-for="s in 3"
        :key="s"
        class="flex items-center gap-3"
        :class="s < 3 ? 'flex-1' : ''"
      >
        <button
          type="button"
          @click="goToStep(s)"
          :disabled="s > step && s > furthestStepReached"
          class="w-9 h-9 rounded-full flex items-center justify-center font-bold text-sm shrink-0"
          :class="step === s
            ? 'bg-orange-600 text-white'
            : s <= furthestStepReached
              ? 'bg-orange-100 text-orange-700'
              : 'bg-slate-100 text-slate-400'"
        >
          {{ s }}
        </button>
        <span
          class="text-sm font-semibold"
          :class="step === s ? 'text-orange-700' : 'text-slate-500'"
        >
          {{ $t(`newReceiving.wizard.step${s}`) }}
        </span>
        <div v-if="s < 3" class="flex-1 h-px bg-slate-200"></div>
      </div>
    </div>

    <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ formError }}
    </div>

    <!-- Step 1: Document -->
    <template v-if="step === 1">
      <FormSection :title="$t('newReceiving.documentInfo')">
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label class="label">{{ $t('newReceiving.fields.receivingType') }}</label>
            <select v-model="form.receivingType" class="input">
              <option value="PO">{{ $t('newReceiving.receivingTypeOptions.po') }}</option>
              <option value="ASN">{{ $t('newReceiving.receivingTypeOptions.asn') }}</option>
              <option value="RETURN">{{ $t('newReceiving.receivingTypeOptions.return') }}</option>
              <option value="TRANSFER">{{ $t('newReceiving.receivingTypeOptions.transfer') }}</option>
            </select>
          </div>

          <div>
            <label class="label">{{ $t('newReceiving.fields.referenceNo') }}</label>
            <input
              v-model="form.referenceNo"
              type="text"
              class="input"
              :placeholder="$t('newReceiving.fields.referenceNoPlaceholder')"
            />
          </div>

          <div>
            <label class="label">{{ $t('newReceiving.fields.receivingDate') }}</label>
            <input
              v-model="form.receivingDate"
              type="date"
              class="input"
            />
          </div>

          <div>
            <label class="label">{{ $t('newReceiving.fields.supplier') }}</label>
            <SearchableSelect
              v-model="form.supplierId"
              v-model:display-value="supplierDisplayName"
              :fetch-options="fetchSupplierOptions"
              :placeholder="$t('newReceiving.fields.selectSupplier')"
            />
          </div>

          <div>
            <label class="label">{{ $t('newReceiving.fields.vendor') }}</label>
            <input
              v-model="form.vendor"
              type="text"
              class="input"
              :placeholder="$t('newReceiving.fields.vendorPlaceholder')"
            />
          </div>

          <div>
            <label class="label">{{ $t('newReceiving.fields.warehouse') }}</label>
            <select v-model="form.warehouseId" class="input">
              <option value="">{{ $t('newReceiving.fields.selectWarehouse') }}</option>
              <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
            </select>
          </div>

          <div>
            <label class="label">{{ $t('newReceiving.fields.stagingLocation') }}</label>
            <select v-model="form.stagingLocationId" class="input" :disabled="!form.warehouseId">
              <option value="">{{ $t('newReceiving.fields.selectStagingLocation') }}</option>
              <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.code }}</option>
            </select>
          </div>
        </div>

        <div class="mt-4">
          <label class="label">{{ $t('newReceiving.fields.remark') }}</label>
          <textarea
            v-model="form.remark"
            rows="3"
            class="input"
            :placeholder="$t('newReceiving.fields.remarkPlaceholder')"
          ></textarea>
        </div>
      </FormSection>

      <FormSection :title="$t('newReceiving.logistics.title')">
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label class="label">{{ $t('newReceiving.logistics.invoiceNo') }}</label>
            <input v-model="form.invoiceNo" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.deliveryNoteNo') }}</label>
            <input v-model="form.deliveryNoteNo" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.currency') }}</label>
            <input v-model="form.currency" type="text" class="input" placeholder="THB" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.containerNo') }}</label>
            <input v-model="form.containerNo" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.sealNo') }}</label>
            <input v-model="form.sealNo" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.transportCompany') }}</label>
            <input v-model="form.transportCompany" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.driverName') }}</label>
            <input v-model="form.driverName" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.vehicleRegNo') }}</label>
            <input v-model="form.vehicleRegNo" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.dockDoor') }}</label>
            <input v-model="form.dockDoor" type="text" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.arrivalTime') }}</label>
            <input v-model="form.arrivalTime" type="datetime-local" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.unloadingStartTime') }}</label>
            <input v-model="form.unloadingStartTime" type="datetime-local" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('newReceiving.logistics.unloadingEndTime') }}</label>
            <input v-model="form.unloadingEndTime" type="datetime-local" class="input" />
          </div>
        </div>
      </FormSection>

      <FormSection>
        <div class="flex justify-end gap-3">
          <button
            @click="saveDraft"
            :disabled="submitting"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('newReceiving.actions.saveDraft') }}
          </button>
          <button
            @click="goNextFromDocument"
            class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold"
          >
            {{ $t('newReceiving.wizard.next') }}
          </button>
        </div>
      </FormSection>
    </template>

    <!-- Step 2: Scan & Count -->
    <template v-if="step === 2">
    <FormSection>
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">
            {{ $t('newReceiving.itemsSection.title') }}
          </h2>
          <p class="text-slate-500 text-sm">
            {{ $t('newReceiving.itemsSection.subtitle') }}
          </p>
        </div>

        <button
          @click="addItem"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newReceiving.itemsSection.addItem') }}
        </button>
      </div>

      <!-- Barcode Scan -->
      <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5 mb-5">
        <label class="label">{{ $t('newReceiving.scan.label') }}</label>

        <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
          <input
            v-model="scanText"
            type="text"
            class="input bg-white"
            :placeholder="$t('newReceiving.scan.placeholder')"
            @keyup.enter="scanItem"
          />

          <input
            v-model.number="scanQty"
            type="number"
            class="input bg-white"
            :placeholder="$t('newReceiving.scan.qtyPlaceholder')"
          />

          <select v-model="scanLocationId" class="input bg-white">
            <option value="">{{ $t('newReceiving.scan.selectLocation') }}</option>
            <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.code }}</option>
          </select>

          <button
            @click="scanItem"
            :disabled="scanning"
            class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60"
          >
            {{ scanning ? $t('newReceiving.scan.lookingUp') : $t('newReceiving.scan.addScan') }}
          </button>
        </div>
      </div>

      <!-- Item Table -->
      <DataTable
        :is-empty="items.length === 0"
        :empty-text="$t('newReceiving.table.noItems')"
        :colspan="10"
        min-width="1200px"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('newReceiving.table.sku') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.productName') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.barcode') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.expectedQty') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.receiveQty') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.uom') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.lotNo') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.expiryDate') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.location') }}</th>
            <th class="table-th text-right">{{ $t('newReceiving.table.action') }}</th>
          </tr>
        </template>

        <tr
          v-for="(item, index) in items"
          :key="item.id"
          class="border-t hover:bg-orange-50"
        >
          <td class="table-td">
            <input
              v-model="item.sku"
              class="table-input"
              :class="{ 'border-red-400': item.sku && !item.productId }"
              :placeholder="$t('newReceiving.table.skuPlaceholder')"
              @blur="lookupItemProduct(item)"
              @keyup.enter="lookupItemProduct(item)"
            />
          </td>

          <td class="table-td">
            <input v-model="item.productName" class="table-input" readonly />
          </td>

          <td class="table-td">
            <input v-model="item.barcode" class="table-input" readonly />
          </td>

          <td class="table-td">
            <input v-model.number="item.expectedQty" type="number" class="table-input input-number" />
          </td>

          <td class="table-td">
            <input v-model.number="item.receiveQty" type="number" class="table-input input-number" />
          </td>

          <td class="table-td">
            <select v-model="item.uom" class="table-input">
              <option value="PCS">PCS</option>
              <option value="BOX">BOX</option>
              <option value="ROLL">ROLL</option>
              <option value="KG">KG</option>
            </select>
          </td>

          <td class="table-td">
            <input v-model="item.lotNo" class="table-input" />
          </td>

          <td class="table-td">
            <input v-model="item.expiryDate" type="date" class="table-input" />
          </td>

          <td class="table-td">
            <select v-model="item.locationId" class="table-input">
              <option value="">{{ $t('newReceiving.scan.selectLocation') }}</option>
              <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.code }}</option>
            </select>
          </td>

          <td class="table-td text-right whitespace-nowrap">
            <button
              @click="toggleSplits(item)"
              class="text-orange-600 hover:text-orange-700 font-semibold mr-3"
            >
              {{ item.splitsOpen ? $t('newReceiving.splits.hide') : $t('newReceiving.splits.manage') }}
              <span v-if="item.splits.length">({{ item.splits.length }})</span>
            </button>
            <button
              @click="removeItem(index)"
              class="text-red-600 hover:text-red-700 font-semibold"
            >
              {{ $t('newReceiving.table.remove') }}
            </button>
          </td>
        </tr>

        <!-- Lot/Expiry/Pallet splits for this line — when present, these
             (not the line's own single lot_no/expiry_date/location above)
             are what gets sent to Confirm; each split's quantity must sum
             to the line's own Receive Qty. -->
        <tr v-for="item in itemsWithOpenSplits" :key="'splits-' + item.id" class="border-t bg-slate-50">
          <td colspan="10" class="p-4">
            <div class="flex items-center justify-between mb-3">
              <p class="text-sm font-semibold text-slate-700">
                {{ $t('newReceiving.splits.title') }}
              </p>
              <button
                @click="addSplit(item)"
                class="bg-orange-600 hover:bg-orange-700 text-white px-3 py-2 rounded-lg text-sm font-semibold"
              >
                {{ $t('newReceiving.splits.addSplit') }}
              </button>
            </div>

            <p v-if="item.splits.length === 0" class="text-sm text-slate-500">
              {{ $t('newReceiving.splits.none') }}
            </p>

            <div v-else class="space-y-3">
              <div
                v-for="(split, sIdx) in item.splits"
                :key="sIdx"
                class="bg-white border border-slate-200 rounded-xl p-3 space-y-2"
              >
                <div class="grid grid-cols-2 md:grid-cols-6 gap-2 items-center">
                  <input v-model.number="split.quantity" type="number" class="table-input input-number" :placeholder="$t('newReceiving.splits.qty')" />
                  <input v-model="split.lotNo" class="table-input" :placeholder="$t('newReceiving.table.lotNo')" />
                  <input v-model="split.mfgDate" type="date" class="table-input" :title="$t('newReceiving.review.mfgDate')" />
                  <input v-model="split.expiryDate" type="date" class="table-input" :title="$t('newReceiving.table.expiryDate')" />
                  <input v-model="split.palletId" class="table-input" :placeholder="$t('newReceiving.splits.palletId')" />
                  <input v-model.number="split.cartonCount" type="number" class="table-input input-number" :placeholder="$t('newReceiving.splits.cartonCount')" />
                </div>
                <div class="grid grid-cols-2 md:grid-cols-5 gap-2 items-center">
                  <select v-model="split.locationId" class="table-input">
                    <option value="">{{ $t('newReceiving.scan.selectLocation') }}</option>
                    <option v-for="l in locations" :key="l.id" :value="l.id">{{ l.code }}</option>
                  </select>
                  <input
                    v-model.number="split.temperatureC"
                    type="number"
                    step="0.1"
                    class="table-input input-number"
                    :placeholder="$t('newReceiving.review.temperature') + ' (°C)'"
                  />
                  <input v-model="split.countryOfOrigin" class="table-input" :placeholder="$t('newReceiving.review.countryOfOrigin')" />
                  <input v-model="split.conditionNote" class="table-input md:col-span-1" :placeholder="$t('newReceiving.review.condition')" />
                  <button @click="removeSplit(item, sIdx)" class="text-red-600 hover:text-red-700 font-semibold text-sm text-right">
                    {{ $t('newReceiving.table.remove') }}
                  </button>
                </div>
              </div>

              <p
                class="text-sm font-semibold"
                :class="splitQtySum(item) === Number(item.receiveQty || 0) ? 'text-green-600' : 'text-red-600'"
              >
                {{ $t('newReceiving.splits.sumOfQty', { sum: splitQtySum(item), total: Number(item.receiveQty || 0) }) }}
              </p>
            </div>
          </td>
        </tr>
      </DataTable>
    </FormSection>

      <FormSection>
        <div class="flex justify-between gap-3">
          <button
            @click="step = 1"
            class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
          >
            {{ $t('newReceiving.wizard.back') }}
          </button>
          <div class="flex gap-3">
            <button
              @click="saveDraft"
              :disabled="submitting"
              class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ $t('newReceiving.actions.saveDraft') }}
            </button>
            <button
              @click="goNextFromItems"
              class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold"
            >
              {{ $t('newReceiving.wizard.next') }}
            </button>
          </div>
        </div>
      </FormSection>
    </template>

    <!-- Step 3: Review -->
    <template v-if="step === 3">
      <ReceivingReviewStep
        :form="form"
        :items="items"
        :locations="locations"
        :warehouse-label="warehouseLabel"
        :staging-location-label="stagingLocationLabel"
        :supplier-display-name="supplierDisplayName"
        :submitting="submitting"
        :can-confirm="canConfirm"
        :can-approve-exceptions="canApproveExceptions"
        @edit-document="editDocument"
        @edit-items="editItems"
        @save-draft="saveDraft"
        @confirm="confirmReceivingClick"
      />
    </template>

    <ConfirmDialog
      :open="showSubmitConfirm"
      :title="$t('newReceiving.actions.confirmReceiving')"
      :message="$t('newReceiving.messages.confirmSubmitDialog')"
      :confirm-label="$t('newReceiving.actions.confirmReceiving')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('newReceiving.actions.submitting')"
      :loading="submitting"
      :error-message="submitError"
      @cancel="showSubmitConfirm = false"
      @confirm="submitReceiving"
    />
  </div>
</template>

<script>
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { listSuppliers } from '../../services/suppliers'
import { getBrand } from '../../services/brands'
import { createReceipt, confirmReceipt, newIdempotencyKey } from '../../services/receiving'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import SearchableSelect from '../../components/ui/SearchableSelect.vue'
import ReceivingReviewStep from './ReceivingReviewStep.vue'
import { validateReceivingReview } from './receivingReview'

function todayDate() {
  return new Date().toISOString().slice(0, 10)
}

// <input type="date"> gives "YYYY-MM-DD"; the backend's *time.Time
// fields decode with Go's standard RFC3339 JSON unmarshaling, which a
// bare date string doesn't satisfy.
function toApiDate(dateStr) {
  return dateStr ? `${dateStr}T00:00:00Z` : null
}

// <input type="datetime-local"> gives "YYYY-MM-DDTHH:mm" (no seconds/
// zone) — append both so it round-trips through Go's RFC3339 decoding.
function toApiDateTime(dateTimeStr) {
  return dateTimeStr ? `${dateTimeStr}:00Z` : null
}

export default {
  name: 'NewReceivingView',

  components: { PageHeader, FormSection, DataTable, ConfirmDialog, SearchableSelect, ReceivingReviewStep },

  data() {
    return {
      step: 1,
      furthestStepReached: 1,

      scanText: '',
      scanQty: 1,
      scanLocationId: '',

      warehouses: [],
      locations: [],
      supplierDisplayName: '',

      // brandCache resolves product.brand_id -> display name on demand
      // (lazy, one lookup per distinct brand actually used on this
      // receipt) — the Step 3 review needs a brand name, not just the
      // id the product list endpoint already returns.
      brandCache: {},

      formError: '',
      submitting: false,
      scanning: false,
      showSubmitConfirm: false,
      submitError: '',

      form: {
        receivingType: 'PO',
        referenceNo: '',
        receivingDate: todayDate(),
        vendor: '',
        supplierId: '',
        invoiceNo: '',
        deliveryNoteNo: '',
        containerNo: '',
        sealNo: '',
        transportCompany: '',
        driverName: '',
        vehicleRegNo: '',
        dockDoor: '',
        arrivalTime: '',
        unloadingStartTime: '',
        unloadingEndTime: '',
        currency: '',
        warehouseId: '',
        stagingLocationId: '',
        remark: ''
      },

      items: []
    }
  },

  computed: {
    totalReceiveQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.receiveQty || 0)
      }, 0)
    },

    canConfirm() {
      return hasPermission('receiving.confirm')
    },

    // receiving.approve is reserved for supervisor-level exceptions
    // (see internal/rbac/catalog.go's own doc) — reused here, unchanged,
    // to gate the Step 3 review's "acknowledge warnings" override rather
    // than inventing a new permission for it.
    canApproveExceptions() {
      return hasPermission('receiving.approve')
    },

    itemsWithOpenSplits() {
      return this.items.filter((item) => item.splitsOpen)
    },

    warehouseLabel() {
      const w = this.warehouses.find((w) => w.id === this.form.warehouseId)
      return w ? `${w.code} — ${w.name}` : '—'
    },

    stagingLocationLabel() {
      const l = this.locations.find((l) => l.id === this.form.stagingLocationId)
      return l ? l.code : '—'
    }
  },

  watch: {
    'form.warehouseId'(warehouseId) {
      this.form.stagingLocationId = ''
      this.scanLocationId = ''
      this.fetchLocationsForWarehouse(warehouseId)
    }
  },

  mounted() {
    this.fetchWarehouses()
  },

  methods: {
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.formError = err.message
      }
    },

    async fetchLocationsForWarehouse(warehouseId) {
      this.locations = []
      if (!warehouseId) return
      try {
        const zonesRes = await listZones(warehouseId)
        const allLocations = []
        for (const zone of zonesRes.data) {
          const locRes = await listLocations(zone.id)
          allLocations.push(...locRes.data)
        }
        this.locations = allLocations
      } catch (err) {
        this.formError = err.message
      }
    },

    // Exact SKU or barcode match only — a partial/fuzzy match here would
    // silently receive the wrong product into stock.
    async findProduct(text) {
      const trimmed = (text || '').trim()
      if (!trimmed) return null
      const res = await listProducts({ search: trimmed, limit: 20 })
      return res.data.find((p) => p.sku === trimmed || p.barcode === trimmed) || null
    },

    async lookupItemProduct(item) {
      if (!item.sku) return
      try {
        const product = await this.findProduct(item.sku)
        if (!product) {
          item.productId = null
          item.productName = ''
          item.barcode = ''
          item.product = null
          item.brandName = ''
          this.formError = this.$t('newReceiving.messages.noProductFound', { value: item.sku })
          return
        }
        this.applyProductToItem(item, product)
      } catch (err) {
        this.formError = err.message
      }
    },

    // applyProductToItem is shared by lookupItemProduct (SKU typed/
    // blurred) and scanItem (barcode scan) — both resolve the same
    // product.Response shape and need the same full set of fields on
    // the item for Step 3's review to have real data to show, not just
    // the handful of columns Step 2's own table displays.
    applyProductToItem(item, product) {
      item.productId = product.id
      item.sku = product.sku
      item.productName = product.name
      item.barcode = product.barcode || ''
      item.uom = product.base_uom || item.uom
      item.product = product
      item.brandName = ''
      if (product.brand_id) this.resolveBrandName(item, product.brand_id)
    },

    // resolveBrandName is a lazy, per-brand-id-cached lookup — the
    // product list/search endpoint only returns brand_id, not a
    // resolved name, and most receipts touch only one or two distinct
    // brands, so caching avoids repeat GET /brands/{id} calls across
    // items/rescans of the same brand.
    async resolveBrandName(item, brandId) {
      try {
        if (!(brandId in this.brandCache)) {
          const res = await getBrand(brandId)
          this.brandCache[brandId] = res.data.name
        }
        item.brandName = this.brandCache[brandId] || ''
      } catch {
        // Non-critical: the review simply shows "—" for brand.
      }
    },

    // fetchSupplierOptions backs the Supplier SearchableSelect's remote
    // search — only ACTIVE suppliers are offered; the backend itself
    // re-verifies ownership regardless, this is just the picker.
    async fetchSupplierOptions(search) {
      try {
        const res = await listSuppliers({ search, status: 'ACTIVE', limit: 20 })
        return res.data.map((s) => ({ value: s.id, label: `${s.code} — ${s.company_name}` }))
      } catch {
        return []
      }
    },

    goToStep(target) {
      if (target <= this.furthestStepReached) this.step = target
    },

    // editDocument/editItems back Step 3's own named buttons — always
    // allowed (the furthestStepReached gate above only exists to stop
    // jumping AHEAD before a step's own validation has passed; going
    // BACK from the review step never needs that check). Neither resets
    // any form/items state, so every value the user already entered is
    // exactly as they left it when they return to Step 3.
    editDocument() {
      this.step = 1
    },

    editItems() {
      this.step = 2
    },

    goNextFromDocument() {
      const error = this.validateForm({ requireReceiveQty: false, requireItems: false })
      if (error) {
        this.formError = error
        return
      }
      this.formError = ''
      this.step = 2
      this.furthestStepReached = Math.max(this.furthestStepReached, 2)
    },

    goNextFromItems() {
      const error = this.validateForm({ requireReceiveQty: false, requireItems: true })
      if (error) {
        this.formError = error
        return
      }
      this.formError = ''
      this.step = 3
      this.furthestStepReached = Math.max(this.furthestStepReached, 3)
    },

    toggleSplits(item) {
      item.splitsOpen = !item.splitsOpen
    },

    addSplit(item) {
      item.splits.push({
        quantity: 0, lotNo: '', mfgDate: '', expiryDate: '', palletId: '', cartonCount: 0,
        locationId: item.locationId || this.form.stagingLocationId || '',
        temperatureC: null, conditionNote: '',
        countryOfOrigin: (item.product && item.product.country_of_origin) || ''
      })
    },

    removeSplit(item, index) {
      item.splits.splice(index, 1)
    },

    splitQtySum(item) {
      return item.splits.reduce((sum, s) => sum + Number(s.quantity || 0), 0)
    },

    addItem() {
      this.items.push({
        id: Date.now(),
        productId: null,
        sku: '',
        productName: '',
        barcode: '',
        product: null,
        brandName: '',
        expectedQty: 0,
        receiveQty: 0,
        uom: 'PCS',
        lotNo: '',
        expiryDate: '',
        locationId: this.form.stagingLocationId || '',
        splits: [],
        splitsOpen: false
      })
    },

    async scanItem() {
      if (!this.scanText) {
        this.formError = this.$t('newReceiving.messages.barcodeRequired')
        return
      }

      this.scanning = true
      this.formError = ''
      try {
        const product = await this.findProduct(this.scanText)
        if (!product) {
          this.formError = this.$t('newReceiving.messages.noProductFound', { value: this.scanText })
          return
        }

        const existingItem = this.items.find((item) => item.productId === product.id)
        if (existingItem) {
          existingItem.receiveQty = Number(existingItem.receiveQty || 0) + Number(this.scanQty || 1)
        } else {
          const newItem = {
            id: Date.now(),
            productId: null,
            sku: '',
            productName: '',
            barcode: '',
            product: null,
            brandName: '',
            expectedQty: 0,
            receiveQty: Number(this.scanQty || 1),
            uom: 'PCS',
            lotNo: '',
            expiryDate: '',
            locationId: this.scanLocationId || this.form.stagingLocationId || '',
            splits: [],
            splitsOpen: false
          }
          this.applyProductToItem(newItem, product)
          this.items.push(newItem)
        }

        this.scanText = ''
        this.scanQty = 1
      } catch (err) {
        this.formError = err.message
      } finally {
        this.scanning = false
      }
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    // Shared by Save Draft and the wizard's per-step Next buttons.
    // requireItems is false for Step 1 -> 2 (a draft may be saved
    // header-only, matching the backend's own "items optional" Create);
    // Confirm additionally requires at least one line to actually
    // receive a quantity.
    validateForm({ requireReceiveQty, requireItems = true }) {
      if (!this.form.referenceNo) return this.$t('newReceiving.messages.referenceNoRequired')
      if (!this.form.supplierId && !this.form.vendor) return this.$t('newReceiving.messages.supplierOrVendorRequired')
      if (!this.form.warehouseId) return this.$t('newReceiving.messages.warehouseRequired')
      if (!this.form.stagingLocationId) return this.$t('newReceiving.messages.stagingLocationRequired')
      if (requireItems && this.items.length === 0) return this.$t('newReceiving.messages.atLeastOneItem')
      if (this.items.some((item) => !item.productId)) {
        return this.$t('newReceiving.messages.everyItemMustResolve')
      }
      if (this.items.some((item) => !item.locationId)) return this.$t('newReceiving.messages.everyItemMustHaveLocation')
      if (this.items.some((item) => (item.splits || []).length > 0 && this.splitQtySum(item) !== Number(item.receiveQty || 0))) {
        return this.$t('newReceiving.messages.splitQtyMustMatch')
      }
      if (requireReceiveQty && this.totalReceiveQty <= 0) {
        return this.$t('newReceiving.messages.enterReceiveQty')
      }
      return ''
    },

    buildCreatePayload(status) {
      return {
        receiving_type: this.form.receivingType,
        reference_no: this.form.referenceNo,
        vendor: this.form.vendor || null,
        supplier_id: this.form.supplierId || null,
        invoice_no: this.form.invoiceNo || null,
        delivery_note_no: this.form.deliveryNoteNo || null,
        container_no: this.form.containerNo || null,
        seal_no: this.form.sealNo || null,
        transport_company: this.form.transportCompany || null,
        driver_name: this.form.driverName || null,
        vehicle_reg_no: this.form.vehicleRegNo || null,
        dock_door: this.form.dockDoor || null,
        arrival_time: toApiDateTime(this.form.arrivalTime),
        unloading_start_time: toApiDateTime(this.form.unloadingStartTime),
        unloading_end_time: toApiDateTime(this.form.unloadingEndTime),
        currency: this.form.currency || null,
        warehouse_id: this.form.warehouseId,
        staging_location_id: this.form.stagingLocationId,
        status,
        receiving_date: toApiDate(this.form.receivingDate),
        remark: this.form.remark || null,
        items: this.items.map((item) => ({
          product_id: item.productId,
          expected_qty: Number(item.expectedQty || 0),
          uom: item.uom,
          lot_no: item.lotNo || '',
          expiry_date: toApiDate(item.expiryDate),
          location_id: item.locationId
        }))
      }
    },

    async saveDraft() {
      const error = this.validateForm({ requireReceiveQty: false, requireItems: this.items.length > 0 })
      if (error) {
        this.formError = error
        return
      }

      this.submitting = true
      this.formError = ''
      try {
        await createReceipt(this.buildCreatePayload('DRAFT'))
        showToast(this.$t('newReceiving.messages.draftSaved'), 'success')
        this.$router.push('/inbound/receiving')
      } catch (err) {
        this.formError = err.message
      } finally {
        this.submitting = false
      }
    },

    // Validates the form up front and, if it passes, opens the
    // confirmation dialog instead of submitting immediately — the
    // actual submit logic runs from submitReceiving() when the dialog
    // is confirmed. Uses the exact same validateReceivingReview errors
    // the Step 3 review screen itself renders and gates its own Confirm
    // button on, so this is a defense-in-depth re-check (e.g. if this
    // method is ever invoked directly, as the existing test suite
    // does), not a second, differently-worded source of truth.
    confirmReceivingClick() {
      const basicError = this.validateForm({ requireReceiveQty: true, requireItems: true })
      if (basicError) {
        this.formError = basicError
        return
      }
      const { errors } = validateReceivingReview(this.form, this.items)
      if (errors.length > 0) {
        this.formError = this.$t(errors[0].message)
        return
      }
      this.formError = ''
      this.submitError = ''
      this.showSubmitConfirm = true
    },

    async submitReceiving() {
      this.submitting = true
      this.submitError = ''
      try {
        const created = await createReceipt(this.buildCreatePayload('WAITING'))

        // Items come back in the same order they were submitted in
        // (backend creates each line via one insert per request item, in
        // request order), so index correlation back to this.items is safe.
        const confirmItems = created.data.items
          .map((respItem, idx) => ({ respItem, srcItem: this.items[idx] }))
          .filter(({ srcItem }) => Number(srcItem.receiveQty || 0) > 0)
          .map(({ respItem, srcItem }) => ({
            item_id: respItem.id,
            quantity: Number(srcItem.receiveQty),
            lot_no: srcItem.lotNo || '',
            expiry_date: toApiDate(srcItem.expiryDate),
            location_id: srcItem.locationId,
            // Splits (Phase 31A): when the line was broken into multiple
            // lot/pallet batches, send each one; the backend posts one
            // AddStock + one receipt_item_splits row per entry instead of
            // the single implicit split above.
            splits: (srcItem.splits || []).length > 0
              ? srcItem.splits.map((s) => ({
                  quantity: Number(s.quantity || 0),
                  lot_no: s.lotNo || '',
                  mfg_date: toApiDate(s.mfgDate),
                  expiry_date: toApiDate(s.expiryDate),
                  location_id: s.locationId || srcItem.locationId,
                  pallet_id: s.palletId || '',
                  carton_count: Number(s.cartonCount || 0),
                  temperature_c: s.temperatureC === '' || s.temperatureC === null || s.temperatureC === undefined ? null : Number(s.temperatureC),
                  condition_note: s.conditionNote || '',
                  country_of_origin: s.countryOfOrigin || ''
                }))
              : undefined
          }))

        await confirmReceipt(created.data.id, {
          idempotency_key: newIdempotencyKey(),
          items: confirmItems
        })

        this.showSubmitConfirm = false
        showToast(this.$t('newReceiving.messages.receivingConfirmed'), 'success')
        this.$router.push('/inbound/receiving')
      } catch (err) {
        this.submitError = err.message
      } finally {
        this.submitting = false
      }
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
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}

.table-th {
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  white-space: nowrap;
}

.table-td {
  padding: 10px 12px;
  color: #475569;
  white-space: nowrap;
}

.table-input {
  width: 100%;
  min-width: 120px;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 9px 10px;
  outline: none;
}

.table-input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.summary-card {
  background: #f8fafc;
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

.text-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.input-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>
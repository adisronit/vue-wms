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

    <!-- Document Header -->
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
          <label class="label">{{ $t('newReceiving.fields.vendor') }}</label>
          <!-- Vendor option values are mock/seed business data (vendor
               names), not UI chrome — left untranslated per this
               phase's "preserve original business values" instruction. -->
          <select v-model="form.vendor" class="input">
            <option value="">{{ $t('newReceiving.fields.selectVendor') }}</option>
            <option value="ABC Supplier">ABC Supplier</option>
            <option value="Global Parts">Global Parts</option>
            <option value="Fast Supply">Fast Supply</option>
            <option value="Orange Logistics">Orange Logistics</option>
          </select>
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

      <div v-if="formError" class="mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ formError }}
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

    <!-- Scan / Add Item -->
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

          <td class="table-td text-right">
            <button
              @click="removeItem(index)"
              class="text-red-600 hover:text-red-700 font-semibold"
            >
              {{ $t('newReceiving.table.remove') }}
            </button>
          </td>
        </tr>
      </DataTable>
    </FormSection>

    <!-- Summary -->
    <FormSection :title="$t('newReceiving.summary.title')">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.totalLines') }}</p>
          <h2 class="summary-value">{{ items.length }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.expectedQty') }}</p>
          <h2 class="summary-value">{{ totalExpectedQty }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.receiveQty') }}</p>
          <h2 class="summary-value text-orange-600">{{ totalReceiveQty }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('newReceiving.summary.variance') }}</p>
          <h2
            class="summary-value"
            :class="totalVariance < 0 ? 'text-red-600' : 'text-green-600'"
          >
            {{ totalVariance }}
          </h2>
        </div>
      </div>
    </FormSection>

    <!-- Action Bar -->
    <FormSection>
      <div class="flex flex-col md:flex-row justify-end gap-3">
        <button
          @click="saveDraft"
          :disabled="submitting"
          class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('newReceiving.actions.saveDraft') }}
        </button>

        <button
          v-if="canConfirm"
          @click="confirmReceivingClick"
          :disabled="submitting"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('newReceiving.actions.confirmReceiving') }}
        </button>
        <p v-else class="text-sm text-slate-500 self-center">
          {{ $t('newReceiving.actions.viewOnlyNote') }}
        </p>
      </div>
    </FormSection>

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
import { createReceipt, confirmReceipt, newIdempotencyKey } from '../../services/receiving'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

function todayDate() {
  return new Date().toISOString().slice(0, 10)
}

// <input type="date"> gives "YYYY-MM-DD"; the backend's *time.Time
// fields decode with Go's standard RFC3339 JSON unmarshaling, which a
// bare date string doesn't satisfy.
function toApiDate(dateStr) {
  return dateStr ? `${dateStr}T00:00:00Z` : null
}

export default {
  name: 'NewReceivingView',

  components: { PageHeader, FormSection, DataTable, ConfirmDialog },

  data() {
    return {
      scanText: '',
      scanQty: 1,
      scanLocationId: '',

      warehouses: [],
      locations: [],

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
        warehouseId: '',
        stagingLocationId: '',
        remark: ''
      },

      items: []
    }
  },

  computed: {
    totalExpectedQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.expectedQty || 0)
      }, 0)
    },

    totalReceiveQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.receiveQty || 0)
      }, 0)
    },

    totalVariance() {
      return this.totalReceiveQty - this.totalExpectedQty
    },

    canConfirm() {
      return hasPermission('receiving.approve')
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
          this.formError = this.$t('newReceiving.messages.noProductFound', { value: item.sku })
          return
        }
        item.productId = product.id
        item.sku = product.sku
        item.productName = product.name
        item.barcode = product.barcode || ''
        item.uom = product.base_uom || item.uom
      } catch (err) {
        this.formError = err.message
      }
    },

    addItem() {
      this.items.push({
        id: Date.now(),
        productId: null,
        sku: '',
        productName: '',
        barcode: '',
        expectedQty: 0,
        receiveQty: 0,
        uom: 'PCS',
        lotNo: '',
        expiryDate: '',
        locationId: this.form.stagingLocationId || ''
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
          this.items.push({
            id: Date.now(),
            productId: product.id,
            sku: product.sku,
            productName: product.name,
            barcode: product.barcode || '',
            expectedQty: 0,
            receiveQty: Number(this.scanQty || 1),
            uom: product.base_uom || 'PCS',
            lotNo: '',
            expiryDate: '',
            locationId: this.scanLocationId || this.form.stagingLocationId || ''
          })
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

    // Shared by Save Draft and Confirm Receiving — Confirm additionally
    // requires at least one line to actually receive a quantity.
    validateForm({ requireReceiveQty }) {
      if (!this.form.referenceNo) return this.$t('newReceiving.messages.referenceNoRequired')
      if (!this.form.vendor) return this.$t('newReceiving.messages.vendorRequired')
      if (!this.form.warehouseId) return this.$t('newReceiving.messages.warehouseRequired')
      if (!this.form.stagingLocationId) return this.$t('newReceiving.messages.stagingLocationRequired')
      if (this.items.length === 0) return this.$t('newReceiving.messages.atLeastOneItem')
      if (this.items.some((item) => !item.productId)) {
        return this.$t('newReceiving.messages.everyItemMustResolve')
      }
      if (this.items.some((item) => !item.locationId)) return this.$t('newReceiving.messages.everyItemMustHaveLocation')
      if (requireReceiveQty && this.totalReceiveQty <= 0) {
        return this.$t('newReceiving.messages.enterReceiveQty')
      }
      return ''
    },

    buildCreatePayload(status) {
      return {
        receiving_type: this.form.receivingType,
        reference_no: this.form.referenceNo,
        vendor: this.form.vendor,
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
      const error = this.validateForm({ requireReceiveQty: false })
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

    // Validates the form up front (same rule as before) and, if it
    // passes, opens the confirmation dialog instead of submitting
    // immediately — the actual submit logic is unchanged and now runs
    // from submitReceiving() when the dialog is confirmed.
    confirmReceivingClick() {
      const error = this.validateForm({ requireReceiveQty: true })
      if (error) {
        this.formError = error
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
            location_id: srcItem.locationId
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
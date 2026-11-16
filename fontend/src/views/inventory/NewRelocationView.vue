<template>
  <div class="space-y-6">
    <PageHeader :title="$t('newRelocation.title')" :subtitle="$t('newRelocation.subtitle')">
      <template #actions>
        <button
          @click="$router.push('/inventory/relocation')"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newRelocation.backButton') }}
        </button>
      </template>
    </PageHeader>

    <FormSection :title="$t('newRelocation.taskInfo')">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="label">{{ $t('newRelocation.fields.warehouse') }}</label>
          <select v-model="form.warehouseId" class="input">
            <option value="">{{ $t('newRelocation.fields.selectWarehouse') }}</option>
            <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
          </select>
        </div>
      </div>

      <div v-if="formError" class="mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ formError }}
      </div>

      <div class="mt-4">
        <label class="label">{{ $t('common.labels.remark') }}</label>
        <textarea v-model="form.remark" rows="3" class="input" :placeholder="$t('newRelocation.fields.remarkPlaceholder')"></textarea>
      </div>
    </FormSection>

    <!-- Scan Area -->
    <FormSection>
      <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5">
        <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-4">
          <div>
            <h2 class="text-lg font-bold text-slate-800">{{ $t('newRelocation.scan.title') }}</h2>
            <p class="text-sm text-slate-500">{{ $t('newRelocation.scan.subtitle') }}</p>
          </div>
          <StatusBadge :tone="scanning ? 'blue' : 'orange'">
            {{ scanning ? $t('newRelocation.scan.lookingUp') : $t('newRelocation.scan.scannerReady') }}
          </StatusBadge>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <input
            v-model="scanForm.sku"
            type="text"
            class="input bg-white"
            :placeholder="$t('newRelocation.scan.skuPlaceholder')"
            @keyup.enter="focusSourceLocation"
            ref="skuInput"
          />
          <input
            v-model="scanForm.lotNo"
            type="text"
            class="input bg-white"
            :placeholder="$t('newRelocation.scan.lotPlaceholder')"
          />
        </div>

        <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
          <input
            v-model="scanForm.sourceLocation"
            type="text"
            class="input bg-white"
            :placeholder="$t('newRelocation.scan.sourcePlaceholder')"
            @keyup.enter="checkAvailability"
            ref="sourceInput"
          />

          <input
            v-model="scanForm.destinationLocation"
            type="text"
            class="input bg-white"
            :placeholder="$t('newRelocation.scan.destinationPlaceholder')"
            @keyup.enter="focusQty"
            ref="destinationInput"
          />

          <input
            v-model.number="scanForm.qty"
            type="number"
            class="input bg-white input-number"
            :placeholder="$t('newRelocation.scan.qtyPlaceholder')"
            @keyup.enter="scanItem"
            ref="qtyInput"
          />

          <input
            v-model="scanForm.lpn"
            type="text"
            class="input bg-white"
            :placeholder="$t('newRelocation.scan.lpnPlaceholder')"
          />

          <button
            @click="scanItem"
            :disabled="scanning"
            class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('newRelocation.scan.addLine') }}
          </button>
        </div>

        <p v-if="availableQty !== null" class="text-sm text-slate-600 mt-3">
          {{ $t('newRelocation.scan.availableStock', { qty: availableQty }) }}
        </p>
      </div>
    </FormSection>

    <!-- Relocation Items -->
    <FormSection>
      <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4 mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">{{ $t('newRelocation.itemsSection.title') }}</h2>
          <p class="text-sm text-slate-500">{{ $t('newRelocation.itemsSection.subtitle') }}</p>
        </div>
      </div>

      <DataTable
        :is-empty="items.length === 0"
        :empty-text="$t('newRelocation.table.noItems')"
        :colspan="9"
        min-width="1300px"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('newRelocation.table.sku') }}</th>
            <th class="table-th">{{ $t('newRelocation.table.productName') }}</th>
            <th class="table-th">{{ $t('newRelocation.table.lotNo') }}</th>
            <th class="table-th">{{ $t('newRelocation.table.lpn') }}</th>
            <th class="table-th">{{ $t('newRelocation.table.fromLocation') }}</th>
            <th class="table-th">{{ $t('newRelocation.table.toLocation') }}</th>
            <th class="table-th text-right">{{ $t('newRelocation.table.qty') }}</th>
            <th class="table-th">{{ $t('newRelocation.table.uom') }}</th>
            <th class="table-th text-right">{{ $t('newRelocation.table.action') }}</th>
          </tr>
        </template>

        <tr v-for="(item, index) in items" :key="item.id" class="border-t hover:bg-orange-50">
          <td class="table-td">{{ item.sku }}</td>
          <td class="table-td min-w-[200px]">{{ item.productName }}</td>
          <td class="table-td">{{ item.lotNo || '—' }}</td>
          <td class="table-td">{{ item.lpn || '—' }}</td>
          <td class="table-td">{{ item.sourceLocationCode }}</td>
          <td class="table-td">{{ item.destinationLocationCode }}</td>
          <td class="table-td text-right">
            <input v-model.number="item.requestedQty" type="number" class="table-input input-number" />
          </td>
          <td class="table-td">{{ item.uom }}</td>
          <td class="table-td text-right">
            <button @click="removeItem(index)" class="text-red-600 hover:text-red-700 font-semibold">
              {{ $t('common.buttons.remove') }}
            </button>
          </td>
        </tr>
      </DataTable>
    </FormSection>

    <!-- Summary -->
    <FormSection :title="$t('newRelocation.summary.title')">
      <div class="grid grid-cols-1 md:grid-cols-2 gap-5">
        <div class="summary-card">
          <p class="summary-label">{{ $t('newRelocation.summary.totalLines') }}</p>
          <h2 class="summary-value text-number">{{ items.length }}</h2>
        </div>
        <div class="summary-card">
          <p class="summary-label">{{ $t('newRelocation.summary.totalQty') }}</p>
          <h2 class="summary-value text-number">{{ totalQty }}</h2>
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
          {{ $t('common.buttons.saveDraft') }}
        </button>

        <button
          v-if="canReady"
          @click="saveAndMarkReadyClick"
          :disabled="submitting"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ $t('newRelocation.actions.saveAndMarkReady') }}
        </button>
        <p v-else class="text-sm text-slate-500 self-center">
          {{ $t('newRelocation.actions.viewOnlyNote') }}
        </p>
      </div>
    </FormSection>

    <ConfirmDialog
      :open="showSubmitConfirm"
      :title="$t('newRelocation.actions.saveAndMarkReady')"
      :message="$t('newRelocation.messages.confirmSubmitDialog')"
      :confirm-label="$t('newRelocation.actions.saveAndMarkReady')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      :loading="submitting"
      :error-message="submitError"
      @cancel="showSubmitConfirm = false"
      @confirm="saveAndMarkReady"
    />
  </div>
</template>

<script>
import { listWarehouses, listZones, listLocations } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { listInventory } from '../../services/inventory'
import { createRelocationTask, readyRelocationTask } from '../../services/relocation'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

export default {
  name: 'NewRelocationView',

  components: { PageHeader, FormSection, DataTable, StatusBadge, ConfirmDialog },

  data() {
    return {
      warehouses: [],
      locations: [],

      formError: '',
      submitting: false,
      scanning: false,
      showSubmitConfirm: false,
      submitError: '',
      availableQty: null,

      form: {
        warehouseId: '',
        remark: ''
      },

      scanForm: {
        sku: '',
        lotNo: '',
        lpn: '',
        sourceLocation: '',
        destinationLocation: '',
        qty: 1
      },

      items: []
    }
  },

  computed: {
    totalQty() {
      return this.items.reduce((sum, item) => sum + Number(item.requestedQty || 0), 0)
    },
    canReady() {
      return hasPermission('relocation.approve')
    }
  },

  watch: {
    'form.warehouseId'(warehouseId) {
      this.fetchLocationsForWarehouse(warehouseId)
    }
  },

  mounted() {
    this.fetchWarehouses()
    this.$nextTick(() => {
      if (this.$refs.skuInput) this.$refs.skuInput.focus()
    })
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

    async findProduct(text) {
      const trimmed = (text || '').trim()
      if (!trimmed) return null
      const res = await listProducts({ search: trimmed, limit: 20 })
      return res.data.find((p) => p.sku === trimmed || p.barcode === trimmed) || null
    },

    findLocationByCode(code) {
      const trimmed = (code || '').trim()
      if (!trimmed) return null
      return this.locations.find((l) => l.code === trimmed) || null
    },

    focusSourceLocation() {
      if (this.$refs.sourceInput) this.$refs.sourceInput.focus()
    },
    focusQty() {
      if (this.$refs.qtyInput) this.$refs.qtyInput.focus()
    },

    // Available/unreserved stock preview — reuses the existing read-only
    // Inventory Core endpoint (GET /inventory) rather than inventing a
    // relocation-specific lookup; the authoritative check still happens
    // atomically inside Confirm's own MoveAvailableStockTx call.
    async checkAvailability() {
      this.availableQty = null
      const product = await this.findProduct(this.scanForm.sku)
      const source = this.findLocationByCode(this.scanForm.sourceLocation)
      if (!product || !source) return
      try {
        const res = await listInventory({ productId: product.id, locationId: source.id, lotNo: this.scanForm.lotNo || undefined, limit: 1 })
        const row = res.data[0]
        this.availableQty = row ? row.available_qty : 0
      } catch {
        // Non-critical: the preview simply stays hidden.
      }
    },

    async scanItem() {
      if (!this.scanForm.sku) {
        this.formError = this.$t('newRelocation.messages.skuRequired')
        return
      }
      if (!this.scanForm.sourceLocation || !this.scanForm.destinationLocation) {
        this.formError = this.$t('newRelocation.messages.sourceAndDestinationRequired')
        return
      }
      if (this.scanForm.sourceLocation.trim() === this.scanForm.destinationLocation.trim()) {
        this.formError = this.$t('newRelocation.messages.fromAndToMustDiffer')
        return
      }

      this.scanning = true
      this.formError = ''
      try {
        const product = await this.findProduct(this.scanForm.sku)
        if (!product) {
          this.formError = this.$t('newRelocation.messages.noProductFound', { value: this.scanForm.sku })
          return
        }
        const source = this.findLocationByCode(this.scanForm.sourceLocation)
        if (!source) {
          this.formError = this.$t('newRelocation.messages.noLocationFound', { value: this.scanForm.sourceLocation })
          return
        }
        const destination = this.findLocationByCode(this.scanForm.destinationLocation)
        if (!destination) {
          this.formError = this.$t('newRelocation.messages.noLocationFound', { value: this.scanForm.destinationLocation })
          return
        }

        this.items.push({
          id: Date.now(),
          productId: product.id,
          sku: product.sku,
          productName: product.name,
          lotNo: this.scanForm.lotNo || '',
          lpn: this.scanForm.lpn || '',
          sourceLocationId: source.id,
          sourceLocationCode: source.code,
          destinationLocationId: destination.id,
          destinationLocationCode: destination.code,
          requestedQty: Number(this.scanForm.qty || 1),
          uom: product.base_uom || 'PCS'
        })

        this.scanForm = { sku: '', lotNo: '', lpn: '', sourceLocation: '', destinationLocation: '', qty: 1 }
        this.availableQty = null

        this.$nextTick(() => {
          if (this.$refs.skuInput) this.$refs.skuInput.focus()
        })
      } catch (err) {
        this.formError = err.message
      } finally {
        this.scanning = false
      }
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    validateForm() {
      if (!this.form.warehouseId) return this.$t('newRelocation.messages.warehouseRequired')
      if (this.items.length === 0) return this.$t('newRelocation.messages.atLeastOneItem')
      if (this.items.some((item) => !item.requestedQty || item.requestedQty <= 0)) {
        return this.$t('newRelocation.messages.everyItemNeedsQtyGreaterThanZero')
      }
      return ''
    },

    buildCreatePayload() {
      return {
        warehouse_id: this.form.warehouseId,
        remark: this.form.remark || null,
        items: this.items.map((item) => ({
          product_id: item.productId,
          lot_no: item.lotNo || '',
          lpn: item.lpn || null,
          source_location_id: item.sourceLocationId,
          destination_location_id: item.destinationLocationId,
          requested_qty: Number(item.requestedQty || 0),
          uom: item.uom
        }))
      }
    },

    async saveDraft() {
      const error = this.validateForm()
      if (error) {
        this.formError = error
        return
      }

      this.submitting = true
      this.formError = ''
      try {
        await createRelocationTask(this.buildCreatePayload())
        showToast(this.$t('newRelocation.messages.draftSaved'), 'success')
        this.$router.push('/inventory/relocation')
      } catch (err) {
        this.formError = err.message
      } finally {
        this.submitting = false
      }
    },

    saveAndMarkReadyClick() {
      const error = this.validateForm()
      if (error) {
        this.formError = error
        return
      }
      this.formError = ''
      this.submitError = ''
      this.showSubmitConfirm = true
    },

    async saveAndMarkReady() {
      this.submitting = true
      this.submitError = ''
      try {
        const created = await createRelocationTask(this.buildCreatePayload())
        await readyRelocationTask(created.data.id)
        this.showSubmitConfirm = false
        showToast(this.$t('newRelocation.messages.markedReady'), 'success')
        this.$router.push('/inventory/relocation')
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
  min-width: 100px;
  border: 1px solid #cbd5e1;
  border-radius: 10px;
  padding: 9px 10px;
  outline: none;
}

.table-input:focus {
  border-color: #ea580c;
  box-shadow: 0 0 0 2px #fed7aa;
}

.input-number,
.text-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
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
</style>

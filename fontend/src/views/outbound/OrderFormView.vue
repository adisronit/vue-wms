<template>
  <div class="space-y-6 pb-24">
    <PageHeader :title="isEdit ? $t('orders.form.editTitle') : $t('orders.form.newTitle')" :subtitle="$t('orders.form.pageSubtitle')">
      <template #actions>
        <button
          @click="$router.push('/outbound/orders')"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newRelocation.backButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>
    <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('orders.loadingOrders') }}</div>

    <template v-else>
      <FormSection :title="$t('orders.form.headerTitle')">
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label class="label">{{ $t('orders.form.orderTypeLabel') }}</label>
            <select v-model="form.orderType" class="input">
              <option value="SALES_ORDER">{{ $t('orders.form.orderTypeSales') }}</option>
              <option value="INTERNAL_TRANSFER">{{ $t('orders.form.orderTypeTransfer') }}</option>
            </select>
          </div>
          <div>
            <label class="label">{{ $t('orders.form.priorityLabel') }}</label>
            <select v-model="form.priority" class="input">
              <option value="LOW">{{ $t('orders.release.priorityLow') }}</option>
              <option value="NORMAL">{{ $t('orders.release.priorityNormal') }}</option>
              <option value="HIGH">{{ $t('orders.release.priorityHigh') }}</option>
            </select>
          </div>
          <div>
            <label class="label">{{ $t('orders.form.selectWarehouse') }}</label>
            <select v-model="form.warehouseId" class="input" :disabled="isEdit && items.length > 0">
              <option value="">{{ $t('orders.form.selectWarehouse') }}</option>
              <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
            </select>
          </div>

          <div>
            <label class="label">{{ $t('orders.form.customerLabel') }}</label>
            <SearchableSelect
              v-model="form.customerId"
              v-model:display-value="customerDisplayName"
              :fetch-options="fetchCustomerOptions"
              :placeholder="$t('orders.form.selectCustomer')"
              @change="onCustomerChange"
            />
          </div>

          <div v-if="form.customerId">
            <label class="label">{{ $t('orders.form.shipToLabel') }}</label>
            <select v-model="form.shipToId" class="input" :disabled="shipToLoading">
              <option value="">{{ $t('orders.form.selectShipTo') }}</option>
              <option v-for="s in shipToOptions" :key="s.id" :value="s.id">
                {{ s.name }}{{ s.is_default ? ' ★' : '' }}
              </option>
            </select>
          </div>
          <input v-else v-model="form.customer" type="text" :placeholder="$t('orders.form.customerPlaceholder')" class="input" />

          <input v-model="form.externalReference" type="text" :placeholder="$t('orders.form.externalReferencePlaceholder')" class="input" />

          <div>
            <label class="label">{{ $t('orders.form.requiredShippingDateLabel') }}</label>
            <input v-model="form.requiredShippingDate" type="date" class="input" />
          </div>
          <div>
            <label class="label">{{ $t('orders.form.requiredDeliveryDateLabel') }}</label>
            <input v-model="form.requiredDeliveryDate" type="date" class="input" />
          </div>
          <input v-model="form.carrier" type="text" :placeholder="$t('orders.form.carrierPlaceholder')" class="input" />
          <input v-model="form.shippingMethod" type="text" :placeholder="$t('orders.form.shippingMethodPlaceholder')" class="input" />
          <input v-model="form.preparedBy" type="text" :placeholder="$t('orders.form.preparedByPlaceholder')" class="input" />
          <input v-model="form.approvedBy" type="text" :placeholder="$t('orders.form.approvedByPlaceholder')" class="input" />

          <textarea v-model="form.deliveryInstructions" :placeholder="$t('orders.form.deliveryInstructionsPlaceholder')" class="input md:col-span-3" rows="2"></textarea>
          <textarea v-model="form.remark" :placeholder="$t('orders.form.remarkPlaceholder')" class="input md:col-span-3" rows="2"></textarea>
        </div>
      </FormSection>

      <FormSection :title="$t('orders.form.itemsTitle')">
        <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5 mb-4">
          <div class="grid grid-cols-1 md:grid-cols-5 gap-3">
            <input
              v-model="itemDraft.sku"
              type="text"
              class="input bg-white md:col-span-2"
              :placeholder="$t('orders.form.skuPlaceholder')"
              @keyup.enter="addItem"
              ref="skuInput"
            />
            <input v-model.number="itemDraft.orderedQty" type="number" min="0" step="0.01" class="input bg-white input-number" :placeholder="$t('orders.form.qtyPlaceholder')" @keyup.enter="addItem" />
            <input v-model="itemDraft.preferredLotNo" type="text" class="input bg-white" :placeholder="$t('orders.form.lotHintPlaceholder')" />
            <button @click="addItem" :disabled="addingItem" class="px-3 py-2 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
              {{ $t('orders.form.addItem') }}
            </button>
          </div>
          <label class="flex items-center gap-2 mt-3 text-sm text-slate-600">
            <input v-model="itemDraft.fefoPreferred" type="checkbox" class="rounded" />
            {{ $t('orders.form.fefoHint') }}
          </label>
        </div>

        <DataTable
          :is-empty="items.length === 0"
          :empty-text="$t('orders.form.noItemsYet')"
          :colspan="10"
          min-width="1400px"
        >
          <template #head>
            <tr>
              <th class="table-th">{{ $t('orders.form.tableSku') }}</th>
              <th class="table-th">{{ $t('orders.form.tableProduct') }}</th>
              <th class="table-th">{{ $t('orders.review.storage') }}</th>
              <th class="table-th text-right">{{ $t('orders.form.tableQty') }}</th>
              <th class="table-th">{{ $t('orders.form.tableUom') }}</th>
              <th class="table-th text-right">{{ $t('orders.review.available') }}</th>
              <th class="table-th text-right">{{ $t('orders.review.reserved') }}</th>
              <th class="table-th">{{ $t('orders.form.lotHintColumn') }}</th>
              <th class="table-th">{{ $t('orders.form.lineNotesColumn') }}</th>
              <th class="table-th"></th>
            </tr>
          </template>
          <tr v-for="(it, idx) in items" :key="idx" class="border-t hover:bg-orange-50">
            <td class="table-td">{{ it.sku }}</td>
            <td class="table-td min-w-[200px]">{{ it.productName }}</td>
            <td class="table-td">{{ it.storageType || '—' }}</td>
            <td class="table-td text-right">
              <input v-model.number="it.orderedQty" type="number" min="0" step="0.01" class="table-input input-number" />
            </td>
            <td class="table-td">{{ it.uom }}</td>
            <td class="table-td text-right" :class="shortageFor(it) ? 'text-red-600 font-semibold' : ''">
              {{ it.stockLoading ? '…' : it.availableQty }}
            </td>
            <td class="table-td text-right">{{ it.stockLoading ? '…' : it.reservedQty }}</td>
            <td class="table-td">
              <input v-model="it.preferredLotNo" type="text" class="table-input" />
            </td>
            <td class="table-td">
              <input v-model="it.lineNotes" type="text" class="table-input" />
            </td>
            <td class="table-td text-right">
              <button @click="items.splice(idx, 1)" class="text-red-600 hover:text-red-700 font-semibold">{{ $t('common.buttons.remove') }}</button>
            </td>
          </tr>
        </DataTable>
      </FormSection>

      <p v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ formError }}</p>
    </template>

    <!-- Sticky footer -->
    <div class="fixed bottom-0 left-0 right-0 bg-white border-t border-slate-200 p-4 flex justify-end gap-3 z-40">
      <button @click="$router.push('/outbound/orders')" class="px-6 py-3 rounded-xl bg-slate-100 text-slate-700 font-semibold">
        {{ $t('common.buttons.cancel') }}
      </button>
      <button @click="submitForm" :disabled="submitting || loading" class="px-6 py-3 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60">
        {{ submitting ? $t('orders.form.saving') : (isEdit ? $t('orders.form.saveChanges') : $t('orders.form.createOrder')) }}
      </button>
    </div>
  </div>
</template>

<script>
import { getOrder, createOrder, updateOrder } from '../../services/orders'
import { listWarehouses } from '../../services/warehouses'
import { listProducts } from '../../services/products'
import { listCustomers } from '../../services/customers'
import { listCustomerDeliveryLocations } from '../../services/customerDeliveryLocations'
import { listInventory } from '../../services/inventory'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'
import SearchableSelect from '../../components/ui/SearchableSelect.vue'

export default {
  name: 'OrderFormView',
  components: { PageHeader, FormSection, DataTable, SearchableSelect },
  props: { id: { type: [String, Number], default: null } },
  data() {
    return {
      loading: false,
      loadError: '',
      formError: '',
      submitting: false,
      addingItem: false,
      warehouses: [],
      form: {
        orderType: 'SALES_ORDER', priority: 'NORMAL', warehouseId: '',
        customer: '', customerId: null, shipToId: '',
        externalReference: '', requiredShippingDate: '', requiredDeliveryDate: '',
        carrier: '', shippingMethod: '', deliveryInstructions: '',
        preparedBy: '', approvedBy: '', remark: ''
      },
      customerDisplayName: '',
      shipToOptions: [],
      shipToLoading: false,
      itemDraft: { sku: '', orderedQty: null, preferredLotNo: '', fefoPreferred: true },
      items: []
    }
  },
  computed: {
    isEdit() {
      return !!this.id
    }
  },
  async mounted() {
    await this.fetchWarehouses()
    if (this.isEdit) {
      await this.loadOrder()
    }
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
        this.loadError = err.message
      }
    },

    async loadOrder() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getOrder(this.id)
        const o = res.data
        if (o.status !== 'PENDING') {
          this.loadError = this.$t('orders.messages.orderNotEditable')
          return
        }
        this.form = {
          orderType: o.order_type || 'SALES_ORDER', priority: o.priority || 'NORMAL', warehouseId: o.warehouse_id,
          customer: o.customer || '', customerId: o.customer_id || null, shipToId: o.ship_to_id || '',
          externalReference: o.external_reference || '',
          requiredShippingDate: (o.required_shipping_date || '').slice(0, 10),
          requiredDeliveryDate: (o.required_delivery_date || '').slice(0, 10),
          carrier: o.carrier || '', shippingMethod: o.shipping_method || '',
          deliveryInstructions: o.delivery_instructions || '',
          preparedBy: o.prepared_by || '', approvedBy: o.approved_by || '', remark: o.remark || ''
        }
        this.customerDisplayName = o.customer || ''
        if (o.customer_id) await this.loadShipToOptions(o.customer_id)

        this.items = o.items.map((it) => ({
          productId: it.product_id, sku: it.sku, productName: it.product_name,
          orderedQty: it.ordered_qty, uom: it.uom, storageType: it.storage_type,
          preferredLotNo: it.preferred_lot_no || '', fefoPreferred: it.fefo_preferred, lineNotes: it.line_notes || '',
          availableQty: 0, reservedQty: 0, stockLoading: true
        }))
        for (const it of this.items) this.refreshStockPreview(it)
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadOrderDetailFailed')
      } finally {
        this.loading = false
      }
    },

    async fetchCustomerOptions(search) {
      try {
        const res = await listCustomers({ search, status: 'ACTIVE', limit: 20 })
        return res.data.map((c) => ({ value: c.id, label: `${c.code} — ${c.company_name}` }))
      } catch {
        return []
      }
    },

    async loadShipToOptions(customerId) {
      this.shipToLoading = true
      try {
        const res = await listCustomerDeliveryLocations(customerId, { status: 'ACTIVE' })
        this.shipToOptions = res.data
      } catch {
        this.shipToOptions = []
      } finally {
        this.shipToLoading = false
      }
    },

    async onCustomerChange(opt) {
      this.form.shipToId = ''
      this.shipToOptions = []
      if (!opt) return
      await this.loadShipToOptions(opt.value)
      const def = this.shipToOptions.find((s) => s.is_default)
      if (def) this.form.shipToId = def.id
    },

    // refreshStockPreview is a non-authoritative, display-only preview —
    // the same warehouse-wide aggregation internal/inventory.Service.
    // AvailabilityByProductWarehouse computes server-side, summed
    // client-side here across every (location, lot) row GET /inventory
    // returns for this product+warehouse. Never reserves anything.
    async refreshStockPreview(item) {
      if (!this.form.warehouseId) {
        item.availableQty = 0
        item.reservedQty = 0
        item.stockLoading = false
        return
      }
      item.stockLoading = true
      try {
        const res = await listInventory({ productId: item.productId, warehouseId: this.form.warehouseId, limit: 100 })
        item.availableQty = res.data.reduce((sum, row) => sum + Number(row.available_qty || 0), 0)
        item.reservedQty = res.data.reduce((sum, row) => sum + Number(row.reserved_qty || 0), 0)
      } catch {
        item.availableQty = 0
        item.reservedQty = 0
      } finally {
        item.stockLoading = false
      }
    },

    shortageFor(item) {
      return !item.stockLoading && Number(item.orderedQty || 0) > Number(item.availableQty || 0)
    },

    async addItem() {
      const sku = (this.itemDraft.sku || '').trim()
      if (!sku || !this.itemDraft.orderedQty) {
        this.formError = this.$t('orders.messages.skuQtyRequired')
        return
      }
      this.addingItem = true
      try {
        const res = await listProducts({ search: sku, limit: 20 })
        const product = res.data.find((p) => p.sku === sku || p.barcode === sku)
        if (!product) {
          this.formError = this.$t('orders.messages.noProductFound', { value: sku })
          return
        }
        const item = {
          productId: product.id, sku: product.sku, productName: product.name,
          orderedQty: this.itemDraft.orderedQty, uom: product.base_uom || '', storageType: product.storage_type || '',
          preferredLotNo: this.itemDraft.preferredLotNo || '', fefoPreferred: this.itemDraft.fefoPreferred, lineNotes: '',
          availableQty: 0, reservedQty: 0, stockLoading: true
        }
        this.items.push(item)
        this.refreshStockPreview(item)
        this.itemDraft = { sku: '', orderedQty: null, preferredLotNo: '', fefoPreferred: true }
        this.formError = ''
        this.$nextTick(() => {
          if (this.$refs.skuInput) this.$refs.skuInput.focus()
        })
      } catch (err) {
        this.formError = err.message || this.$t('orders.messages.lookupProductFailed')
      } finally {
        this.addingItem = false
      }
    },

    buildPayload() {
      return {
        order_type: this.form.orderType,
        priority: this.form.priority,
        warehouse_id: this.form.warehouseId,
        customer: this.form.customerId ? undefined : (this.form.customer || undefined),
        customer_id: this.form.customerId || undefined,
        ship_to_id: this.form.customerId ? (this.form.shipToId || undefined) : undefined,
        external_reference: this.form.externalReference || undefined,
        required_shipping_date: this.form.requiredShippingDate ? new Date(this.form.requiredShippingDate).toISOString() : undefined,
        required_delivery_date: this.form.requiredDeliveryDate ? new Date(this.form.requiredDeliveryDate).toISOString() : undefined,
        carrier: this.form.carrier || undefined,
        shipping_method: this.form.shippingMethod || undefined,
        delivery_instructions: this.form.deliveryInstructions || undefined,
        prepared_by: this.form.preparedBy || undefined,
        approved_by: this.form.approvedBy || undefined,
        remark: this.form.remark || undefined,
        items: this.items.map((it) => ({
          product_id: it.productId, ordered_qty: it.orderedQty, uom: it.uom || undefined,
          preferred_lot_no: it.preferredLotNo || undefined, fefo_preferred: it.fefoPreferred, line_notes: it.lineNotes || undefined
        }))
      }
    },

    async submitForm() {
      this.formError = ''
      if (!this.form.warehouseId) {
        this.formError = this.$t('orders.messages.warehouseRequired')
        return
      }
      if (this.items.length === 0) {
        this.formError = this.$t('orders.messages.atLeastOneItem')
        return
      }

      this.submitting = true
      try {
        let orderId = this.id
        if (this.isEdit) {
          await updateOrder(this.id, this.buildPayload())
        } else {
          const res = await createOrder(this.buildPayload())
          orderId = res.data.id
        }
        showToast(this.$t('orders.form.saveSuccess'), 'success')
        this.$router.push(`/outbound/orders/${orderId}`)
      } catch (err) {
        this.formError = err.message || this.$t('orders.messages.saveOrderFailed')
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

.input-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>

<template>
  <div class="space-y-6 pb-24">
    <PageHeader :title="$t('pickingBatches.new.title')" :subtitle="$t('pickingBatches.new.subtitle')">
      <template #actions>
        <button
          @click="$router.push('/outbound/picking-batches')"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('pickingBatches.new.backButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ formError }}
    </div>

    <FormSection :title="$t('pickingBatches.new.selectWarehouseLabel')">
      <select v-model="warehouseId" class="input max-w-sm" @change="onWarehouseChange">
        <option value="">{{ $t('pickingBatches.new.selectWarehouse') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }} — {{ w.name }}</option>
      </select>
    </FormSection>

    <FormSection v-if="warehouseId" :title="$t('pickingBatches.new.availableOrdersTitle')">
      <div v-if="ordersLoading" class="text-sm text-slate-400">{{ $t('pickingBatches.loading') }}</div>
      <div v-else-if="availableOrders.length === 0" class="text-sm text-slate-400">
        {{ $t('pickingBatches.new.noAvailableOrders') }}
      </div>
      <template v-else>
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-slate-200">
                <th class="table-th"></th>
                <th class="table-th">{{ $t('pickingBatches.new.orderNo') }}</th>
                <th class="table-th">{{ $t('pickingBatches.new.customer') }}</th>
                <th class="table-th">{{ $t('pickingBatches.new.shipTo') }}</th>
                <th class="table-th text-right">{{ $t('pickingBatches.new.items') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="o in availableOrders" :key="o.order_id" class="border-t hover:bg-orange-50">
                <td class="table-td"><input type="checkbox" v-model="selectedOrderIds" :value="o.order_id" /></td>
                <td class="table-td font-semibold text-slate-800">{{ o.order_no }}</td>
                <td class="table-td">{{ o.customer || '—' }}</td>
                <td class="table-td">{{ o.ship_to_name || '—' }}</td>
                <td class="table-td text-right">{{ o.item_count }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="text-xs text-slate-400 mt-3">
          {{ $t('pickingBatches.new.selectedCount', { count: selectedOrderIds.length }) }}
          — {{ $t('pickingBatches.new.minOrdersHint') }}
        </p>
      </template>
    </FormSection>

    <div v-if="warehouseId" class="fixed bottom-0 left-0 right-0 bg-white border-t border-slate-200 p-4 flex justify-end gap-3">
      <button
        @click="createBatch"
        :disabled="selectedOrderIds.length < 2 || submitting"
        class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-50"
      >
        {{ submitting ? $t('common.buttons.submitting') : $t('pickingBatches.new.createButton') }}
      </button>
    </div>
  </div>
</template>

<script>
import { createPickingBatch, listAvailableOrdersForBatch } from '../../services/pickingBatches'
import { listWarehouses } from '../../services/warehouses'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'

export default {
  name: 'NewPickingBatchPage',
  components: { PageHeader, FormSection },
  data() {
    return {
      warehouses: [],
      warehouseId: '',
      availableOrders: [],
      selectedOrderIds: [],
      ordersLoading: false,
      submitting: false,
      formError: ''
    }
  },
  async mounted() {
    try {
      const res = await listWarehouses({ limit: 100 })
      this.warehouses = res.data
    } catch (err) {
      this.formError = err.message || this.$t('pickingBatches.messages.loadWarehousesFailed')
    }
  },
  methods: {
    async onWarehouseChange() {
      this.selectedOrderIds = []
      this.availableOrders = []
      if (!this.warehouseId) return
      this.ordersLoading = true
      this.formError = ''
      try {
        const res = await listAvailableOrdersForBatch(this.warehouseId)
        this.availableOrders = res.data
      } catch (err) {
        this.formError = err.message || this.$t('pickingBatches.messages.loadAvailableOrdersFailed')
      } finally {
        this.ordersLoading = false
      }
    },

    async createBatch() {
      if (this.selectedOrderIds.length < 2) return
      this.submitting = true
      this.formError = ''
      try {
        const res = await createPickingBatch({ warehouseId: this.warehouseId, orderIds: this.selectedOrderIds })
        showToast(this.$t('pickingBatches.new.createSuccess'), 'success')
        this.$router.push(`/outbound/picking-batches/${res.data.id}`)
      } catch (err) {
        this.formError = err.message || this.$t('pickingBatches.messages.createFailed')
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

.table-th {
  padding: 10px 12px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  white-space: nowrap;
  text-align: left;
}

.table-td {
  padding: 10px 12px;
  color: #475569;
  white-space: nowrap;
}
</style>

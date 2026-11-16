<template>
  <div class="space-y-6 pb-10">
    <PageHeader :title="batch ? batch.batch_no : ''" :subtitle="batch ? batch.warehouse_code : ''">
      <template #actions>
        <button
          @click="$router.push('/outbound/picking-batches')"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('pickingBatches.detail.backButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>
    <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('pickingBatches.loading') }}</div>

    <template v-else-if="batch">
      <div class="bg-white rounded-2xl border border-slate-200 p-6 flex flex-wrap items-center gap-6">
        <div>
          <p class="text-xs text-slate-400">{{ $t('common.labels.status') }}</p>
          <StatusBadge :tone="statusTone(batch.status)">{{ statusLabel(batch.status) }}</StatusBadge>
        </div>
        <div v-if="batch.pick_no">
          <p class="text-xs text-slate-400">{{ $t('pickingBatches.table.pickNo') }}</p>
          <p class="font-semibold text-slate-800">{{ batch.pick_no }}</p>
        </div>

        <div class="ml-auto flex gap-3">
          <button
            v-if="batch.status === 'DRAFT' && canConfirm"
            @click="confirmBatch"
            :disabled="submitting"
            class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('pickingBatches.detail.confirmButton') }}
          </button>
          <button
            v-if="batch.status === 'DRAFT' && canCancel"
            @click="showCancelConfirm = true"
            :disabled="submitting"
            class="bg-white border border-red-300 hover:bg-red-50 text-red-600 px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ $t('pickingBatches.detail.cancelButton') }}
          </button>
          <button
            v-if="batch.status === 'CONFIRMED' || batch.status === 'COMPLETED'"
            @click="$router.push('/outbound/picking')"
            class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
          >
            {{ $t('pickingBatches.detail.goToPicking') }}
          </button>
          <button
            v-if="batch.status === 'CONFIRMED' || batch.status === 'COMPLETED'"
            @click="$router.push(`/outbound/picking-batches/${id}/sortation`)"
            class="bg-slate-800 hover:bg-slate-900 text-white px-5 py-3 rounded-xl font-semibold"
          >
            {{ $t('pickingBatches.detail.goToSortation') }}
          </button>
          <button
            v-if="batch.status === 'CONFIRMED' || batch.status === 'COMPLETED'"
            @click="printPickList"
            class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
          >
            📄 {{ $t('pickingBatches.detail.printPickListButton') }}
          </button>
          <button
            @click="showHistory = true"
            class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
          >
            📜 {{ $t('pickingBatches.detail.historyButton') }}
          </button>
        </div>
      </div>

      <DocumentHistoryPanel
        :open="showHistory"
        document-type="PICKING_BATCH_LIST"
        :source-id="id"
        :title="$t('pickingBatches.detail.historyButton')"
        @close="showHistory = false"
      />

      <div v-if="batch.status !== 'DRAFT' && batch.status !== 'CANCELLED'" class="bg-orange-50 border border-orange-200 text-orange-700 rounded-xl p-3 text-sm">
        {{ $t('pickingBatches.detail.confirmedNote') }}
      </div>

      <FormSection :title="$t('pickingBatches.detail.ordersTitle')">
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-slate-200">
                <th class="table-th">{{ $t('pickingBatches.detail.ordersTable.orderNo') }}</th>
                <th class="table-th">{{ $t('pickingBatches.detail.ordersTable.customer') }}</th>
                <th class="table-th">{{ $t('pickingBatches.detail.ordersTable.shipTo') }}</th>
                <th class="table-th">{{ $t('pickingBatches.detail.ordersTable.status') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="o in batch.orders" :key="o.order_id" class="border-t">
                <td class="table-td font-semibold text-slate-800">{{ o.order_no }}</td>
                <td class="table-td">{{ o.customer || '—' }}</td>
                <td class="table-td">{{ o.ship_to_name || '—' }}</td>
                <td class="table-td">{{ o.status }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </FormSection>

      <FormSection :title="$t('pickingBatches.detail.linesTitle')">
        <div v-for="(line, idx) in batch.lines" :key="idx" class="border border-slate-200 rounded-xl p-4 mb-4">
          <div class="flex flex-wrap gap-6 mb-3">
            <div>
              <p class="text-xs text-slate-400">{{ $t('pickingBatches.detail.linesTable.sku') }}</p>
              <p class="font-semibold text-slate-800">{{ line.sku }} — {{ line.product_name }}</p>
            </div>
            <div>
              <p class="text-xs text-slate-400">{{ $t('pickingBatches.detail.linesTable.lot') }}</p>
              <p class="font-semibold text-slate-800">{{ line.lot_no || '—' }}</p>
            </div>
            <div>
              <p class="text-xs text-slate-400">{{ $t('pickingBatches.detail.linesTable.location') }}</p>
              <p class="font-semibold text-slate-800">{{ line.location_code }}</p>
            </div>
            <div>
              <p class="text-xs text-slate-400">{{ $t('pickingBatches.detail.linesTable.qty') }}</p>
              <p class="font-semibold text-slate-800">{{ line.qty }} {{ line.uom }}</p>
            </div>
          </div>

          <p class="text-xs text-slate-400 mb-1">{{ $t('pickingBatches.detail.allocationsTitle') }}</p>
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-slate-100">
                <th class="table-th">{{ $t('pickingBatches.detail.allocationsTable.orderNo') }}</th>
                <th class="table-th text-right">{{ $t('pickingBatches.detail.allocationsTable.qty') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(a, aidx) in line.allocations" :key="aidx" class="border-t border-slate-100">
                <td class="table-td">{{ a.order_no }}</td>
                <td class="table-td text-right">{{ a.allocated_qty }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </FormSection>
    </template>

    <ConfirmDialog
      :open="showCancelConfirm"
      :title="$t('pickingBatches.detail.cancelButton')"
      :message="$t('pickingBatches.detail.confirmCancelMessage')"
      :confirm-label="$t('pickingBatches.detail.cancelButton')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      danger
      :loading="submitting"
      @cancel="showCancelConfirm = false"
      @confirm="cancelBatch"
    />
  </div>
</template>

<script>
import { getPickingBatch, confirmPickingBatch, cancelPickingBatch, viewConsolidatedPickList } from '../../services/pickingBatches'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import DocumentHistoryPanel from '../../components/documents/DocumentHistoryPanel.vue'

export default {
  name: 'PickingBatchDetailPage',
  components: { PageHeader, FormSection, StatusBadge, ConfirmDialog, DocumentHistoryPanel },
  props: { id: { type: [String, Number], required: true } },
  data() {
    return {
      loading: false,
      loadError: '',
      batch: null,
      submitting: false,
      showCancelConfirm: false,
      showHistory: false
    }
  },
  computed: {
    canConfirm() { return hasPermission('picking_batch.confirm') },
    canCancel() { return hasPermission('picking_batch.cancel') }
  },
  async mounted() {
    await this.fetchBatch()
  },
  methods: {
    async fetchBatch() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getPickingBatch(this.id)
        this.batch = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('pickingBatches.messages.loadDetailFailed')
      } finally {
        this.loading = false
      }
    },

    async confirmBatch() {
      this.submitting = true
      try {
        await confirmPickingBatch(this.id)
        showToast(this.$t('pickingBatches.detail.confirmSuccess'), 'success')
        await this.fetchBatch()
      } catch (err) {
        showToast(err.message || this.$t('pickingBatches.messages.confirmFailed'), 'error')
      } finally {
        this.submitting = false
      }
    },

    async cancelBatch() {
      this.submitting = true
      try {
        await cancelPickingBatch(this.id)
        showToast(this.$t('pickingBatches.detail.cancelSuccess'), 'success')
        this.showCancelConfirm = false
        await this.fetchBatch()
      } catch (err) {
        showToast(err.message || this.$t('pickingBatches.messages.cancelFailed'), 'error')
      } finally {
        this.submitting = false
      }
    },

    async printPickList() {
      try {
        await viewConsolidatedPickList(this.id)
      } catch (err) {
        showToast(err.message || this.$t('pickingBatches.messages.loadDetailFailed'), 'error')
      }
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'CONFIRMED') return 'orange'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'DRAFT': return this.$t('pickingBatches.statusLabels.draft')
        case 'CONFIRMED': return this.$t('pickingBatches.statusLabels.confirmed')
        case 'COMPLETED': return this.$t('pickingBatches.statusLabels.completed')
        case 'CANCELLED': return this.$t('pickingBatches.statusLabels.cancelled')
        default: return status
      }
    }
  }
}
</script>

<style scoped>
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

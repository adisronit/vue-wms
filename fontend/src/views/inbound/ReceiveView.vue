<template>
  <div>
    <PageHeader :title="$t('receiving.title')" :subtitle="$t('receiving.subtitle')">
      <template #actions>
        <button
          @click="exportCsv"
          :disabled="exporting"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ exporting ? $t('common.buttons.exporting') : '⬇ ' + $t('common.buttons.exportCsv') }}
        </button>

        <button
          v-if="canCreate"
          @click="$router.push('/inbound/receiving/new')"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('receiving.newButton') }}
        </button>
      </template>
    </PageHeader>

    <!-- Summary Cards: one card per real, distinct status — the previous
         version showed "Receiving" and "Partial Received" as two
         separate cards that both read the backend's single
         PARTIALLY_RECEIVED count, which looked like two different
         numbers when they were always identical. -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-5 mb-6">
      <div class="summary-card">
        <p class="summary-label">{{ $t('receiving.summary.waitingReceive') }}</p>
        <h2 class="summary-value">{{ waitingCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('receiving.summary.partialReceived') }}</p>
        <h2 class="summary-value">{{ partiallyReceivedCount }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('receiving.summary.completed') }}</p>
        <h2 class="summary-value">{{ completedCount }}</h2>
      </div>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-6">
      {{ loadError }}
    </div>

    <FilterBar>
      <input
        v-model="filters.keyword"
        type="text"
        :placeholder="$t('receiving.filters.searchPlaceholder')"
        class="input"
      />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('receiving.filters.allStatus') }}</option>
        <option value="WAITING">{{ $t('receiving.filters.statusWaiting') }}</option>
        <option value="PARTIALLY_RECEIVED">{{ $t('receiving.filters.statusPartial') }}</option>
        <option value="COMPLETED">{{ $t('receiving.filters.statusCompleted') }}</option>
      </select>

      <input v-model="filters.date" type="date" class="input" />

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="receivingList.length === 0"
      :loading-text="$t('receiving.loadingDocuments')"
      :empty-text="$t('receiving.noDocuments')"
      :colspan="9"
      min-width="1100px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('receiving.table.documentNo') }}</th>
          <th class="table-th">{{ $t('receiving.table.asnNo') }}</th>
          <th class="table-th">{{ $t('receiving.table.vendor') }}</th>
          <th class="table-th">{{ $t('receiving.table.warehouse') }}</th>
          <th class="table-th">{{ $t('receiving.table.expectedDate') }}</th>
          <th class="table-th">{{ $t('receiving.table.items') }}</th>
          <th class="table-th">{{ $t('receiving.table.progress') }}</th>
          <th class="table-th">{{ $t('receiving.table.status') }}</th>
          <th class="table-th text-right">{{ $t('receiving.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="item in receivingList"
        :key="item.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">{{ item.documentNo }}</td>
        <td class="table-td">{{ item.asnNo }}</td>
        <td class="table-td">{{ item.vendor }}</td>
        <td class="table-td">{{ item.warehouse }}</td>
        <td class="table-td">{{ item.expectedDate }}</td>
        <td class="table-td text-right">{{ item.items }}</td>

        <td class="table-td">
          <div class="w-32">
            <div class="h-2 bg-slate-200 rounded-full overflow-hidden">
              <div class="h-2 bg-orange-600 rounded-full" :style="{ width: item.progress + '%' }"></div>
            </div>
            <p class="text-xs text-slate-500 mt-1 tabular-nums">{{ item.progress }}%</p>
          </div>
        </td>

        <td class="table-td">
          <StatusBadge :tone="statusTone(item.status)">{{ statusText(item.status) }}</StatusBadge>
        </td>

        <td class="table-td text-right">
          <button @click="openReceiving(item)" class="text-orange-600 hover:text-orange-700 font-semibold">
            {{ $t('receiving.table.startReceive') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <!-- Receiving Detail Panel -->
    <div v-if="selectedReceiving" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">{{ $t('receiving.detail.title') }}</h2>
            <p class="text-slate-500">{{ selectedReceiving.documentNo }}</p>
          </div>

          <button @click="selectedReceiving = null" class="text-slate-500 hover:text-red-600">✕</button>
        </div>

        <div class="space-y-4">
          <div class="detail-row">
            <span>{{ $t('receiving.detail.vendor') }}</span>
            <strong>{{ selectedReceiving.vendor }}</strong>
          </div>

          <div class="detail-row">
            <span>{{ $t('receiving.detail.warehouse') }}</span>
            <strong>{{ selectedReceiving.warehouse }}</strong>
          </div>

          <div class="detail-row">
            <span>{{ $t('receiving.detail.expectedDate') }}</span>
            <strong>{{ selectedReceiving.expectedDate }}</strong>
          </div>

          <div class="detail-row">
            <span>{{ $t('receiving.detail.status') }}</span>
            <StatusBadge :tone="statusTone(selectedReceiving.status)">{{ statusText(selectedReceiving.status) }}</StatusBadge>
          </div>
        </div>

        <!-- Cancellation details (Phase 18) -->
        <div v-if="selectedReceiving.status === 'CANCELLED'" class="mt-4 bg-red-50 border border-red-200 rounded-xl p-4 space-y-2">
          <div class="detail-row">
            <span>{{ $t('receiving.detail.cancelledAt') }}</span>
            <strong>{{ formatDateTime(selectedReceiving.cancelledAt) }}</strong>
          </div>
          <div class="detail-row">
            <span>{{ $t('receiving.detail.cancelledBy') }}</span>
            <strong>{{ selectedReceiving.cancelledByName || '—' }}</strong>
          </div>
          <div class="detail-row">
            <span>{{ $t('receiving.detail.cancellationRemark') }}</span>
            <strong>{{ selectedReceiving.cancelRemark || '—' }}</strong>
          </div>
        </div>

        <div v-if="detailError" class="mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
          {{ detailError }}
        </div>

        <div class="mt-8">
          <h3 class="font-bold text-slate-800 mb-4">{{ $t('receiving.detail.itemsTitle') }}</h3>

          <p v-if="detailLoading" class="text-slate-400 text-sm">{{ $t('receiving.detail.loadingItems') }}</p>
          <p v-else-if="detailItems.length === 0" class="text-slate-400 text-sm">{{ $t('receiving.detail.noLines') }}</p>

          <div v-for="line in detailItems" :key="line.itemId" class="border border-slate-200 rounded-xl p-4 mb-3">
            <div class="flex justify-between">
              <div>
                <p class="font-semibold text-slate-800">{{ line.sku }}</p>
                <p class="text-sm text-slate-500">{{ line.name }}</p>
                <p class="text-xs text-slate-400 mt-1">
                  {{ $t('receiving.detail.receivedOf', { received: line.receivedQty, expected: line.expectedQty || '—' }) }}
                </p>
              </div>

              <div class="text-right">
                <p class="text-sm text-slate-500">{{ $t('receiving.detail.expectedQty') }}</p>
                <p class="font-bold tabular-nums">{{ line.expectedQty }}</p>
              </div>
            </div>

            <div v-if="line.status !== 'COMPLETED'" class="grid grid-cols-2 gap-3 mt-4">
              <input v-model.number="line.receiveQty" type="number" :placeholder="$t('receiving.detail.receiveQtyPlaceholder')" class="input" />

              <select v-model="line.locationId" class="input">
                <option value="">{{ $t('receiving.detail.selectLocation') }}</option>
                <option v-for="l in detailLocations" :key="l.id" :value="l.id">{{ l.code }}</option>
              </select>
            </div>
            <p v-else class="text-sm text-green-600 mt-3 font-semibold">{{ $t('receiving.detail.fullyReceived') }}</p>
          </div>

          <button
            v-if="canConfirm && detailItems.some((l) => l.status !== 'COMPLETED')"
            @click="confirmDetailReceiving"
            :disabled="confirmSubmitting"
            class="w-full bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold mt-4 disabled:opacity-60"
          >
            {{ confirmSubmitting ? $t('receiving.detail.confirming') : $t('receiving.detail.confirmButton') }}
          </button>
          <p v-else-if="!canConfirm" class="text-sm text-slate-500 mt-4">
            {{ $t('receiving.detail.viewOnlyNote') }}
          </p>

          <div v-if="canCancelReceiving && isReceivingCancellable" class="mt-6 pt-6 border-t border-slate-200">
            <input v-model="cancelRemark" type="text" class="input mb-3" :placeholder="$t('receiving.detail.cancelRemarkPlaceholder')" />
            <button
              @click="showCancelConfirm = true"
              :disabled="confirmSubmitting"
              class="w-full bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
            >
              {{ $t('receiving.detail.cancelButton') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="showCancelConfirm"
      :title="$t('receiving.detail.cancelButton')"
      :message="$t('receiving.messages.confirmCancelReceipt')"
      :confirm-label="$t('receiving.detail.cancelButton')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('receiving.detail.confirming')"
      danger
      :loading="confirmSubmitting"
      @cancel="showCancelConfirm = false"
      @confirm="cancelDetailReceiving"
    />
  </div>
</template>

<script>
import { listReceipts, getReceipt, confirmReceipt, cancelReceipt, newIdempotencyKey, exportReceipts } from '../../services/receiving'
import { listZones, listLocations } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import i18n from '../../i18n'

let searchDebounce = null

// Called from the template (methods.statusText below), not cached onto
// a row at map time, so the displayed label stays reactive to the
// active locale — switching language must not require re-fetching.
function statusText(status) {
  const t = i18n.global.t
  switch (status) {
    case 'DRAFT': return t('common.status.draft')
    case 'WAITING': return t('receiving.filters.statusWaiting')
    case 'PARTIALLY_RECEIVED': return t('receiving.filters.statusPartial')
    case 'COMPLETED': return t('common.status.completed')
    case 'CANCELLED': return t('common.status.cancelled')
    default: return status
  }
}

function statusTone(status) {
  if (status === 'PARTIALLY_RECEIVED') return 'orange'
  if (status === 'COMPLETED') return 'green'
  if (status === 'CANCELLED') return 'red'
  return 'gray'
}

// The real backend's field names (receipt_no, reference_no, ...) are
// mapped onto the mock's existing display field names (documentNo,
// asnNo, ...) so the template needs no further changes.
function mapReceiptListFromApi(r) {
  return {
    id: r.id,
    documentNo: r.receipt_no,
    asnNo: r.reference_no || '—',
    vendor: r.vendor || '—',
    warehouse: r.warehouse_code,
    expectedDate: r.receiving_date ? r.receiving_date.slice(0, 10) : '—',
    items: r.item_count,
    progress: Math.round(r.progress_pct),
    status: r.status
  }
}

function mapDetailItem(item) {
  return {
    itemId: item.id,
    productId: item.product_id,
    sku: item.sku,
    name: item.product_name,
    expectedQty: item.expected_qty,
    receivedQty: item.received_qty,
    uom: item.uom,
    lotNo: item.lot_no,
    expiryDate: item.expiry_date ? item.expiry_date.slice(0, 10) : '',
    locationId: item.location_id,
    status: item.status,
    receiveQty: ''
  }
}

export default {
  name: 'ReceivingPage',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, ConfirmDialog },

  data() {
    return {
      loading: false,
      loadError: '',
      exporting: false,
      receivingList: [],
      allReceipts: [],

      selectedReceiving: null,
      detailItems: [],
      detailLocations: [],
      detailLoading: false,
      detailError: '',
      confirmSubmitting: false,
      cancelRemark: '',
      showCancelConfirm: false,

      filters: {
        keyword: '',
        status: '',
        date: ''
      }
    }
  },

  computed: {
    waitingCount() {
      return this.allReceipts.filter((r) => r.status === 'WAITING').length
    },

    partiallyReceivedCount() {
      return this.allReceipts.filter((r) => r.status === 'PARTIALLY_RECEIVED').length
    },

    completedCount() {
      return this.allReceipts.filter((r) => r.status === 'COMPLETED').length
    },

    canCreate() {
      return hasPermission('receiving.create')
    },

    canConfirm() {
      return hasPermission('receiving.approve')
    },

    // Cancel is gated on receiving.update, not receiving.approve — see
    // the backend's own internal/receiving/handler.go doc: it only ever
    // runs before any stock has been received, the same authority level
    // as editing the receipt.
    canCancelReceiving() {
      return hasPermission('receiving.update')
    },

    isReceivingCancellable() {
      return !!this.selectedReceiving &&
        (this.selectedReceiving.status === 'DRAFT' || this.selectedReceiving.status === 'WAITING')
    }
  },

  watch: {
    'filters.keyword'() {
      clearTimeout(searchDebounce)
      searchDebounce = setTimeout(() => this.fetchReceipts(), 300)
    },
    'filters.status'() {
      this.fetchReceipts()
    },
    'filters.date'() {
      this.fetchReceipts()
    }
  },

  mounted() {
    this.fetchReceipts()
    this.fetchSummary()
  },

  methods: {
    statusText,
    statusTone,

    async fetchReceipts() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listReceipts({
          search: this.filters.keyword,
          status: this.filters.status,
          date: this.filters.date,
          limit: 100
        })
        this.receivingList = res.data.map(mapReceiptListFromApi)
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    // A separate, unfiltered fetch backing the summary cards — there is
    // no dedicated stats endpoint this phase, so the cards read from the
    // same list API with a larger page size instead of the current
    // (possibly filtered) table data.
    async fetchSummary() {
      try {
        const res = await listReceipts({ limit: 200 })
        this.allReceipts = res.data
      } catch {
        // Non-critical: the table's own error banner already surfaces API failures.
      }
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.date = ''
      this.fetchReceipts()
    },

    async exportCsv() {
      this.exporting = true
      try {
        await exportReceipts({ search: this.filters.keyword, status: this.filters.status, date: this.filters.date })
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.exporting = false
      }
    },

    async openReceiving(item) {
      this.selectedReceiving = item
      this.detailItems = []
      this.detailLocations = []
      this.detailError = ''
      this.cancelRemark = ''
      this.detailLoading = true
      try {
        const res = await getReceipt(item.id)
        this.detailItems = res.data.items.map(mapDetailItem)
        await this.fetchDetailLocations(res.data.warehouse_id)
        this.selectedReceiving = {
          ...item,
          cancelledAt: res.data.cancelled_at,
          cancelledByName: res.data.cancelled_by_name,
          cancelRemark: res.data.cancel_remark
        }
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.detailLoading = false
      }
    },

    async fetchDetailLocations(warehouseId) {
      if (!warehouseId) return
      const zonesRes = await listZones(warehouseId)
      const allLocations = []
      for (const zone of zonesRes.data) {
        const locRes = await listLocations(zone.id)
        allLocations.push(...locRes.data)
      }
      this.detailLocations = allLocations
    },

    async confirmDetailReceiving() {
      const linesToConfirm = this.detailItems
        .filter((line) => Number(line.receiveQty || 0) > 0)
        .map((line) => ({
          item_id: line.itemId,
          quantity: Number(line.receiveQty),
          lot_no: line.lotNo || '',
          expiry_date: line.expiryDate ? `${line.expiryDate}T00:00:00Z` : null,
          location_id: line.locationId
        }))

      if (linesToConfirm.length === 0) {
        this.detailError = this.$t('receiving.messages.enterReceiveQty')
        return
      }
      if (linesToConfirm.some((l) => !l.location_id)) {
        this.detailError = this.$t('receiving.messages.locationRequired')
        return
      }

      this.confirmSubmitting = true
      this.detailError = ''
      try {
        await confirmReceipt(this.selectedReceiving.id, {
          idempotency_key: newIdempotencyKey(),
          items: linesToConfirm
        })
        showToast(this.$t('receiving.messages.receivingConfirmed'), 'success')
        this.selectedReceiving = null
        await this.fetchReceipts()
        await this.fetchSummary()
      } catch (err) {
        this.detailError = err.message
      } finally {
        this.confirmSubmitting = false
      }
    },

    async cancelDetailReceiving() {
      this.confirmSubmitting = true
      this.detailError = ''
      try {
        await cancelReceipt(this.selectedReceiving.id, {
          remark: this.cancelRemark || undefined
        })
        this.showCancelConfirm = false
        showToast(this.$t('receiving.messages.receiptCancelled'), 'success')
        this.selectedReceiving = null
        await this.fetchReceipts()
        await this.fetchSummary()
      } catch (err) {
        this.detailError = err.message
        this.showCancelConfirm = false
      } finally {
        this.confirmSubmitting = false
      }
    },

    formatDateTime(isoString) {
      if (!isoString) return '—'
      return this.$d(new Date(isoString), 'long')
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
  padding: 14px 16px;
  font-weight: var(--fw-table-header);
  font-size: var(--fs-table-header);
  white-space: nowrap;
}

.table-td {
  padding: 14px 16px;
  color: #475569;
  white-space: nowrap;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  border-bottom: 1px solid #e2e8f0;
  padding-bottom: 12px;
  color: #475569;
}
</style>

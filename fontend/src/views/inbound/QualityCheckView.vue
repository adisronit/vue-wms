<template>
  <div class="space-y-6">
    <PageHeader :title="$t('qualityCheck.title')" :subtitle="$t('qualityCheck.subtitle')">
      <template #actions>
        <button
          @click="$router.push('/inbound/quality-check/new')"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('qualityCheck.newButton') }}
        </button>
      </template>
    </PageHeader>

    <!-- Summary Cards -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('qualityCheck.summary.pendingQc') }}</p>
        <h2 class="summary-value">{{ statusCounts.PENDING }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.inProgress') }}</p>
        <h2 class="summary-value">{{ statusCounts.IN_PROGRESS }}</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.completed') }}</p>
        <h2 class="summary-value text-green-600">{{ statusCounts.COMPLETED }}</h2>
      </div>
    </div>

    <FilterBar>
      <input
        v-model="filters.search"
        type="text"
        :placeholder="$t('qualityCheck.filters.searchPlaceholder')"
        class="input"
        @keyup.enter="fetchInspections"
      />

      <select v-model="filters.status" class="input" @change="fetchInspections">
        <option value="">{{ $t('qualityCheck.filters.allStatus') }}</option>
        <option value="PENDING">{{ $t('common.status.pending') }}</option>
        <option value="IN_PROGRESS">{{ $t('common.status.inProgress') }}</option>
        <option value="COMPLETED">{{ $t('common.status.completed') }}</option>
      </select>

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <DataTable
      :is-empty="inspections.length === 0"
      :empty-text="loading ? $t('common.buttons.loading') : $t('common.pagination.noData')"
      :colspan="7"
      min-width="1000px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('qualityCheck.table.qcNo') }}</th>
          <th class="table-th">{{ $t('qualityCheck.table.poNo') }}</th>
          <th class="table-th">{{ $t('newReceiving.fields.warehouse') }}</th>
          <th class="table-th">{{ $t('newQualityCheck.fields.qcType') }}</th>
          <th class="table-th text-right">{{ $t('newQualityCheck.summary.totalLines') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th text-right">{{ $t('qualityCheck.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="item in inspections" :key="item.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ item.inspection_no }}</td>
        <td class="table-td">{{ item.receipt_no }}</td>
        <td class="table-td">{{ item.warehouse_code }}</td>
        <td class="table-td">{{ item.qc_type }}</td>
        <td class="table-td text-right tabular-nums">{{ item.item_count }}</td>

        <td class="table-td">
          <StatusBadge :tone="statusTone(item.status)">{{ statusLabel(item.status) }}</StatusBadge>
        </td>

        <td class="table-td text-right">
          <button @click="openInspection(item)" class="text-orange-600 hover:text-orange-700 font-semibold">
            {{ $t('qualityCheck.table.inspect') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <!-- QC Drawer -->
    <div v-if="selected" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-3xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">{{ $t('qualityCheck.drawer.title') }}</h2>
            <p class="text-slate-500">{{ selected.inspection_no }} / {{ selected.receipt_no }}</p>
          </div>

          <button @click="closeDrawer" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
        </div>

        <!-- Phase 31D — synchronous, read-only PDF document. -->
        <div class="mb-6">
          <button
            @click="viewQCReportPdf"
            class="bg-slate-100 hover:bg-slate-200 text-slate-700 text-sm px-3 py-2 rounded-lg font-semibold"
          >
            📄 {{ $t('qualityCheck.documents.viewReport') }}
          </button>
        </div>

        <div v-if="drawerError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mb-4">
          {{ drawerError }}
        </div>

        <div class="space-y-5">
          <div v-for="item in selected.items" :key="item.id" class="bg-slate-50 rounded-2xl p-5">
            <div class="flex flex-wrap items-baseline justify-between gap-2 mb-3">
              <p class="font-bold text-slate-800">{{ item.sku }} — {{ item.product_name }}</p>
              <StatusBadge :tone="itemStatusTone(item.status)">{{ item.status }}</StatusBadge>
            </div>

            <div class="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm mb-4">
              <div><p class="detail-label">{{ $t('qualityCheck.drawer.lotNo') }}</p><p class="detail-value">{{ item.lot_no || '—' }}</p></div>
              <div><p class="detail-label">{{ $t('newReceiving.table.expiryDate') }}</p><p class="detail-value">{{ $d(item.expiry_date) }}</p></div>
              <div><p class="detail-label">{{ $t('newReceiving.table.location') }}</p><p class="detail-value">{{ item.location_code }}</p></div>
              <div><p class="detail-label">{{ $t('qualityCheck.drawer.remainingHeldQty') }}</p><p class="detail-value tabular-nums">{{ item.remaining_held_qty }}</p></div>
            </div>

            <!-- Disposition history -->
            <div v-if="item.dispositions && item.dispositions.length" class="mb-4">
              <p class="detail-label mb-2">{{ $t('qualityCheck.drawer.dispositionHistory') }}</p>
              <ul class="space-y-1 text-sm">
                <li v-for="d in item.dispositions" :key="d.id" class="flex justify-between bg-white rounded-lg px-3 py-2 border border-slate-200">
                  <span>
                    <StatusBadge :tone="resultTone(d.disposition)">{{ d.disposition }}</StatusBadge>
                    <span class="ml-2 tabular-nums">{{ d.quantity }}</span>
                    <span v-if="d.defect_code" class="ml-2 text-slate-500">({{ d.defect_code }})</span>
                  </span>
                  <span class="text-slate-400">{{ d.decided_by_name || '—' }}</span>
                </li>
              </ul>
            </div>

            <!-- Pending disposition entries for this item -->
            <div v-if="item.remaining_held_qty > 0">
              <p class="detail-label mb-2">{{ $t('qualityCheck.drawer.addDisposition') }}</p>

              <div v-for="(p, pIdx) in pendingByItem[item.id]" :key="pIdx" class="flex items-center gap-2 mb-2">
                <StatusBadge :tone="resultTone(p.disposition)">{{ p.disposition }}</StatusBadge>
                <span class="tabular-nums text-sm">{{ p.quantity }}</span>
                <span v-if="p.defectCode" class="text-sm text-slate-500">({{ p.defectCode }})</span>
                <button @click="removePending(item.id, pIdx)" class="text-red-600 hover:text-red-700 text-sm font-semibold ml-auto">
                  {{ $t('common.buttons.remove') }}
                </button>
              </div>

              <div class="grid grid-cols-2 md:grid-cols-5 gap-2">
                <select v-model="draft[item.id].disposition" class="input bg-white">
                  <option value="PASS">{{ $t('qualityCheck.results.pass') }}</option>
                  <option value="HOLD">{{ $t('qualityCheck.results.hold') }}</option>
                  <option value="REJECT">{{ $t('qualityCheck.results.fail') }}</option>
                </select>
                <input v-model.number="draft[item.id].quantity" type="number" class="input bg-white input-number" :placeholder="$t('qualityCheck.table.sampleQty')" />
                <select v-model="draft[item.id].defectCode" class="input bg-white">
                  <option value="">{{ $t('qualityCheck.defectReasons.none') }}</option>
                  <option value="DAMAGED">{{ $t('qualityCheck.defectReasons.damaged') }}</option>
                  <option value="WRONG_ITEM">{{ $t('qualityCheck.defectReasons.wrongItem') }}</option>
                  <option value="EXPIRED">{{ $t('qualityCheck.defectReasons.expired') }}</option>
                  <option value="SHORTAGE">{{ $t('qualityCheck.defectReasons.shortage') }}</option>
                  <option value="PACKAGING_ISSUE">{{ $t('qualityCheck.defectReasons.packagingIssue') }}</option>
                </select>
                <input v-model="draft[item.id].evidenceReference" class="input bg-white" :placeholder="$t('qualityCheck.drawer.evidenceReference')" />
                <button @click="addPending(item)" class="bg-slate-800 hover:bg-slate-900 text-white rounded-xl font-semibold">
                  {{ $t('common.buttons.add') }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-3 mt-6 sticky bottom-0 bg-white pt-4 border-t">
          <button @click="closeDrawer" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-6 py-3 rounded-xl font-semibold">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button
            @click="confirmClick"
            :disabled="confirming || totalPendingCount === 0"
            class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ confirming ? $t('newReceiving.actions.submitting') : $t('qualityCheck.drawer.confirmDispositions') }}
          </button>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="showConfirmDialog"
      :title="$t('qualityCheck.drawer.confirmDispositions')"
      :message="$t('newQualityCheck.messages.confirmSubmitDialog')"
      :confirm-label="$t('qualityCheck.drawer.confirmDispositions')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading="confirming"
      :error-message="confirmError"
      @cancel="showConfirmDialog = false"
      @confirm="submitConfirm"
    />
  </div>
</template>

<script>
import { listInspections, getInspection, confirmInspection, newIdempotencyKey, viewQCReport } from '../../services/qualitycheck'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'
import i18n from '../../i18n'

const { t } = i18n.global

function statusLabel(status) {
  switch (status) {
    case 'PENDING': return t('common.status.pending')
    case 'IN_PROGRESS': return t('common.status.inProgress')
    case 'COMPLETED': return t('common.status.completed')
    default: return status
  }
}

function statusTone(status) {
  if (status === 'IN_PROGRESS') return 'orange'
  if (status === 'COMPLETED') return 'green'
  return 'gray'
}

function resultTone(disposition) {
  if (disposition === 'PASS') return 'green'
  if (disposition === 'REJECT') return 'red'
  if (disposition === 'HOLD') return 'blue'
  return 'gray'
}

export default {
  name: 'QualityCheckView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge, ConfirmDialog },

  data() {
    return {
      inspections: [],
      loading: false,
      loadError: '',

      filters: { search: '', status: '' },

      selected: null,
      drawerError: '',
      draft: {},
      pendingByItem: {},
      confirming: false,
      showConfirmDialog: false,
      confirmError: ''
    }
  },

  computed: {
    statusCounts() {
      const counts = { PENDING: 0, IN_PROGRESS: 0, COMPLETED: 0 }
      for (const item of this.inspections) {
        if (counts[item.status] !== undefined) counts[item.status]++
      }
      return counts
    },

    totalPendingCount() {
      return Object.values(this.pendingByItem).reduce((sum, list) => sum + list.length, 0)
    }
  },

  mounted() {
    this.fetchInspections()
  },

  methods: {
    statusLabel,
    statusTone,
    resultTone,

    itemStatusTone(status) {
      if (status === 'IN_PROGRESS') return 'orange'
      if (status === 'COMPLETED') return 'green'
      return 'gray'
    },

    async fetchInspections() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listInspections({
          search: this.filters.search, status: this.filters.status, limit: 100
        })
        this.inspections = res.data
      } catch (err) {
        this.loadError = err.message
      } finally {
        this.loading = false
      }
    },

    clearFilter() {
      this.filters.search = ''
      this.filters.status = ''
      this.fetchInspections()
    },

    async openInspection(row) {
      this.drawerError = ''
      try {
        const res = await getInspection(row.id)
        this.selected = res.data
        this.draft = {}
        this.pendingByItem = {}
        for (const item of this.selected.items) {
          this.draft[item.id] = { disposition: 'PASS', quantity: item.remaining_held_qty, defectCode: '', evidenceReference: '' }
          this.pendingByItem[item.id] = []
        }
      } catch (err) {
        this.loadError = err.message
      }
    },

    closeDrawer() {
      this.selected = null
    },

    async viewQCReportPdf() {
      try {
        await viewQCReport(this.selected.id)
      } catch (err) {
        showToast(err.message, 'error')
      }
    },

    addPending(item) {
      const d = this.draft[item.id]
      if (!d.quantity || d.quantity <= 0) {
        this.drawerError = this.$t('qualityCheck.messages.sampleQtyRequired')
        return
      }
      if (d.disposition === 'REJECT' && !d.defectCode) {
        this.drawerError = this.$t('qualityCheck.messages.defectReasonRequired')
        return
      }
      this.drawerError = ''
      this.pendingByItem[item.id].push({ ...d })
      this.draft[item.id] = { disposition: 'PASS', quantity: 0, defectCode: '', evidenceReference: '' }
    },

    removePending(itemId, index) {
      this.pendingByItem[itemId].splice(index, 1)
    },

    confirmClick() {
      if (this.totalPendingCount === 0) return
      this.confirmError = ''
      this.showConfirmDialog = true
    },

    async submitConfirm() {
      this.confirming = true
      this.confirmError = ''
      try {
        const dispositions = []
        for (const [itemId, list] of Object.entries(this.pendingByItem)) {
          for (const p of list) {
            dispositions.push({
              item_id: itemId,
              disposition: p.disposition,
              quantity: Number(p.quantity),
              defect_code: p.defectCode || '',
              evidence_reference: p.evidenceReference || ''
            })
          }
        }

        await confirmInspection(this.selected.id, {
          idempotency_key: newIdempotencyKey(),
          dispositions
        })

        this.showConfirmDialog = false
        showToast(this.$t('qualityCheck.messages.qcSaved', { result: '' }), 'success')
        this.selected = null
        await this.fetchInspections()
      } catch (err) {
        this.confirmError = err.message
      } finally {
        this.confirming = false
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

.input-number {
  text-align: right;
  font-variant-numeric: tabular-nums;
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

.detail-label {
  color: #64748b;
  font-size: 13px;
}

.detail-value {
  color: #0f172a;
  font-weight: 700;
  margin-top: 4px;
}
</style>

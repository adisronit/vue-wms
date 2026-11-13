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
    <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
      <div class="summary-card">
        <p class="summary-label">{{ $t('qualityCheck.summary.pendingQc') }}</p>
        <h2 class="summary-value">9</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('common.status.inProgress') }}</p>
        <h2 class="summary-value">4</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('qualityCheck.summary.passed') }}</p>
        <h2 class="summary-value">21</h2>
      </div>

      <div class="summary-card">
        <p class="summary-label">{{ $t('qualityCheck.summary.rejected') }}</p>
        <h2 class="summary-value text-red-600">2</h2>
      </div>
    </div>

    <FilterBar>
      <input
        v-model="filters.keyword"
        type="text"
        :placeholder="$t('qualityCheck.filters.searchPlaceholder')"
        class="input"
      />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('qualityCheck.filters.allStatus') }}</option>
        <option value="PENDING">{{ $t('common.status.pending') }}</option>
        <option value="IN_PROGRESS">{{ $t('common.status.inProgress') }}</option>
        <option value="PASSED">{{ $t('qualityCheck.summary.passed') }}</option>
        <option value="REJECTED">{{ $t('qualityCheck.summary.rejected') }}</option>
      </select>

      <select v-model="filters.result" class="input">
        <option value="">{{ $t('qualityCheck.filters.allResult') }}</option>
        <option value="PASS">{{ $t('qualityCheck.results.pass') }}</option>
        <option value="FAIL">{{ $t('qualityCheck.results.fail') }}</option>
        <option value="HOLD">{{ $t('qualityCheck.results.hold') }}</option>
      </select>

      <button
        @click="clearFilter"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold"
      >
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :is-empty="filteredQcList.length === 0"
      :empty-text="$t('common.pagination.noData')"
      :colspan="10"
      min-width="1100px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('qualityCheck.table.qcNo') }}</th>
          <th class="table-th">{{ $t('qualityCheck.table.poNo') }}</th>
          <th class="table-th">{{ $t('qualityCheck.table.sku') }}</th>
          <th class="table-th">{{ $t('qualityCheck.table.productName') }}</th>
          <th class="table-th text-right">{{ $t('qualityCheck.table.receivedQty') }}</th>
          <th class="table-th text-right">{{ $t('qualityCheck.table.sampleQty') }}</th>
          <th class="table-th text-right">{{ $t('qualityCheck.table.defectQty') }}</th>
          <th class="table-th">{{ $t('qualityCheck.table.result') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th text-right">{{ $t('qualityCheck.table.action') }}</th>
        </tr>
      </template>

      <tr
        v-for="item in filteredQcList"
        :key="item.id"
        class="border-t hover:bg-orange-50"
      >
        <td class="table-td font-semibold text-slate-800">{{ item.qcNo }}</td>
        <td class="table-td">{{ item.poNo }}</td>
        <td class="table-td">{{ item.sku }}</td>
        <td class="table-td">{{ item.productName }}</td>
        <td class="table-td text-right">{{ item.receivedQty }}</td>
        <td class="table-td text-right">{{ item.sampleQty }}</td>
        <td class="table-td text-right">{{ item.defectQty }}</td>

        <td class="table-td">
          <StatusBadge :tone="resultTone(item.result)">{{ resultLabel(item.result) }}</StatusBadge>
        </td>

        <td class="table-td">
          <StatusBadge :tone="statusTone(item.status)">{{ statusLabel(item.status) }}</StatusBadge>
        </td>

        <td class="table-td text-right">
          <button @click="openQc(item)" class="text-orange-600 hover:text-orange-700 font-semibold">
            {{ $t('qualityCheck.table.inspect') }}
          </button>
        </td>
      </tr>
    </DataTable>

    <!-- QC Drawer -->
    <div v-if="selectedQc" class="fixed inset-0 bg-black/40 flex justify-end z-50">
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">{{ $t('qualityCheck.drawer.title') }}</h2>
            <p class="text-slate-500">{{ selectedQc.qcNo }} / {{ selectedQc.sku }}</p>
          </div>

          <button @click="selectedQc = null" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
        </div>

        <!-- Item Info -->
        <div class="bg-slate-50 rounded-2xl p-5 mb-6">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <p class="detail-label">{{ $t('qualityCheck.drawer.product') }}</p>
              <p class="detail-value">{{ selectedQc.productName }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('qualityCheck.table.poNo') }}</p>
              <p class="detail-value">{{ selectedQc.poNo }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('qualityCheck.table.receivedQty') }}</p>
              <p class="detail-value tabular-nums">{{ selectedQc.receivedQty }}</p>
            </div>

            <div>
              <p class="detail-label">{{ $t('qualityCheck.drawer.lotNo') }}</p>
              <p class="detail-value">{{ selectedQc.lotNo }}</p>
            </div>
          </div>
        </div>

        <!-- QC Form -->
        <div class="space-y-4">
          <div>
            <label class="label">{{ $t('qualityCheck.table.sampleQty') }}</label>
            <input v-model="qcForm.sampleQty" type="number" class="input" :placeholder="$t('qualityCheck.drawer.sampleQtyPlaceholder')" />
          </div>

          <div>
            <label class="label">{{ $t('qualityCheck.table.defectQty') }}</label>
            <input v-model="qcForm.defectQty" type="number" class="input" :placeholder="$t('qualityCheck.drawer.defectQtyPlaceholder')" />
          </div>

          <div>
            <label class="label">{{ $t('qualityCheck.drawer.inspectionResult') }}</label>
            <select v-model="qcForm.result" class="input">
              <option value="">{{ $t('qualityCheck.drawer.selectResult') }}</option>
              <option value="PASS">{{ $t('qualityCheck.results.pass') }}</option>
              <option value="FAIL">{{ $t('qualityCheck.results.fail') }}</option>
              <option value="HOLD">{{ $t('qualityCheck.results.hold') }}</option>
            </select>
          </div>

          <div>
            <label class="label">{{ $t('qualityCheck.drawer.defectReason') }}</label>
            <select v-model="qcForm.defectReason" class="input">
              <option value="">{{ $t('qualityCheck.defectReasons.none') }}</option>
              <option value="DAMAGED">{{ $t('qualityCheck.defectReasons.damaged') }}</option>
              <option value="WRONG_ITEM">{{ $t('qualityCheck.defectReasons.wrongItem') }}</option>
              <option value="EXPIRED">{{ $t('qualityCheck.defectReasons.expired') }}</option>
              <option value="SHORTAGE">{{ $t('qualityCheck.defectReasons.shortage') }}</option>
              <option value="PACKAGING_ISSUE">{{ $t('qualityCheck.defectReasons.packagingIssue') }}</option>
            </select>
          </div>

          <div>
            <label class="label">{{ $t('common.labels.remark') }}</label>
            <textarea v-model="qcForm.remark" rows="4" class="input" :placeholder="$t('qualityCheck.drawer.remarkPlaceholder')"></textarea>
          </div>

          <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
            {{ formError }}
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-3 pt-4">
            <button @click="submitQc('PASS')" class="bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold">
              {{ $t('qualityCheck.drawer.pass') }}
            </button>

            <button @click="submitQc('HOLD')" class="bg-blue-600 hover:bg-blue-700 text-white py-3 rounded-xl font-semibold">
              {{ $t('qualityCheck.drawer.hold') }}
            </button>

            <button @click="submitQc('FAIL')" class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold">
              {{ $t('qualityCheck.drawer.reject') }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import i18n from '../../i18n'

const { t } = i18n.global

function resultLabel(result) {
  switch (result) {
    case 'PASS': return t('qualityCheck.results.pass')
    case 'FAIL': return t('qualityCheck.results.fail')
    case 'HOLD': return t('qualityCheck.results.hold')
    case 'WAITING': return t('qualityCheck.results.waiting')
    default: return result
  }
}

function resultTone(result) {
  if (result === 'PASS') return 'green'
  if (result === 'FAIL') return 'red'
  if (result === 'HOLD') return 'blue'
  return 'gray'
}

// Called from the template (methods.statusLabel below), not cached
// onto a qcList row, so the displayed label stays reactive to the
// active locale — switching language must not require a remount.
function statusLabel(status) {
  switch (status) {
    case 'PASSED': return t('qualityCheck.statusLabels.passed')
    case 'IN_PROGRESS': return t('qualityCheck.statusLabels.inProgress')
    case 'REJECTED': return t('qualityCheck.statusLabels.rejected')
    case 'PENDING': return t('qualityCheck.statusLabels.pending')
    default: return status
  }
}

function statusTone(status) {
  if (status === 'IN_PROGRESS') return 'orange'
  if (status === 'PASSED') return 'green'
  if (status === 'REJECTED') return 'red'
  return 'gray'
}

export default {
  name: 'QualityCheckView',

  components: { PageHeader, FilterBar, DataTable, StatusBadge },

  data() {
    return {
      selectedQc: null,
      formError: '',

      filters: {
        keyword: '',
        status: '',
        result: ''
      },

      qcForm: {
        sampleQty: '',
        defectQty: '',
        result: '',
        defectReason: '',
        remark: ''
      },

      // Phase 1 scope note: there is no quality_check backend module in
      // this codebase yet (only the permission catalog entries exist —
      // see internal/rbac/catalog.go's quality_check.* codes with no
      // matching internal/qualitycheck package). This screen's data has
      // always been local-only; this redesign restyles the markup
      // without inventing an API integration that does not exist, per
      // CLAUDE.md's "Do not invent missing functions or use mock data
      // in production" — the correct response to that rule here is to
      // leave the known-mock data exactly as it already was, not to
      // fabricate a fake backend call.
      qcList: [
        {
          id: 1,
          qcNo: 'QC-20260608-001',
          poNo: 'PO-20260608-001',
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          receivedQty: 100,
          sampleQty: 10,
          defectQty: 0,
          lotNo: 'LOT-0001',
          result: 'PASS',
          status: 'PASSED'
        },
        {
          id: 2,
          qcNo: 'QC-20260608-002',
          poNo: 'PO-20260608-002',
          sku: 'SKU-0002',
          productName: 'Packing Box Size M',
          receivedQty: 250,
          sampleQty: 20,
          defectQty: 3,
          lotNo: 'LOT-0002',
          result: 'HOLD',
          status: 'IN_PROGRESS'
        },
        {
          id: 3,
          qcNo: 'QC-20260608-003',
          poNo: 'PO-20260608-003',
          sku: 'SKU-0003',
          productName: 'RFID Tag',
          receivedQty: 500,
          sampleQty: 30,
          defectQty: 8,
          lotNo: 'LOT-0003',
          result: 'FAIL',
          status: 'REJECTED'
        },
        {
          id: 4,
          qcNo: 'QC-20260608-004',
          poNo: 'PO-20260608-004',
          sku: 'SKU-0004',
          productName: 'Carton Box Large',
          receivedQty: 300,
          sampleQty: 0,
          defectQty: 0,
          lotNo: 'LOT-0004',
          result: 'WAITING',
          status: 'PENDING'
        }
      ]
    }
  },

  computed: {
    filteredQcList() {
      return this.qcList.filter((item) => {
        const keyword = this.filters.keyword.toLowerCase()

        const matchKeyword =
          item.qcNo.toLowerCase().includes(keyword) ||
          item.poNo.toLowerCase().includes(keyword) ||
          item.sku.toLowerCase().includes(keyword) ||
          item.productName.toLowerCase().includes(keyword)

        const matchStatus =
          !this.filters.status || item.status === this.filters.status

        const matchResult =
          !this.filters.result || item.result === this.filters.result

        return matchKeyword && matchStatus && matchResult
      })
    }
  },

  methods: {
    resultLabel,
    resultTone,
    statusLabel,
    statusTone,

    openQc(item) {
      this.selectedQc = item
      this.formError = ''

      this.qcForm.sampleQty = item.sampleQty
      this.qcForm.defectQty = item.defectQty
      this.qcForm.result = item.result === 'WAITING' ? '' : item.result
      this.qcForm.defectReason = ''
      this.qcForm.remark = ''
    },

    clearFilter() {
      this.filters.keyword = ''
      this.filters.status = ''
      this.filters.result = ''
    },

    submitQc(result) {
      this.qcForm.result = result
      this.formError = ''

      if (!this.qcForm.sampleQty) {
        this.formError = this.$t('qualityCheck.messages.sampleQtyRequired')
        return
      }

      if (result === 'FAIL' && !this.qcForm.defectReason) {
        this.formError = this.$t('qualityCheck.messages.defectReasonRequired')
        return
      }

      showToast(this.$t('qualityCheck.messages.qcSaved', { result: resultLabel(result) }), 'success')
      this.selectedQc = null
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

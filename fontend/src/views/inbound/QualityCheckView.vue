<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">
          {{ $t('qualityCheck.title') }}
        </h1>
        <p class="text-slate-500">
          {{ $t('qualityCheck.subtitle') }}
        </p>
      </div>

      <button
        @click="$router.push('/inbound/quality-check/new')"
        class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
      >
        {{ $t('qualityCheck.newButton') }}
      </button>
    </div>

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

    <!-- Filter -->
    <div class="bg-white rounded-2xl border border-slate-200 p-5">
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
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
      </div>
    </div>

    <!-- QC Table -->
    <div class="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden">
      <div class="w-full overflow-x-auto">
        <table class="min-w-[1100px] w-full text-sm">
          <thead class="bg-slate-50 text-slate-500">
            <tr>
              <th class="table-th">{{ $t('qualityCheck.table.qcNo') }}</th>
              <th class="table-th">{{ $t('qualityCheck.table.poNo') }}</th>
              <th class="table-th">{{ $t('qualityCheck.table.sku') }}</th>
              <th class="table-th">{{ $t('qualityCheck.table.productName') }}</th>
              <th class="table-th">{{ $t('qualityCheck.table.receivedQty') }}</th>
              <th class="table-th">{{ $t('qualityCheck.table.sampleQty') }}</th>
              <th class="table-th">{{ $t('qualityCheck.table.defectQty') }}</th>
              <th class="table-th">{{ $t('qualityCheck.table.result') }}</th>
              <th class="table-th">{{ $t('common.labels.status') }}</th>
              <th class="table-th text-right">{{ $t('qualityCheck.table.action') }}</th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="item in filteredQcList"
              :key="item.id"
              class="border-t hover:bg-orange-50"
            >
              <td class="table-td font-semibold text-slate-800">
                {{ item.qcNo }}
              </td>

              <td class="table-td">
                {{ item.poNo }}
              </td>

              <td class="table-td">
                {{ item.sku }}
              </td>

              <td class="table-td">
                {{ item.productName }}
              </td>

              <td class="table-td">
                {{ item.receivedQty }}
              </td>

              <td class="table-td">
                {{ item.sampleQty }}
              </td>

              <td class="table-td">
                {{ item.defectQty }}
              </td>

              <td class="table-td">
                <span :class="getResultClass(item.result)">
                  {{ resultLabel(item.result) }}
                </span>
              </td>

              <td class="table-td">
                <span :class="getStatusClass(item.status)">
                  {{ statusLabel(item.status) }}
                </span>
              </td>

              <td class="table-td text-right">
                <button
                  @click="openQc(item)"
                  class="text-orange-600 hover:text-orange-700 font-semibold"
                >
                  {{ $t('qualityCheck.table.inspect') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- QC Drawer -->
    <div
      v-if="selectedQc"
      class="fixed inset-0 bg-black/40 flex justify-end z-50"
    >
      <div class="w-full max-w-2xl bg-white h-full p-6 overflow-y-auto">
        <div class="flex items-center justify-between mb-6">
          <div>
            <h2 class="text-xl font-bold text-slate-800">
              {{ $t('qualityCheck.drawer.title') }}
            </h2>
            <p class="text-slate-500">
              {{ selectedQc.qcNo }} / {{ selectedQc.sku }}
            </p>
          </div>

          <button
            @click="selectedQc = null"
            class="text-slate-500 hover:text-red-600 text-xl"
          >
            ✕
          </button>
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
              <p class="detail-value">{{ selectedQc.receivedQty }}</p>
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
            <input
              v-model="qcForm.sampleQty"
              type="number"
              class="input"
              :placeholder="$t('qualityCheck.drawer.sampleQtyPlaceholder')"
            />
          </div>

          <div>
            <label class="label">{{ $t('qualityCheck.table.defectQty') }}</label>
            <input
              v-model="qcForm.defectQty"
              type="number"
              class="input"
              :placeholder="$t('qualityCheck.drawer.defectQtyPlaceholder')"
            />
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
            <textarea
              v-model="qcForm.remark"
              rows="4"
              class="input"
              :placeholder="$t('qualityCheck.drawer.remarkPlaceholder')"
            ></textarea>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-3 gap-3 pt-4">
            <button
              @click="submitQc('PASS')"
              class="bg-green-600 hover:bg-green-700 text-white py-3 rounded-xl font-semibold"
            >
              {{ $t('qualityCheck.drawer.pass') }}
            </button>

            <button
              @click="submitQc('HOLD')"
              class="bg-blue-600 hover:bg-blue-700 text-white py-3 rounded-xl font-semibold"
            >
              {{ $t('qualityCheck.drawer.hold') }}
            </button>

            <button
              @click="submitQc('FAIL')"
              class="bg-red-600 hover:bg-red-700 text-white py-3 rounded-xl font-semibold"
            >
              {{ $t('qualityCheck.drawer.reject') }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
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

export default {
  name: 'QualityCheckView',

  data() {
    return {
      selectedQc: null,

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
    statusLabel,

    openQc(item) {
      this.selectedQc = item

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

      if (!this.qcForm.sampleQty) {
        alert(this.$t('qualityCheck.messages.sampleQtyRequired'))
        return
      }

      if (result === 'FAIL' && !this.qcForm.defectReason) {
        alert(this.$t('qualityCheck.messages.defectReasonRequired'))
        return
      }

      alert(this.$t('qualityCheck.messages.qcSaved', { result: resultLabel(result) }))
      this.selectedQc = null
    },

    getResultClass(result) {
      if (result === 'PASS') return 'badge-green'
      if (result === 'FAIL') return 'badge-red'
      if (result === 'HOLD') return 'badge-blue'
      return 'badge-gray'
    },

    getStatusClass(status) {
      if (status === 'PENDING') return 'badge-gray'
      if (status === 'IN_PROGRESS') return 'badge-orange'
      if (status === 'PASSED') return 'badge-green'
      if (status === 'REJECTED') return 'badge-red'
      return 'badge-gray'
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
  font-weight: 600;
  font-size: 14px;
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
  font-size: 30px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 8px;
}

.table-th {
  padding: 14px 16px;
  font-weight: 700;
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

.badge-gray {
  background: #f1f5f9;
  color: #475569;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-blue {
  background: #dbeafe;
  color: #2563eb;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-green {
  background: #dcfce7;
  color: #16a34a;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-red {
  background: #fee2e2;
  color: #dc2626;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}
</style>

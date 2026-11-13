<template>
  <div class="space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold text-slate-800">{{ $t('newQualityCheck.title') }}</h1>
        <p class="text-slate-500">
          {{ $t('newQualityCheck.subtitle') }}
        </p>
      </div>

      <button
        @click="$router.push('/inbound/quality-check')"
        class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
      >
        {{ $t('newQualityCheck.backButton') }}
      </button>
    </div>

    <!-- QC Header -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">
        {{ $t('newQualityCheck.documentInfo') }}
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="label">{{ $t('newQualityCheck.fields.receivingNo') }}</label>
          <select v-model="form.receivingNo" class="input">
            <option value="">{{ $t('newQualityCheck.fields.selectReceivingDoc') }}</option>
            <option value="RCV-20260608-001">RCV-20260608-001</option>
            <option value="RCV-20260608-002">RCV-20260608-002</option>
            <option value="RCV-20260608-003">RCV-20260608-003</option>
          </select>
        </div>

        <div>
          <label class="label">{{ $t('newQualityCheck.fields.qcDate') }}</label>
          <input v-model="form.qcDate" type="date" class="input" />
        </div>

        <div>
          <label class="label">{{ $t('newQualityCheck.fields.inspector') }}</label>
          <input
            v-model="form.inspector"
            type="text"
            class="input"
            :placeholder="$t('newQualityCheck.fields.inspectorPlaceholder')"
          />
        </div>

        <div>
          <label class="label">{{ $t('newQualityCheck.fields.warehouse') }}</label>
          <select v-model="form.warehouse" class="input">
            <option value="">{{ $t('newQualityCheck.fields.selectWarehouse') }}</option>
            <option value="WH-01">WH-01</option>
            <option value="WH-02">WH-02</option>
          </select>
        </div>

        <div>
          <label class="label">{{ $t('newQualityCheck.fields.qcType') }}</label>
          <select v-model="form.qcType" class="input">
            <option value="NORMAL">{{ $t('newQualityCheck.qcTypeOptions.normal') }}</option>
            <option value="FULL">{{ $t('newQualityCheck.qcTypeOptions.full') }}</option>
            <option value="RANDOM">{{ $t('newQualityCheck.qcTypeOptions.random') }}</option>
            <option value="DAMAGE">{{ $t('newQualityCheck.qcTypeOptions.damage') }}</option>
          </select>
        </div>

        <div>
          <label class="label">{{ $t('newQualityCheck.fields.overallResult') }}</label>
          <select v-model="form.overallResult" class="input">
            <option value="">{{ $t('common.status.pending') }}</option>
            <option value="PASS">{{ $t('newQualityCheck.resultOptions.pass') }}</option>
            <option value="HOLD">{{ $t('newQualityCheck.resultOptions.hold') }}</option>
            <option value="FAIL">{{ $t('newQualityCheck.resultOptions.fail') }}</option>
          </select>
        </div>
      </div>

      <div class="mt-4">
        <label class="label">{{ $t('common.labels.remark') }}</label>
        <textarea
          v-model="form.remark"
          rows="3"
          class="input"
          :placeholder="$t('newQualityCheck.fields.remarkPlaceholder')"
        ></textarea>
      </div>
    </div>

    <!-- Scan Area -->
    <div class="bg-orange-50 border border-orange-200 rounded-2xl p-5">
      <label class="label">{{ $t('newQualityCheck.scan.label') }}</label>

      <div class="grid grid-cols-1 md:grid-cols-5 gap-4">
        <input
          v-model="scanText"
          type="text"
          class="input bg-white"
          :placeholder="$t('newQualityCheck.scan.placeholder')"
          @keyup.enter="scanItem"
        />

        <input
          v-model.number="scanSampleQty"
          type="number"
          class="input bg-white input-number"
          :placeholder="$t('newQualityCheck.scan.sampleQtyPlaceholder')"
        />

        <input
          v-model.number="scanDefectQty"
          type="number"
          class="input bg-white input-number"
          :placeholder="$t('newQualityCheck.scan.defectQtyPlaceholder')"
        />

        <select v-model="scanResult" class="input bg-white">
          <option value="PASS">{{ $t('newQualityCheck.resultOptions.pass') }}</option>
          <option value="HOLD">{{ $t('newQualityCheck.resultOptions.hold') }}</option>
          <option value="FAIL">{{ $t('newQualityCheck.resultOptions.fail') }}</option>
        </select>

        <button
          @click="scanItem"
          class="bg-orange-600 hover:bg-orange-700 text-white rounded-xl font-semibold"
        >
          {{ $t('newQualityCheck.scan.addQc') }}
        </button>
      </div>
    </div>

    <!-- QC Items -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <div class="flex items-center justify-between mb-5">
        <div>
          <h2 class="text-lg font-bold text-slate-800">{{ $t('newQualityCheck.itemsSection.title') }}</h2>
          <p class="text-sm text-slate-500">
            {{ $t('newQualityCheck.itemsSection.subtitle') }}
          </p>
        </div>

        <button
          @click="addItem"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newQualityCheck.itemsSection.addItem') }}
        </button>
      </div>

      <div class="border border-slate-200 rounded-2xl overflow-hidden">
        <div class="w-full overflow-x-auto">
          <table class="min-w-[1350px] w-full text-sm">
            <thead class="bg-slate-50 text-slate-500">
              <tr>
                <th class="table-th">{{ $t('newQualityCheck.table.sku') }}</th>
                <th class="table-th">{{ $t('newQualityCheck.table.productName') }}</th>
                <th class="table-th">{{ $t('newQualityCheck.table.lotNo') }}</th>
                <th class="table-th">{{ $t('newQualityCheck.table.receivedQty') }}</th>
                <th class="table-th">{{ $t('newQualityCheck.table.sampleQty') }}</th>
                <th class="table-th">{{ $t('newQualityCheck.table.defectQty') }}</th>
                <th class="table-th">{{ $t('newQualityCheck.table.defectReason') }}</th>
                <th class="table-th">{{ $t('newQualityCheck.table.result') }}</th>
                <th class="table-th">{{ $t('common.labels.remark') }}</th>
                <th class="table-th text-right">{{ $t('newQualityCheck.table.action') }}</th>
              </tr>
            </thead>

            <tbody>
              <tr
                v-for="(item, index) in items"
                :key="item.id"
                class="border-t hover:bg-orange-50"
              >
                <td class="table-td">
                  <input v-model="item.sku" class="table-input" />
                </td>

                <td class="table-td">
                  <input v-model="item.productName" class="table-input min-w-[220px]" />
                </td>

                <td class="table-td">
                  <input v-model="item.lotNo" class="table-input" />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.receivedQty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.sampleQty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td">
                  <input
                    v-model.number="item.defectQty"
                    type="number"
                    class="table-input input-number"
                  />
                </td>

                <td class="table-td">
                  <select v-model="item.defectReason" class="table-input min-w-[180px]">
                    <option value="">{{ $t('newQualityCheck.defectReasonOptions.none') }}</option>
                    <option value="DAMAGED">{{ $t('newQualityCheck.defectReasonOptions.damaged') }}</option>
                    <option value="WRONG_ITEM">{{ $t('newQualityCheck.defectReasonOptions.wrongItem') }}</option>
                    <option value="EXPIRED">{{ $t('newQualityCheck.defectReasonOptions.expired') }}</option>
                    <option value="SHORTAGE">{{ $t('newQualityCheck.defectReasonOptions.shortage') }}</option>
                    <option value="PACKAGING_ISSUE">{{ $t('newQualityCheck.defectReasonOptions.packagingIssue') }}</option>
                  </select>
                </td>

                <td class="table-td">
                  <select v-model="item.result" class="table-input">
                    <option value="PASS">{{ $t('newQualityCheck.resultOptions.pass') }}</option>
                    <option value="HOLD">{{ $t('newQualityCheck.resultOptions.hold') }}</option>
                    <option value="FAIL">{{ $t('newQualityCheck.resultOptions.fail') }}</option>
                  </select>
                </td>

                <td class="table-td">
                  <input v-model="item.remark" class="table-input min-w-[220px]" />
                </td>

                <td class="table-td text-right">
                  <button
                    @click="removeItem(index)"
                    class="text-red-600 hover:text-red-700 font-semibold"
                  >
                    {{ $t('common.buttons.remove') }}
                  </button>
                </td>
              </tr>

              <tr v-if="items.length === 0">
                <td colspan="10" class="text-center py-10 text-slate-400">
                  {{ $t('newQualityCheck.table.noItems') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Summary -->
    <div class="bg-white rounded-2xl border border-slate-200 p-6">
      <h2 class="text-lg font-bold text-slate-800 mb-5">
        {{ $t('newQualityCheck.summary.title') }}
      </h2>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-5">
        <div class="summary-card">
          <p class="summary-label">{{ $t('newQualityCheck.summary.totalLines') }}</p>
          <h2 class="summary-value text-number">{{ items.length }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('newQualityCheck.summary.sampleQty') }}</p>
          <h2 class="summary-value text-number">{{ totalSampleQty }}</h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('newQualityCheck.summary.defectQty') }}</p>
          <h2 class="summary-value text-red-600 text-number">
            {{ totalDefectQty }}
          </h2>
        </div>

        <div class="summary-card">
          <p class="summary-label">{{ $t('newQualityCheck.summary.passRate') }}</p>
          <h2 class="summary-value text-green-600 text-number">
            {{ passRate }}%
          </h2>
        </div>
      </div>
    </div>

    <!-- Action Bar -->
    <div class="bg-white border border-slate-200 rounded-2xl p-6">
      <div class="flex flex-col md:flex-row justify-end gap-3">
        <button
          @click="saveDraft"
          class="bg-white hover:bg-slate-100 text-slate-700 border border-slate-300 px-6 py-3 rounded-xl font-semibold"
        >
          {{ $t('common.buttons.saveDraft') }}
        </button>

        <button
          @click="submitQc"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold"
        >
          {{ $t('newQualityCheck.actions.confirmQc') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'NewQualityCheckView',

  data() {
    return {
      scanText: '',
      scanSampleQty: 1,
      scanDefectQty: 0,
      scanResult: 'PASS',

      form: {
        receivingNo: '',
        qcDate: '2026-06-08',
        inspector: 'Admin User',
        warehouse: '',
        qcType: 'NORMAL',
        overallResult: '',
        remark: ''
      },

      items: [
        {
          id: 1,
          sku: 'SKU-0001',
          productName: 'Barcode Label Roll',
          lotNo: 'LOT-0001',
          receivedQty: 100,
          sampleQty: 10,
          defectQty: 0,
          defectReason: '',
          result: 'PASS',
          remark: ''
        }
      ]
    }
  },

  computed: {
    totalSampleQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.sampleQty || 0)
      }, 0)
    },

    totalDefectQty() {
      return this.items.reduce((sum, item) => {
        return sum + Number(item.defectQty || 0)
      }, 0)
    },

    passRate() {
      if (this.totalSampleQty === 0) return 0

      const passQty = this.totalSampleQty - this.totalDefectQty
      return ((passQty / this.totalSampleQty) * 100).toFixed(2)
    }
  },

  methods: {
    addItem() {
      this.items.push({
        id: Date.now(),
        sku: '',
        productName: '',
        lotNo: '',
        receivedQty: 0,
        sampleQty: 0,
        defectQty: 0,
        defectReason: '',
        result: 'PASS',
        remark: ''
      })
    },

    scanItem() {
      if (!this.scanText) {
        alert(this.$t('newQualityCheck.messages.barcodeRequired'))
        return
      }

      const existingItem = this.items.find((item) => {
        return item.sku === this.scanText
      })

      if (existingItem) {
        existingItem.sampleQty += Number(this.scanSampleQty || 1)
        existingItem.defectQty += Number(this.scanDefectQty || 0)
        existingItem.result = this.scanResult
      } else {
        this.items.push({
          id: Date.now(),
          sku: this.scanText,
          productName: 'Scanned Product',
          lotNo: '',
          receivedQty: 0,
          sampleQty: Number(this.scanSampleQty || 1),
          defectQty: Number(this.scanDefectQty || 0),
          defectReason: '',
          result: this.scanResult,
          remark: ''
        })
      }

      this.scanText = ''
      this.scanSampleQty = 1
      this.scanDefectQty = 0
      this.scanResult = 'PASS'
    },

    removeItem(index) {
      this.items.splice(index, 1)
    },

    saveDraft() {
      alert(this.$t('newQualityCheck.messages.draftSaved'))
    },

    submitQc() {
      if (!this.form.receivingNo) {
        alert(this.$t('newQualityCheck.messages.receivingNoRequired'))
        return
      }

      if (!this.form.warehouse) {
        alert(this.$t('newQualityCheck.messages.warehouseRequired'))
        return
      }

      if (this.items.length === 0) {
        alert(this.$t('newQualityCheck.messages.atLeastOneItem'))
        return
      }

      const invalidFailItem = this.items.find((item) => {
        return item.result === 'FAIL' && !item.defectReason
      })

      if (invalidFailItem) {
        alert(this.$t('newQualityCheck.messages.defectReasonRequiredForFail'))
        return
      }

      alert(this.$t('newQualityCheck.messages.qcConfirmed'))
      this.$router.push('/inbound/quality-check')
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

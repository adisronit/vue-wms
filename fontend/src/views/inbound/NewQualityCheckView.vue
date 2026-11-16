<template>
  <div class="space-y-6">
    <PageHeader :title="$t('newQualityCheck.title')" :subtitle="$t('newQualityCheck.subtitle')">
      <template #actions>
        <button
          @click="$router.push('/inbound/quality-check')"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newQualityCheck.backButton') }}
        </button>
      </template>
    </PageHeader>

    <!-- QC Header -->
    <FormSection :title="$t('newQualityCheck.documentInfo')">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="label">{{ $t('newQualityCheck.fields.receivingNo') }}</label>
          <select v-model.number="form.receiptId" class="input" @change="onReceiptChange">
            <option value="">{{ $t('newQualityCheck.fields.selectReceivingDoc') }}</option>
            <option v-for="r in receipts" :key="r.id" :value="r.id">{{ r.receipt_no }}</option>
          </select>
        </div>

        <div>
          <label class="label">{{ $t('newQualityCheck.fields.warehouse') }}</label>
          <input class="input bg-slate-50" :value="receiptDetail ? receiptDetail.warehouse_code : ''" readonly />
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

      <div v-if="formError" class="mt-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ formError }}
      </div>
    </FormSection>

    <!-- Eligible lines preview -->
    <FormSection :title="$t('newQualityCheck.itemsSection.title')">
      <p class="text-sm text-slate-500 mb-4">{{ $t('newQualityCheck.itemsSection.subtitle') }}</p>

      <DataTable
        :is-empty="eligibleSplits.length === 0"
        :empty-text="eligibleLoading ? $t('common.buttons.loading') : $t('newQualityCheck.table.noItems')"
        :colspan="6"
        min-width="900px"
      >
        <template #head>
          <tr>
            <th class="table-th">{{ $t('newQualityCheck.table.sku') }}</th>
            <th class="table-th">{{ $t('newQualityCheck.table.productName') }}</th>
            <th class="table-th">{{ $t('newQualityCheck.table.lotNo') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.expiryDate') }}</th>
            <th class="table-th">{{ $t('newReceiving.table.location') }}</th>
            <th class="table-th text-right">{{ $t('newQualityCheck.table.receivedQty') }}</th>
          </tr>
        </template>

        <tr v-for="s in eligibleSplits" :key="s.receipt_item_split_id" class="border-t hover:bg-orange-50">
          <td class="table-td">{{ s.sku }}</td>
          <td class="table-td">{{ s.product_name }}</td>
          <td class="table-td">{{ s.lot_no }}</td>
          <td class="table-td">{{ $d(s.expiry_date) }}</td>
          <td class="table-td">{{ s.location_code }}</td>
          <td class="table-td text-right tabular-nums">{{ s.held_qty }}</td>
        </tr>
      </DataTable>
    </FormSection>

    <!-- Action Bar -->
    <FormSection>
      <div class="flex flex-col md:flex-row justify-end gap-3">
        <button
          @click="createInspectionClick"
          :disabled="submitting || eligibleSplits.length === 0"
          class="bg-orange-600 hover:bg-orange-700 text-white px-6 py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ submitting ? $t('newQualityCheck.actions.submitting') : $t('newQualityCheck.actions.confirmQc') }}
        </button>
      </div>
    </FormSection>
  </div>
</template>

<script>
import { listReceipts, getReceipt } from '../../services/receiving'
import { listEligibleSplits, createInspection } from '../../services/qualitycheck'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'

export default {
  name: 'NewQualityCheckView',

  components: { PageHeader, FormSection, DataTable },

  data() {
    return {
      receipts: [],
      receiptDetail: null,
      eligibleSplits: [],
      eligibleLoading: false,
      formError: '',
      submitting: false,

      form: {
        receiptId: '',
        qcType: 'NORMAL',
        remark: ''
      }
    }
  },

  mounted() {
    this.fetchReceipts()
  },

  methods: {
    async fetchReceipts() {
      try {
        const res = await listReceipts({ limit: 50 })
        this.receipts = res.data
      } catch (err) {
        this.formError = err.message
      }
    },

    async onReceiptChange() {
      this.eligibleSplits = []
      this.receiptDetail = null
      this.formError = ''
      if (!this.form.receiptId) return

      this.eligibleLoading = true
      try {
        const [eligibleRes, receiptRes] = await Promise.all([
          listEligibleSplits(this.form.receiptId),
          getReceipt(this.form.receiptId)
        ])
        this.eligibleSplits = eligibleRes.data
        this.receiptDetail = receiptRes.data
        if (this.eligibleSplits.length === 0) {
          this.formError = this.$t('newQualityCheck.messages.noEligibleLines')
        }
      } catch (err) {
        this.formError = err.message
      } finally {
        this.eligibleLoading = false
      }
    },

    async createInspectionClick() {
      if (!this.form.receiptId) {
        this.formError = this.$t('newQualityCheck.messages.receivingNoRequired')
        return
      }
      if (this.eligibleSplits.length === 0) {
        this.formError = this.$t('newQualityCheck.messages.noEligibleLines')
        return
      }

      this.submitting = true
      this.formError = ''
      try {
        await createInspection({
          receipt_id: this.form.receiptId,
          warehouse_id: this.receiptDetail.warehouse_id,
          qc_type: this.form.qcType,
          remark: this.form.remark || null
        })
        showToast(this.$t('newQualityCheck.messages.qcConfirmed'), 'success')
        this.$router.push('/inbound/quality-check')
      } catch (err) {
        this.formError = err.message
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
</style>

<template>
  <div class="space-y-6 pb-10">
    <PageHeader :title="$t('sortation.title')" :subtitle="$t('sortation.subtitle')">
      <template #actions>
        <button
          @click="$router.push(`/outbound/picking-batches/${id}`)"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('sortation.backButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>
    <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('sortation.loading') }}</div>

    <template v-else-if="worklist">
      <div class="bg-white rounded-2xl border border-slate-200 p-6 flex flex-wrap items-center gap-6">
        <div>
          <p class="text-xs text-slate-400">{{ $t('common.labels.status') }}</p>
          <StatusBadge :tone="statusTone(worklist.sortation_status)">{{ statusLabel(worklist.sortation_status) }}</StatusBadge>
        </div>
        <div v-if="worklist.pick_no">
          <p class="text-xs text-slate-400">{{ $t('pickingBatches.table.pickNo') }}</p>
          <p class="font-semibold text-slate-800">{{ worklist.pick_no }}</p>
        </div>
        <div v-if="worklist.sortation_station">
          <p class="text-xs text-slate-400">{{ $t('sortation.stationLabel') }}</p>
          <p class="font-semibold text-slate-800">{{ worklist.sortation_station }}</p>
        </div>
      </div>

      <div v-if="worklist.sortation_status === 'COMPLETED'" class="bg-green-50 border border-green-200 text-green-700 rounded-xl p-3 text-sm">
        {{ $t('sortation.completedBanner') }}
      </div>

      <FormSection v-if="canSortation" :title="$t('sortation.assignTitle')">
        <div class="flex flex-wrap gap-3 items-end">
          <div class="flex-1 min-w-[200px]">
            <label class="label">{{ $t('sortation.stationLabel') }}</label>
            <input v-model="assignForm.station" type="text" class="input" :placeholder="$t('sortation.stationPlaceholder')" />
          </div>
          <button
            @click="submitAssign"
            :disabled="assigning || !assignForm.station"
            class="bg-slate-800 hover:bg-slate-900 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ assigning ? $t('common.buttons.submitting') : $t('sortation.assignButton') }}
          </button>
        </div>
      </FormSection>

      <FormSection v-if="canSortation && worklist.sortation_status !== 'COMPLETED'" :title="$t('sortation.scanTitle')">
        <div class="grid grid-cols-1 md:grid-cols-4 gap-3">
          <input
            v-model="scanForm.orderBarcode"
            type="text"
            class="input"
            :placeholder="$t('sortation.orderBarcodePlaceholder')"
            @keyup.enter="focusNext('scanSku')"
          />
          <input
            ref="scanSku"
            v-model="scanForm.sku"
            type="text"
            class="input"
            :placeholder="$t('sortation.skuPlaceholder')"
            @keyup.enter="focusNext('scanLot')"
          />
          <input
            ref="scanLot"
            v-model="scanForm.lotNo"
            type="text"
            class="input"
            :placeholder="$t('sortation.lotPlaceholder')"
            @keyup.enter="focusNext('scanQty')"
          />
          <input
            ref="scanQty"
            v-model.number="scanForm.quantity"
            type="number"
            class="input"
            :placeholder="$t('sortation.qtyPlaceholder')"
            @keyup.enter="submitScan"
          />
        </div>
        <div class="mt-3">
          <button
            @click="submitScan"
            :disabled="scanning || !scanForm.orderBarcode || !scanForm.sku || !scanForm.quantity"
            class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold disabled:opacity-60"
          >
            {{ scanning ? $t('sortation.scanning') : $t('sortation.scanButton') }}
          </button>
        </div>
        <p v-if="scanError" class="text-sm text-red-600 mt-2">{{ scanError }}</p>
      </FormSection>

      <FormSection :title="$t('sortation.worklistTitle')">
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-slate-200">
                <th class="table-th">{{ $t('sortation.table.orderNo') }}</th>
                <th class="table-th">{{ $t('sortation.table.shipTo') }}</th>
                <th class="table-th">{{ $t('sortation.table.sku') }}</th>
                <th class="table-th">{{ $t('sortation.table.lot') }}</th>
                <th class="table-th text-right">{{ $t('sortation.table.allocated') }}</th>
                <th class="table-th text-right">{{ $t('sortation.table.sorted') }}</th>
                <th class="table-th text-right">{{ $t('sortation.table.remaining') }}</th>
                <th class="table-th">{{ $t('sortation.table.status') }}</th>
                <th v-if="canSortation" class="table-th text-right">{{ $t('packing.table.action') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="line in worklist.lines" :key="line.allocation_id" class="border-t border-slate-100">
                <td class="table-td font-semibold text-slate-800">{{ line.order_no }}</td>
                <td class="table-td">{{ line.ship_to_name || '—' }}</td>
                <td class="table-td">{{ line.sku }} — {{ line.product_name }}</td>
                <td class="table-td">{{ line.lot_no || '—' }}</td>
                <td class="table-td text-right">{{ line.allocated_qty }}</td>
                <td class="table-td text-right text-green-600">{{ line.sorted_qty }}</td>
                <td class="table-td text-right">{{ line.remaining }}</td>
                <td class="table-td">
                  <span v-if="line.exception_reason" class="text-red-600 font-semibold">{{ $t('sortation.exceptionButton') }}</span>
                  <span v-else-if="line.resolved" class="text-green-600 font-semibold">{{ $t('sortation.lineResolved') }}</span>
                  <span v-else class="text-slate-500">{{ $t('sortation.linePending') }}</span>
                </td>
                <td v-if="canSortation" class="table-td text-right">
                  <button
                    v-if="!line.resolved"
                    @click="toggleExceptionForm(line)"
                    class="text-red-600 hover:underline font-semibold"
                  >
                    {{ $t('sortation.exceptionButton') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div
          v-if="activeExceptionLine"
          class="fixed inset-0 bg-black/30 flex items-center justify-center z-50 p-4"
        >
          <div class="bg-white rounded-2xl shadow-xl w-full max-w-lg p-6 space-y-4">
            <div class="flex items-center justify-between">
              <h2 class="text-lg font-bold text-slate-800">{{ $t('sortation.exceptionTitle') }}</h2>
              <button @click="activeExceptionLine = null" class="text-slate-400 hover:text-slate-600">✕</button>
            </div>
            <p class="text-sm text-slate-500">
              {{ activeExceptionLine.order_no }} — {{ activeExceptionLine.sku }} ({{ activeExceptionLine.lot_no || '—' }})
            </p>
            <input
              v-model.number="exceptionForm.shortageQty"
              type="number"
              class="input"
              :placeholder="$t('sortation.exceptionQtyPlaceholder')"
            />
            <input
              v-model="exceptionForm.reason"
              type="text"
              class="input"
              :placeholder="$t('sortation.exceptionReasonPlaceholder')"
            />
            <div class="flex justify-end gap-3">
              <button @click="activeExceptionLine = null" class="px-4 py-2 rounded-xl bg-slate-100 text-slate-700 font-semibold">
                {{ $t('common.buttons.cancel') }}
              </button>
              <button
                @click="submitException"
                :disabled="flaggingException || !exceptionForm.reason || !exceptionForm.shortageQty"
                class="px-4 py-2 rounded-xl bg-red-600 hover:bg-red-700 text-white font-semibold disabled:opacity-60"
              >
                {{ flaggingException ? $t('common.buttons.submitting') : $t('sortation.submitExceptionButton') }}
              </button>
            </div>
          </div>
        </div>
      </FormSection>
    </template>
  </div>
</template>

<script>
import { getSortation, scanSortation, flagSortationException, assignSortation } from '../../services/pickingBatches'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

export default {
  name: 'SortationView',
  components: { PageHeader, FormSection, StatusBadge },
  props: { id: { type: [String, Number], required: true } },
  data() {
    return {
      loading: false,
      loadError: '',
      worklist: null,

      scanForm: { orderBarcode: '', sku: '', lotNo: '', quantity: null },
      scanning: false,
      scanError: '',

      assignForm: { station: '' },
      assigning: false,

      activeExceptionLine: null,
      exceptionForm: { shortageQty: null, reason: '' },
      flaggingException: false
    }
  },
  computed: {
    canSortation() { return hasPermission('picking_batch.sortation') }
  },
  async mounted() {
    await this.fetchWorklist()
  },
  methods: {
    async fetchWorklist() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getSortation(this.id)
        this.worklist = res.data
        if (this.worklist.sortation_station) {
          this.assignForm.station = this.worklist.sortation_station
        }
      } catch (err) {
        this.loadError = err.message || this.$t('sortation.notReady')
      } finally {
        this.loading = false
      }
    },

    focusNext(ref) {
      const el = this.$refs[ref]
      if (el) el.focus()
    },

    async submitScan() {
      this.scanError = ''
      this.scanning = true
      try {
        const res = await scanSortation(this.id, {
          orderBarcode: this.scanForm.orderBarcode,
          sku: this.scanForm.sku,
          lotNo: this.scanForm.lotNo,
          quantity: this.scanForm.quantity
        })
        this.worklist = res.data
        this.scanForm = { orderBarcode: '', sku: '', lotNo: '', quantity: null }
        showToast(this.$t('sortation.scanSuccess'), 'success')
      } catch (err) {
        this.scanError = err.message || this.$t('sortation.scanFailed')
      } finally {
        this.scanning = false
      }
    },

    toggleExceptionForm(line) {
      this.activeExceptionLine = line
      this.exceptionForm = { shortageQty: null, reason: '' }
    },

    async submitException() {
      if (!this.activeExceptionLine) return
      this.flaggingException = true
      try {
        const res = await flagSortationException(this.id, {
          allocationId: this.activeExceptionLine.allocation_id,
          shortageQty: this.exceptionForm.shortageQty,
          reason: this.exceptionForm.reason
        })
        this.worklist = res.data
        this.activeExceptionLine = null
        showToast(this.$t('sortation.exceptionFlagged'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('sortation.exceptionFailed'), 'error')
      } finally {
        this.flaggingException = false
      }
    },

    async submitAssign() {
      this.assigning = true
      try {
        const res = await assignSortation(this.id, { station: this.assignForm.station })
        this.worklist = res.data
        showToast(this.$t('sortation.assignSuccess'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('sortation.assignFailed'), 'error')
      } finally {
        this.assigning = false
      }
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'IN_PROGRESS') return 'orange'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'NOT_STARTED': return this.$t('sortation.statusLabels.notStarted')
        case 'IN_PROGRESS': return this.$t('sortation.statusLabels.inProgress')
        case 'COMPLETED': return this.$t('sortation.statusLabels.completed')
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

.label {
  display: block;
  font-size: 0.8rem;
  font-weight: 600;
  color: #64748b;
  margin-bottom: 4px;
}

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
</style>

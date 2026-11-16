<template>
  <div class="space-y-6">
    <PageHeader :title="trip ? trip.trip_no : $t('loading.title')" :subtitle="$t('loading.detail.subtitle')">
      <template #actions>
        <button @click="$router.push('/outbound/loading')" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold">
          {{ $t('newRelocation.backButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ loadError }}</div>
    <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('loading.loadingTrips') }}</div>

    <template v-else-if="trip">
      <FormSection :title="$t('loading.detail.infoTitle')">
        <div class="flex flex-wrap gap-4 items-center mb-4">
          <StatusBadge :tone="statusTone(trip.status)">{{ statusLabel(trip.status) }}</StatusBadge>
          <span class="text-sm text-slate-500">{{ $t('loading.table.progress') }}: {{ trip.loaded_count }}/{{ trip.planned_count }} ({{ trip.short_count }} {{ $t('loading.detail.shortSuffix') }})</span>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm">
          <div><span class="field-label">{{ $t('loading.table.vehicleNo') }}:</span> {{ trip.vehicle_no || '—' }}</div>
          <div><span class="field-label">{{ $t('loading.table.driver') }}:</span> {{ trip.driver_name || '—' }}</div>
          <div><span class="field-label">{{ $t('loading.detail.carrierLabel') }}:</span> {{ trip.carrier || '—' }}</div>
          <div><span class="field-label">{{ $t('loading.detail.dockLabel') }}:</span> {{ trip.loading_dock || '—' }}</div>
          <div><span class="field-label">{{ $t('loading.detail.routeLabel') }}:</span> {{ trip.route || '—' }}</div>
          <div><span class="field-label">{{ $t('loading.table.scheduled') }}:</span> {{ formatDateTime(trip.scheduled_loading_time) }}</div>
          <div v-if="trip.actual_loading_start_at"><span class="field-label">{{ $t('loading.detail.startedAtLabel') }}:</span> {{ formatDateTime(trip.actual_loading_start_at) }}</div>
          <div v-if="trip.actual_loading_end_at"><span class="field-label">{{ $t('loading.detail.endedAtLabel') }}:</span> {{ formatDateTime(trip.actual_loading_end_at) }}</div>
          <div v-if="trip.vehicle_seal"><span class="field-label">{{ $t('loading.detail.sealLabel') }}:</span> {{ trip.vehicle_seal }}</div>
        </div>
        <p v-if="trip.remark" class="mt-2 text-sm"><span class="field-label">{{ $t('orders.detail.remarkLabel') }}:</span> {{ trip.remark }}</p>
        <p v-if="trip.cancel_remark" class="mt-2 text-sm text-slate-600"><span class="field-label">{{ $t('orders.detail.cancelRemarkLabel') }}:</span> {{ trip.cancel_remark }}</p>
      </FormSection>

      <FormSection :title="$t('loading.detail.shipmentsTitle')">
        <div v-if="canUpdate && isTripWorkable" class="bg-slate-50 border border-slate-200 rounded-2xl p-4 mb-4 flex gap-3 items-end">
          <div class="flex-1">
            <label class="label">{{ $t('loading.detail.assignShipmentLabel') }}</label>
            <select v-model="assignShipmentId" class="input" @focus="fetchAssignableShipments">
              <option value="">{{ $t('loading.detail.selectShipment') }}</option>
              <option v-for="s in assignableShipments" :key="s.shipment_id" :value="s.shipment_id">
                {{ s.shipment_no }} — {{ s.order_no }} ({{ s.customer || '—' }}, {{ s.carton_count }} {{ $t('loading.detail.cartonsSuffix') }})
              </option>
            </select>
          </div>
          <button @click="doAssign" :disabled="!assignShipmentId || assigning" class="bg-slate-800 hover:bg-slate-900 text-white px-4 py-3 rounded-xl font-semibold disabled:opacity-60">
            {{ assigning ? $t('common.buttons.submitting') : $t('loading.detail.assignButton') }}
          </button>
        </div>

        <p v-if="trip.shipments.length === 0" class="text-sm text-slate-400">{{ $t('loading.detail.noShipments') }}</p>
        <div v-for="s in trip.shipments" :key="s.shipment_id" class="border border-slate-200 rounded-2xl p-4 mb-3 flex flex-wrap justify-between items-center gap-3">
          <div>
            <p class="font-bold text-slate-800">{{ s.shipment_no }}</p>
            <p class="text-sm text-slate-500">{{ s.order_no }} — {{ s.customer || '—' }}</p>
            <p class="text-xs text-slate-400">{{ $t('packing.table.jobNo') }}: {{ s.job_no }}</p>
          </div>
          <div class="text-sm text-slate-600">
            {{ $t('loading.detail.cartonsSuffix') }}: {{ s.loaded_count }}/{{ s.planned_count }}
            <span v-if="s.short_count > 0" class="text-red-600">({{ s.short_count }} {{ $t('loading.detail.shortSuffix') }})</span>
          </div>
        </div>
      </FormSection>

      <FormSection :title="$t('loading.detail.cartonsTitle')">
        <div v-if="canUpdate && isTripWorkable" class="bg-slate-50 border border-slate-200 rounded-2xl p-4 mb-4 space-y-3">
          <h3 class="font-bold text-slate-800 text-sm">{{ $t('loading.detail.scanTitle') }}</h3>
          <!-- Scan-to-load: a keyboard-wedge scanner types the scanned
               LPN as keystrokes then Enter — matches PackingDetailView.vue's
               own scan-to-pack convention. -->
          <div class="flex gap-3">
            <input
              v-model="scanLpn"
              type="text"
              class="input"
              :placeholder="$t('loading.detail.scanPlaceholder')"
              :disabled="scanning"
              @keyup.enter="doScan"
              ref="scanInput"
            />
            <button @click="doScan" :disabled="!scanLpn || scanning" class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-3 rounded-xl font-semibold disabled:opacity-60">
              {{ scanning ? $t('loading.detail.scanning') : $t('loading.detail.scanButton') }}
            </button>
            <button @click="toggleShortForm" class="bg-red-50 hover:bg-red-100 text-red-600 px-4 py-3 rounded-xl font-semibold">
              {{ $t('loading.detail.shortButton') }}
            </button>
          </div>
          <p v-if="scanError" class="text-sm text-red-600">{{ scanError }}</p>

          <div v-if="showShortForm" class="grid grid-cols-1 md:grid-cols-4 gap-3 bg-red-50 rounded-xl p-3">
            <input v-model="shortForm.lpn" type="text" class="input" :placeholder="$t('loading.detail.scanPlaceholder')" />
            <select v-model="shortForm.reason" class="input">
              <option value="">{{ $t('loading.detail.shortReasonPlaceholder') }}</option>
              <option value="DAMAGED">{{ $t('loading.shortReason.damaged') }}</option>
              <option value="MISSING">{{ $t('loading.shortReason.missing') }}</option>
              <option value="OTHER">{{ $t('loading.shortReason.other') }}</option>
            </select>
            <input v-model="shortForm.remark" type="text" class="input" :placeholder="$t('loading.detail.shortRemarkPlaceholder')" />
            <button
              @click="doShort"
              :disabled="!shortForm.lpn || !shortForm.reason || shortSubmitting"
              class="bg-red-600 hover:bg-red-700 text-white px-4 py-2 rounded-xl font-semibold text-sm disabled:opacity-60"
            >
              {{ shortSubmitting ? $t('common.buttons.submitting') : $t('loading.detail.submitShortButton') }}
            </button>
          </div>
        </div>

        <p v-if="trip.cartons.length === 0" class="text-sm text-slate-400">{{ $t('loading.detail.noCartons') }}</p>
        <table v-else class="w-full text-sm">
          <thead>
            <tr class="text-left text-slate-500">
              <th class="py-2">{{ $t('loading.detail.lpnLabel') }}</th>
              <th class="py-2">{{ $t('loading.table.shipments') }}</th>
              <th class="py-2">{{ $t('common.labels.status') }}</th>
              <th class="py-2">{{ $t('loading.detail.detailLabel') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in trip.cartons" :key="c.id" class="border-t">
              <td class="py-2 font-semibold text-slate-800">{{ c.lpn }}</td>
              <td class="py-2 text-slate-500">{{ c.shipment_no }}</td>
              <td class="py-2"><StatusBadge :tone="cartonStatusTone(c.status)">{{ cartonStatusLabel(c.status) }}</StatusBadge></td>
              <td class="py-2 text-xs text-slate-400">
                <template v-if="c.status === 'LOADED'">{{ formatDateTime(c.loaded_at) }} — {{ c.loaded_by_name || '—' }}</template>
                <template v-else-if="c.status === 'SHORT'">{{ shortReasonLabel(c.short_reason) }}<span v-if="c.short_remark"> — {{ c.short_remark }}</span></template>
                <template v-else>—</template>
              </td>
            </tr>
          </tbody>
        </table>
      </FormSection>

      <FormSection v-if="trip.status !== 'CANCELLED'" :title="$t('loading.detail.finishTitle')">
        <div class="flex flex-wrap gap-3">
          <button
            v-if="canConfirm && trip.status === 'LOADING'"
            @click="doConfirm"
            :disabled="confirming || !allCartonsResolved"
            class="px-5 py-3 rounded-xl bg-green-600 hover:bg-green-700 text-white font-semibold disabled:opacity-60"
          >
            {{ confirming ? $t('common.buttons.submitting') : $t('loading.detail.confirmTripButton') }}
          </button>
          <button
            v-if="canApprove && isTripWorkable"
            @click="showCancelConfirm = true"
            class="px-5 py-3 rounded-xl bg-slate-100 text-slate-700 font-semibold"
          >
            {{ $t('loading.detail.cancelTripButton') }}
          </button>
        </div>
        <p v-if="trip.status === 'LOADING' && !allCartonsResolved" class="text-xs text-slate-400 mt-3">{{ $t('loading.detail.confirmNote') }}</p>
        <p v-if="trip.status === 'PLANNED'" class="text-xs text-slate-400 mt-3">{{ $t('loading.detail.plannedNote') }}</p>
      </FormSection>
    </template>

    <ConfirmDialog
      :open="showCancelConfirm"
      :title="$t('loading.detail.cancelTripButton')"
      :message="$t('loading.detail.cancelConfirmMessage')"
      :confirm-label="$t('loading.detail.cancelTripButton')"
      :cancel-label="$t('common.buttons.cancel')"
      :loading-label="$t('common.buttons.submitting')"
      :loading="cancelling"
      :error-message="cancelError"
      danger
      @cancel="showCancelConfirm = false"
      @confirm="doCancel"
    />
  </div>
</template>

<script>
import {
  getLoadingTrip, listAssignableShipments, assignShipmentToTrip, scanCarton, shortCarton,
  confirmLoadingTrip, cancelLoadingTrip
} from '../../services/loading'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

export default {
  name: 'LoadingTripDetailView',
  components: { PageHeader, FormSection, StatusBadge, ConfirmDialog },
  props: { id: { type: [String, Number], required: true } },

  data() {
    return {
      loading: false,
      loadError: '',
      trip: null,

      assignableShipments: [],
      assignShipmentId: '',
      assigning: false,

      scanLpn: '',
      scanning: false,
      scanError: '',

      showShortForm: false,
      shortForm: { lpn: '', reason: '', remark: '' },
      shortSubmitting: false,

      confirming: false,
      showCancelConfirm: false,
      cancelling: false,
      cancelError: ''
    }
  },

  computed: {
    canUpdate() { return hasPermission('loading.update') },
    canConfirm() { return hasPermission('loading.confirm') },
    canApprove() { return hasPermission('loading.approve') },
    isTripWorkable() {
      return !!this.trip && (this.trip.status === 'PLANNED' || this.trip.status === 'LOADING')
    },
    allCartonsResolved() {
      if (!this.trip || this.trip.cartons.length === 0) return false
      return this.trip.cartons.every((c) => c.status === 'LOADED' || c.status === 'SHORT')
    }
  },

  async mounted() {
    await this.load()
  },

  methods: {
    async load() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getLoadingTrip(this.id)
        this.trip = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('loading.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    async fetchAssignableShipments() {
      try {
        const res = await listAssignableShipments()
        this.assignableShipments = res.data
      } catch {
        // Non-fatal — the assign control simply shows no options.
      }
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'LOADING') return 'orange'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'PLANNED': return this.$t('loading.statusLabels.planned')
        case 'LOADING': return this.$t('loading.statusLabels.loading')
        case 'COMPLETED': return this.$t('common.status.completed')
        case 'CANCELLED': return this.$t('common.status.cancelled')
        default: return status
      }
    },

    cartonStatusTone(status) {
      if (status === 'LOADED') return 'green'
      if (status === 'SHORT') return 'red'
      return 'gray'
    },

    cartonStatusLabel(status) {
      switch (status) {
        case 'PLANNED': return this.$t('loading.cartonStatus.planned')
        case 'LOADED': return this.$t('loading.cartonStatus.loaded')
        case 'SHORT': return this.$t('loading.cartonStatus.short')
        default: return status
      }
    },

    shortReasonLabel(reason) {
      switch (reason) {
        case 'DAMAGED': return this.$t('loading.shortReason.damaged')
        case 'MISSING': return this.$t('loading.shortReason.missing')
        default: return this.$t('loading.shortReason.other')
      }
    },

    formatDateTime(isoString) {
      if (!isoString) return '—'
      return this.$d(new Date(isoString), 'long')
    },

    async doAssign() {
      this.assigning = true
      try {
        const res = await assignShipmentToTrip(this.trip.id, this.assignShipmentId)
        this.trip = res.data
        this.assignShipmentId = ''
        await this.fetchAssignableShipments()
        showToast(this.$t('loading.detail.assignSuccess'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('loading.detail.assignFailed'), 'error')
      } finally {
        this.assigning = false
      }
    },

    async doScan() {
      if (!this.scanLpn) return
      this.scanning = true
      this.scanError = ''
      try {
        const res = await scanCarton(this.trip.id, this.scanLpn)
        this.trip = res.data
        this.scanLpn = ''
        showToast(this.$t('loading.detail.scanSuccess'), 'success')
      } catch (err) {
        this.scanError = err.message || this.$t('loading.detail.scanFailed')
      } finally {
        this.scanning = false
        this.$nextTick(() => this.$refs.scanInput && this.$refs.scanInput.focus())
      }
    },

    toggleShortForm() {
      this.showShortForm = !this.showShortForm
      if (this.showShortForm) this.shortForm = { lpn: this.scanLpn, reason: '', remark: '' }
    },

    async doShort() {
      if (!this.shortForm.lpn || !this.shortForm.reason) return
      this.shortSubmitting = true
      try {
        const res = await shortCarton(this.trip.id, {
          lpn: this.shortForm.lpn, reason: this.shortForm.reason, remark: this.shortForm.remark || undefined
        })
        this.trip = res.data
        this.shortForm = { lpn: '', reason: '', remark: '' }
        this.showShortForm = false
        showToast(this.$t('loading.detail.shortSuccess'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('loading.detail.shortFailed'), 'error')
      } finally {
        this.shortSubmitting = false
      }
    },

    async doConfirm() {
      this.confirming = true
      try {
        const res = await confirmLoadingTrip(this.trip.id, {})
        this.trip = res.data
        showToast(this.$t('loading.detail.confirmSuccess'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('loading.detail.confirmFailed'), 'error')
      } finally {
        this.confirming = false
      }
    },

    async doCancel() {
      this.cancelling = true
      this.cancelError = ''
      try {
        const res = await cancelLoadingTrip(this.trip.id, {})
        this.trip = res.data
        this.showCancelConfirm = false
        showToast(this.$t('loading.detail.cancelSuccess'), 'success')
      } catch (err) {
        this.cancelError = err.message
      } finally {
        this.cancelling = false
      }
    }
  }
}
</script>

<style scoped>
.field-label {
  color: #64748b;
  font-weight: 600;
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

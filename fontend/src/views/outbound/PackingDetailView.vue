<template>
  <div class="space-y-6">
    <PageHeader :title="job ? job.job_no : $t('packing.title')" :subtitle="$t('packing.detail.subtitle')">
      <template #actions>
        <button @click="$router.push('/outbound/packing')" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold">
          {{ $t('newRelocation.backButton') }}
        </button>
        <button v-if="job" @click="viewPdf" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold">
          {{ $t('packing.detail.viewPackingList') }}
        </button>
        <button v-if="job" @click="downloadPdf" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold">
          {{ $t('packing.detail.downloadPackingList') }}
        </button>
        <button v-if="job && job.cartons.length > 0" @click="viewLabels()" class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold">
          {{ $t('packing.detail.viewCartonLabels') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ loadError }}</div>
    <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('packing.loadingJobs') }}</div>

    <template v-else-if="job">
      <FormSection :title="$t('packing.detail.infoTitle')">
        <div class="flex flex-wrap gap-4 items-center mb-4">
          <StatusBadge :tone="statusTone(job.status)">{{ statusLabel(job.status) }}</StatusBadge>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm">
          <div><span class="field-label">{{ $t('packing.table.orderNo') }}:</span> {{ job.order_no }}</div>
          <div><span class="field-label">{{ $t('orders.detail.pickingTaskLabel') }}:</span> {{ job.pick_no }}</div>
          <div><span class="field-label">{{ $t('packing.table.warehouse') }}:</span> {{ job.warehouse_code }}</div>
          <div><span class="field-label">{{ $t('packing.table.customer') }}:</span> {{ job.customer || '—' }}</div>
          <div v-if="job.ship_to_name"><span class="field-label">{{ $t('orders.detail.shipToLabel') }}:</span> {{ job.ship_to_name }}</div>
          <div><span class="field-label">{{ $t('packing.detail.assignedPackerLabel') }}:</span> {{ job.assigned_packer_name || '—' }}</div>
          <div v-if="job.checker_name"><span class="field-label">{{ $t('packing.detail.checkerLabel') }}:</span> {{ job.checker_name }}</div>
        </div>
        <p v-if="job.remark" class="mt-2 text-sm"><span class="field-label">{{ $t('orders.detail.remarkLabel') }}:</span> {{ job.remark }}</p>
        <p v-if="job.cancel_remark" class="mt-2 text-sm text-slate-600"><span class="field-label">{{ $t('orders.detail.cancelRemarkLabel') }}:</span> {{ job.cancel_remark }}</p>

        <div v-if="canAssign && isJobWorkable" class="mt-4 flex gap-3 items-end">
          <div class="flex-1">
            <label class="label">{{ $t('packing.detail.assignPackerLabel') }}</label>
            <select v-model="assignPackerId" class="input">
              <option value="">{{ $t('packing.detail.selectPacker') }}</option>
              <option v-for="u in packers" :key="u.id" :value="u.id">{{ u.full_name || u.username }}</option>
            </select>
          </div>
          <button @click="doAssign" :disabled="!assignPackerId || assigning" class="bg-slate-800 hover:bg-slate-900 text-white px-4 py-3 rounded-xl font-semibold disabled:opacity-60">
            {{ assigning ? $t('common.buttons.submitting') : $t('packing.detail.assignButton') }}
          </button>
        </div>
      </FormSection>

      <FormSection :title="$t('packing.detail.cartonsTitle')">
        <div v-if="canUpdate && isJobWorkable" class="bg-slate-50 border border-slate-200 rounded-2xl p-4 mb-4 space-y-3">
          <h3 class="font-bold text-slate-800 text-sm">{{ $t('packing.detail.addCartonTitle') }}</h3>
          <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
            <select v-model="newCarton.packagingType" class="input">
              <option value="BOX">{{ $t('packing.packagingType.box') }}</option>
              <option value="PALLET">{{ $t('packing.packagingType.pallet') }}</option>
              <option value="BAG">{{ $t('packing.packagingType.bag') }}</option>
              <option value="CRATE">{{ $t('packing.packagingType.crate') }}</option>
              <option value="OTHER">{{ $t('packing.packagingType.other') }}</option>
            </select>
            <input v-model.number="newCarton.grossWeight" type="number" class="input" :placeholder="$t('packing.detail.grossWeightPlaceholder')" />
            <input v-model.number="newCarton.netWeight" type="number" class="input" :placeholder="$t('packing.detail.netWeightPlaceholder')" />
            <input v-model="newCarton.remark" type="text" class="input" :placeholder="$t('packing.detail.cartonRemarkPlaceholder')" />
          </div>
          <div class="grid grid-cols-3 gap-3">
            <input v-model.number="newCarton.length" type="number" class="input" :placeholder="$t('packing.detail.lengthPlaceholder')" />
            <input v-model.number="newCarton.width" type="number" class="input" :placeholder="$t('packing.detail.widthPlaceholder')" />
            <input v-model.number="newCarton.height" type="number" class="input" :placeholder="$t('packing.detail.heightPlaceholder')" />
          </div>
          <button @click="addCarton" :disabled="addingCarton" class="bg-slate-800 hover:bg-slate-900 text-white px-4 py-2 rounded-xl font-semibold text-sm disabled:opacity-60">
            {{ addingCarton ? $t('common.buttons.submitting') : $t('packing.detail.addCartonButton') }}
          </button>
        </div>

        <p v-if="job.cartons.length === 0" class="text-sm text-slate-400">{{ $t('packing.detail.noCartons') }}</p>
        <div v-for="carton in job.cartons" :key="carton.id" class="border border-slate-200 rounded-2xl p-4 mb-3">
          <div class="flex flex-wrap justify-between items-start gap-3">
            <div>
              <p class="font-bold text-slate-800">{{ carton.lpn }}</p>
              <p class="text-sm text-slate-500">{{ packagingTypeLabel(carton.packaging_type) }}</p>
              <p class="text-xs text-slate-400 mt-1" v-if="carton.gross_weight">
                {{ $t('packing.detail.grossWeightLabel') }}: {{ carton.gross_weight }} {{ carton.weight_unit }}
              </p>
              <p class="text-xs text-slate-400" v-if="carton.carton_length && carton.carton_width && carton.carton_height">
                {{ carton.carton_length }}x{{ carton.carton_width }}x{{ carton.carton_height }} {{ carton.dimension_unit }}
              </p>
            </div>
            <div class="flex gap-2 items-center">
              <StatusBadge :tone="carton.status === 'OPEN' ? 'green' : 'gray'">{{ cartonStatusLabel(carton.status) }}</StatusBadge>
              <button @click="viewLabels(carton.id)" class="text-orange-600 hover:underline text-sm font-semibold">{{ $t('packing.detail.printLabel') }}</button>
              <button v-if="canUpdate && carton.status === 'OPEN'" @click="closeCarton(carton)" class="text-slate-500 hover:underline text-sm font-semibold">
                {{ $t('packing.detail.closeCarton') }}
              </button>
            </div>
          </div>

          <div v-if="carton.contents.length > 0" class="mt-3 text-sm text-slate-600 space-y-1">
            <p v-for="c in carton.contents" :key="c.id">
              {{ c.sku }} — {{ c.product_name }} ({{ c.lot_no || '—' }}): {{ c.packed_qty }} {{ c.uom }}
            </p>
          </div>
        </div>
      </FormSection>

      <FormSection :title="$t('packing.detail.itemsTitle')">
        <p v-if="job.items.length === 0" class="text-sm text-slate-400">{{ $t('orders.form.noItemsYet') }}</p>

        <div v-for="item in job.items" :key="item.id" class="border border-slate-200 rounded-2xl p-4 mb-3">
          <div class="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
            <div>
              <p class="font-bold text-slate-800">{{ item.sku }}</p>
              <p class="text-sm text-slate-500">{{ item.product_name }}</p>
              <p v-if="item.lot_no" class="text-sm text-slate-500">{{ $t('packing.detail.lotLabel') }}: {{ item.lot_no }}</p>
              <p class="text-xs mt-1"><span :class="getItemStatusClass(item.status)">{{ itemStatusLabel(item.status) }}</span></p>
              <p v-if="item.carton_lpns && item.carton_lpns.length > 0" class="text-xs text-slate-400 mt-1">
                {{ $t('packing.detail.inCartonsLabel') }}: {{ item.carton_lpns.join(', ') }}
              </p>
            </div>

            <div class="grid grid-cols-3 gap-3 min-w-[260px]">
              <div>
                <p class="detail-label">{{ $t('picking.drawer.order') }}</p>
                <p class="detail-value">{{ item.ordered_qty }}</p>
              </div>
              <div>
                <p class="detail-label">{{ $t('picking.drawer.picked') }}</p>
                <p class="detail-value text-green-600">{{ item.picked_qty }}</p>
              </div>
              <div>
                <p class="detail-label">{{ $t('packing.detail.packedLabel') }}</p>
                <p class="detail-value text-orange-600">{{ item.packed_qty }}</p>
              </div>
            </div>
          </div>

          <div v-if="item.status === 'EXCEPTION'" class="bg-red-50 rounded-xl p-3 mt-3 text-sm">
            <p><span class="font-semibold">{{ $t('packing.detail.exceptionReasonLabel') }}:</span> {{ exceptionReasonLabel(item.exception_reason) }}</p>
            <p v-if="item.exception_qty != null">{{ $t('packing.detail.exceptionQtyLabel') }}: {{ item.exception_qty }}</p>
            <p v-if="item.remark" class="text-slate-500 mt-1">{{ item.remark }}</p>
          </div>

          <template v-else-if="canUpdate && isJobWorkable && item.status !== 'PACKED'">
            <!-- Scan-to-pack: a keyboard-wedge scanner types the scanned
                 code as keystrokes then Enter — matches PickingView.vue's
                 own convention, adapted to product+lot only since a
                 carton has no location concept. -->
            <div class="grid grid-cols-1 md:grid-cols-4 gap-3 mt-4">
              <select v-model="packForms[item.id].cartonId" class="input">
                <option value="">{{ $t('packing.detail.selectCarton') }}</option>
                <option v-for="c in openCartons" :key="c.id" :value="c.id">{{ c.lpn }}</option>
              </select>
              <input
                v-model="packForms[item.id].scannedSku"
                type="text"
                class="input"
                :placeholder="$t('packing.detail.scanSkuPlaceholder')"
                @keyup.enter="packLine(item)"
              />
              <input v-model="packForms[item.id].scannedLotNo" type="text" class="input" :placeholder="$t('packing.detail.scanLotPlaceholder')" />
              <input v-model.number="packForms[item.id].quantity" type="number" class="input" :placeholder="$t('packing.detail.packQtyPlaceholder')" />
            </div>
            <div class="flex gap-3 mt-3">
              <button
                @click="packLine(item)"
                :disabled="packing[item.id] || !packForms[item.id].cartonId"
                class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold text-sm disabled:opacity-60"
              >
                {{ packing[item.id] ? $t('packing.detail.packing') : $t('packing.detail.packButton') }}
              </button>
              <button @click="toggleExceptionForm(item)" class="bg-red-50 hover:bg-red-100 text-red-600 px-4 py-2 rounded-xl font-semibold text-sm">
                {{ $t('packing.detail.flagExceptionButton') }}
              </button>
            </div>

            <div v-if="exceptionForms[item.id] && exceptionForms[item.id].open" class="grid grid-cols-1 md:grid-cols-3 gap-3 mt-3 bg-red-50 rounded-xl p-3">
              <select v-model="exceptionForms[item.id].reason" class="input">
                <option value="">{{ $t('packing.detail.exceptionReasonPlaceholder') }}</option>
                <option value="DAMAGED">{{ $t('packing.exceptionReason.damaged') }}</option>
                <option value="SHORTAGE">{{ $t('packing.exceptionReason.shortage') }}</option>
                <option value="WRONG_ITEM">{{ $t('packing.exceptionReason.wrongItem') }}</option>
                <option value="OTHER">{{ $t('packing.exceptionReason.other') }}</option>
              </select>
              <input v-model.number="exceptionForms[item.id].quantity" type="number" class="input" :placeholder="$t('packing.detail.exceptionQtyPlaceholder')" />
              <input v-model="exceptionForms[item.id].remark" type="text" class="input" :placeholder="$t('packing.detail.exceptionRemarkPlaceholder')" />
              <button
                @click="submitException(item)"
                :disabled="!exceptionForms[item.id].reason || flaggingException[item.id]"
                class="md:col-span-3 bg-red-600 hover:bg-red-700 text-white px-4 py-2 rounded-xl font-semibold text-sm disabled:opacity-60"
              >
                {{ flaggingException[item.id] ? $t('common.buttons.submitting') : $t('packing.detail.submitExceptionButton') }}
              </button>
            </div>
          </template>

          <p v-else-if="item.status === 'PACKED'" class="text-sm text-green-600 font-semibold mt-3">{{ $t('packing.detail.linePacked') }}</p>
        </div>
      </FormSection>

      <FormSection v-if="job.status !== 'CANCELLED'" :title="$t('packing.detail.finishTitle')">
        <div class="flex flex-wrap gap-3">
          <button
            v-if="canConfirm && job.status === 'IN_PROGRESS'"
            @click="doCheckerConfirm"
            :disabled="checkerConfirming"
            class="px-5 py-3 rounded-xl bg-green-600 hover:bg-green-700 text-white font-semibold disabled:opacity-60"
          >
            {{ checkerConfirming ? $t('common.buttons.submitting') : $t('packing.detail.checkerConfirmButton') }}
          </button>
          <button
            v-if="canApprove && isJobWorkable"
            @click="showCancelConfirm = true"
            class="px-5 py-3 rounded-xl bg-slate-100 text-slate-700 font-semibold"
          >
            {{ $t('packing.detail.cancelJobButton') }}
          </button>
        </div>
        <p v-if="job.status === 'DRAFT'" class="text-xs text-slate-400 mt-3">{{ $t('packing.detail.checkerDraftNote') }}</p>
      </FormSection>
    </template>

    <ConfirmDialog
      :open="showCancelConfirm"
      :title="$t('packing.detail.cancelJobButton')"
      :message="$t('packing.detail.cancelConfirmMessage')"
      :confirm-label="$t('packing.detail.cancelJobButton')"
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
  getPackingJob, assignPacker, packItem, flagPackingException, createCarton, updateCarton,
  checkerConfirmPackingJob, cancelPackingJob, newIdempotencyKey, viewPackingList, downloadPackingList, viewCartonLabels
} from '../../services/packing'
import { listUsers } from '../../services/users'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'
import ConfirmDialog from '../../components/ui/ConfirmDialog.vue'

export default {
  name: 'PackingDetailView',
  components: { PageHeader, FormSection, StatusBadge, ConfirmDialog },
  props: { id: { type: [String, Number], required: true } },

  data() {
    return {
      loading: false,
      loadError: '',
      job: null,

      packers: [],
      assignPackerId: '',
      assigning: false,

      newCarton: { packagingType: 'BOX', grossWeight: null, netWeight: null, length: null, width: null, height: null, remark: '' },
      addingCarton: false,

      packForms: {},
      packing: {},

      exceptionForms: {},
      flaggingException: {},

      checkerConfirming: false,
      showCancelConfirm: false,
      cancelling: false,
      cancelError: ''
    }
  },

  computed: {
    canUpdate() { return hasPermission('packing.update') },
    canAssign() { return hasPermission('packing.update') },
    canConfirm() { return hasPermission('packing.confirm') },
    canApprove() { return hasPermission('packing.approve') },
    isJobWorkable() {
      return !!this.job && (this.job.status === 'DRAFT' || this.job.status === 'IN_PROGRESS')
    },
    openCartons() {
      if (!this.job) return []
      return this.job.cartons.filter((c) => c.status === 'OPEN')
    }
  },

  async mounted() {
    await this.load()
    await this.fetchPackers()
  },

  methods: {
    async load() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getPackingJob(this.id)
        this.job = res.data
        this.ensureLineForms()
      } catch (err) {
        this.loadError = err.message || this.$t('packing.messages.loadFailed')
      } finally {
        this.loading = false
      }
    },

    ensureLineForms() {
      for (const item of this.job.items) {
        if (!this.packForms[item.id]) {
          this.packForms[item.id] = { cartonId: '', scannedSku: item.sku, scannedLotNo: item.lot_no || '', quantity: null }
        }
        if (!this.exceptionForms[item.id]) {
          this.exceptionForms[item.id] = { open: false, reason: '', quantity: null, remark: '' }
        }
      }
    },

    async fetchPackers() {
      try {
        const res = await listUsers({ limit: 100 })
        this.packers = res.data
      } catch {
        // Non-fatal — the assign control simply shows no options.
      }
    },

    statusTone(status) {
      if (status === 'COMPLETED') return 'green'
      if (status === 'IN_PROGRESS') return 'orange'
      if (status === 'CANCELLED') return 'red'
      return 'gray'
    },

    statusLabel(status) {
      switch (status) {
        case 'DRAFT': return this.$t('packing.statusLabels.draft')
        case 'IN_PROGRESS': return this.$t('common.status.inProgress')
        case 'COMPLETED': return this.$t('common.status.completed')
        case 'CANCELLED': return this.$t('common.status.cancelled')
        default: return status
      }
    },

    itemStatusLabel(status) {
      switch (status) {
        case 'PENDING': return this.$t('packing.itemStatus.pending')
        case 'PARTIALLY_PACKED': return this.$t('packing.itemStatus.partiallyPacked')
        case 'PACKED': return this.$t('packing.itemStatus.packed')
        case 'EXCEPTION': return this.$t('packing.itemStatus.exception')
        default: return status
      }
    },

    getItemStatusClass(status) {
      if (status === 'PACKED') return 'text-green-600 font-semibold'
      if (status === 'EXCEPTION') return 'text-red-600 font-semibold'
      if (status === 'PARTIALLY_PACKED') return 'text-orange-600 font-semibold'
      return 'text-slate-500'
    },

    cartonStatusLabel(status) {
      return status === 'OPEN' ? this.$t('packing.cartonStatus.open') : this.$t('packing.cartonStatus.closed')
    },

    packagingTypeLabel(type) {
      switch (type) {
        case 'BOX': return this.$t('packing.packagingType.box')
        case 'PALLET': return this.$t('packing.packagingType.pallet')
        case 'BAG': return this.$t('packing.packagingType.bag')
        case 'CRATE': return this.$t('packing.packagingType.crate')
        default: return this.$t('packing.packagingType.other')
      }
    },

    exceptionReasonLabel(reason) {
      switch (reason) {
        case 'DAMAGED': return this.$t('packing.exceptionReason.damaged')
        case 'SHORTAGE': return this.$t('packing.exceptionReason.shortage')
        case 'WRONG_ITEM': return this.$t('packing.exceptionReason.wrongItem')
        default: return this.$t('packing.exceptionReason.other')
      }
    },

    async doAssign() {
      this.assigning = true
      try {
        const res = await assignPacker(this.job.id, this.assignPackerId)
        this.job = res.data
        showToast(this.$t('packing.detail.assignSuccess'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.assignFailed'), 'error')
      } finally {
        this.assigning = false
      }
    },

    async addCarton() {
      this.addingCarton = true
      try {
        await createCarton(this.job.id, {
          packaging_type: this.newCarton.packagingType,
          gross_weight: this.newCarton.grossWeight || undefined,
          net_weight: this.newCarton.netWeight || undefined,
          carton_length: this.newCarton.length || undefined,
          carton_width: this.newCarton.width || undefined,
          carton_height: this.newCarton.height || undefined,
          remark: this.newCarton.remark || undefined
        })
        this.newCarton = { packagingType: 'BOX', grossWeight: null, netWeight: null, length: null, width: null, height: null, remark: '' }
        await this.load()
        showToast(this.$t('packing.detail.cartonAdded'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.addCartonFailed'), 'error')
      } finally {
        this.addingCarton = false
      }
    },

    async closeCarton(carton) {
      try {
        await updateCarton(this.job.id, carton.id, {
          packaging_type: carton.packaging_type, weight_unit: carton.weight_unit, dimension_unit: carton.dimension_unit,
          gross_weight: carton.gross_weight, net_weight: carton.net_weight,
          carton_length: carton.carton_length, carton_width: carton.carton_width, carton_height: carton.carton_height,
          status: 'CLOSED'
        })
        await this.load()
        showToast(this.$t('packing.detail.cartonClosed'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.closeCartonFailed'), 'error')
      }
    },

    async packLine(item) {
      const form = this.packForms[item.id]
      if (!form.cartonId) return
      this.packing = { ...this.packing, [item.id]: true }
      try {
        const res = await packItem(this.job.id, {
          idempotency_key: newIdempotencyKey(),
          item_id: item.id,
          carton_id: form.cartonId,
          scanned_sku: form.scannedSku,
          scanned_lot_no: form.scannedLotNo || undefined,
          quantity: form.quantity
        })
        this.job = res.data
        this.ensureLineForms()
        this.packForms[item.id] = { cartonId: form.cartonId, scannedSku: item.sku, scannedLotNo: item.lot_no || '', quantity: null }
        showToast(this.$t('packing.detail.packSuccess'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.packFailed'), 'error')
      } finally {
        this.packing = { ...this.packing, [item.id]: false }
      }
    },

    toggleExceptionForm(item) {
      const current = this.exceptionForms[item.id] || { open: false, reason: '', quantity: null, remark: '' }
      this.exceptionForms = { ...this.exceptionForms, [item.id]: { ...current, open: !current.open } }
    },

    async submitException(item) {
      const form = this.exceptionForms[item.id]
      if (!form.reason) return
      this.flaggingException = { ...this.flaggingException, [item.id]: true }
      try {
        const res = await flagPackingException(this.job.id, item.id, {
          reason: form.reason, quantity: form.quantity || undefined, remark: form.remark || undefined
        })
        this.job = res.data
        this.ensureLineForms()
        showToast(this.$t('packing.detail.exceptionFlagged'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.flagExceptionFailed'), 'error')
      } finally {
        this.flaggingException = { ...this.flaggingException, [item.id]: false }
      }
    },

    async doCheckerConfirm() {
      this.checkerConfirming = true
      try {
        const res = await checkerConfirmPackingJob(this.job.id, {})
        this.job = res.data
        showToast(this.$t('packing.detail.checkerConfirmSuccess'), 'success')
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.checkerConfirmFailed'), 'error')
      } finally {
        this.checkerConfirming = false
      }
    },

    async doCancel() {
      this.cancelling = true
      this.cancelError = ''
      try {
        const res = await cancelPackingJob(this.job.id, {})
        this.job = res.data
        this.showCancelConfirm = false
        showToast(this.$t('packing.detail.cancelSuccess'), 'success')
      } catch (err) {
        this.cancelError = err.message
      } finally {
        this.cancelling = false
      }
    },

    async viewPdf() {
      try {
        await viewPackingList(this.job.id)
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.pdfFailed'), 'error')
      }
    },

    async downloadPdf() {
      try {
        await downloadPackingList(this.job.id)
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.pdfFailed'), 'error')
      }
    },

    async viewLabels(cartonId) {
      try {
        await viewCartonLabels(this.job.id, cartonId)
      } catch (err) {
        showToast(err.message || this.$t('packing.detail.pdfFailed'), 'error')
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

.detail-label {
  font-size: 0.75rem;
  color: #94a3b8;
  font-weight: 600;
}

.detail-value {
  font-size: 1rem;
  font-weight: 700;
  color: #1e293b;
}
</style>

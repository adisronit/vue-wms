<template>
  <div class="space-y-6">
    <PageHeader :title="$t('orders.review.title')" :subtitle="order ? $t('orders.review.subtitle', { orderNo: order.order_no }) : ''">
      <template #actions>
        <button
          @click="$router.push(`/outbound/orders/${id}`)"
          class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('newRelocation.backButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">{{ loadError }}</div>
    <div v-else-if="loading" class="text-sm text-slate-400">{{ $t('orders.loadingOrders') }}</div>

    <template v-else-if="order && availability">
      <FormSection :title="$t('orders.review.availabilityTitle')" :description="$t('orders.review.availabilityDescription')">
        <div v-if="availability.has_shortage" class="bg-amber-50 border border-amber-300 text-amber-700 rounded-xl p-4 text-sm mb-4 font-semibold">
          {{ $t('orders.review.shortageWarning') }}
        </div>
        <div v-else class="bg-green-50 border border-green-300 text-green-700 rounded-xl p-4 text-sm mb-4 font-semibold">
          {{ $t('orders.review.noShortage') }}
        </div>

        <DataTable :is-empty="availability.lines.length === 0" :empty-text="$t('orders.form.noItemsYet')" :colspan="6" min-width="900px">
          <template #head>
            <tr>
              <th class="table-th">{{ $t('orders.detail.tableSku') }}</th>
              <th class="table-th">{{ $t('orders.detail.tableProduct') }}</th>
              <th class="table-th text-right">{{ $t('orders.detail.tableQty') }}</th>
              <th class="table-th text-right">{{ $t('orders.review.available') }}</th>
              <th class="table-th text-right">{{ $t('orders.review.reserved') }}</th>
              <th class="table-th">{{ $t('orders.review.shortage') }}</th>
            </tr>
          </template>
          <tr v-for="line in availability.lines" :key="line.order_item_id" class="border-t" :class="line.has_shortage ? 'bg-red-50' : ''">
            <td class="table-td">{{ line.sku }}</td>
            <td class="table-td">{{ line.product_name }}</td>
            <td class="table-td text-right">{{ line.ordered_qty }}</td>
            <td class="table-td text-right" :class="line.has_shortage ? 'text-red-600 font-semibold' : ''">{{ line.available_qty }}</td>
            <td class="table-td text-right">{{ line.reserved_qty }}</td>
            <td class="table-td">
              <span v-if="line.has_shortage" class="text-red-600 font-semibold">{{ $t('orders.review.shortfallOf', { qty: line.shortfall_qty }) }}</span>
              <span v-else class="text-green-600">{{ $t('orders.review.ok') }}</span>
            </td>
          </tr>
        </DataTable>
      </FormSection>

      <FormSection :title="$t('orders.review.releaseTitle')">
        <div v-if="availability.has_shortage" class="space-y-3">
          <label class="flex items-start gap-2 text-sm" :class="canConfirm ? 'text-slate-700' : 'text-slate-400'">
            <input v-model="approveShortage" type="checkbox" class="rounded mt-0.5" :disabled="!canConfirm" />
            <span>{{ $t('orders.review.approveShortageLabel') }}</span>
          </label>
          <p v-if="!canConfirm" class="text-xs text-slate-400">{{ $t('orders.review.approveShortageNoPermission') }}</p>
        </div>

        <textarea v-model="releaseRemark" :placeholder="$t('orders.review.remarkPlaceholder')" class="input mt-3" rows="2"></textarea>

        <p v-if="releaseError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm mt-3">{{ releaseError }}</p>

        <div class="flex justify-end gap-3 mt-4">
          <button @click="$router.push(`/outbound/orders/${id}`)" class="px-6 py-3 rounded-xl bg-slate-100 text-slate-700 font-semibold">
            {{ $t('common.buttons.cancel') }}
          </button>
          <button
            @click="submitRelease"
            :disabled="releasing || !canConfirm || (availability.has_shortage && !approveShortage)"
            class="px-6 py-3 rounded-xl bg-orange-600 hover:bg-orange-700 text-white font-semibold disabled:opacity-60"
          >
            {{ releasing ? $t('orders.review.releasing') : $t('orders.review.releaseButton') }}
          </button>
        </div>
      </FormSection>
    </template>
  </div>
</template>

<script>
import { getOrder, getOrderAvailability, releaseOrder } from '../../services/orders'
import { hasPermission } from '../../services/auth'
import { showToast } from '../../services/toast'
import PageHeader from '../../components/ui/PageHeader.vue'
import FormSection from '../../components/ui/FormSection.vue'
import DataTable from '../../components/ui/DataTable.vue'

export default {
  name: 'OrderReviewView',
  components: { PageHeader, FormSection, DataTable },
  props: { id: { type: [String, Number], required: true } },
  data() {
    return {
      loading: false,
      loadError: '',
      order: null,
      availability: null,
      approveShortage: false,
      releaseRemark: '',
      releaseError: '',
      releasing: false
    }
  },
  computed: {
    canConfirm() { return hasPermission('order.confirm') }
  },
  async mounted() {
    await this.load()
  },
  methods: {
    async load() {
      this.loading = true
      this.loadError = ''
      try {
        const [orderRes, availRes] = await Promise.all([getOrder(this.id), getOrderAvailability(this.id)])
        this.order = orderRes.data
        this.availability = availRes.data
        if (this.order.status !== 'PENDING') {
          this.loadError = this.$t('orders.review.notPending')
        }
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadAvailabilityFailed')
      } finally {
        this.loading = false
      }
    },

    async submitRelease() {
      this.releaseError = ''
      this.releasing = true
      try {
        await releaseOrder(this.id, { approve_shortage: this.approveShortage, remark: this.releaseRemark || undefined })
        showToast(this.$t('orders.review.releaseSuccess'), 'success')
        this.$router.push(`/outbound/orders/${this.id}`)
      } catch (err) {
        this.releaseError = err.message || this.$t('orders.review.releaseFailed')
      } finally {
        this.releasing = false
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

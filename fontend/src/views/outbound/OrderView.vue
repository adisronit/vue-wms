<template>
  <div class="space-y-6">
    <PageHeader :title="$t('orders.title')" :subtitle="$t('orders.subtitle')">
      <template #actions>
        <button
          v-if="canCreate"
          @click="$router.push('/outbound/orders/new')"
          class="bg-orange-600 hover:bg-orange-700 text-white px-5 py-3 rounded-xl font-semibold"
        >
          {{ $t('orders.newButton') }}
        </button>
      </template>
    </PageHeader>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
      {{ loadError }}
    </div>

    <FilterBar>
      <input v-model="filters.search" type="text" :placeholder="$t('orders.filters.searchPlaceholder')" class="input" />

      <select v-model="filters.status" class="input">
        <option value="">{{ $t('orders.filters.allStatus') }}</option>
        <option value="PENDING">{{ $t('common.status.pending') }}</option>
        <option value="RELEASED">{{ $t('orders.statusLabels.released') }}</option>
        <option value="PICKING">{{ $t('nav.picking') }}</option>
        <option value="CANCELLED">{{ $t('common.status.cancelled') }}</option>
      </select>

      <select v-model="filters.warehouseId" class="input">
        <option value="">{{ $t('orders.filters.allWarehouses') }}</option>
        <option v-for="w in warehouses" :key="w.id" :value="w.id">{{ w.code }}</option>
      </select>

      <button @click="clearFilter" class="bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-semibold">
        {{ $t('common.buttons.clearFilter') }}
      </button>
    </FilterBar>

    <DataTable
      :loading="loading"
      :is-empty="orderList.length === 0"
      :loading-text="$t('orders.loadingOrders')"
      :empty-text="$t('orders.noOrders')"
      :colspan="10"
      min-width="1050px"
    >
      <template #head>
        <tr>
          <th class="table-th">{{ $t('orders.table.orderNo') }}</th>
          <th class="table-th">{{ $t('orders.form.orderTypeLabel') }}</th>
          <th class="table-th">{{ $t('orders.table.externalRef') }}</th>
          <th class="table-th">{{ $t('orders.table.customer') }}</th>
          <th class="table-th">{{ $t('common.labels.warehouse') }}</th>
          <th class="table-th text-right">{{ $t('orders.table.items') }}</th>
          <th class="table-th">{{ $t('common.labels.status') }}</th>
          <th class="table-th">{{ $t('orders.table.fulfillment') }}</th>
          <th class="table-th">{{ $t('orders.table.createdAt') }}</th>
          <th class="table-th text-right">{{ $t('orders.table.action') }}</th>
        </tr>
      </template>

      <tr v-for="o in orderList" :key="o.id" class="border-t hover:bg-orange-50">
        <td class="table-td font-semibold text-slate-800">{{ o.order_no }}</td>
        <td class="table-td">{{ o.order_type }}</td>
        <td class="table-td">{{ o.external_reference || '—' }}</td>
        <td class="table-td">{{ o.customer || '—' }}</td>
        <td class="table-td">{{ o.warehouse_code }}</td>
        <td class="table-td text-right">{{ o.item_count }}</td>
        <td class="table-td"><StatusBadge :tone="statusTone(o.status)">{{ statusLabel(o.status) }}</StatusBadge></td>
        <td class="table-td"><StatusBadge :tone="statusTone(o.fulfillment_status)">{{ statusLabel(o.fulfillment_status) }}</StatusBadge></td>
        <td class="table-td">{{ formatDate(o.created_at) }}</td>
        <td class="table-td text-right">
          <button @click="$router.push(`/outbound/orders/${o.id}`)" class="text-orange-600 hover:underline font-semibold">{{ $t('common.buttons.view') }}</button>
        </td>
      </tr>
    </DataTable>
  </div>
</template>

<script>
import { listOrders } from '../../services/orders'
import { listWarehouses } from '../../services/warehouses'
import { hasPermission } from '../../services/auth'
import PageHeader from '../../components/ui/PageHeader.vue'
import FilterBar from '../../components/ui/FilterBar.vue'
import DataTable from '../../components/ui/DataTable.vue'
import StatusBadge from '../../components/ui/StatusBadge.vue'

export default {
  name: 'OrderPage',
  components: { PageHeader, FilterBar, DataTable, StatusBadge },
  data() {
    return {
      loading: false,
      loadError: '',
      orderList: [],
      warehouses: [],
      filters: { search: '', status: '', warehouseId: '' }
    }
  },
  computed: {
    canCreate() { return hasPermission('order.create') }
  },
  async mounted() {
    await this.fetchWarehouses()
    await this.fetchOrders()
  },
  methods: {
    async fetchWarehouses() {
      try {
        const res = await listWarehouses({ limit: 100 })
        this.warehouses = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadWarehousesFailed')
      }
    },

    async fetchOrders() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await listOrders(this.filters)
        this.orderList = res.data
      } catch (err) {
        this.loadError = err.message || this.$t('orders.messages.loadOrdersFailed')
      } finally {
        this.loading = false
      }
    },

    clearFilter() {
      this.filters = { search: '', status: '', warehouseId: '' }
      this.fetchOrders()
    },

    formatDate(value) {
      if (!value) return '—'
      return this.$d(new Date(value), 'long')
    },

    statusTone(status) {
      if (status === 'DELIVERED' || status === 'PICKED' || status === 'COMPLETED') return 'green'
      if (status === 'PICKING' || status === 'SHIPPING' || status === 'RELEASED') return 'orange'
      if (status === 'SHIP_ISSUE') return 'red'
      return 'gray'
    },

    // Covers both order.status (PENDING/RELEASED/PICKING/CANCELLED) and
    // the richer, derived fulfillment_status (adds PICKING_CANCELLED/
    // PICKED/SHIPPING/SHIP_ISSUE/DELIVERED) — see backend internal/
    // order's dto.go FulfillmentStatus. Called from the template, not
    // cached at map time, so the label stays reactive to the locale.
    statusLabel(status) {
      switch (status) {
        case 'PENDING': return this.$t('orders.statusLabels.pending')
        case 'RELEASED': return this.$t('orders.statusLabels.released')
        case 'PICKING': return this.$t('orders.statusLabels.picking')
        case 'PICKING_CANCELLED': return this.$t('orders.statusLabels.pickingCancelled')
        case 'PICKED': return this.$t('orders.statusLabels.picked')
        case 'SHIPPING': return this.$t('orders.statusLabels.shipping')
        case 'SHIP_ISSUE': return this.$t('orders.statusLabels.shipIssue')
        case 'DELIVERED': return this.$t('orders.statusLabels.delivered')
        case 'CANCELLED': return this.$t('orders.statusLabels.cancelled')
        default: return status
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
  padding: 14px 16px;
  color: #475569;
  white-space: nowrap;
}
</style>

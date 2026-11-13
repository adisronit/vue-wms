<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <router-link to="/platform/tenants" class="text-slate-500 hover:text-orange-600 text-sm font-semibold">
        &larr; {{ $t('platformAdmin.detail.backButton') }}
      </router-link>
    </div>

    <div v-if="loadError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-4 text-sm">
      {{ loadError }}
    </div>

    <div v-else-if="loading" class="text-center py-16 text-slate-400">
      {{ $t('platformAdmin.tenants.table.loading') }}
    </div>

    <template v-else-if="tenant">
      <div class="bg-white rounded-2xl border border-slate-200 p-6">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <div>
            <h1 class="text-2xl font-bold text-slate-800">{{ tenant.name }}</h1>
            <p class="text-slate-500">{{ tenant.slug }}</p>
          </div>

          <div class="flex items-center gap-3">
            <span :class="statusClass(tenant.status)">
              {{ $t('platformAdmin.status.' + tenant.status.toLowerCase()) }}
            </span>
            <button
              @click="showForm = true"
              class="bg-slate-100 hover:bg-slate-200 text-slate-700 px-4 py-2 rounded-xl font-semibold"
            >
              {{ $t('platformAdmin.detail.editButton') }}
            </button>
            <button
              @click="showStatus = true"
              class="bg-orange-600 hover:bg-orange-700 text-white px-4 py-2 rounded-xl font-semibold"
            >
              {{ $t('platformAdmin.detail.changeStatusButton') }}
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6 pt-6 border-t border-slate-200">
          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.slug') }}</p>
            <p class="detail-value">{{ tenant.slug }}</p>
          </div>

          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.status') }}</p>
            <p class="detail-value">{{ $t('platformAdmin.status.' + tenant.status.toLowerCase()) }}</p>
          </div>

          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.createdAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.created_at), 'long') }}</p>
          </div>

          <div>
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.updatedAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.updated_at), 'long') }}</p>
          </div>

          <div v-if="tenant.suspended_at">
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.suspendedAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.suspended_at), 'long') }}</p>
          </div>

          <div v-if="tenant.cancelled_at">
            <p class="detail-label">{{ $t('platformAdmin.detail.fields.cancelledAt') }}</p>
            <p class="detail-value">{{ $d(new Date(tenant.cancelled_at), 'long') }}</p>
          </div>
        </div>
      </div>
    </template>

    <TenantFormDrawer
      v-if="showForm && tenant"
      :tenant="tenant"
      @close="showForm = false"
      @saved="onSaved"
    />

    <TenantStatusModal
      v-if="showStatus && tenant"
      :tenant="tenant"
      @close="showStatus = false"
      @updated="onStatusUpdated"
    />
  </div>
</template>

<script>
import { getTenant } from '../../services/platformTenants'
import TenantFormDrawer from './TenantFormDrawer.vue'
import TenantStatusModal from './TenantStatusModal.vue'

export default {
  name: 'TenantDetailView',

  components: { TenantFormDrawer, TenantStatusModal },

  data() {
    return {
      tenant: null,
      loading: false,
      loadError: '',
      showForm: false,
      showStatus: false
    }
  },

  mounted() {
    this.fetchTenant()
  },

  methods: {
    async fetchTenant() {
      this.loading = true
      this.loadError = ''
      try {
        const res = await getTenant(this.$route.params.id)
        this.tenant = res.data
      } catch (err) {
        if (err.status === 404) {
          this.loadError = this.$t('platformAdmin.detail.notFound')
        } else {
          this.loadError = err.message || this.$t('platformAdmin.detail.loadFailed')
        }
      } finally {
        this.loading = false
      }
    },

    onSaved(savedTenant) {
      this.showForm = false
      this.tenant = savedTenant
    },

    onStatusUpdated(updatedTenant) {
      this.showStatus = false
      this.tenant = updatedTenant
    },

    statusClass(status) {
      if (status === 'ACTIVE') return 'badge badge-green'
      if (status === 'TRIAL') return 'badge badge-orange'
      if (status === 'SUSPENDED') return 'badge badge-gray'
      if (status === 'CANCELLED') return 'badge badge-red'
      return 'badge badge-gray'
    }
  }
}
</script>

<style scoped>
.detail-label {
  font-size: 12px;
  color: #94a3b8;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 4px;
}

.detail-value {
  font-size: 15px;
  color: #334155;
  font-weight: 600;
}

.badge {
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-gray {
  background: #f1f5f9;
  color: #475569;
}

.badge-green {
  background: #dcfce7;
  color: #16a34a;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
}

.badge-red {
  background: #fee2e2;
  color: #dc2626;
}
</style>

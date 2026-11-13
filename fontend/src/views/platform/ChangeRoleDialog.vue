<template>
  <div class="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-md p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold text-slate-800">{{ $t('platformAdmin.tenantUsers.changeRole.title') }}</h2>
        <button @click="$emit('close')" class="text-slate-400 hover:text-slate-600">✕</button>
      </div>

      <div>
        <p class="font-semibold text-slate-700">{{ member.full_name }}</p>
        <p class="text-xs text-slate-400">{{ member.username }} &middot; {{ member.email }}</p>
      </div>

      <div>
        <label class="label">{{ $t('platformAdmin.tenantUsers.changeRole.currentRole') }}</label>
        <p class="font-semibold text-slate-700">{{ $t('platformAdmin.tenantUsers.roles.' + roleKey(member.role.code)) }}</p>
      </div>

      <div>
        <label class="label">{{ $t('platformAdmin.tenantUsers.changeRole.newRole') }}</label>
        <select v-model="newRoleCode" class="input">
          <option v-for="code in TENANT_ROLE_CODES" :key="code" :value="code">
            {{ $t('platformAdmin.tenantUsers.roles.' + roleKey(code)) }}
          </option>
        </select>
      </div>

      <div v-if="roleError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ roleError }}
      </div>

      <div class="flex gap-3">
        <button
          @click="$emit('close')"
          class="flex-1 bg-slate-100 hover:bg-slate-200 text-slate-700 py-3 rounded-xl font-semibold"
        >
          {{ $t('common.buttons.cancel') }}
        </button>
        <button
          @click="save"
          :disabled="saving || newRoleCode === member.role.code"
          class="flex-1 bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ saving ? $t('platformAdmin.tenantUsers.form.saving') : $t('platformAdmin.tenantUsers.changeRole.saveButton') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
import { changeTenantUserRole, TENANT_ROLE_CODES } from '../../services/platformTenantUsers'

const ROLE_NAME_KEYS = {
  ADMIN: 'admin',
  WAREHOUSE_MANAGER: 'warehouseManager',
  RECEIVING: 'receiving',
  QC: 'qc',
  PUTAWAY: 'putaway',
  PICKING: 'picking',
  SHIPPING: 'shipping'
}

export default {
  name: 'ChangeRoleDialog',

  props: {
    tenantId: {
      type: [String, Number],
      required: true
    },
    member: {
      type: Object,
      required: true
    }
  },

  emits: ['close', 'updated'],

  data() {
    return {
      TENANT_ROLE_CODES,
      newRoleCode: this.member.role.code,
      saving: false,
      roleError: ''
    }
  },

  methods: {
    roleKey(code) {
      return ROLE_NAME_KEYS[code] || code
    },

    async save() {
      this.roleError = ''
      this.saving = true
      try {
        const res = await changeTenantUserRole(this.tenantId, this.member.user_id, { role_code: this.newRoleCode })
        this.$emit('updated', res.data)
      } catch (err) {
        if (err.status === 409) {
          this.roleError = this.$t('platformAdmin.tenantUsers.changeRole.messages.lastAdmin')
        } else {
          this.roleError = err.message || this.$t('platformAdmin.tenantUsers.form.messages.saveFailed')
        }
      } finally {
        this.saving = false
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
</style>

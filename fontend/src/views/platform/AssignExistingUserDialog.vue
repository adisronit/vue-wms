<template>
  <div class="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
    <div class="bg-white rounded-2xl shadow-xl w-full max-w-md p-6 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold text-slate-800">{{ $t('platformAdmin.tenantUsers.assignExisting.title') }}</h2>
        <button @click="$emit('close')" class="text-slate-400 hover:text-slate-600">✕</button>
      </div>

      <p class="text-sm text-slate-500">{{ $t('platformAdmin.tenantUsers.assignExisting.description') }}</p>

      <div>
        <label class="label">{{ $t('platformAdmin.tenantUsers.assignExisting.identifierLabel') }}</label>
        <input
          v-model="identifier"
          type="text"
          class="input"
          :placeholder="$t('platformAdmin.tenantUsers.assignExisting.identifierPlaceholder')"
        />
      </div>

      <div>
        <label class="label">{{ $t('platformAdmin.tenantUsers.form.fields.role') }}</label>
        <select v-model="roleCode" class="input">
          <option v-for="code in TENANT_ROLE_CODES" :key="code" :value="code">
            {{ $t('platformAdmin.tenantUsers.roles.' + roleKey(code)) }}
          </option>
        </select>
      </div>

      <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
        {{ formError }}
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
          :disabled="saving"
          class="flex-1 bg-orange-600 hover:bg-orange-700 text-white py-3 rounded-xl font-semibold disabled:opacity-60"
        >
          {{ saving ? $t('platformAdmin.tenantUsers.form.saving') : $t('platformAdmin.tenantUsers.assignExisting.assignButton') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
import { createTenantUser, TENANT_ROLE_CODES } from '../../services/platformTenantUsers'

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
  name: 'AssignExistingUserDialog',

  props: {
    tenantId: {
      type: [String, Number],
      required: true
    }
  },

  emits: ['close', 'created'],

  data() {
    return {
      TENANT_ROLE_CODES,
      identifier: '',
      roleCode: TENANT_ROLE_CODES[0],
      saving: false,
      formError: ''
    }
  },

  methods: {
    roleKey(code) {
      return ROLE_NAME_KEYS[code] || code
    },

    async save() {
      this.formError = ''
      const identifier = this.identifier.trim()
      if (!identifier) {
        this.formError = this.$t('platformAdmin.tenantUsers.assignExisting.messages.identifierRequired')
        return
      }

      this.saving = true
      try {
        const res = await createTenantUser(this.tenantId, {
          mode: 'EXISTING',
          role_code: this.roleCode,
          identifier
        })
        this.$emit('created', res.data)
      } catch (err) {
        if (err.status === 404) {
          this.formError = this.$t('platformAdmin.tenantUsers.assignExisting.messages.userNotFound')
        } else if (err.status === 409) {
          this.formError = this.$t('platformAdmin.tenantUsers.assignExisting.messages.alreadyMember')
        } else {
          this.formError = err.message || this.$t('platformAdmin.tenantUsers.form.messages.saveFailed')
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
  font-weight: 600;
  font-size: 14px;
}
</style>

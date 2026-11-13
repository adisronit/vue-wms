<template>
  <div class="fixed inset-0 bg-black/40 flex justify-end z-50">
    <div class="w-full max-w-lg bg-white h-full p-6 overflow-y-auto">
      <div class="flex items-center justify-between mb-6">
        <div>
          <h2 class="text-xl font-bold text-slate-800">{{ $t('platformAdmin.tenantUsers.form.title') }}</h2>
          <p class="text-slate-500">{{ $t('platformAdmin.tenantUsers.form.subtitle') }}</p>
        </div>

        <button @click="$emit('close')" class="text-slate-500 hover:text-red-600 text-xl">✕</button>
      </div>

      <div class="space-y-4">
        <div>
          <label class="label">{{ $t('platformAdmin.tenantUsers.form.fields.fullName') }}</label>
          <input v-model="form.full_name" type="text" class="input" />
        </div>

        <div>
          <label class="label">{{ $t('platformAdmin.tenantUsers.form.fields.username') }}</label>
          <input v-model="form.username" type="text" class="input" />
        </div>

        <div>
          <label class="label">{{ $t('platformAdmin.tenantUsers.form.fields.email') }}</label>
          <input v-model="form.email" type="email" class="input" />
        </div>

        <div>
          <label class="label">{{ $t('platformAdmin.tenantUsers.form.fields.password') }}</label>
          <input v-model="form.password" type="password" class="input" :placeholder="$t('platformAdmin.tenantUsers.form.fields.passwordHint')" />
          <p class="text-xs text-slate-400 mt-1">{{ $t('platformAdmin.tenantUsers.form.fields.passwordHint') }}</p>
        </div>

        <div>
          <label class="label">{{ $t('platformAdmin.tenantUsers.form.fields.role') }}</label>
          <select v-model="form.role_code" class="input">
            <option v-for="code in TENANT_ROLE_CODES" :key="code" :value="code">
              {{ $t('platformAdmin.tenantUsers.roles.' + roleKey(code)) }}
            </option>
          </select>
        </div>

        <div v-if="formError" class="bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm">
          {{ formError }}
        </div>

        <div class="flex gap-3 mt-4">
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
            {{ saving ? $t('platformAdmin.tenantUsers.form.saving') : $t('platformAdmin.tenantUsers.form.saveButton') }}
          </button>
        </div>
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
  name: 'TenantUserForm',

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
      saving: false,
      formError: '',
      form: {
        full_name: '',
        username: '',
        email: '',
        password: '',
        role_code: TENANT_ROLE_CODES[0]
      }
    }
  },

  methods: {
    roleKey(code) {
      return ROLE_NAME_KEYS[code] || code
    },

    async save() {
      this.formError = ''

      const fullName = this.form.full_name.trim()
      const username = this.form.username.trim()
      const email = this.form.email.trim()
      if (!fullName || !username || !email) {
        this.formError = this.$t('platformAdmin.tenantUsers.form.messages.requiredFields')
        return
      }
      if (this.form.password.length < 8) {
        this.formError = this.$t('platformAdmin.tenantUsers.form.messages.passwordTooShort')
        return
      }

      this.saving = true
      try {
        const res = await createTenantUser(this.tenantId, {
          mode: 'NEW',
          role_code: this.form.role_code,
          full_name: fullName,
          username,
          email,
          password: this.form.password
        })
        this.$emit('created', res.data)
      } catch (err) {
        if (err.status === 409) {
          this.formError = this.$t('platformAdmin.tenantUsers.form.messages.duplicateUser')
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
  font-weight: var(--fw-label);
  font-size: var(--fs-label);
}
</style>

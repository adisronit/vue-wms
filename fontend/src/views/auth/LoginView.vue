<template>
  <div
    class="min-h-screen bg-gradient-to-br from-orange-700 via-orange-600 to-amber-500 flex items-center justify-center p-6"
  >
    <div
      class="w-full max-w-5xl bg-white rounded-3xl shadow-2xl overflow-hidden grid md:grid-cols-2"
    >
      <!-- Left Section -->
      <div
        class="hidden md:flex flex-col justify-center bg-slate-900 text-white p-12"
      >
        <h1 class="text-5xl font-bold mb-6">
          {{ $t('auth.login.brandTitle') }}
        </h1>

        <h2 class="text-2xl font-semibold mb-4">
          {{ $t('auth.login.brandSubtitle') }}
        </h2>

        <p class="text-slate-300 leading-8">
          {{ $t('auth.login.brandDescription') }}
        </p>
      </div>

      <!-- Right Section -->
      <div class="p-10 md:p-14">
        <div class="max-w-md mx-auto">
          <div class="flex justify-end mb-4">
            <LanguageSwitcher />
          </div>

          <div
            class="w-16 h-16 rounded-full bg-orange-600 flex items-center justify-center text-white text-2xl font-bold mx-auto shadow-lg"
          >
            📦
          </div>

          <h2
            class="text-center text-3xl font-bold text-slate-800 mt-6"
          >
            {{ $t('auth.login.signInTitle') }}
          </h2>

          <p
            class="text-center text-slate-500 mt-2 mb-8"
          >
            {{ $t('auth.login.signInSubtitle') }}
          </p>

          <form @submit.prevent="handleLogin">
            <!-- Username -->
            <div class="mb-5">
              <label
                class="block text-sm font-medium text-slate-700 mb-2"
              >
                {{ $t('auth.login.username') }}
              </label>

              <input
                v-model="form.username"
                type="text"
                :placeholder="$t('auth.login.usernamePlaceholder')"
                class="w-full border border-slate-300 rounded-xl px-4 py-3 focus:ring-2 focus:ring-orange-500 focus:outline-none"
              />
            </div>

            <!-- Password -->
            <div class="mb-5">
              <label
                class="block text-sm font-medium text-slate-700 mb-2"
              >
                {{ $t('auth.login.password') }}
              </label>

              <input
                v-model="form.password"
                type="password"
                :placeholder="$t('auth.login.passwordPlaceholder')"
                class="w-full border border-slate-300 rounded-xl px-4 py-3 focus:ring-2 focus:ring-orange-500 focus:outline-none"
              />
            </div>

            <!-- Remember -->
            <div
              class="flex items-center justify-between mb-6"
            >
              <label
                class="flex items-center gap-2 text-sm text-slate-600"
              >
                <input
                  v-model="form.rememberMe"
                  type="checkbox"
                />
                {{ $t('auth.login.rememberMe') }}
              </label>

              <a
                href="#"
                class="text-orange-600 text-sm hover:underline"
              >
                {{ $t('auth.login.forgotPassword') }}
              </a>
            </div>

            <!-- Error -->
            <div
              v-if="errorMessage"
              class="mb-4 bg-red-50 border border-red-200 text-red-600 rounded-xl p-3 text-sm"
            >
              {{ errorMessage }}
            </div>

            <!-- Submit -->
            <button
              type="submit"
              :disabled="loading"
              class="w-full bg-orange-600 hover:bg-orange-700 text-white font-semibold py-3 rounded-xl transition-all duration-300 shadow-lg hover:shadow-xl disabled:opacity-60"
            >
              {{ loading ? $t('auth.login.signingIn') : $t('auth.login.signIn') }}
            </button>
          </form>

          <div
            class="mt-8 text-center text-sm text-slate-400"
          >
            {{ $t('auth.login.copyright') }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { login } from '../../services/auth'
import LanguageSwitcher from '../../components/LanguageSwitcher.vue'

export default {
  name: 'LoginPage',

  components: { LanguageSwitcher },

  data() {
    return {
      errorMessage: '',
      loading: false,
      form: {
        username: '',
        password: '',
        rememberMe: false
      }
    }
  },

  methods: {
    async handleLogin() {
      this.errorMessage = ''

      if (!this.form.username) {
        this.errorMessage = this.$t('auth.login.usernameRequired')
        return
      }

      if (!this.form.password) {
        this.errorMessage = this.$t('auth.login.passwordRequired')
        return
      }

      this.loading = true
      try {
        await login(this.form.username, this.form.password)
        this.$router.push('/dashboard')
      } catch (err) {
        // err.message comes from the Go backend's own error envelope
        // (see services/auth.js) and is not run through i18n — only
        // the client-side fallback below is translated. Translating
        // server-generated text client-side would require a fragile
        // string-matching map; out of this phase's scope (backend is
        // not modified).
        this.errorMessage = err.message || this.$t('auth.login.loginFailed')
      } finally {
        this.loading = false
      }
    }
  }
}
</script>

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
          Smart WMS
        </h1>

        <h2 class="text-2xl font-semibold mb-4">
          Warehouse Management System
        </h2>

        <p class="text-slate-300 leading-8">
          Manage inbound receiving, inventory control,
          picking, packing and shipping operations
          through a modern warehouse platform.
        </p>
      </div>

      <!-- Right Section -->
      <div class="p-10 md:p-14">
        <div class="max-w-md mx-auto">
          <div
            class="w-16 h-16 rounded-full bg-orange-600 flex items-center justify-center text-white text-2xl font-bold mx-auto shadow-lg"
          >
            📦
          </div>

          <h2
            class="text-center text-3xl font-bold text-slate-800 mt-6"
          >
            Sign In
          </h2>

          <p
            class="text-center text-slate-500 mt-2 mb-8"
          >
            Login to Smart WMS
          </p>

          <form @submit.prevent="handleLogin">
            <!-- Username -->
            <div class="mb-5">
              <label
                class="block text-sm font-medium text-slate-700 mb-2"
              >
                Username
              </label>

              <input
                v-model="form.username"
                type="text"
                placeholder="Enter username"
                class="w-full border border-slate-300 rounded-xl px-4 py-3 focus:ring-2 focus:ring-orange-500 focus:outline-none"
              />
            </div>

            <!-- Password -->
            <div class="mb-5">
              <label
                class="block text-sm font-medium text-slate-700 mb-2"
              >
                Password
              </label>

              <input
                v-model="form.password"
                type="password"
                placeholder="Enter password"
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
                Remember Me
              </label>

              <a
                href="#"
                class="text-orange-600 text-sm hover:underline"
              >
                Forgot Password?
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
              {{ loading ? 'Signing in...' : 'Sign In' }}
            </button>
          </form>

          <div
            class="mt-8 text-center text-sm text-slate-400"
          >
            © 2026 Smart WMS
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { login } from '../../services/auth'

export default {
  name: 'LoginPage',

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
        this.errorMessage = 'Username is required'
        return
      }

      if (!this.form.password) {
        this.errorMessage = 'Password is required'
        return
      }

      this.loading = true
      try {
        await login(this.form.username, this.form.password)
        this.$router.push('/dashboard')
      } catch (err) {
        this.errorMessage = err.message || 'Login failed'
      } finally {
        this.loading = false
      }
    }
  }
}
</script>
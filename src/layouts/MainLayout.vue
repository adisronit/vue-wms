<template>
  <div class="h-screen flex bg-slate-100 overflow-hidden">
    <!-- Sidebar -->
    <aside class="w-72 shrink-0 bg-slate-900 text-white hidden lg:flex flex-col">
      <div class="h-20 flex items-center px-6 border-b border-slate-700 shrink-0">
        <div class="w-11 h-11 shrink-0 bg-orange-600 rounded-xl flex items-center justify-center text-xl">
          📦
        </div>

        <div class="ml-3 min-w-0">
          <h1 class="text-lg font-bold truncate">Smart WMS</h1>
          <p class="text-xs text-slate-400 truncate">Warehouse System</p>
        </div>
      </div>

      <nav class="flex-1 px-4 py-6 space-y-2 overflow-y-auto sidebar-scroll">
        <router-link
          to="/dashboard"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🏠</span>
          <span class="menu-text">Dashboard</span>
        </router-link>

        <div class="menu-title">Inbound</div>

        <router-link
          to="/inbound/receiving"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📥</span>
          <span class="menu-text">Receiving</span>
        </router-link>

        <router-link
          to="/inbound/quality-check"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">✅</span>
          <span class="menu-text">Quality Check</span>
        </router-link>

        <router-link
          to="/inbound/put-away"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📍</span>
          <span class="menu-text">Putaway</span>
        </router-link>

        <div class="menu-title">Inventory</div>

        <router-link
          to="/inventory/stock-inquiry"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📦</span>
          <span class="menu-text">Stock Inquiry</span>
        </router-link>

        <router-link
          to="/inventory/stock-adjustment"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🔄</span>
          <span class="menu-text">Stock Adjustment</span>
        </router-link>

        <router-link
          to="/inventory/check-stock"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🧾</span>
          <span class="menu-text">Check Stock</span>
        </router-link>

        <div class="menu-title">Outbound</div>

        <router-link
          to="/outbound/picking"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">📤</span>
          <span class="menu-text">Picking</span>
        </router-link>

        <router-link
          to="/outbound/shipping"
          class="menu-item"
          active-class="menu-active"
        >
          <span class="menu-icon">🚚</span>
          <span class="menu-text">Shipping</span>
        </router-link>

        <div class="menu-title">Administration</div>

          <router-link
            to="/administration/user-management"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">👤</span>
            <span class="menu-text">User Management</span>
          </router-link>

          <router-link
            to="/administration/roles"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">🔐</span>
            <span class="menu-text">Roles & Permissions</span>
          </router-link>

          <router-link
            to="/administration/warehouse"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">🏭</span>
            <span class="menu-text">Warehouse Setup</span>
          </router-link>

          <router-link
            to="/administration/products"
            class="menu-item"
            active-class="menu-active"
          >
            <span class="menu-icon">📦</span>
            <span class="menu-text">Product Master</span>
          </router-link>
      </nav>

      <div class="p-4 border-t border-slate-700">
        <button @click="logout"       
          class="w-full bg-slate-800 hover:bg-red-600 text-white py-3 rounded-xl transition"
        >
          Logout
        </button>
      </div>
    </aside>

    <!-- Main Area -->
    <div class="flex-1 min-w-0 flex flex-col">
      <!-- Topbar -->
      <header class="h-20 shrink-0 bg-white border-b border-slate-200 flex items-center justify-between px-6">
        <div class="min-w-0">
          <h2 class="text-xl font-bold text-slate-800 truncate">
            Warehouse Management System
          </h2>
          <p class="text-sm text-slate-500 truncate">
            Manage warehouse operations efficiently
          </p>
        </div>

        <div class="flex items-center gap-4 shrink-0">
          <button class="relative w-10 h-10 rounded-full bg-slate-100 hover:bg-slate-200">
            🔔
            <span class="absolute top-1 right-1 w-2 h-2 bg-orange-500 rounded-full"></span>
          </button>

          <div class="flex items-center gap-3">
            <div class="text-right hidden sm:block">
              <p class="text-sm font-semibold text-slate-800">Admin User</p>
              <p class="text-xs text-slate-500">Warehouse Manager</p>
            </div>

            <div class="w-11 h-11 bg-orange-600 text-white rounded-full flex items-center justify-center font-bold">
              A
            </div>
          </div>
        </div>
      </header>

      <!-- Page Content -->
      <main class="flex-1 min-w-0 overflow-y-auto overflow-x-hidden p-6">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script>
export default {
  methods: {
    logout() {
      this.$router.push('/login')
    }
  }
}
</script>

<style scoped>

.menu-item {
  display: flex;
  align-items: center;
  gap: 10px;

  padding: 12px 14px;

  border-radius: 12px;

  color: #cbd5e1;

  transition: all .2s;

  text-decoration: none;
}

.menu-item:hover {
  background: #1e293b;
}

.menu-active {
  background: #ea580c;
  color: white;
}

.menu-title {
  margin-top: 20px;
  margin-bottom: 8px;

  color: #64748b;

  font-size: 11px;

  text-transform: uppercase;

  letter-spacing: 1px;

  font-weight: 700;
}

/* Custom Scrollbar */

.sidebar-scroll::-webkit-scrollbar {
  width: 6px;
}

.sidebar-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-scroll::-webkit-scrollbar-thumb {
  background: #475569;
  border-radius: 10px;
}

.sidebar-scroll::-webkit-scrollbar-thumb:hover {
  background: #64748b;
}

</style>
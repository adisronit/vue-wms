<template>
  <div>
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-slate-800">
        Dashboard
      </h1>
      <p class="text-slate-500">
        Overview of warehouse operations
      </p>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-6">
      <div class="summary-card">
        <p class="summary-label">Pending Receiving</p>
        <h2 class="summary-value">24</h2>
        <p class="summary-note text-orange-600">Inbound orders</p>
      </div>

      <div class="summary-card">
        <p class="summary-label">Available Stock</p>
        <h2 class="summary-value">12,450</h2>
        <p class="summary-note text-green-600">Items in warehouse</p>
      </div>

      <div class="summary-card">
        <p class="summary-label">Pending Picking</p>
        <h2 class="summary-value">38</h2>
        <p class="summary-note text-blue-600">Outbound tasks</p>
      </div>

      <div class="summary-card">
        <p class="summary-label">Low Stock</p>
        <h2 class="summary-value">7</h2>
        <p class="summary-note text-red-600">Need attention</p>
      </div>
    </div>

    <div class="grid grid-cols-1 xl:grid-cols-3 gap-6 mt-6">
      <div class="bg-white rounded-2xl shadow-sm border border-slate-200 p-6 xl:col-span-2">
        <h2 class="text-lg font-bold text-slate-800 mb-4">
          Recent Inbound Activity
        </h2>

        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-left text-slate-500 border-b">
                <th class="py-3">Document No.</th>
                <th class="py-3">Vendor</th>
                <th class="py-3">Status</th>
                <th class="py-3">Date</th>
              </tr>
            </thead>

            <tbody>
              <tr class="border-b">
                <td class="py-3 font-medium">PO-20260608-001</td>
                <td class="py-3">ABC Supplier</td>
                <td class="py-3">
                  <span class="badge-orange">Receiving</span>
                </td>
                <td class="py-3">08/06/2026</td>
              </tr>

              <tr class="border-b">
                <td class="py-3 font-medium">PO-20260608-002</td>
                <td class="py-3">Global Parts</td>
                <td class="py-3">
                  <span class="badge-green">Completed</span>
                </td>
                <td class="py-3">08/06/2026</td>
              </tr>

              <tr>
                <td class="py-3 font-medium">PO-20260608-003</td>
                <td class="py-3">Fast Supply</td>
                <td class="py-3">
                  <span class="badge-gray">Waiting</span>
                </td>
                <td class="py-3">08/06/2026</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="bg-white rounded-2xl shadow-sm border border-slate-200 p-6">
        <h2 class="text-lg font-bold text-slate-800 mb-4">
          Quick Actions
        </h2>

        <div class="space-y-3">
          <button class="quick-button">
            📥 Start Receiving
          </button>

          <button class="quick-button">
            📦 Stock Inquiry
          </button>

          <button class="quick-button">
            📤 Create Picking Task
          </button>

          <button class="quick-button">
            🔄 Stock Adjustment
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'DashboardPage'
}
</script>

<style scoped>
.summary-card {
  background: white;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  padding: 22px;
  box-shadow: 0 1px 3px rgba(15, 23, 42, 0.08);
}

.summary-label {
  color: #64748b;
  font-size: 14px;
}

.summary-value {
  font-size: 32px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 8px;
}

.summary-note {
  font-size: 13px;
  margin-top: 6px;
  font-weight: 600;
}

.badge-orange {
  background: #ffedd5;
  color: #ea580c;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-green {
  background: #dcfce7;
  color: #16a34a;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.badge-gray {
  background: #f1f5f9;
  color: #475569;
  padding: 5px 10px;
  border-radius: 999px;
  font-weight: 600;
  font-size: 12px;
}

.quick-button {
  width: 100%;
  text-align: left;
  padding: 14px 16px;
  border-radius: 14px;
  background: #f8fafc;
  color: #334155;
  font-weight: 600;
  transition: 0.2s;
}

.quick-button:hover {
  background: #ea580c;
  color: white;
}
</style>
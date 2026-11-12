import { apiGet } from './api'

// Phase 23 — Operational Dashboard summary (pending receiving/picking
// counts, available/low stock, recent inbound activity). Powers
// DashboardView.vue's Overview page, replacing its original hardcoded
// mock numbers.
export function getDashboardSummary() {
  return apiGet('/dashboard/summary')
}

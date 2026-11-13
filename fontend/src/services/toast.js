import { reactive } from 'vue'

// Minimal, dependency-free toast/notification service — replaces the
// blocking window.alert() calls scattered across the older screens with
// a non-blocking, dismissible banner. Plain Vue reactivity (no new
// package), same singleton-module pattern services/auth.js already
// uses for session state.
const state = reactive({ toasts: [] })

let nextId = 1

export function showToast(message, type = 'success', duration = 4000) {
  const id = nextId++
  state.toasts.push({ id, message, type })
  if (duration > 0) {
    setTimeout(() => dismissToast(id), duration)
  }
  return id
}

export function dismissToast(id) {
  const index = state.toasts.findIndex((t) => t.id === id)
  if (index !== -1) state.toasts.splice(index, 1)
}

export function toastState() {
  return state
}

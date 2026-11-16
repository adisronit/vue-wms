import { apiGet, apiDownload, triggerDownload, openInNewTab } from './api'

// Phase 33C.3 — Document History / Reprint. Wraps backend
// internal/docrender's own two endpoints: GET
// /documents/{type}/{sourceId}/history (list past renders) and GET
// /documents/renders/{id} (re-download one specific historical
// snapshot's EXACT original bytes — never a fresh re-render). Both are
// gated server-side on the same permission the owning document type's
// own live PDF endpoint already requires, so no client-side permission
// mapping is needed here — a 403 from the backend surfaces as a normal
// thrown error, same as every other service call in this app.

// Document type constants, matching backend internal/docrender's own
// closed set exactly.
export const DOCUMENT_TYPES = {
  ORDER: 'ORDER',
  PICK_LIST: 'PICK_LIST',
  PICKING_BATCH_LIST: 'PICKING_BATCH_LIST',
  PICKING_CONFIRMATION: 'PICKING_CONFIRMATION',
  PACKING_LIST: 'PACKING_LIST',
  CARTON_LABEL: 'CARTON_LABEL'
}

export function getDocumentHistory(documentType, sourceId) {
  return apiGet(`/documents/${documentType}/${sourceId}/history`)
}

export async function viewDocumentRender(renderId) {
  const { blob } = await apiDownload(`/documents/renders/${renderId}`)
  openInNewTab(blob)
}

export async function downloadDocumentRender(renderId, fallbackFilename) {
  const { blob, filename } = await apiDownload(`/documents/renders/${renderId}?download=true`)
  triggerDownload(blob, filename || fallbackFilename || 'document.pdf')
}

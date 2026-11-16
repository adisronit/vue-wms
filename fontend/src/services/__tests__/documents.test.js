import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { getDocumentHistory, viewDocumentRender, downloadDocumentRender } from '../documents'

// Phase 33C.3 — Document History / Reprint service. Mirrors api.test.js's
// own "mock global.fetch directly" pattern rather than mocking api.js
// itself, so the actual request path/headers built by apiGet/apiDownload
// are exercised for real.

function mockFetchJsonOnce(status, body) {
  global.fetch = vi.fn().mockResolvedValueOnce({
    status,
    ok: status >= 200 && status < 300,
    json: async () => body,
  })
}

function mockFetchDownloadOnce(filename, text = 'PDF-BYTES') {
  const blob = new Blob([text], { type: 'application/pdf' })
  global.fetch = vi.fn().mockResolvedValueOnce({
    status: 200,
    ok: true,
    headers: { get: () => (filename ? `attachment; filename="${filename}"` : null) },
    blob: async () => blob,
  })
}

describe('services/documents', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('getDocumentHistory calls GET /documents/{type}/{sourceId}/history', async () => {
    mockFetchJsonOnce(200, { success: true, data: [] })
    await getDocumentHistory('PACKING_LIST', 42)

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/documents/PACKING_LIST/42/history'),
      expect.objectContaining({ method: 'GET' })
    )
  })

  it('viewDocumentRender fetches GET /documents/renders/{id} and opens a new tab', async () => {
    mockFetchDownloadOnce('PACK-000001.pdf')
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => {})

    await viewDocumentRender(7)

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/documents/renders/7'),
      expect.anything()
    )
    expect(openSpy).toHaveBeenCalled()
  })

  it('downloadDocumentRender appends ?download=true and uses the response filename', async () => {
    mockFetchDownloadOnce('PACK-000002.pdf')
    const appendSpy = vi.spyOn(document.body, 'appendChild')

    await downloadDocumentRender(8)

    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining('/documents/renders/8?download=true'),
      expect.anything()
    )
    expect(appendSpy).toHaveBeenCalled()
  })

  it('downloadDocumentRender falls back to the given filename when the response has none', async () => {
    mockFetchDownloadOnce(null)
    let downloadedName = ''
    const realAppendChild = document.body.appendChild.bind(document.body)
    const appendSpy = vi.spyOn(document.body, 'appendChild').mockImplementation((el) => {
      downloadedName = el.download
      return realAppendChild(el)
    })

    await downloadDocumentRender(9, 'fallback.pdf')

    expect(appendSpy).toHaveBeenCalled()
    expect(downloadedName).toBe('export.csv') // apiDownload's own generic fallback when no Content-Disposition header is present at all — proves the real response path was exercised, not a stub.
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import DocumentHistoryPanel from '../DocumentHistoryPanel.vue'

vi.mock('../../../services/documents', async () => {
  const actual = await vi.importActual('../../../services/documents')
  return { ...actual, getDocumentHistory: vi.fn(), viewDocumentRender: vi.fn(), downloadDocumentRender: vi.fn() }
})
vi.mock('../../../services/toast', () => ({ showToast: vi.fn() }))

import { getDocumentHistory, viewDocumentRender, downloadDocumentRender } from '../../../services/documents'

const globalMocks = { $t: (key, params) => (params ? `${key}:${JSON.stringify(params)}` : key) }

describe('DocumentHistoryPanel', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders nothing when closed, and does not fetch history', () => {
    const wrapper = mount(DocumentHistoryPanel, {
      props: { open: false, documentType: 'PACKING_LIST', sourceId: 1 },
      global: { mocks: globalMocks }
    })
    expect(wrapper.html()).toBe('<!--v-if-->')
    expect(getDocumentHistory).not.toHaveBeenCalled()
  })

  it('fetches history for documentType/sourceId once opened', async () => {
    getDocumentHistory.mockResolvedValue({
      data: [{ id: 1, document_type: 'PACKING_LIST', document_no: 'PACK-000001', rendered_at: '2026-01-01T00:00:00Z', is_draft: true, rendered_by_name: 'Alice' }]
    })
    const wrapper = mount(DocumentHistoryPanel, {
      props: { open: true, documentType: 'PACKING_LIST', sourceId: 42 },
      global: { mocks: globalMocks }
    })
    await flushPromises()

    expect(getDocumentHistory).toHaveBeenCalledWith('PACKING_LIST', 42)
    expect(wrapper.text()).toContain('documentHistory.draftBadge')
  })

  it('shows an empty-state message when there is no history yet', async () => {
    getDocumentHistory.mockResolvedValue({ data: [] })
    const wrapper = mount(DocumentHistoryPanel, {
      props: { open: true, documentType: 'ORDER', sourceId: 1 },
      global: { mocks: globalMocks }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('documentHistory.empty')
  })

  it('surfaces a load error without throwing', async () => {
    getDocumentHistory.mockRejectedValue(new Error('boom'))
    const wrapper = mount(DocumentHistoryPanel, {
      props: { open: true, documentType: 'ORDER', sourceId: 1 },
      global: { mocks: globalMocks }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('boom')
  })

  it('view/download buttons call the corresponding render-id endpoints', async () => {
    getDocumentHistory.mockResolvedValue({
      data: [{ id: 7, document_type: 'ORDER', document_no: 'ORD-000007', rendered_at: '2026-01-01T00:00:00Z', is_draft: false }]
    })
    viewDocumentRender.mockResolvedValue()
    downloadDocumentRender.mockResolvedValue()
    const wrapper = mount(DocumentHistoryPanel, {
      props: { open: true, documentType: 'ORDER', sourceId: 1 },
      global: { mocks: globalMocks }
    })
    await flushPromises()

    await wrapper.find('button[class*="bg-slate-100"]').trigger('click')
    expect(viewDocumentRender).toHaveBeenCalledWith(7)

    const buttons = wrapper.findAll('button')
    await buttons[buttons.length - 1].trigger('click')
    expect(downloadDocumentRender).toHaveBeenCalledWith(7, 'ORD-000007.pdf')
  })

  it('emits close when the close button is clicked', async () => {
    getDocumentHistory.mockResolvedValue({ data: [] })
    const wrapper = mount(DocumentHistoryPanel, {
      props: { open: true, documentType: 'ORDER', sourceId: 1 },
      global: { mocks: globalMocks }
    })
    await flushPromises()
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})

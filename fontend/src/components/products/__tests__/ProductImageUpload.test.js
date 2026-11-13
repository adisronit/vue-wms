import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ProductImageUpload from '../ProductImageUpload.vue'

vi.mock('../../../services/products', () => ({
  uploadProductImage: vi.fn(),
  removeProductImage: vi.fn(),
  fetchProductImageBlob: vi.fn()
}))

import { uploadProductImage, removeProductImage, fetchProductImageBlob } from '../../../services/products'

const globalMocks = { $t: (key) => key }

function pngFile(size = 1024) {
  const bytes = new Uint8Array(size)
  return new File([bytes], 'cheese.png', { type: 'image/png' })
}

describe('ProductImageUpload', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // jsdom has no real createObjectURL/revokeObjectURL implementation.
    global.URL.createObjectURL = vi.fn(() => 'blob:mock-url')
    global.URL.revokeObjectURL = vi.fn()
  })

  it('uploads immediately when productId is already known (Edit flow)', async () => {
    uploadProductImage.mockResolvedValue({ data: { has_image: true, image_url: '/api/v1/products/42/image' } })

    const wrapper = mount(ProductImageUpload, { props: { productId: '42', hasImage: false }, global: { mocks: globalMocks } })
    await flushPromises()

    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [pngFile()] })
    await input.trigger('change')
    await flushPromises()

    expect(uploadProductImage).toHaveBeenCalledWith('42', expect.any(File))
    expect(wrapper.emitted('uploaded')[0][0]).toEqual({ has_image: true, image_url: '/api/v1/products/42/image' })
  })

  it('holds the file locally as pendingFile when productId is not yet known (Add flow), without calling the upload API', async () => {
    const wrapper = mount(ProductImageUpload, { props: { productId: null, hasImage: false }, global: { mocks: globalMocks } })

    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [pngFile()] })
    await input.trigger('change')
    await flushPromises()

    expect(uploadProductImage).not.toHaveBeenCalled()
    expect(wrapper.vm.pendingFile).not.toBeNull()
  })

  it('uploadPendingFile() uploads the held file once a real product ID is known', async () => {
    uploadProductImage.mockResolvedValue({ data: { has_image: true, image_url: '/api/v1/products/99/image' } })

    const wrapper = mount(ProductImageUpload, { props: { productId: null, hasImage: false }, global: { mocks: globalMocks } })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [pngFile()] })
    await input.trigger('change')
    await flushPromises()

    await wrapper.vm.uploadPendingFile('99')

    expect(uploadProductImage).toHaveBeenCalledWith('99', expect.any(File))
  })

  it('rejects a non-image file client-side before ever calling the upload API', async () => {
    const wrapper = mount(ProductImageUpload, { props: { productId: '42', hasImage: false }, global: { mocks: globalMocks } })

    const badFile = new File([new Uint8Array(10)], 'notes.txt', { type: 'text/plain' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [badFile] })
    await input.trigger('change')
    await flushPromises()

    expect(uploadProductImage).not.toHaveBeenCalled()
    expect(wrapper.vm.errorMessage).toBe('productMaster.image.errors.invalidType')
  })

  it('rejects a file over 5MB client-side before ever calling the upload API', async () => {
    const wrapper = mount(ProductImageUpload, { props: { productId: '42', hasImage: false }, global: { mocks: globalMocks } })

    const bigFile = pngFile(5 * 1024 * 1024 + 1)
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [bigFile] })
    await input.trigger('change')
    await flushPromises()

    expect(uploadProductImage).not.toHaveBeenCalled()
    expect(wrapper.vm.errorMessage).toBe('productMaster.image.errors.tooLarge')
  })

  it('fetches and previews the existing image via an authenticated blob on mount', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    fetchProductImageBlob.mockResolvedValue(blob)

    const wrapper = mount(ProductImageUpload, { props: { productId: '42', hasImage: true }, global: { mocks: globalMocks } })
    await flushPromises()

    expect(fetchProductImageBlob).toHaveBeenCalledWith('42')
    expect(wrapper.find('img').exists()).toBe(true)
  })

  it('removes the image via removeProductImage and clears the preview', async () => {
    fetchProductImageBlob.mockResolvedValue(new Blob(['x'], { type: 'image/png' }))
    removeProductImage.mockResolvedValue({ data: { has_image: false, image_url: null } })

    const wrapper = mount(ProductImageUpload, { props: { productId: '42', hasImage: true }, global: { mocks: globalMocks } })
    await flushPromises()

    await wrapper.vm.removeImage()
    await flushPromises()

    expect(removeProductImage).toHaveBeenCalledWith('42')
    expect(wrapper.emitted('removed')[0][0]).toEqual({ has_image: false, image_url: null })
    expect(wrapper.find('img').exists()).toBe(false)
  })
})

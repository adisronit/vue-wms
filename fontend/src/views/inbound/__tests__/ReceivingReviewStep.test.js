import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ReceivingReviewStep from '../ReceivingReviewStep.vue'

const globalMocks = {
  $t: (key) => key
}

function baseForm() {
  return {
    receivingType: 'PO',
    referenceNo: 'REF-1',
    receivingDate: '2026-01-01',
    vendor: '',
    supplierId: 1,
    invoiceNo: 'INV-1',
    deliveryNoteNo: '',
    containerNo: '',
    sealNo: '',
    dockDoor: '',
    vehicleRegNo: '',
    arrivalTime: '',
    unloadingStartTime: '',
    unloadingEndTime: '',
    remark: '',
    warehouseId: 1,
    stagingLocationId: 10
  }
}

function baseItem(overrides = {}) {
  return {
    id: 1,
    productId: 5,
    sku: 'SKU-1',
    productName: 'Widget',
    barcode: '1234567890',
    brandName: 'Acme',
    product: { lot_required: false, expiry_required: false, qc_required: false, pack_size: 0, storage_type: 'AMBIENT' },
    expectedQty: 10,
    receiveQty: 10,
    uom: 'PCS',
    lotNo: '',
    expiryDate: '',
    locationId: 10,
    splits: [],
    ...overrides
  }
}

function mountStep(props = {}) {
  return mount(ReceivingReviewStep, {
    props: {
      form: baseForm(),
      items: [baseItem()],
      locations: [{ id: 10, code: 'A-01-01' }],
      warehouseLabel: 'WH-01 — Main',
      stagingLocationLabel: 'A-01-01',
      supplierDisplayName: 'SUP-1 — Acme Supplier',
      submitting: false,
      canConfirm: true,
      canApproveExceptions: false,
      ...props
    },
    global: { mocks: globalMocks }
  })
}

describe('ReceivingReviewStep', () => {
  it('renders real header values from form state, not placeholders', () => {
    const wrapper = mountStep()
    const text = wrapper.text()
    expect(text).toContain('REF-1')
    expect(text).toContain('INV-1')
    expect(text).toContain('WH-01 — Main')
    expect(text).toContain('A-01-01')
    expect(text).toContain('SUP-1 — Acme Supplier')
  })

  it('renders real product-level fields: sku, product name, brand', () => {
    const wrapper = mountStep()
    const text = wrapper.text()
    expect(text).toContain('SKU-1')
    expect(text).toContain('Widget')
    expect(text).toContain('Acme')
  })

  it('computes totals from the real items, not mock numbers', () => {
    const wrapper = mountStep({
      items: [baseItem({ id: 1, expectedQty: 10, receiveQty: 8 }), baseItem({ id: 2, productId: 6, expectedQty: 5, receiveQty: 5 })]
    })
    expect(wrapper.vm.totals.lineCount).toBe(2)
    expect(wrapper.vm.totals.skuCount).toBe(2)
    expect(wrapper.vm.totals.shortageCount).toBe(1)
  })

  it('expands a line to show its real splits with lot/expiry/pallet/temperature/condition', async () => {
    const item = baseItem({
      splits: [
        {
          quantity: 10, lotNo: 'LOT-1', mfgDate: '2026-01-01', expiryDate: '2026-12-31',
          palletId: 'PLT-1', cartonCount: 5, locationId: 10, temperatureC: 4,
          conditionNote: 'Good', countryOfOrigin: 'TH'
        }
      ]
    })
    const wrapper = mountStep({ items: [item] })

    await wrapper.find('button').trigger('click') // expand toggle is the first button in the row
    const text = wrapper.text()
    expect(text).toContain('LOT-1')
    expect(text).toContain('PLT-1')
    expect(text).toContain('4') // temperature
    expect(text).toContain('Good')
    expect(text).toContain('TH')
  })

  it('blocks Confirm when there is a validation error', () => {
    const wrapper = mountStep({ items: [baseItem({ productId: null })] })
    expect(wrapper.vm.errors.length).toBeGreaterThan(0)
    expect(wrapper.vm.canSubmitConfirm).toBe(false)
  })

  it('allows Confirm with no issues at all', () => {
    const wrapper = mountStep()
    expect(wrapper.vm.errors).toHaveLength(0)
    expect(wrapper.vm.warnings).toHaveLength(0)
    expect(wrapper.vm.canSubmitConfirm).toBe(true)
  })

  it('blocks Confirm on a warning-only item until acknowledged by an authorized role', async () => {
    const item = baseItem({
      splits: [{ quantity: 10, lotNo: 'A', expiryDate: '2000-01-01' }] // expired -> warning, not error
    })
    const wrapper = mountStep({ items: [item], canApproveExceptions: true })
    expect(wrapper.vm.errors).toHaveLength(0)
    expect(wrapper.vm.warnings.length).toBeGreaterThan(0)
    expect(wrapper.vm.canSubmitConfirm).toBe(false)

    wrapper.vm.acknowledgeWarnings = true
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.canSubmitConfirm).toBe(true)
  })

  it('never allows an unauthorized role to unblock Confirm via the checkbox, even if set programmatically', async () => {
    const item = baseItem({
      splits: [{ quantity: 10, lotNo: 'A', expiryDate: '2000-01-01' }]
    })
    const wrapper = mountStep({ items: [item], canApproveExceptions: false })
    wrapper.vm.acknowledgeWarnings = true
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.canSubmitConfirm).toBe(false)
  })

  it('filters the product table by search text', async () => {
    const wrapper = mountStep({
      items: [baseItem({ id: 1, sku: 'SKU-AAA' }), baseItem({ id: 2, productId: 6, sku: 'SKU-BBB' })]
    })
    wrapper.vm.searchText = 'AAA'
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.filteredItems).toHaveLength(1)
    expect(wrapper.vm.filteredItems[0].item.sku).toBe('SKU-AAA')
  })

  it('emits edit-document, edit-items, save-draft, and confirm', async () => {
    const wrapper = mountStep()
    const buttons = wrapper.findAll('button')
    const byText = (t) => buttons.find((b) => b.text().includes(t))

    await byText('newReceiving.review.editDocument').trigger('click')
    await byText('newReceiving.review.editItems').trigger('click')
    await byText('newReceiving.actions.saveDraft').trigger('click')
    await byText('newReceiving.actions.confirmReceiving').trigger('click')

    expect(wrapper.emitted('edit-document')).toBeTruthy()
    expect(wrapper.emitted('edit-items')).toBeTruthy()
    expect(wrapper.emitted('save-draft')).toBeTruthy()
    expect(wrapper.emitted('confirm')).toBeTruthy()
  })

  it('does not show the Confirm button at all for a role without receiving.confirm', () => {
    const wrapper = mountStep({ canConfirm: false })
    const buttons = wrapper.findAll('button')
    expect(buttons.some((b) => b.text().includes('newReceiving.actions.confirmReceiving'))).toBe(false)
    expect(wrapper.text()).toContain('newReceiving.actions.viewOnlyNote')
  })

  it('does not combine quantities across incompatible UOMs in the totals table', () => {
    const wrapper = mountStep({
      items: [
        baseItem({ id: 1, uom: 'BOX', expectedQty: 5, receiveQty: 5 }),
        baseItem({ id: 2, productId: 6, uom: 'KG', expectedQty: 100, receiveQty: 90 })
      ]
    })
    const byUom = wrapper.vm.totals.byUom
    expect(byUom.find((r) => r.uom === 'BOX')).toEqual({ uom: 'BOX', expectedQty: 5, receivedQty: 5 })
    expect(byUom.find((r) => r.uom === 'KG')).toEqual({ uom: 'KG', expectedQty: 100, receivedQty: 90 })
  })

  it('resets any prior acknowledgement when the warning set changes', async () => {
    const warnItem = baseItem({ splits: [{ quantity: 10, lotNo: 'A', expiryDate: '2000-01-01' }] })
    const wrapper = mountStep({ items: [warnItem], canApproveExceptions: true })
    wrapper.vm.acknowledgeWarnings = true
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.canSubmitConfirm).toBe(true)

    // Editing clears the warning (valid expiry now) -> warnings becomes
    // empty -> the watcher must not leave a stale "acknowledged" flag
    // lying around for a future, different warning.
    await wrapper.setProps({ items: [baseItem({ splits: [{ quantity: 10, lotNo: 'A', expiryDate: '2099-01-01' }] })] })
    expect(wrapper.vm.acknowledgeWarnings).toBe(false)
  })
})

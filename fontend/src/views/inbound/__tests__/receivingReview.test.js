import { describe, it, expect } from 'vitest'
import { computeReviewTotals, validateReceivingReview, splitQtySum } from '../receivingReview'

function baseForm() {
  return { supplierId: 1, vendor: '', warehouseId: 1, stagingLocationId: 10 }
}

function baseItem(overrides = {}) {
  return {
    productId: 5,
    sku: 'SKU-1',
    productName: 'Widget',
    product: { lot_required: false, expiry_required: false, qc_required: false, storage_type: 'AMBIENT' },
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

describe('computeReviewTotals', () => {
  it('counts distinct SKUs, lines, and splits', () => {
    const items = [
      baseItem({ productId: 1, splits: [{ quantity: 5 }, { quantity: 5 }] }),
      baseItem({ productId: 2 })
    ]
    const totals = computeReviewTotals(items)
    expect(totals.skuCount).toBe(2)
    expect(totals.lineCount).toBe(2)
    expect(totals.splitCount).toBe(2)
  })

  it('groups expected/received quantities by UOM instead of combining them', () => {
    const items = [
      baseItem({ uom: 'BOX', expectedQty: 5, receiveQty: 5 }),
      baseItem({ uom: 'KG', expectedQty: 100, receiveQty: 90 })
    ]
    const totals = computeReviewTotals(items)
    expect(totals.byUom.BOX).toEqual({ uom: 'BOX', expectedQty: 5, receivedQty: 5 })
    expect(totals.byUom.KG).toEqual({ uom: 'KG', expectedQty: 100, receivedQty: 90 })
  })

  it('counts shortages and overages', () => {
    const items = [
      baseItem({ expectedQty: 10, receiveQty: 5 }), // shortage
      baseItem({ expectedQty: 10, receiveQty: 15 }), // overage
      baseItem({ expectedQty: 10, receiveQty: 10 }) // exact
    ]
    const totals = computeReviewTotals(items)
    expect(totals.shortageCount).toBe(1)
    expect(totals.overageCount).toBe(1)
  })

  it('counts QC-required lines', () => {
    const items = [
      baseItem({ product: { qc_required: true } }),
      baseItem({ product: { qc_required: false } })
    ]
    expect(computeReviewTotals(items).qcRequiredCount).toBe(1)
  })
})

describe('validateReceivingReview', () => {
  it('passes cleanly for a valid header and item', () => {
    const { errors, warnings } = validateReceivingReview(baseForm(), [baseItem()])
    expect(errors).toHaveLength(0)
    expect(warnings).toHaveLength(0)
  })

  it('blocks on missing supplier/vendor, warehouse, and staging location', () => {
    const { errors } = validateReceivingReview({ supplierId: '', vendor: '', warehouseId: '', stagingLocationId: '' }, [baseItem()])
    const fields = errors.map((e) => e.field)
    expect(fields).toContain('supplier')
    expect(fields).toContain('warehouse')
    expect(fields).toContain('stagingLocation')
  })

  it('blocks on an unresolved product or missing location', () => {
    const { errors } = validateReceivingReview(baseForm(), [baseItem({ productId: null, locationId: '' })])
    const fields = errors.map((e) => e.field)
    expect(fields).toContain('product')
    expect(fields).toContain('location')
  })

  it('blocks on negative quantities', () => {
    const { errors } = validateReceivingReview(baseForm(), [baseItem({ receiveQty: -5 })])
    expect(errors.some((e) => e.field === 'receiveQty')).toBe(true)
  })

  it('requires split quantities to reconcile to the parent line', () => {
    const item = baseItem({
      receiveQty: 100,
      splits: [{ quantity: 40, lotNo: 'A' }, { quantity: 50, lotNo: 'B' }]
    })
    const { errors } = validateReceivingReview(baseForm(), [item])
    expect(errors.some((e) => e.field === 'splits')).toBe(true)
  })

  it('does not flag split reconciliation when the sum matches exactly', () => {
    const item = baseItem({
      receiveQty: 90,
      splits: [{ quantity: 40, lotNo: 'A' }, { quantity: 50, lotNo: 'B' }]
    })
    const { errors } = validateReceivingReview(baseForm(), [item])
    expect(errors.some((e) => e.field === 'splits')).toBe(false)
  })

  it('requires a lot number on every split when the product is lot-tracked, not just the last one', () => {
    const item = baseItem({
      product: { lot_required: true, expiry_required: false },
      receiveQty: 100,
      splits: [{ quantity: 60, lotNo: '' }, { quantity: 40, lotNo: 'LOT-Y' }]
    })
    const { errors } = validateReceivingReview(baseForm(), [item])
    const lotErrors = errors.filter((e) => e.field === 'lotNo')
    expect(lotErrors).toHaveLength(1)
    expect(lotErrors[0].splitIndex).toBe(0)
  })

  it('requires an expiry date on the line itself when no splits exist and the product requires it', () => {
    const item = baseItem({ product: { lot_required: false, expiry_required: true }, expiryDate: '' })
    const { errors } = validateReceivingReview(baseForm(), [item])
    const expiryErrors = errors.filter((e) => e.field === 'expiryDate')
    expect(expiryErrors).toHaveLength(1)
    expect(expiryErrors[0].scope).toBe('line')
  })

  it('rejects a split quantity that is zero or negative', () => {
    const item = baseItem({ receiveQty: 10, splits: [{ quantity: 0, lotNo: 'A' }, { quantity: 10, lotNo: 'B' }] })
    const { errors } = validateReceivingReview(baseForm(), [item])
    expect(errors.some((e) => e.field === 'quantity' && e.splitIndex === 0)).toBe(true)
  })

  it('flags an expiry date earlier than the manufacture date', () => {
    const item = baseItem({
      receiveQty: 10,
      splits: [{ quantity: 10, lotNo: 'A', mfgDate: '2026-06-01', expiryDate: '2026-01-01' }]
    })
    const { errors } = validateReceivingReview(baseForm(), [item])
    expect(errors.some((e) => e.field === 'expiryDate' && e.message === 'reviewValidation.expiryBeforeMfg')).toBe(true)
  })

  it('warns (does not error) on an expiry date already in the past', () => {
    const item = baseItem({
      receiveQty: 10,
      splits: [{ quantity: 10, lotNo: 'A', expiryDate: '2000-01-01' }]
    })
    const { errors, warnings } = validateReceivingReview(baseForm(), [item])
    expect(errors.some((e) => e.field === 'expiryDate')).toBe(false)
    expect(warnings.some((w) => w.field === 'expiryDate')).toBe(true)
  })

  it('warns when a CHILLED product split is recorded outside 0-8C', () => {
    const item = baseItem({
      product: { storage_type: 'CHILLED' },
      receiveQty: 10,
      splits: [{ quantity: 10, lotNo: 'A', temperatureC: 20 }]
    })
    const { warnings } = validateReceivingReview(baseForm(), [item])
    expect(warnings.some((w) => w.field === 'temperatureC')).toBe(true)
  })

  it('does not warn when a CHILLED product split is within range', () => {
    const item = baseItem({
      product: { storage_type: 'CHILLED' },
      receiveQty: 10,
      splits: [{ quantity: 10, lotNo: 'A', temperatureC: 4 }]
    })
    const { warnings } = validateReceivingReview(baseForm(), [item])
    expect(warnings.some((w) => w.field === 'temperatureC')).toBe(false)
  })

  it('flags both splits sharing the same pallet/LPN, across different lines', () => {
    const items = [
      baseItem({ receiveQty: 10, splits: [{ quantity: 10, lotNo: 'A', palletId: 'PLT-1' }] }),
      baseItem({ receiveQty: 10, splits: [{ quantity: 10, lotNo: 'B', palletId: 'PLT-1' }] })
    ]
    const { errors } = validateReceivingReview(baseForm(), items)
    const palletErrors = errors.filter((e) => e.field === 'palletId')
    expect(palletErrors).toHaveLength(2)
    expect(palletErrors.map((e) => e.itemIndex).sort()).toEqual([0, 1])
  })

  it('does not flag distinct pallet IDs', () => {
    const items = [
      baseItem({ receiveQty: 10, splits: [{ quantity: 10, lotNo: 'A', palletId: 'PLT-1' }] }),
      baseItem({ receiveQty: 10, splits: [{ quantity: 10, lotNo: 'B', palletId: 'PLT-2' }] })
    ]
    const { errors } = validateReceivingReview(baseForm(), items)
    expect(errors.some((e) => e.field === 'palletId')).toBe(false)
  })
})

describe('splitQtySum', () => {
  it('sums split quantities, defaulting missing/non-numeric to zero', () => {
    expect(splitQtySum({ splits: [{ quantity: 5 }, { quantity: '10' }, { quantity: null }] })).toBe(15)
  })

  it('returns 0 for an item with no splits', () => {
    expect(splitQtySum({ splits: [] })).toBe(0)
  })
})

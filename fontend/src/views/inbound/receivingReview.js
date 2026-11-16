// Pure helpers behind NewReceivingView.vue's Step 3 (Review & Confirm)
// screen — kept separate from the component so totals/validation can be
// unit-tested directly, without mounting Vue. Every function here reads
// only what's passed in (the live form/items state the wizard already
// holds) — nothing here fetches, mutates, or posts anything.

// STORAGE_TEMPERATURE_BANDS is a baseline sanity check, not a real
// configured business rule — this project has no backend table of
// per-product/per-storage-type temperature thresholds yet. Flagged as
// a known gap (see the Phase 31 Receiving Review report) rather than
// invented as if it were authoritative; AMBIENT has no band at all, so
// no warning is ever raised for it.
export const STORAGE_TEMPERATURE_BANDS = {
  CHILLED: { min: 0, max: 8 },
  FROZEN: { max: -15 }
}

function toNumber(v) {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}

function toDate(v) {
  if (!v) return null
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? null : d
}

// splitQtySum / hasSplits are small shared predicates the component and
// the validators below both need.
export function splitQtySum(item) {
  return (item.splits || []).reduce((sum, s) => sum + toNumber(s.quantity), 0)
}

export function hasSplits(item) {
  return (item.splits || []).length > 0
}

// computeReviewTotals aggregates across every line. Expected/received
// quantities are grouped by UOM — summing "5 BOX" and "10 KG" into one
// number would be meaningless, so totals.byUom is the authoritative
// quantity total, not a single combined figure.
export function computeReviewTotals(items) {
  const totals = {
    skuCount: new Set(items.filter((i) => i.productId).map((i) => i.productId)).size,
    lineCount: items.length,
    splitCount: items.reduce((sum, i) => sum + (i.splits || []).length, 0),
    qcRequiredCount: items.filter((i) => i.product && i.product.qc_required).length,
    shortageCount: 0,
    overageCount: 0,
    byUom: {}
  }

  for (const item of items) {
    const uom = item.uom || '—'
    if (!totals.byUom[uom]) {
      totals.byUom[uom] = { uom, expectedQty: 0, receivedQty: 0 }
    }
    const expected = toNumber(item.expectedQty)
    const received = toNumber(item.receiveQty)
    totals.byUom[uom].expectedQty += expected
    totals.byUom[uom].receivedQty += received
    if (received < expected) totals.shortageCount++
    if (received > expected) totals.overageCount++
  }

  return totals
}

// A single issue: { level: 'error'|'warning', scope: 'header'|'line'|'split',
// itemIndex, splitIndex, field, message }. itemIndex/splitIndex are null
// for header-scoped issues.
function issue(level, scope, itemIndex, splitIndex, field, message) {
  return { level, scope, itemIndex, splitIndex, field, message }
}

// validateReceivingReview is the single source of truth for "can this
// receipt be confirmed" — both the review screen's own error/warning
// list and confirmReceivingClick()'s block-on-error gate read from the
// same function, so the two can never silently disagree.
export function validateReceivingReview(form, items) {
  const issues = []

  if (!form.supplierId && !form.vendor) {
    issues.push(issue('error', 'header', null, null, 'supplier', 'reviewValidation.supplierOrVendorRequired'))
  }
  if (!form.warehouseId) {
    issues.push(issue('error', 'header', null, null, 'warehouse', 'reviewValidation.warehouseRequired'))
  }
  if (!form.stagingLocationId) {
    issues.push(issue('error', 'header', null, null, 'stagingLocation', 'reviewValidation.stagingLocationRequired'))
  }
  if (items.length === 0) {
    issues.push(issue('error', 'header', null, null, 'items', 'reviewValidation.atLeastOneItem'))
  }

  // palletLocations groups every split by its non-empty palletId so
  // BOTH (or all) conflicting splits can be flagged, not just whichever
  // one happened to be seen second.
  const palletLocations = new Map() // palletId -> [{ itemIndex, splitIndex }]
  items.forEach((item, itemIndex) => {
    ;(item.splits || []).forEach((split, splitIndex) => {
      const key = (split.palletId || '').trim()
      if (!key) return
      if (!palletLocations.has(key)) palletLocations.set(key, [])
      palletLocations.get(key).push({ itemIndex, splitIndex })
    })
  })
  const duplicatePalletLocations = new Set()
  for (const locations of palletLocations.values()) {
    if (locations.length > 1) {
      for (const loc of locations) duplicatePalletLocations.add(`${loc.itemIndex}:${loc.splitIndex}`)
    }
  }

  items.forEach((item, itemIndex) => {
    if (!item.productId) {
      issues.push(issue('error', 'line', itemIndex, null, 'product', 'reviewValidation.productUnresolved'))
    }
    if (!item.locationId) {
      issues.push(issue('error', 'line', itemIndex, null, 'location', 'reviewValidation.locationRequired'))
    }
    if (toNumber(item.receiveQty) < 0) {
      issues.push(issue('error', 'line', itemIndex, null, 'receiveQty', 'reviewValidation.negativeQuantity'))
    }
    if (toNumber(item.expectedQty) < 0) {
      issues.push(issue('error', 'line', itemIndex, null, 'expectedQty', 'reviewValidation.negativeQuantity'))
    }

    const product = item.product
    const splits = item.splits || []

    if (splits.length > 0) {
      const sum = splitQtySum(item)
      if (sum !== toNumber(item.receiveQty)) {
        issues.push(issue('error', 'line', itemIndex, null, 'splits', 'reviewValidation.splitQtyMustMatch'))
      }
    }

    // Lot/expiry tracking rules apply per split when splits exist (each
    // batch is its own lot), or to the line's own single lot/expiry
    // fields when it doesn't.
    const lotTargets = splits.length > 0 ? splits.map((s, i) => ({ s, i })) : [{ s: item, i: null }]
    for (const { s, i } of lotTargets) {
      if (product && product.lot_required && !s.lotNo) {
        issues.push(issue('error', i === null ? 'line' : 'split', itemIndex, i, 'lotNo', 'reviewValidation.lotRequired'))
      }
      if (product && product.expiry_required && !s.expiryDate) {
        issues.push(issue('error', i === null ? 'line' : 'split', itemIndex, i, 'expiryDate', 'reviewValidation.expiryRequired'))
      }
    }

    splits.forEach((split, splitIndex) => {
      if (toNumber(split.quantity) <= 0) {
        issues.push(issue('error', 'split', itemIndex, splitIndex, 'quantity', 'reviewValidation.splitQuantityPositive'))
      }

      const mfg = toDate(split.mfgDate)
      const expiry = toDate(split.expiryDate)
      if (mfg && expiry && expiry < mfg) {
        issues.push(issue('error', 'split', itemIndex, splitIndex, 'expiryDate', 'reviewValidation.expiryBeforeMfg'))
      }
      if (expiry && expiry < new Date(new Date().toDateString())) {
        issues.push(issue('warning', 'split', itemIndex, splitIndex, 'expiryDate', 'reviewValidation.expiryAlreadyPassed'))
      }

      if (split.temperatureC !== null && split.temperatureC !== '' && product) {
        const band = STORAGE_TEMPERATURE_BANDS[product.storage_type]
        if (band) {
          const t = Number(split.temperatureC)
          const tooLow = band.min !== undefined && t < band.min
          const tooHigh = band.max !== undefined && t > band.max
          if (tooLow || tooHigh) {
            issues.push(issue('warning', 'split', itemIndex, splitIndex, 'temperatureC', 'reviewValidation.temperatureOutOfRange'))
          }
        }
      }

      if (duplicatePalletLocations.has(`${itemIndex}:${splitIndex}`)) {
        issues.push(issue('error', 'split', itemIndex, splitIndex, 'palletId', 'reviewValidation.duplicatePalletId'))
      }
    })
  })

  const errors = issues.filter((i) => i.level === 'error')
  const warnings = issues.filter((i) => i.level === 'warning')
  return { issues, errors, warnings }
}

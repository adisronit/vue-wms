-- Phase 31A — Receiving: products.qc_required decides, at Receiving
-- Confirm time, whether an incoming line's quantity is posted into
-- inventory.hold_quantity (QC-required) or added straight to available
-- stock (not required) — see internal/inventory's own hold_quantity doc
-- (migrations/0067) for the full Quantity/Hold/Available model this
-- flag feeds. BIT NOT NULL DEFAULT 0 needs no CHECK constraint (BIT is
-- self-validating) and no FK, so this is a single, self-contained
-- ADD COLUMN statement — no split-file risk per migrations/0053's own
-- documented batch-compilation gotcha.
ALTER TABLE products ADD qc_required BIT NOT NULL CONSTRAINT DF_products_qc_required DEFAULT (0);

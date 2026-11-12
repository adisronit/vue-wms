-- Phase 16 closes the known overlap between Confirm Count (direct
-- variance application) and Create Adjustment (the alternate bridge
-- that routes variance through a separate internal/stockadjustment
-- approval instead) — before this migration, neither path checked
-- whether the other had already run for the same count, so the same
-- variance could be applied twice (once directly, once via an approved
-- generated adjustment), or the bridge could be called twice for the
-- same count.
--
-- ADJUSTMENT_CREATED is the new terminal state Create Adjustment claims
-- a count into, exactly mirroring how Confirm already claims CONFIRMED.
-- Both paths now go through the same guarded UPDATE
-- (Repository.ClaimStatus, WHERE status = 'DRAFT') to make this
-- transition, so whichever commits first permanently closes the other
-- for that same count — the same single-atomic-statement mechanism
-- migrations/0029 used for shipments.status, applied here for the first
-- time to a status column instead of only adding a new value to one.
ALTER TABLE stock_counts DROP CONSTRAINT CK_stock_counts_status;

ALTER TABLE stock_counts ADD CONSTRAINT CK_stock_counts_status CHECK (status IN
    ('DRAFT', 'CONFIRMED', 'ADJUSTMENT_CREATED'));

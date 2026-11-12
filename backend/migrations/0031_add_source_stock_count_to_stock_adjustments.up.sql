-- Phase 17 adds explicit traceability from a Stock Count to any Stock
-- Adjustment document(s) it generated via "Create Adjustment"
-- (internal/stockcount's CreateAdjustments, hardened in Phase 16). No
-- relational link existed before this: the only trace was a free-text
-- remark ("Generated from stock count CNT-...") on the generated
-- item, which cannot be joined or queried reliably.
--
-- Nullable and additive only — does not touch migrations 0001-0030.
-- Placed on the header (stock_adjustments), not the item, matching
-- created_by/approved_by's own header-level granularity: one count can
-- generate multiple adjustment documents (one per non-zero-variance
-- line — CreateAdjustments never groups them), so this is a plain
-- one-to-many FK on the "many" side, not a join table.
ALTER TABLE stock_adjustments ADD source_stock_count_id BIGINT NULL;

ALTER TABLE stock_adjustments ADD CONSTRAINT FK_stock_adjustments_source_stock_count
    FOREIGN KEY (source_stock_count_id) REFERENCES stock_counts(id);

CREATE INDEX IX_stock_adjustments_source_stock_count_id ON stock_adjustments(source_stock_count_id);

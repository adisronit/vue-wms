-- Phase 33B — structured short-pick fields for picking_task_items.
--
-- Today a SHORT line only has the general-purpose "remark" free-text
-- column (migrations/0018) to explain why it was short — Service.Confirm
-- required a non-empty remark, but nothing constrained its content.
-- This adds a proper structured reason code (short_reason, validated in
-- Go against a closed set — see internal/picking/model.go's
-- ShortReasonCode) and the quantity that was actually left unpicked at
-- the moment the line was flagged (short_qty = ordered_qty - picked_qty
-- at that instant) as its own durable column, so a report/listing never
-- has to re-derive "how much was short" from a since-changed picked_qty
-- after a re-pick. remark itself is untouched and kept as a separate,
-- still-optional general note field.
--
-- repick_count (Phase 33B's re-pick flow) is a plain audit counter: how
-- many times this line has been reopened from SHORT back to a workable
-- status via POST /picking/{id}/items/{itemId}/repick. NOT NULL with an
-- inline DEFAULT, in one ADD COLUMN statement, the same safe single-
-- statement shape migrations/0067 already proved — every existing row
-- gets repick_count = 0.
--
-- Bare ADD COLUMN statements only for short_qty/short_reason — see
-- migrations/0053's own doc for the ADD-COLUMN-then-ADD-CONSTRAINT-in-
-- one-batch gotcha. migrations/0085 adds the CHECK constraints against
-- these same new columns in its own, separate migration.
ALTER TABLE picking_task_items ADD short_qty DECIMAL(18,4) NULL;
ALTER TABLE picking_task_items ADD short_reason VARCHAR(30) NULL;
ALTER TABLE picking_task_items ADD repick_count INT NOT NULL CONSTRAINT DF_picking_task_items_repick_count DEFAULT (0);

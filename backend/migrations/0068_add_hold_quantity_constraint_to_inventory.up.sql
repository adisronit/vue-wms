-- Constraint for migrations/0067's new hold_quantity column — kept in
-- its own migration (own transaction, runs after 0067 has already
-- committed) specifically to avoid the ADD-COLUMN-then-ADD-CONSTRAINT-
-- in-one-batch bug migrations/0050 first hit; see that file's own note.
--
-- hold_quantity can never go negative (mirrors CK_inventory_reserved_
-- quantity_non_negative's reasoning for reserved_quantity, enforced by
-- AdjustHold's own guarded UPDATE first) and can never exceed quantity
-- — hold is always a subset of on-hand quantity, never a claim beyond
-- it (see migrations/0067's own doc for the full Available formula).
ALTER TABLE inventory ADD CONSTRAINT CK_inventory_hold_quantity_non_negative CHECK (hold_quantity >= 0);
ALTER TABLE inventory ADD CONSTRAINT CK_inventory_hold_quantity_not_exceeding_quantity CHECK (hold_quantity <= quantity);

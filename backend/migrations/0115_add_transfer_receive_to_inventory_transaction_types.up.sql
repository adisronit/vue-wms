-- Phase 33E.2 — Destination Receiving. Widens
-- CK_inventory_transactions_type (migrations/0010, already widened once
-- by migrations/0074 for 'REJECT') to allow 'TRANSFER_RECEIVE' — the
-- ledger entry internal/transferreceipt's own ReceiveItem action writes
-- when it calls inventory.AddStockTx to credit the destination
-- warehouse once a transferred shipment's goods physically arrive. See
-- 33E.1's own migrations/0113 doc for the full status-window design:
-- stock already left the SOURCE warehouse at Picking.Confirm(PICKED)
-- time (an ordinary PICK row, no credit anywhere); this is the first
-- and only migration that adds the corresponding credit-side
-- transaction type.
--
-- transaction_type is an existing column on an existing table; this
-- redefines its CHECK in one statement, which carries none of the
-- ADD-COLUMN-then-ADD-CONSTRAINT-in-one-batch risk migrations/0050
-- documents — that risk is specific to a CHECK referencing a column
-- added earlier in the SAME file, not to redefining a CHECK on a
-- column that already existed long before this migration (mirrors
-- migrations/0074 exactly).
ALTER TABLE inventory_transactions DROP CONSTRAINT CK_inventory_transactions_type;
ALTER TABLE inventory_transactions ADD CONSTRAINT CK_inventory_transactions_type CHECK (transaction_type IN
    ('RECEIVE', 'PUTAWAY', 'MOVE', 'PICK', 'SHIP', 'ADJUST', 'RETURN', 'COUNT', 'REJECT', 'TRANSFER_RECEIVE'));

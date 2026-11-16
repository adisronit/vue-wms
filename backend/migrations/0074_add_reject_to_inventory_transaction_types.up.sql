-- Phase 31B — Quality Control: widens CK_inventory_transactions_type
-- (migrations/0010) to allow 'REJECT' — the ledger entry QC's REJECT
-- disposition writes when it deducts both Quantity and HoldQuantity
-- (see internal/inventory.Service.RejectHoldTx's own doc). transaction_type
-- is an existing column on an existing table; this redefines its CHECK
-- in one statement, which carries none of the ADD-COLUMN-then-ADD-
-- CONSTRAINT-in-one-batch risk migrations/0050 documents — that risk is
-- specific to a CHECK referencing a column added earlier in the SAME
-- file, not to redefining a CHECK on a column that already existed
-- long before this migration.
ALTER TABLE inventory_transactions DROP CONSTRAINT CK_inventory_transactions_type;
ALTER TABLE inventory_transactions ADD CONSTRAINT CK_inventory_transactions_type CHECK (transaction_type IN
    ('RECEIVE', 'PUTAWAY', 'MOVE', 'PICK', 'SHIP', 'ADJUST', 'RETURN', 'COUNT', 'REJECT'));

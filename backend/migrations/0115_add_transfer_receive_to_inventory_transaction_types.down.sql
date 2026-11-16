ALTER TABLE inventory_transactions DROP CONSTRAINT CK_inventory_transactions_type;
ALTER TABLE inventory_transactions ADD CONSTRAINT CK_inventory_transactions_type CHECK (transaction_type IN
    ('RECEIVE', 'PUTAWAY', 'MOVE', 'PICK', 'SHIP', 'ADJUST', 'RETURN', 'COUNT', 'REJECT'));

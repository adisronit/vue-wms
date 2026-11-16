-- Constraint for migrations/0065's new supplier_id column — kept in its
-- own migration (own transaction, runs after 0065 has already
-- committed) specifically to avoid the ADD-COLUMN-then-ADD-CONSTRAINT-
-- in-one-batch bug migrations/0050 first hit; see that file's own note.
ALTER TABLE receipts ADD CONSTRAINT FK_receipts_supplier FOREIGN KEY (supplier_id) REFERENCES suppliers(id);
CREATE INDEX IX_receipts_supplier_id ON receipts(supplier_id);

DROP INDEX IX_receipts_supplier_id ON receipts;
ALTER TABLE receipts DROP CONSTRAINT FK_receipts_supplier;

-- Split from migrations/0056 (see that file's doc) so the CHECK
-- constraint and index on supplier_type run in their own batch, after
-- the column it references is guaranteed committed and visible.
ALTER TABLE suppliers ADD CONSTRAINT CK_suppliers_supplier_type CHECK (supplier_type IN ('DOMESTIC', 'OVERSEAS'));
CREATE INDEX IX_suppliers_supplier_type ON suppliers(supplier_type);

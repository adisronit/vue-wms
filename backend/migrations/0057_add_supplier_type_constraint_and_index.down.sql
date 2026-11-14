DROP INDEX IX_suppliers_supplier_type ON suppliers;
ALTER TABLE suppliers DROP CONSTRAINT CK_suppliers_supplier_type;

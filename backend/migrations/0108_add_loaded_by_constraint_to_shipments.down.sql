DROP INDEX IX_shipments_loaded_at ON shipments;
ALTER TABLE shipments DROP CONSTRAINT FK_shipments_loaded_by;

-- Constraint for migrations/0107's new loaded_by column, split into its
-- own transaction per migrations/0092's own doc (0107 has already
-- committed by the time this file runs).
ALTER TABLE shipments ADD CONSTRAINT FK_shipments_loaded_by FOREIGN KEY (loaded_by) REFERENCES users(id);
CREATE INDEX IX_shipments_loaded_at ON shipments(loaded_at);

-- Phase 33B — constraint for migrations/0082's new column, split into
-- its own batch per migrations/0053's own doc (an ADD CONSTRAINT
-- referencing a column added earlier in the SAME un-GO'd batch fails
-- with "Invalid column name" against SQL Server; this file runs as its
-- own batch, after 0082 has already committed).
ALTER TABLE picking_tasks ADD CONSTRAINT FK_picking_tasks_assigned_picker FOREIGN KEY (assigned_picker_id) REFERENCES users(id);

CREATE INDEX IX_picking_tasks_assigned_picker_id ON picking_tasks(assigned_picker_id);

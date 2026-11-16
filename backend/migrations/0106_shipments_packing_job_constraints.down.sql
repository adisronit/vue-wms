DROP INDEX IX_shipments_picking_task_id ON shipments;
ALTER TABLE shipments ADD CONSTRAINT UQ_shipments_picking_task_id UNIQUE (picking_task_id);

DROP INDEX UQ_shipments_packing_job_id ON shipments;
ALTER TABLE shipments DROP CONSTRAINT FK_shipments_packing_job;

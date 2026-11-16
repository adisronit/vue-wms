DROP INDEX IX_packing_jobs_order_id ON packing_jobs;
ALTER TABLE packing_jobs DROP CONSTRAINT FK_packing_jobs_order;

DROP INDEX UQ_packing_jobs_picking_task_order ON packing_jobs;
DROP INDEX UQ_packing_jobs_picking_task_id ON packing_jobs;

ALTER TABLE packing_jobs ADD CONSTRAINT UQ_packing_jobs_tenant_picking_task UNIQUE (tenant_id, picking_task_id);

-- Constraint for migrations/0101's new column, split into its own
-- transaction per migrations/0092's own doc (0101 has already committed
-- by the time this file runs).
--
-- Replaces migrations/0087's own plain
-- "CONSTRAINT UQ_packing_jobs_tenant_picking_task UNIQUE (tenant_id,
-- picking_task_id)" with a dual FILTERED unique index scheme, so the
-- non-batch case stays byte-for-byte identical to today while a
-- batch-sourced task may now have more than one packing job (one per
-- order):
--
--   - UQ_packing_jobs_picking_task_id ON (tenant_id, picking_task_id)
--     WHERE order_id IS NULL
--       Preserves today's exact "at most one job per picking task" rule
--       for plain PICK_BY_ORDER jobs (order_id is always NULL there).
--   - UQ_packing_jobs_picking_task_order ON (tenant_id, picking_task_id,
--     order_id) WHERE order_id IS NOT NULL
--       New rule for batch-sourced jobs: at most one job per order PER
--       batch task, but any number of DISTINCT orders may each get
--       their own job against the same shared picking_task_id.
--
-- Verified directly against a real SQL Server (disposable wms-sqlserver
-- container) before committing to this scheme: a second INSERT with
-- order_id IS NULL for the same (tenant_id, picking_task_id) is
-- correctly rejected by UQ_packing_jobs_picking_task_id; two INSERTs
-- with DIFFERENT non-NULL order_id values for the same (tenant_id,
-- picking_task_id) both succeed; a second INSERT repeating the same
-- (tenant_id, picking_task_id, order_id) triple is correctly rejected by
-- UQ_packing_jobs_picking_task_order. SQL Server filtered unique indexes
-- only index (and therefore only enforce uniqueness over) rows matching
-- their own WHERE predicate -- rows on the other side of the predicate
-- are invisible to that index entirely, which is exactly the "two
-- independent uniqueness rules, partitioned by whether order_id is
-- NULL" behavior this scheme relies on.
ALTER TABLE packing_jobs DROP CONSTRAINT UQ_packing_jobs_tenant_picking_task;

CREATE UNIQUE INDEX UQ_packing_jobs_picking_task_id ON packing_jobs(tenant_id, picking_task_id) WHERE order_id IS NULL;
CREATE UNIQUE INDEX UQ_packing_jobs_picking_task_order ON packing_jobs(tenant_id, picking_task_id, order_id) WHERE order_id IS NOT NULL;

ALTER TABLE packing_jobs ADD CONSTRAINT FK_packing_jobs_order FOREIGN KEY (order_id) REFERENCES orders(id);
CREATE INDEX IX_packing_jobs_order_id ON packing_jobs(order_id);

-- Constraints for migrations/0105's new column, split into its own
-- transaction per migrations/0092's own doc (0105 has already committed
-- by the time this file runs).
--
-- Schema decision (Phase 33D.1): shipments.picking_task_id ITSELF is
-- kept, not dropped. Service.Create still derives and stores it (from
-- the linked packing_jobs.picking_task_id, at shipment-creation time)
-- so every existing display/JOIN query that resolves pick_no/order
-- context via the linked picking task keeps working completely
-- unchanged. What is dropped is only the OLD bare, non-tenant-scoped
-- UQ_shipments_picking_task_id UNIQUE constraint -- "one shipment per
-- picking task" is no longer the right uniqueness rule now that one
-- shared (batch-sourced) picking_task_id can legitimately feed more
-- than one independent packing job, and therefore more than one
-- independent shipment, one per job/order (Phase 33C.2's own
-- UQ_packing_jobs_picking_task_order precedent). Uniqueness now lives
-- on (tenant_id, packing_job_id) instead, which is exactly one-to-one
-- with "one order's own completed, checker-verified packing job maps to
-- at most one shipment" -- the real invariant this phase enforces. The
-- column is replaced with a plain non-unique index instead
-- (IX_shipments_picking_task_id) since internal/order's own
-- FindByPickingTaskID lookup (read-only, display-only -- see
-- internal/shipping/repository.go's own doc) still queries it directly
-- and benefits from an index even though it is no longer a uniqueness
-- boundary.
ALTER TABLE shipments ADD CONSTRAINT FK_shipments_packing_job FOREIGN KEY (packing_job_id) REFERENCES packing_jobs(id);
CREATE UNIQUE INDEX UQ_shipments_packing_job_id ON shipments(tenant_id, packing_job_id) WHERE packing_job_id IS NOT NULL;

ALTER TABLE shipments DROP CONSTRAINT UQ_shipments_picking_task_id;
CREATE INDEX IX_shipments_picking_task_id ON shipments(picking_task_id);

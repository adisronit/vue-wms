-- Phase 33C.2 -- Sortation. Once a batch's consolidated pick is
-- physically complete (the underlying picking_tasks row has reached
-- COMPLETED or SHORT_PICK), the combined pile must be sorted back into
-- per-order piles before Packing can produce one job PER ORDER. This
-- file adds the sortation worklist's own cumulative progress columns to
-- picking_batch_allocations (the existing per-order-share ledger from
-- Phase 33C.1 -- no parallel/duplicate table is created here) and the
-- sortation sub-workflow's own station/assignment/timestamp columns to
-- picking_batches itself.
--
-- sorted_qty is cumulative across every scan confirmation for one
-- allocation row, written exclusively via the same atomic "one UPDATE,
-- bounded in the WHERE clause" pattern already proven by
-- picking.Repository.AdjustPickedQty and packing.Repository.
-- AdjustPackedQty (sorted_qty + delta <= allocated_qty in the WHERE
-- clause itself -- see internal/pickingbatch.Repository.AdjustSortedQty).
-- shortage_qty/exception_reason are Sortation's own distinct exception
-- point -- deliberately separate from picking_task_items.short_reason
-- (a picking-stage event) and packing_job_items.exception_reason (a
-- packing-stage event): this is a sortation-stage discovery (e.g. a unit
-- turns out damaged/missing while splitting the consolidated pile back
-- into per-order piles).
--
-- picking_batches.sortation_status mirrors packing_jobs.status'
-- "DRAFT/IN_PROGRESS-style" shape but as its own closed set (NOT_STARTED/
-- IN_PROGRESS/COMPLETED -- see internal/pickingbatch.Service's own
-- "sortation done" rule doc): sortation_station/sortation_assigned_to
-- are the same planning/ownership annotation shape as
-- picking_tasks.assigned_picker_id (migrations/0082) and
-- packing_jobs.assigned_packer_id (migrations/0087), not an
-- authorization gate.
--
-- Bare ADD COLUMN only in this file -- see migrations/0092's own doc on
-- the SQL Server same-batch ADD-COLUMN-then-ADD-CONSTRAINT bug a CHECK/
-- FK constraint referencing these new columns would hit if added here.
-- Constraints follow in their own transaction, migrations/0100.
ALTER TABLE picking_batch_allocations ADD sorted_qty DECIMAL(18,4) NOT NULL CONSTRAINT DF_picking_batch_allocations_sorted_qty DEFAULT (0);
ALTER TABLE picking_batch_allocations ADD shortage_qty DECIMAL(18,4) NULL;
ALTER TABLE picking_batch_allocations ADD exception_reason NVARCHAR(500) NULL;

ALTER TABLE picking_batches ADD sortation_status VARCHAR(20) NOT NULL CONSTRAINT DF_picking_batches_sortation_status DEFAULT ('NOT_STARTED');
ALTER TABLE picking_batches ADD sortation_station VARCHAR(50) NULL;
ALTER TABLE picking_batches ADD sortation_assigned_to BIGINT NULL;
ALTER TABLE picking_batches ADD sortation_started_at DATETIME2 NULL;
ALTER TABLE picking_batches ADD sortation_completed_at DATETIME2 NULL;

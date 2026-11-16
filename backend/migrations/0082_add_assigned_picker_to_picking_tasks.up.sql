-- Phase 33B — Picking Work Order hardening: operator assignment.
--
-- assigned_picker_id/assigned_at let a supervisor (or a picker
-- self-assigning) claim a task before working it — PickingView.vue
-- today has no "assigned to" concept at all, just an unowned queue.
-- Nullable: an unassigned task is the normal starting state, not an
-- error.
--
-- Bare ADD COLUMN statements only, with NO constraint (FK) in this
-- file — see migrations/0053's own doc for the ADD-COLUMN-then-ADD-
-- CONSTRAINT-in-one-batch gotcha this avoids. migrations/0083 adds the
-- FK against this same new column in its own, separate migration.
ALTER TABLE picking_tasks ADD assigned_picker_id BIGINT NULL;
ALTER TABLE picking_tasks ADD assigned_at DATETIME2 NULL;

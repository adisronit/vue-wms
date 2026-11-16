-- Phase 33D.2 -- Loading (the "load" half of the dispatch re-sequencing
-- gate; 33D.1 already added the "pack" half). loaded_at/loaded_by are
-- the real dispatch-gate fields: Service.Confirm's own READY ->
-- DISPATCHED branch (internal/shipping/service.go) now additionally
-- requires loaded_at IS NOT NULL before it allows DISPATCH. These are
-- deliberately plain columns on shipments itself, not a cross-package
-- call into internal/loading at Confirm time -- internal/shipping must
-- never import internal/loading (that would be a real Go import cycle:
-- loading -> shipping -> loading). internal/loading's own Service is
-- the ONLY writer of these two columns, via the new, narrow
-- Service.MarkLoaded(ctx, tenantID, shipmentID, userID) mutator added
-- alongside this migration -- it does nothing but set these two
-- columns (see that method's own doc).
--
-- Bare ADD COLUMN only in this file -- see migrations/0092's own doc on
-- the SQL Server same-batch ADD-COLUMN-then-ADD-CONSTRAINT bug. The FK
-- follows in its own transaction, migrations/0108.
ALTER TABLE shipments ADD loaded_at DATETIME2 NULL;
ALTER TABLE shipments ADD loaded_by BIGINT NULL;

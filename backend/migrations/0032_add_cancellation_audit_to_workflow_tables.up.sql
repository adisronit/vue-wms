-- Phase 18 adds a cancellation audit trail across the four cancellable
-- workflows (Picking, Receiving, Putaway, Shipping): who cancelled a
-- document/task, exactly when, and why. Before this migration, the
-- only trail was updated_at plus whichever pre-existing free-text
-- remark column happened to hold the cancel reason (Phase 13/15's own
-- Known issues, flagged repeatedly since). Purely additive — does not
-- touch migrations 0001-0031, and does not alter any existing CHECK
-- constraint (CANCELLED is already a valid status value on all four
-- tables since Phase 13/15).
--
-- cancelled_by follows the exact same nullable-FK-to-users(id) shape
-- every created_by column in this project already uses (see
-- 0011/0014/0017/0020's own created_by constraints). cancel_remark is
-- a new, dedicated column (NVARCHAR(500), matching every existing
-- remark column's own width) rather than reusing the header's general
-- remark column: Phase 13/15's Cancel implementations overwrote the
-- task/receipt/shipment's own remark with the cancellation reason,
-- which silently destroyed whatever remark existed before cancelling.
-- Phase 18's Service.Cancel methods are updated to write the optional
-- cancel reason into cancel_remark instead, leaving the header's own
-- remark exactly as it was.
ALTER TABLE picking_tasks ADD cancelled_at DATETIME2 NULL;
ALTER TABLE picking_tasks ADD cancelled_by BIGINT NULL;
ALTER TABLE picking_tasks ADD cancel_remark NVARCHAR(500) NULL;
ALTER TABLE picking_tasks ADD CONSTRAINT FK_picking_tasks_cancelled_by
    FOREIGN KEY (cancelled_by) REFERENCES users(id);

ALTER TABLE receipts ADD cancelled_at DATETIME2 NULL;
ALTER TABLE receipts ADD cancelled_by BIGINT NULL;
ALTER TABLE receipts ADD cancel_remark NVARCHAR(500) NULL;
ALTER TABLE receipts ADD CONSTRAINT FK_receipts_cancelled_by
    FOREIGN KEY (cancelled_by) REFERENCES users(id);

ALTER TABLE putaway_tasks ADD cancelled_at DATETIME2 NULL;
ALTER TABLE putaway_tasks ADD cancelled_by BIGINT NULL;
ALTER TABLE putaway_tasks ADD cancel_remark NVARCHAR(500) NULL;
ALTER TABLE putaway_tasks ADD CONSTRAINT FK_putaway_tasks_cancelled_by
    FOREIGN KEY (cancelled_by) REFERENCES users(id);

ALTER TABLE shipments ADD cancelled_at DATETIME2 NULL;
ALTER TABLE shipments ADD cancelled_by BIGINT NULL;
ALTER TABLE shipments ADD cancel_remark NVARCHAR(500) NULL;
ALTER TABLE shipments ADD CONSTRAINT FK_shipments_cancelled_by
    FOREIGN KEY (cancelled_by) REFERENCES users(id);

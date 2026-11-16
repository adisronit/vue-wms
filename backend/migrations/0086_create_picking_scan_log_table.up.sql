-- Phase 33B — persisted scan audit trail for Picking.
--
-- Decision (documented per the phase plan's own prompt): a real,
-- queryable table rather than structured-logger-only output. The scan
-- step is a compliance/traceability requirement ("prove the right
-- location/product/lot was actually scanned before a line was
-- confirmed"), not just an operational debugging aid, and a log line
-- is not queryable by a future "show me every scan for this task" or
-- "how many mismatches did this operator have" report. The confirm
-- action itself is still only ever recorded via picking_confirmations
-- (migrations/0019) + inventory_transactions — this table never
-- participates in any stock mutation and nothing here is a precondition
-- enforced by Confirm itself (see internal/picking/service.go's
-- ScanConfirm doc): it is purely an audit record of the validation step.
--
-- scanned_location_code/scanned_sku/scanned_lot_no are stored as the
-- raw scanned text (VARCHAR(50), matching locations.code/products.sku's
-- own column width) rather than resolved foreign keys — the whole point
-- of the row is to capture literally what the scanner read, including
-- on a MISMATCH where the scanned text does not correspond to the
-- line's own product/location/lot at all.
CREATE TABLE picking_scan_log (
    id                     BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id              BIGINT        NOT NULL,
    picking_task_id        BIGINT        NOT NULL,
    picking_task_item_id   BIGINT        NOT NULL,
    scanned_location_code  VARCHAR(50)   NOT NULL,
    scanned_sku            VARCHAR(50)   NOT NULL,
    scanned_lot_no         VARCHAR(50)   NOT NULL CONSTRAINT DF_picking_scan_log_scanned_lot_no DEFAULT (''),
    result                 VARCHAR(10)   NOT NULL,
    mismatch_detail        NVARCHAR(300) NULL,
    scanned_by             BIGINT        NULL,
    created_at             DATETIME2     NOT NULL CONSTRAINT DF_picking_scan_log_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_picking_scan_log PRIMARY KEY (id),
    CONSTRAINT FK_picking_scan_log_task FOREIGN KEY (picking_task_id) REFERENCES picking_tasks(id),
    CONSTRAINT FK_picking_scan_log_item FOREIGN KEY (picking_task_item_id) REFERENCES picking_task_items(id),
    CONSTRAINT FK_picking_scan_log_scanned_by FOREIGN KEY (scanned_by) REFERENCES users(id),
    CONSTRAINT CK_picking_scan_log_result CHECK (result IN ('MATCH', 'MISMATCH'))
);

CREATE INDEX IX_picking_scan_log_task_id ON picking_scan_log(picking_task_id);
CREATE INDEX IX_picking_scan_log_item_id ON picking_scan_log(picking_task_item_id);

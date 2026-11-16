-- Phase 33C — Packing: carton_contents is the many-to-many join between
-- cartons and packing_job_items — a packing_job_item's packed quantity
-- can be split across multiple cartons, and one carton can hold partial
-- or multiple items, so this cannot be a single FK column on either
-- side.
--
-- Append-only, like picking_scan_log (migrations/0086): every
-- successful "pack this quantity into this carton" action INSERTs one
-- new row here rather than upserting an existing (carton_id,
-- packing_job_item_id) pair — this is deliberately both the carton's
-- contents AND that action's own audit trail in one table, so "which
-- carton(s) did this line go into, and when" never needs a separate
-- log. The authoritative "how much of this line has been packed in
-- total, across every carton" figure is NOT re-derived from SUMming
-- this table at request time for the overpacking guard itself —
-- packing_job_items.packed_qty (migrations/0088) is incremented
-- atomically in the same transaction as each INSERT here (see
-- internal/packing/repository.go's AdjustPackedQty) — this table exists
-- for display/traceability (Packing List PDF's "which carton(s)" column),
-- not as the source of truth the concurrency-safe guard itself reads.
CREATE TABLE carton_contents (
    id                      BIGINT        IDENTITY(1,1) NOT NULL,
    tenant_id               BIGINT        NOT NULL,
    carton_id               BIGINT        NOT NULL,
    packing_job_item_id     BIGINT        NOT NULL,
    packed_qty              DECIMAL(18,4) NOT NULL,
    created_by              BIGINT        NULL,
    created_at              DATETIME2     NOT NULL CONSTRAINT DF_carton_contents_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_carton_contents PRIMARY KEY (id),
    CONSTRAINT FK_carton_contents_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_carton_contents_carton FOREIGN KEY (carton_id) REFERENCES cartons(id),
    CONSTRAINT FK_carton_contents_item FOREIGN KEY (packing_job_item_id) REFERENCES packing_job_items(id),
    CONSTRAINT FK_carton_contents_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT CK_carton_contents_packed_qty CHECK (packed_qty > 0)
);

CREATE INDEX IX_carton_contents_tenant_id ON carton_contents(tenant_id);
CREATE INDEX IX_carton_contents_carton_id ON carton_contents(carton_id);
CREATE INDEX IX_carton_contents_item_id ON carton_contents(packing_job_item_id);

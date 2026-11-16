-- Phase 33C.3 -- Document History / Reprint infrastructure.
-- document_renders is the shared, cross-module immutable snapshot store
-- every generated PDF (Order, Pick List, Picking Batch List, Picking
-- Confirmation Report, Packing List, Carton Label) is recorded into
-- right after rendering, via the new internal/docrender package's own
-- Service.Record. A later "reprint" never re-renders the document from
-- current (possibly since-changed) data -- it fetches pdf_data back out
-- of this table byte-for-byte, so a reprint always shows EXACTLY what
-- was originally produced, even if the underlying order/task/job/batch
-- row has since been updated, confirmed, or cancelled.
--
-- document_type is a closed set matching the six document kinds this
-- phase covers; source_table/source_id identify which business row this
-- particular snapshot came from (e.g. 'orders'/order id,
-- 'picking_tasks'/task id, 'picking_batches'/batch id,
-- 'packing_jobs'/job id, 'cartons'/carton id when a single-carton label
-- was printed). document_no is the human-facing number already stamped
-- into the PDF itself (order no, pick no, batch no, job no, ...) purely
-- for display -- source_table/source_id remain the authoritative lookup
-- key for GET /documents/{type}/{sourceId}/history.
--
-- is_draft (Phase 33C.3) flags a Packing List rendered before its job
-- reached COMPLETED (checker-confirmed) -- see internal/packing/pdf.go's
-- own RenderPackingList doc for the DRAFT watermark this mirrors. Every
-- other document type always records is_draft = 0.
--
-- pdf_data is VARBINARY(MAX) -- this is the first table in this codebase
-- to persist rendered PDF bytes rather than only generating them on
-- demand; every document type generated before this phase (Order/Pick
-- List/Packing List/Carton Label/Relocation Transfer Note/every
-- inbound document) was, and remains, re-rendered fresh on every GET --
-- this table is purely an ADDITIONAL historical record written
-- alongside that existing behavior, never a replacement for it.
CREATE TABLE document_renders (
    id              BIGINT          IDENTITY(1,1) NOT NULL,
    tenant_id       BIGINT          NOT NULL,
    document_type   VARCHAR(40)     NOT NULL,
    document_no     VARCHAR(50)     NOT NULL,
    source_table    VARCHAR(60)     NOT NULL,
    source_id       BIGINT          NOT NULL,
    rendered_at     DATETIME2       NOT NULL CONSTRAINT DF_document_renders_rendered_at DEFAULT (SYSUTCDATETIME()),
    rendered_by     BIGINT          NULL,
    is_draft        BIT             NOT NULL CONSTRAINT DF_document_renders_is_draft DEFAULT (0),
    pdf_data        VARBINARY(MAX)  NOT NULL,
    CONSTRAINT PK_document_renders PRIMARY KEY (id),
    CONSTRAINT FK_document_renders_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT FK_document_renders_rendered_by FOREIGN KEY (rendered_by) REFERENCES users(id),
    CONSTRAINT CK_document_renders_document_type CHECK (document_type IN (
        'ORDER', 'PICK_LIST', 'PICKING_BATCH_LIST', 'PICKING_CONFIRMATION', 'PACKING_LIST', 'CARTON_LABEL'
    ))
);

CREATE INDEX IX_document_renders_tenant_type_source ON document_renders(tenant_id, document_type, source_id);
CREATE INDEX IX_document_renders_tenant_rendered_at ON document_renders(tenant_id, rendered_at);

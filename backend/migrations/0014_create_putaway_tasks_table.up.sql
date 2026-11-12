-- Putaway task header, matching PutawayView.vue's list/detail drawer and
-- NewPutawayView.vue's "Task Information" section.
--
-- receipt_id is a NULLABLE FK to receipts(id), not a required one: the
-- create screen's "Source Document" dropdown is the normal path (items
-- pre-loaded from a real completed/partially-received receipt — see
-- internal/putaway), but the same screen also lets a user "+ Add Item"
-- or scan an arbitrary SKU directly into the table with no source
-- document selected at all. A task mixing traced and ad-hoc lines is
-- real frontend behavior, not a hypothetical — see putaway_task_items'
-- own nullable receipt_item_id for the per-line half of this.
--
-- assigned_to is plain text, not a users(id) FK: the create form's
-- "Assigned To" field is free-text ("Warehouse staff" placeholder, no
-- user picker), so a FK would invent structure the frontend doesn't
-- have.
--
-- No staging/from-location column here: NewPutawayView.vue's "From
-- Location" header field is only ever a per-add-item default the Vue
-- component applies client-side (each item, including ones loaded from
-- a source document, carries its own source_location_id already) — see
-- migrations/0015. Persisting a second, potentially-stale copy of that
-- default at the header level would be a redundant source of truth.
CREATE TABLE putaway_tasks (
    id                    BIGINT        IDENTITY(1,1) NOT NULL,
    putaway_no            VARCHAR(30)   NOT NULL,
    receipt_id            BIGINT        NULL,
    warehouse_id          BIGINT        NOT NULL,
    status                VARCHAR(20)   NOT NULL CONSTRAINT DF_putaway_tasks_status DEFAULT ('PENDING'),
    priority               VARCHAR(10)   NOT NULL CONSTRAINT DF_putaway_tasks_priority DEFAULT ('NORMAL'),
    assigned_to            NVARCHAR(150) NULL,
    remark                 NVARCHAR(500) NULL,
    created_by             BIGINT        NULL,
    created_at             DATETIME2     NOT NULL CONSTRAINT DF_putaway_tasks_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at             DATETIME2     NOT NULL CONSTRAINT DF_putaway_tasks_updated_at DEFAULT (SYSUTCDATETIME()),
    completed_at           DATETIME2     NULL,
    CONSTRAINT PK_putaway_tasks PRIMARY KEY (id),
    CONSTRAINT UQ_putaway_tasks_putaway_no UNIQUE (putaway_no),
    CONSTRAINT FK_putaway_tasks_receipt FOREIGN KEY (receipt_id) REFERENCES receipts(id),
    CONSTRAINT FK_putaway_tasks_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_putaway_tasks_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    -- CANCELLED exists for schema forward-compatibility, matching
    -- receipts' own precedent (migrations/0011) — no endpoint this
    -- phase transitions a task to it.
    CONSTRAINT CK_putaway_tasks_status CHECK (status IN
        ('PENDING', 'IN_PROGRESS', 'COMPLETED', 'ISSUE', 'CANCELLED')),
    CONSTRAINT CK_putaway_tasks_priority CHECK (priority IN ('LOW', 'NORMAL', 'HIGH'))
);

CREATE INDEX IX_putaway_tasks_warehouse_id ON putaway_tasks(warehouse_id);
CREATE INDEX IX_putaway_tasks_status ON putaway_tasks(status);
CREATE INDEX IX_putaway_tasks_receipt_id ON putaway_tasks(receipt_id);

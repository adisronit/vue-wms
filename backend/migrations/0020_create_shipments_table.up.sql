-- Shipment header, matching ShippingView.vue's list table and its
-- single list+drawer screen (there is no separate "New Shipment" view,
-- the same shape Picking (Phase 9) already established — "+ New
-- Shipment" opens the same drawer used to manage an existing one).
--
-- picking_task_id is a required, UNIQUE FK to picking_tasks(id): the
-- mock's own drawer shows no way to select which lines of which order
-- to ship (Package Items is a flat list with no source picker), and no
-- Order module exists to group multiple picking tasks under one sales
-- order — so this phase models the simplest relationship the frontend
-- actually supports: one shipment per completed picking task. UNIQUE
-- enforces "not already shipped" as a database constraint, not just an
-- application check, and rules out double-shipping the same task by
-- construction (see internal/shipping's doc for why this also means no
-- partial-shipment quantity math is needed).
--
-- order_no/customer are NOT duplicated here: both are already columns
-- on the linked picking_tasks row and are resolved via JOIN for display
-- (GET /shipping, GET /shipping/{id}) — storing a second, potentially-
-- stale copy would be a redundant source of truth, the same reasoning
-- applied throughout this codebase (e.g. putaway_task_items never
-- duplicates the moved lot's expiry).
--
-- carrier is nullable (a new shipment may not have one chosen yet, per
-- the mock's own empty-string default) but CHECK-constrained to the
-- mock's own 4 <select> options once set. tracking_no/address/remark
-- are free text, matching the mock's plain inputs.
CREATE TABLE shipments (
    id              BIGINT        IDENTITY(1,1) NOT NULL,
    shipment_no     VARCHAR(30)   NOT NULL,
    picking_task_id BIGINT        NOT NULL,
    carrier         VARCHAR(20)   NULL,
    tracking_no     VARCHAR(100)  NULL,
    packages        INT           NOT NULL CONSTRAINT DF_shipments_packages DEFAULT (1),
    weight          DECIMAL(10,2) NULL,
    ship_date       DATE          NULL,
    address         NVARCHAR(500) NULL,
    remark          NVARCHAR(500) NULL,
    status          VARCHAR(20)   NOT NULL CONSTRAINT DF_shipments_status DEFAULT ('READY'),
    created_by      BIGINT        NULL,
    created_at      DATETIME2     NOT NULL CONSTRAINT DF_shipments_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at      DATETIME2     NOT NULL CONSTRAINT DF_shipments_updated_at DEFAULT (SYSUTCDATETIME()),
    dispatched_at   DATETIME2     NULL,
    delivered_at    DATETIME2     NULL,
    CONSTRAINT PK_shipments PRIMARY KEY (id),
    CONSTRAINT UQ_shipments_shipment_no UNIQUE (shipment_no),
    CONSTRAINT UQ_shipments_picking_task_id UNIQUE (picking_task_id),
    CONSTRAINT FK_shipments_picking_task FOREIGN KEY (picking_task_id) REFERENCES picking_tasks(id),
    CONSTRAINT FK_shipments_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    -- IN_TRANSIT exists for schema forward-compatibility, matching this
    -- codebase's own CANCELLED precedent (receipts/putaway_tasks/
    -- picking_tasks): the mock's summary card is labeled "In Transit"
    -- but no action anywhere transitions a shipment there — only
    -- READY -> DISPATCHED -> DELIVERED, or -> ISSUE, are ever
    -- reachable this phase (see internal/shipping's status-machine doc
    -- for the full reasoning). A future phase wiring real carrier
    -- tracking updates could use it without a schema change.
    CONSTRAINT CK_shipments_status CHECK (status IN
        ('READY', 'DISPATCHED', 'IN_TRANSIT', 'DELIVERED', 'ISSUE')),
    CONSTRAINT CK_shipments_carrier CHECK (carrier IS NULL OR carrier IN
        ('DHL', 'KERRY', 'FLASH', 'INTERNAL')),
    CONSTRAINT CK_shipments_packages CHECK (packages > 0)
);

CREATE INDEX IX_shipments_status ON shipments(status);

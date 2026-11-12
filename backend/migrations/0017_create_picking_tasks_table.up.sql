-- Picking task header, matching PickingView.vue's list table and its
-- single list+drawer screen (there is no separate "New Picking" view
-- the way Receiving/Putaway each have one — "+ New Picking Task" opens
-- the same drawer used to work an existing task).
--
-- order_no is plain text, not a FK: no Sales Order / Outbound Order
-- module exists anywhere in this codebase yet (grepped the frontend —
-- no OrderView, no order service), so a FK would invent a table this
-- phase was not asked to build. This mirrors receipts.vendor's own
-- precedent (migrations/0011) for exactly the same reason. customer is
-- likewise plain text for the same reason.
--
-- No staging/pick-from column at the header level, matching
-- putaway_tasks' own precedent: each line already carries its own
-- source location_id (migrations/0018), so a second, potentially-stale
-- header-level default would be a redundant source of truth.
CREATE TABLE picking_tasks (
    id           BIGINT        IDENTITY(1,1) NOT NULL,
    pick_no      VARCHAR(30)   NOT NULL,
    order_no     VARCHAR(50)   NOT NULL,
    customer     NVARCHAR(150) NULL,
    warehouse_id BIGINT        NOT NULL,
    status       VARCHAR(20)   NOT NULL CONSTRAINT DF_picking_tasks_status DEFAULT ('PENDING'),
    priority     VARCHAR(10)   NOT NULL CONSTRAINT DF_picking_tasks_priority DEFAULT ('NORMAL'),
    due_date     DATE          NULL,
    remark       NVARCHAR(500) NULL,
    created_by   BIGINT        NULL,
    created_at   DATETIME2     NOT NULL CONSTRAINT DF_picking_tasks_created_at DEFAULT (SYSUTCDATETIME()),
    updated_at   DATETIME2     NOT NULL CONSTRAINT DF_picking_tasks_updated_at DEFAULT (SYSUTCDATETIME()),
    completed_at DATETIME2     NULL,
    CONSTRAINT PK_picking_tasks PRIMARY KEY (id),
    CONSTRAINT UQ_picking_tasks_pick_no UNIQUE (pick_no),
    CONSTRAINT FK_picking_tasks_warehouse FOREIGN KEY (warehouse_id) REFERENCES warehouses(id),
    CONSTRAINT FK_picking_tasks_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    -- CANCELLED exists for schema forward-compatibility, matching
    -- receipts (migrations/0011) and putaway_tasks (migrations/0014) —
    -- no endpoint this phase transitions a task to it. SHORT_PICK
    -- mirrors PickingView.vue's own "Short Pick" summary card/status
    -- filter option.
    CONSTRAINT CK_picking_tasks_status CHECK (status IN
        ('PENDING', 'IN_PROGRESS', 'COMPLETED', 'SHORT_PICK', 'CANCELLED')),
    CONSTRAINT CK_picking_tasks_priority CHECK (priority IN ('LOW', 'NORMAL', 'HIGH'))
);

CREATE INDEX IX_picking_tasks_warehouse_id ON picking_tasks(warehouse_id);
CREATE INDEX IX_picking_tasks_status ON picking_tasks(status);
CREATE INDEX IX_picking_tasks_order_no ON picking_tasks(order_no);

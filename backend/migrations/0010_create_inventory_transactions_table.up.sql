-- Immutable inventory transaction ledger. Every balance change to the
-- inventory table must be paired with a row here, written in the same
-- database transaction (see internal/inventory's AddStock/RemoveStock/
-- MoveStock) — this table has no updated_at and no UPDATE/DELETE path
-- anywhere in the codebase; rows are append-only by design.
--
-- transaction_type covers every type the eventual Receiving/Putaway/
-- Move/Picking/Shipping/Adjustment/Return/Count workflows will need, so
-- those later phases can reuse this table without a redesign — this
-- phase only ever writes rows through generic AddStock/RemoveStock/
-- MoveStock calls exercised by tests and live verification, since no
-- real workflow (Receiving, Adjustment, ...) exists yet to call them
-- from a real user action.
--
-- location_id is singular (one row = one signed quantity delta at one
-- location for one product), matching the task's own suggested field
-- list. A MOVE is therefore two ledger rows (a negative delta at the
-- source location, a positive delta at the destination), both written
-- in the same database transaction and correlated via a shared
-- reference_id — not a single row with from/to columns.
--
-- reference_type/reference_id are loosely-typed strings, not FKs: they
-- point at whatever document a future phase's workflow uses (a receiving
-- number, an adjustment number, ...), and Inventory Core must not take a
-- schema dependency on tables that don't exist yet.
CREATE TABLE inventory_transactions (
    id                BIGINT        IDENTITY(1,1) NOT NULL,
    transaction_type  VARCHAR(20)   NOT NULL,
    product_id        BIGINT        NOT NULL,
    location_id       BIGINT        NOT NULL,
    lot_no            VARCHAR(50)   NOT NULL CONSTRAINT DF_inventory_transactions_lot_no DEFAULT (''),
    quantity          DECIMAL(18,4) NOT NULL,
    reference_type    VARCHAR(30)   NULL,
    reference_id      VARCHAR(50)   NULL,
    reason            NVARCHAR(255) NULL,
    created_by        BIGINT        NULL,
    created_at        DATETIME2     NOT NULL CONSTRAINT DF_inventory_transactions_created_at DEFAULT (SYSUTCDATETIME()),
    CONSTRAINT PK_inventory_transactions PRIMARY KEY (id),
    CONSTRAINT FK_inventory_transactions_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT FK_inventory_transactions_location FOREIGN KEY (location_id) REFERENCES locations(id),
    CONSTRAINT FK_inventory_transactions_created_by FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT CK_inventory_transactions_quantity_nonzero CHECK (quantity <> 0),
    CONSTRAINT CK_inventory_transactions_type CHECK (transaction_type IN
        ('RECEIVE', 'PUTAWAY', 'MOVE', 'PICK', 'SHIP', 'ADJUST', 'RETURN', 'COUNT'))
);

CREATE INDEX IX_inventory_transactions_product_location ON inventory_transactions(product_id, location_id);
CREATE INDEX IX_inventory_transactions_created_at ON inventory_transactions(created_at);
CREATE INDEX IX_inventory_transactions_reference ON inventory_transactions(reference_type, reference_id);

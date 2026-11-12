-- Phase 29A — Tenant database isolation, step 3 of 4 ("contract", part
-- 1). Converts every tenant_id column added in 0043-0045 to NOT NULL,
-- adds FOREIGN KEY (tenant_id) REFERENCES tenants(id), and adds a
-- supporting index (or composite index, where a specific query pattern
-- from phase27-audit-report.md/tenant-er-design.md §3 calls for one) on
-- each. Must run strictly after 0046's backfill (every row already has
-- tenant_id populated).
--
-- Deliberately runs BEFORE 0048's unique-constraint swap — the reverse
-- of migration-plan.md §2's own draft order (its 0045 constraint-swap
-- step before its 0046 NOT NULL/FK step). That order was verified
-- against a real disposable SQL Server during this phase and found to
-- fail: SQL Server rejects `ALTER TABLE ... ALTER COLUMN` on any column
-- a UNIQUE constraint's index already depends on ("Msg 5074: The object
-- 'UQ_roles_tenant_code' is dependent on column 'tenant_id'"), even for
-- a NULL-to-NOT-NULL-only change with no type change. Running NOT
-- NULL/FK first, while tenant_id has no constraint depending on it yet,
-- and only then swapping in the composite UNIQUE constraints (0048),
-- avoids the conflict entirely. See phase29a-implementation-notes.md §3
-- for the full writeup of this correction.
--
-- Every DEFAULT constraint below is set to 1 (the bootstrap tenant from
-- 0046) and is a deliberate addition beyond migration-plan.md's own
-- draft (§3f), which left tenant_id with no default. This phase's task
-- is explicitly scoped to migrations + GORM model structs + migration
-- tests only — no repository/service/handler code is touched (Phase 29B+
-- per saas-module-roadmap.md). Every existing Create() call in this
-- codebase constructs its model struct without setting TenantID, so
-- GORM would otherwise send tenant_id = 0 on every future INSERT once
-- the column is NOT NULL, which the FK constraint below would then
-- reject outright — silently breaking every write path in the running
-- application without a single application-code change. The DEFAULT
-- constraint means GORM (which omits a zero-value field from the INSERT
-- when its schema has a configured default — see the corresponding Go
-- struct tags added in internal/*/model.go) lets the database supply
-- tenant_id = 1 instead, so every existing, unmodified code path keeps
-- behaving exactly as it did before this table had a tenant_id column
-- at all, until Phase 29B explicitly threads a real tenant_id through
-- each service/repository call.
ALTER TABLE roles ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE roles ADD CONSTRAINT DF_roles_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE roles ADD CONSTRAINT FK_roles_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE warehouses ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE warehouses ADD CONSTRAINT DF_warehouses_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE warehouses ADD CONSTRAINT FK_warehouses_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE zones ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE zones ADD CONSTRAINT DF_zones_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE zones ADD CONSTRAINT FK_zones_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE locations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE locations ADD CONSTRAINT DF_locations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE locations ADD CONSTRAINT FK_locations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE products ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE products ADD CONSTRAINT DF_products_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE products ADD CONSTRAINT FK_products_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE inventory ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE inventory ADD CONSTRAINT DF_inventory_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE inventory ADD CONSTRAINT FK_inventory_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_inventory_tenant_product_location_lot ON inventory(tenant_id, product_id, location_id, lot_no);

ALTER TABLE inventory_transactions ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE inventory_transactions ADD CONSTRAINT DF_inventory_transactions_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE inventory_transactions ADD CONSTRAINT FK_inventory_transactions_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_inventory_transactions_tenant_created_at ON inventory_transactions(tenant_id, created_at);

ALTER TABLE receipts ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE receipts ADD CONSTRAINT DF_receipts_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE receipts ADD CONSTRAINT FK_receipts_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE receipt_items ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE receipt_items ADD CONSTRAINT DF_receipt_items_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE receipt_items ADD CONSTRAINT FK_receipt_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_receipt_items_tenant_id ON receipt_items(tenant_id);

ALTER TABLE receipt_confirmations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE receipt_confirmations ADD CONSTRAINT DF_receipt_confirmations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE receipt_confirmations ADD CONSTRAINT FK_receipt_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_receipt_confirmations_tenant_id ON receipt_confirmations(tenant_id);

ALTER TABLE putaway_tasks ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE putaway_tasks ADD CONSTRAINT DF_putaway_tasks_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE putaway_tasks ADD CONSTRAINT FK_putaway_tasks_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE putaway_task_items ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE putaway_task_items ADD CONSTRAINT DF_putaway_task_items_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE putaway_task_items ADD CONSTRAINT FK_putaway_task_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_putaway_task_items_tenant_id ON putaway_task_items(tenant_id);

ALTER TABLE putaway_confirmations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE putaway_confirmations ADD CONSTRAINT DF_putaway_confirmations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE putaway_confirmations ADD CONSTRAINT FK_putaway_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_putaway_confirmations_tenant_id ON putaway_confirmations(tenant_id);

ALTER TABLE picking_tasks ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE picking_tasks ADD CONSTRAINT DF_picking_tasks_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE picking_tasks ADD CONSTRAINT FK_picking_tasks_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE picking_task_items ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE picking_task_items ADD CONSTRAINT DF_picking_task_items_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE picking_task_items ADD CONSTRAINT FK_picking_task_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_picking_task_items_tenant_id ON picking_task_items(tenant_id);

ALTER TABLE picking_confirmations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE picking_confirmations ADD CONSTRAINT DF_picking_confirmations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE picking_confirmations ADD CONSTRAINT FK_picking_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_picking_confirmations_tenant_id ON picking_confirmations(tenant_id);

ALTER TABLE shipments ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE shipments ADD CONSTRAINT DF_shipments_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE shipments ADD CONSTRAINT FK_shipments_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE shipment_items ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE shipment_items ADD CONSTRAINT DF_shipment_items_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE shipment_items ADD CONSTRAINT FK_shipment_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_shipment_items_tenant_id ON shipment_items(tenant_id);

ALTER TABLE shipment_confirmations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE shipment_confirmations ADD CONSTRAINT DF_shipment_confirmations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE shipment_confirmations ADD CONSTRAINT FK_shipment_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_shipment_confirmations_tenant_id ON shipment_confirmations(tenant_id);

ALTER TABLE orders ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE orders ADD CONSTRAINT DF_orders_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE orders ADD CONSTRAINT FK_orders_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);

ALTER TABLE order_items ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE order_items ADD CONSTRAINT DF_order_items_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE order_items ADD CONSTRAINT FK_order_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_order_items_tenant_id ON order_items(tenant_id);

ALTER TABLE stock_adjustments ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE stock_adjustments ADD CONSTRAINT DF_stock_adjustments_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE stock_adjustments ADD CONSTRAINT FK_stock_adjustments_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_stock_adjustments_tenant_status ON stock_adjustments(tenant_id, status);

ALTER TABLE stock_adjustment_items ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE stock_adjustment_items ADD CONSTRAINT DF_stock_adjustment_items_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE stock_adjustment_items ADD CONSTRAINT FK_stock_adjustment_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_stock_adjustment_items_tenant_id ON stock_adjustment_items(tenant_id);

ALTER TABLE stock_adjustment_confirmations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE stock_adjustment_confirmations ADD CONSTRAINT DF_stock_adjustment_confirmations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE stock_adjustment_confirmations ADD CONSTRAINT FK_stock_adjustment_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_stock_adjustment_confirmations_tenant_id ON stock_adjustment_confirmations(tenant_id);

ALTER TABLE stock_adjustment_reminders ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE stock_adjustment_reminders ADD CONSTRAINT DF_stock_adjustment_reminders_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE stock_adjustment_reminders ADD CONSTRAINT FK_stock_adjustment_reminders_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_stock_adjustment_reminders_tenant_id ON stock_adjustment_reminders(tenant_id);

ALTER TABLE stock_counts ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE stock_counts ADD CONSTRAINT DF_stock_counts_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE stock_counts ADD CONSTRAINT FK_stock_counts_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_stock_counts_tenant_status ON stock_counts(tenant_id, status);

ALTER TABLE stock_count_items ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE stock_count_items ADD CONSTRAINT DF_stock_count_items_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE stock_count_items ADD CONSTRAINT FK_stock_count_items_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_stock_count_items_tenant_id ON stock_count_items(tenant_id);

ALTER TABLE stock_count_confirmations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE stock_count_confirmations ADD CONSTRAINT DF_stock_count_confirmations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE stock_count_confirmations ADD CONSTRAINT FK_stock_count_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_stock_count_confirmations_tenant_id ON stock_count_confirmations(tenant_id);

ALTER TABLE replenishment_tasks ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE replenishment_tasks ADD CONSTRAINT DF_replenishment_tasks_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE replenishment_tasks ADD CONSTRAINT FK_replenishment_tasks_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_replenishment_tasks_tenant_warehouse ON replenishment_tasks(tenant_id, warehouse_id);

ALTER TABLE replenishment_confirmations ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE replenishment_confirmations ADD CONSTRAINT DF_replenishment_confirmations_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE replenishment_confirmations ADD CONSTRAINT FK_replenishment_confirmations_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_replenishment_confirmations_tenant_id ON replenishment_confirmations(tenant_id);

ALTER TABLE notifications ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE notifications ADD CONSTRAINT DF_notifications_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE notifications ADD CONSTRAINT FK_notifications_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_notifications_tenant_user ON notifications(tenant_id, user_id);

ALTER TABLE export_jobs ALTER COLUMN tenant_id BIGINT NOT NULL;
ALTER TABLE export_jobs ADD CONSTRAINT DF_export_jobs_tenant_id DEFAULT (1) FOR tenant_id;
ALTER TABLE export_jobs ADD CONSTRAINT FK_export_jobs_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id);
CREATE INDEX IX_export_jobs_tenant_requested_by ON export_jobs(tenant_id, requested_by);

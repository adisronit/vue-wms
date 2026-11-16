-- Customer Ship-to -> Order/Shipping Integration: additive, nullable
-- columns only. customer_id/ship_to_id link an order to the tenant-
-- scoped Customer/Customer Delivery Location masters (internal/customer,
-- internal/customerdeliverylocation); the ship_to_* columns are a
-- server-generated snapshot of the chosen delivery location's own
-- fields, copied once at order-create/update time and never re-synced
-- if the source Customer Delivery Location is edited afterward — the
-- order's own delivery details must stay exactly what was true when the
-- order was placed, not drift with a later master-data edit.
--
-- picking_tasks gets the identical set of columns for the same reason
-- orders.customer (free text) is already denormalized onto
-- picking_tasks (migrations/0017): internal/shipping's own
-- FindDetail/List queries join to picking_tasks, never to orders
-- directly (picking_tasks has no order_id FK at all — see
-- docs/proposals or internal/picking's own tenant-isolation report for
-- why), so Shipping can only ever display what Order copied forward at
-- CreatePickingTask time. Once copied, these columns on picking_tasks
-- are themselves immutable — nothing in internal/picking's own Confirm/
-- Cancel ever touches them.
--
-- Deliberately bare ADD COLUMN statements only, with NO constraint
-- (FK/index) in this file — see migrations/0053's own note for the
-- real SQL Server batch-compilation bug this split avoids ("Invalid
-- column name" when a constraint in the same batch references a column
-- added earlier in that same batch). Constraints follow in
-- migrations/0063, its own transaction.
ALTER TABLE orders ADD customer_id BIGINT NULL;
ALTER TABLE orders ADD ship_to_id BIGINT NULL;
ALTER TABLE orders ADD ship_to_name NVARCHAR(200) NULL;
ALTER TABLE orders ADD ship_to_address NVARCHAR(500) NULL;
ALTER TABLE orders ADD ship_to_province VARCHAR(100) NULL;
ALTER TABLE orders ADD ship_to_district VARCHAR(100) NULL;
ALTER TABLE orders ADD ship_to_postal_code VARCHAR(20) NULL;
ALTER TABLE orders ADD ship_to_contact_person VARCHAR(150) NULL;
ALTER TABLE orders ADD ship_to_contact_phone VARCHAR(50) NULL;

ALTER TABLE picking_tasks ADD customer_id BIGINT NULL;
ALTER TABLE picking_tasks ADD ship_to_id BIGINT NULL;
ALTER TABLE picking_tasks ADD ship_to_name NVARCHAR(200) NULL;
ALTER TABLE picking_tasks ADD ship_to_address NVARCHAR(500) NULL;
ALTER TABLE picking_tasks ADD ship_to_province VARCHAR(100) NULL;
ALTER TABLE picking_tasks ADD ship_to_district VARCHAR(100) NULL;
ALTER TABLE picking_tasks ADD ship_to_postal_code VARCHAR(20) NULL;
ALTER TABLE picking_tasks ADD ship_to_contact_person VARCHAR(150) NULL;
ALTER TABLE picking_tasks ADD ship_to_contact_phone VARCHAR(50) NULL;

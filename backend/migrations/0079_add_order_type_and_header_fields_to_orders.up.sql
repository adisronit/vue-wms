-- Phase 33A — Order Management redesign (full-page Create/Edit/Review &
-- Release, replacing the old popup). Adds order_type and the header
-- fields the new full-page form/PDF need. Deliberately bare ADD COLUMN
-- statements only, with NO constraint (CHECK/FK/index) in this file —
-- see migrations/0053's own note for the real SQL Server batch-
-- compilation bug this split avoids ("Invalid column name" when a
-- constraint in the same batch references a column added earlier in
-- that same batch). Constraints follow in migrations/0081, its own
-- transaction, mirroring the 0062/0063 Customer Ship-to precedent.
--
-- order_type (migrations/0038's own CHECK on orders.status is widened
-- separately, see 0081) defaults to 'SALES_ORDER' so every existing row
-- backfills safely; 'INTERNAL_TRANSFER' is also accepted by the new
-- CHECK constraint so Phase 33E can reuse this same column later — no
-- behavior is built around that value in this batch (CLAUDE.md's own
-- "do not build transfer logic now" instruction).
--
-- priority/required_shipping_date/required_delivery_date/carrier/
-- shipping_method/delivery_instructions/prepared_by/approved_by mirror
-- the header fields a real sales order form/printed document needs —
-- Customer/ship-to snapshot fields already exist (migrations/0062), not
-- duplicated here. prepared_by/approved_by are free-text display/
-- signature fields (who prepared/approved the paper document), distinct
-- from created_by/released_by (the actual authenticated user ids, which
-- already exist resp. are added below) — a warehouse's real paper
-- process often names a different person than whoever happened to be
-- logged in.
--
-- released_at/released_by/shortage_approved back the new Review &
-- Release step (Service.Release): PENDING -> RELEASED is a new,
-- explicit transition gating CreatePickingTask (previously gated on
-- PENDING directly) — see internal/order/model.go's own Status doc.
-- shortage_approved records whether Release was confirmed with a known
-- availability shortfall (the "Controlled shortage approval" override),
-- for audit/display on the Order PDF; it is never consulted by any
-- inventory mutation — Order 33A only ever reads inventory balances.
ALTER TABLE orders ADD order_type VARCHAR(30) NOT NULL CONSTRAINT DF_orders_order_type DEFAULT ('SALES_ORDER');
ALTER TABLE orders ADD priority VARCHAR(10) NOT NULL CONSTRAINT DF_orders_priority DEFAULT ('NORMAL');
ALTER TABLE orders ADD required_shipping_date DATE NULL;
ALTER TABLE orders ADD required_delivery_date DATE NULL;
ALTER TABLE orders ADD carrier NVARCHAR(150) NULL;
ALTER TABLE orders ADD shipping_method VARCHAR(50) NULL;
ALTER TABLE orders ADD delivery_instructions NVARCHAR(500) NULL;
ALTER TABLE orders ADD prepared_by NVARCHAR(150) NULL;
ALTER TABLE orders ADD approved_by NVARCHAR(150) NULL;
ALTER TABLE orders ADD released_at DATETIME2 NULL;
ALTER TABLE orders ADD released_by BIGINT NULL;
ALTER TABLE orders ADD shortage_approved BIT NOT NULL CONSTRAINT DF_orders_shortage_approved DEFAULT (0);

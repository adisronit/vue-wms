-- Phase 33A — Order Management redesign: line-level fields the new
-- full-page product-lines editor and Review & Release screen need.
-- Bare ADD COLUMN statements only — see migrations/0053's own doc for
-- why constraints (migrations/0081) are split into a separate file/
-- transaction.
--
-- Location/lot are still NOT stored here — an order line stays
-- location-agnostic until CreatePickingTask (see internal/order/
-- model.go's own Item doc); preferred_lot_no/fefo_preferred are a
-- picker HINT only (surfaced to whoever later fills in
-- CreatePickingTaskRequest's own LocationAssignment.LotNo), never
-- consulted by any inventory call Order itself makes — Order 33A never
-- reserves or mutates inventory.
--
-- shortage_flag/shortfall_qty are written once, by Service.Release's
-- own availability check (never by Create/Update — Save Draft must
-- never touch inventory-derived state), and are purely informational:
-- nothing in internal/picking's own reservation path reads them.
ALTER TABLE order_items ADD preferred_lot_no VARCHAR(50) NULL;
ALTER TABLE order_items ADD fefo_preferred BIT NOT NULL CONSTRAINT DF_order_items_fefo_preferred DEFAULT (1);
ALTER TABLE order_items ADD shortage_flag BIT NOT NULL CONSTRAINT DF_order_items_shortage_flag DEFAULT (0);
ALTER TABLE order_items ADD shortfall_qty DECIMAL(18,4) NULL;
ALTER TABLE order_items ADD line_notes NVARCHAR(500) NULL;

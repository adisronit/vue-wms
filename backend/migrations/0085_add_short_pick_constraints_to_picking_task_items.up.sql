-- Phase 33B — constraints for migrations/0084's new columns, split into
-- its own batch per migrations/0053's own doc.
ALTER TABLE picking_task_items ADD CONSTRAINT CK_picking_task_items_short_qty CHECK (short_qty IS NULL OR short_qty >= 0);

ALTER TABLE picking_task_items ADD CONSTRAINT CK_picking_task_items_short_reason CHECK (short_reason IS NULL OR short_reason IN
    ('OUT_OF_STOCK', 'DAMAGED', 'LOCATION_EMPTY', 'WRONG_LOT', 'MISCOUNT', 'OTHER'));

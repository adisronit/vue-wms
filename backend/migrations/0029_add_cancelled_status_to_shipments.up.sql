-- Phase 15 adds shipment cancellation (POST /shipping/{id}/cancel,
-- allowed only from READY — before dispatch). Unlike receipts,
-- putaway_tasks, and picking_tasks, shipments (migrations/0020) never
-- got a CANCELLED forward-compatibility placeholder in its own CHECK
-- constraint — this is the first migration in the project that ALTERs
-- an existing constraint rather than only adding new tables, since the
-- value genuinely didn't exist here yet.
ALTER TABLE shipments DROP CONSTRAINT CK_shipments_status;

ALTER TABLE shipments ADD CONSTRAINT CK_shipments_status CHECK (status IN
    ('READY', 'DISPATCHED', 'IN_TRANSIT', 'DELIVERED', 'ISSUE', 'CANCELLED'));

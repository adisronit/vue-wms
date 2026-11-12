ALTER TABLE shipments DROP CONSTRAINT CK_shipments_status;

ALTER TABLE shipments ADD CONSTRAINT CK_shipments_status CHECK (status IN
    ('READY', 'DISPATCHED', 'IN_TRANSIT', 'DELIVERED', 'ISSUE'));

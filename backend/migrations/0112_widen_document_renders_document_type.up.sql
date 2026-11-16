-- Phase 33D.3 -- Delivery Note (internal/shipping) and Loading Manifest
-- (internal/loading): widens CK_document_renders_document_type
-- (migrations/0104) to allow 'DELIVERY_NOTE' and 'LOADING_MANIFEST'
-- alongside the six document kinds that migration already covers.
-- document_type is an existing column on an existing table; this
-- redefines its CHECK in one statement, which carries none of the ADD-
-- COLUMN-then-ADD-CONSTRAINT-in-one-batch risk migrations/0050
-- documents -- that risk is specific to a CHECK referencing a column
-- added earlier in the SAME file, not to redefining a CHECK on a column
-- that already existed long before this migration. Same drop-and-
-- recreate approach migrations/0074 already used to widen
-- CK_inventory_transactions_type for 'REJECT'.
ALTER TABLE document_renders DROP CONSTRAINT CK_document_renders_document_type;
ALTER TABLE document_renders ADD CONSTRAINT CK_document_renders_document_type CHECK (document_type IN (
    'ORDER', 'PICK_LIST', 'PICKING_BATCH_LIST', 'PICKING_CONFIRMATION', 'PACKING_LIST', 'CARTON_LABEL',
    'DELIVERY_NOTE', 'LOADING_MANIFEST'
));

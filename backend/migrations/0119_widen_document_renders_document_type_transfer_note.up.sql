-- Phase 33E.3 -- Inter-Warehouse Transfer Note (internal/transferreceipt):
-- widens CK_document_renders_document_type (migrations/0104, already
-- widened once in migrations/0112 for 'DELIVERY_NOTE'/'LOADING_MANIFEST')
-- to also allow 'TRANSFER_NOTE' -- the closing document of Phase 33E's
-- own inter-warehouse transfer chain (Order -> Dispatch -> Destination
-- Receiving -> Transfer Note). document_type is an existing column on an
-- existing table; this redefines its CHECK in one statement, same
-- drop-and-recreate approach as migrations/0074/0112.
ALTER TABLE document_renders DROP CONSTRAINT CK_document_renders_document_type;
ALTER TABLE document_renders ADD CONSTRAINT CK_document_renders_document_type CHECK (document_type IN (
    'ORDER', 'PICK_LIST', 'PICKING_BATCH_LIST', 'PICKING_CONFIRMATION', 'PACKING_LIST', 'CARTON_LABEL',
    'DELIVERY_NOTE', 'LOADING_MANIFEST', 'TRANSFER_NOTE'
));

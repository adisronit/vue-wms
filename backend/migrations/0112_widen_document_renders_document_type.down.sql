ALTER TABLE document_renders DROP CONSTRAINT CK_document_renders_document_type;
ALTER TABLE document_renders ADD CONSTRAINT CK_document_renders_document_type CHECK (document_type IN (
    'ORDER', 'PICK_LIST', 'PICKING_BATCH_LIST', 'PICKING_CONFIRMATION', 'PACKING_LIST', 'CARTON_LABEL'
));

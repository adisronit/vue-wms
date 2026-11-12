ALTER TABLE stock_counts DROP CONSTRAINT CK_stock_counts_status;

ALTER TABLE stock_counts ADD CONSTRAINT CK_stock_counts_status CHECK (status IN
    ('DRAFT', 'CONFIRMED'));

ALTER TABLE products DROP CONSTRAINT FK_products_category;
DROP INDEX IF EXISTS IX_products_category_id ON products;
ALTER TABLE products DROP COLUMN category_id;

DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS product_categories;

DROP INDEX IX_products_preferred_zone_id ON products;
DROP INDEX IX_products_preferred_warehouse_id ON products;
DROP INDEX IX_products_default_supplier_id ON products;
DROP INDEX IX_products_family_id ON products;
DROP INDEX IX_products_brand_id ON products;

ALTER TABLE products DROP CONSTRAINT CK_products_base_uom;
ALTER TABLE products ADD CONSTRAINT CK_products_base_uom
  CHECK (base_uom IN ('PCS', 'BOX', 'ROLL', 'KG'));

ALTER TABLE products DROP CONSTRAINT CK_products_weight_unit;
ALTER TABLE products DROP CONSTRAINT CK_products_rotation_rule;
ALTER TABLE products DROP CONSTRAINT CK_products_storage_type;

ALTER TABLE products DROP CONSTRAINT FK_products_preferred_zone;
ALTER TABLE products DROP CONSTRAINT FK_products_preferred_warehouse;
ALTER TABLE products DROP CONSTRAINT FK_products_default_supplier;
ALTER TABLE products DROP CONSTRAINT FK_products_family;
ALTER TABLE products DROP CONSTRAINT FK_products_brand;

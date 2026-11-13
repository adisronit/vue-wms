-- Constraints for migrations/0053's new columns — kept in its own
-- migration (own transaction, runs after 0053 has already committed)
-- specifically to avoid the ADD-COLUMN-then-ADD-CONSTRAINT-in-one-batch
-- bug migrations/0050 hit; see that file's own note.
ALTER TABLE products ADD CONSTRAINT FK_products_brand FOREIGN KEY (brand_id) REFERENCES brands(id);
ALTER TABLE products ADD CONSTRAINT FK_products_family FOREIGN KEY (family_id) REFERENCES product_families(id);
ALTER TABLE products ADD CONSTRAINT FK_products_default_supplier FOREIGN KEY (default_supplier_id) REFERENCES suppliers(id);
ALTER TABLE products ADD CONSTRAINT FK_products_preferred_warehouse FOREIGN KEY (preferred_warehouse_id) REFERENCES warehouses(id);
ALTER TABLE products ADD CONSTRAINT FK_products_preferred_zone FOREIGN KEY (preferred_zone_id) REFERENCES zones(id);

-- Allows '' alongside NULL for the same reason migrations/0050's
-- CK_locations_type does: StorageType/RotationRule/WeightUnit are
-- plain Go strings (not pointers), so GORM writes a product created
-- without one as '' on INSERT, never actual SQL NULL — a product that
-- simply never set these EWTA fields must not violate the constraint.
ALTER TABLE products ADD CONSTRAINT CK_products_storage_type
  CHECK (storage_type IS NULL OR storage_type = '' OR storage_type IN ('AMBIENT', 'CHILLED', 'FROZEN'));

ALTER TABLE products ADD CONSTRAINT CK_products_rotation_rule
  CHECK (rotation_rule IS NULL OR rotation_rule = '' OR rotation_rule IN ('FIFO', 'FEFO'));

ALTER TABLE products ADD CONSTRAINT CK_products_weight_unit
  CHECK (weight_unit IS NULL OR weight_unit = '' OR weight_unit IN ('G', 'KG'));

-- EWTA imported-food products (Dairy/Olive Oil/Pasta/Frozen/Deli)
-- need weight/volume base units the original fixed-manufacturing-supply
-- set (PCS/BOX/ROLL/KG) didn't anticipate. Widening, not replacing:
-- every existing PCS/BOX/ROLL/KG row and every caller still validates
-- unchanged.
ALTER TABLE products DROP CONSTRAINT CK_products_base_uom;
ALTER TABLE products ADD CONSTRAINT CK_products_base_uom
  CHECK (base_uom IN ('PCS', 'BOX', 'ROLL', 'KG', 'G', 'L', 'ML'));

CREATE INDEX IX_products_brand_id ON products(brand_id);
CREATE INDEX IX_products_family_id ON products(family_id);
CREATE INDEX IX_products_default_supplier_id ON products(default_supplier_id);
CREATE INDEX IX_products_preferred_warehouse_id ON products(preferred_warehouse_id);
CREATE INDEX IX_products_preferred_zone_id ON products(preferred_zone_id);

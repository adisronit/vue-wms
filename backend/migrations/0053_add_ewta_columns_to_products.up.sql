-- EWTA Product Master enhancement: additive, nullable columns the
-- simplified 3-section Product Add/Edit form needs. Every column here
-- is NULL-able — no existing row has a sensible non-NULL value to
-- backfill (an assumed storage type or country of origin would be a
-- fabricated value, not a migrated one; see CLAUDE.md's "never invent
-- product data").
--
-- Deliberately bare ADD COLUMN statements only, with NO constraint
-- (CHECK/FK) in this file: migrations/0050 already hit a real, reproducible
-- SQL Server batch-compilation bug where an ADD CONSTRAINT referencing a
-- column added earlier IN THE SAME UN-GO'D BATCH fails with "Invalid
-- column name" — cmd/migrate's runner sends each file as one batch, no
-- GO support. Splitting columns (this file) from constraints
-- (migrations/0054, its own transaction) sidesteps that bug entirely
-- rather than repeating it.
ALTER TABLE products ADD name_th NVARCHAR(200) NULL;
ALTER TABLE products ADD brand_id BIGINT NULL;
ALTER TABLE products ADD family_id BIGINT NULL;
ALTER TABLE products ADD variant_name VARCHAR(100) NULL;
ALTER TABLE products ADD pack_size_unit VARCHAR(20) NULL;
ALTER TABLE products ADD storage_type VARCHAR(20) NULL;
ALTER TABLE products ADD rotation_rule VARCHAR(10) NULL;
ALTER TABLE products ADD default_supplier_id BIGINT NULL;
ALTER TABLE products ADD country_of_origin VARCHAR(100) NULL;
ALTER TABLE products ADD preferred_warehouse_id BIGINT NULL;
ALTER TABLE products ADD preferred_zone_id BIGINT NULL;
ALTER TABLE products ADD manufacturer VARCHAR(200) NULL;
ALTER TABLE products ADD gross_weight DECIMAL(10, 3) NULL;
ALTER TABLE products ADD net_weight DECIMAL(10, 3) NULL;
ALTER TABLE products ADD weight_unit VARCHAR(10) NULL;
ALTER TABLE products ADD length_mm DECIMAL(10, 2) NULL;
ALTER TABLE products ADD width_mm DECIMAL(10, 2) NULL;
ALTER TABLE products ADD height_mm DECIMAL(10, 2) NULL;
-- image_path is a server-generated, UUID-based relative path under
-- IMAGE_STORAGE_DIR — never a user-supplied filename (no path
-- traversal surface). image_original_name is kept purely for display
-- (e.g. a download filename), never used to build a filesystem path.
ALTER TABLE products ADD image_path VARCHAR(500) NULL;
ALTER TABLE products ADD image_original_name VARCHAR(255) NULL;
ALTER TABLE products ADD image_mime_type VARCHAR(100) NULL;
ALTER TABLE products ADD image_size_bytes INT NULL;
ALTER TABLE products ADD image_uploaded_at DATETIME2 NULL;

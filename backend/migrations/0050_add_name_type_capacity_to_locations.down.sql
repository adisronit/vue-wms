ALTER TABLE locations DROP CONSTRAINT CK_locations_capacity;
ALTER TABLE locations DROP CONSTRAINT CK_locations_type;
ALTER TABLE locations DROP COLUMN capacity;
ALTER TABLE locations DROP COLUMN type;
ALTER TABLE locations DROP COLUMN name;

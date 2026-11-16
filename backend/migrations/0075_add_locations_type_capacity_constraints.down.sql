IF EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_locations_capacity')
BEGIN
    ALTER TABLE locations DROP CONSTRAINT CK_locations_capacity;
END

IF EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_locations_type')
BEGIN
    ALTER TABLE locations DROP CONSTRAINT CK_locations_type;
END

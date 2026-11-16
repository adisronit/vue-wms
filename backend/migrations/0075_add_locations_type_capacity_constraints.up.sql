-- Split out of migrations/0050 (Phase 31E) — see that file's own doc
-- for why. Guarded with IF NOT EXISTS rather than assumed-absent: any
-- environment that already got past the original (broken-from-scratch)
-- 0050 via the documented manual sqlcmd workaround already has these
-- two constraints sitting on the table, and this migration must still
-- apply cleanly there the first time it runs.
IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_locations_type')
BEGIN
    -- Independent from zones.type's own CK_zones_type vocabulary — a
    -- location's type describes the individual bin's own role (which can
    -- differ from its zone's overall purpose, e.g. a QC bin living inside
    -- a general STORAGE zone), not a duplicate of it. Reuses the same
    -- 7-value set as zone.Type purely because it's already a validated,
    -- familiar vocabulary for this schema, not because the two columns are
    -- meant to be kept in sync.
    --
    -- Allows '' alongside NULL: Location.Type is a plain Go string (not a
    -- pointer), matching this table's own existing aisle/rack/bin columns
    -- (migrations/0007) — GORM writes a non-pointer string field's zero
    -- value as '' on every INSERT/UPDATE, never actual SQL NULL, so a
    -- location created without a type must not violate this constraint.
    ALTER TABLE locations ADD CONSTRAINT CK_locations_type
      CHECK (type IS NULL OR type = '' OR type IN ('RECEIVING', 'STORAGE', 'PICKING', 'PACKING', 'SHIPPING', 'QC', 'HOLD'));
END

IF NOT EXISTS (SELECT 1 FROM sys.check_constraints WHERE name = 'CK_locations_capacity')
BEGIN
    ALTER TABLE locations ADD CONSTRAINT CK_locations_capacity
      CHECK (capacity IS NULL OR capacity >= 0);
END

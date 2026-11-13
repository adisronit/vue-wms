-- Location Master Management: additive, nullable columns the new
-- standalone Location Management page's field list requires beyond
-- what migrations/0007 originally modeled (code/aisle/rack/bin/status).
-- Mirrors the exact shape existing sibling tables already use for the
-- same concepts — zones.name/zones.type (migrations/0006) and
-- warehouses.capacity (migrations/0005) — rather than inventing a new
-- pattern. All three are NULL-able: every existing location row
-- predates these fields and has no sensible non-NULL default to
-- backfill (an assumed "STORAGE" type or "0" capacity would be a
-- fabricated value, not a migrated one).
ALTER TABLE locations ADD name NVARCHAR(200) NULL;
ALTER TABLE locations ADD type VARCHAR(20) NULL;
ALTER TABLE locations ADD capacity INT NULL;

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

ALTER TABLE locations ADD CONSTRAINT CK_locations_capacity
  CHECK (capacity IS NULL OR capacity >= 0);

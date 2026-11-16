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
--
-- The two CHECK constraints this migration originally also added here
-- (CK_locations_type, CK_locations_capacity) were moved out to
-- migrations/0075: SQL Server resolves an entire .sql file as one
-- batch before executing any of it (this project's driver sends each
-- migration file as a single ExecContext call, with no "GO" batch-
-- separator support), so a CHECK constraint referencing a column added
-- earlier in the SAME file fails with "Invalid column name" on a
-- genuinely fresh database — confirmed via `go run ./cmd/migrate up`
-- against a brand-new, never-migrated database (Phase 31E's own fresh-
-- DB verification). Editing this file (rather than leaving the bug and
-- only patching forward) is safe: any environment that already has
-- '0050' recorded in schema_migrations never re-runs this file's
-- content, so this change only ever affects a database that hasn't
-- gotten past 0050 yet — exactly the case that was broken.
ALTER TABLE locations ADD name NVARCHAR(200) NULL;
ALTER TABLE locations ADD type VARCHAR(20) NULL;
ALTER TABLE locations ADD capacity INT NULL;

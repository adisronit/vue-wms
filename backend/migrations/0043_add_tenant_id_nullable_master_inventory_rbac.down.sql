-- Non-destructive: drops only the column this migration added. No
-- backfill, constraint, or index from 0046-0048 can still be applied
-- when this runs, per the rollback ordering in migration-plan.md §7.
ALTER TABLE roles DROP COLUMN tenant_id;
ALTER TABLE warehouses DROP COLUMN tenant_id;
ALTER TABLE zones DROP COLUMN tenant_id;
ALTER TABLE locations DROP COLUMN tenant_id;
ALTER TABLE products DROP COLUMN tenant_id;
ALTER TABLE inventory DROP COLUMN tenant_id;
ALTER TABLE inventory_transactions DROP COLUMN tenant_id;

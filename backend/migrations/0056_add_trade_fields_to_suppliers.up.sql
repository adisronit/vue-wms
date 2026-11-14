-- Supplier Master UX & EWTA Import/Local Supplier Enhancement: adds a
-- DOMESTIC/OVERSEAS supplier_type plus the Trade Details fields the
-- redesigned Add/Edit drawer needs, so one Supplier Master can carry
-- both domestic and overseas (e.g. EWTA import) vendors without two
-- separate screens or tables. Every new column is additive; no existing
-- column is renamed, dropped, or retyped, so the current API contract
-- (company_name, tax_id, contact_person, contact_phone, address, email,
-- phone, status) stays byte-for-byte compatible.
--
-- This file only ADDs columns. The CHECK constraint and index on
-- supplier_type live in migrations/0057, not here — see
-- migrations/0053 (add columns) / 0054 (add constraints) for the EWTA
-- products precedent this follows: a column added by one ALTER TABLE
-- statement is not visible to a CHECK constraint referencing it in a
-- *later* statement of the *same* batch/transaction (this migration
-- runner executes each file's full contents as a single batch) — SQL
-- Server fails that with "Invalid column name", exactly the bug left
-- unfixed in migrations/0050 (locations.capacity). Splitting across
-- migration files is this schema's established fix.
--
-- supplier_type gets a NOT NULL + DEFAULT ('DOMESTIC') in one step (not
-- the phased nullable -> backfill -> not-null dance 0043-0048 needed)
-- because a straight ALTER ... ADD ... DEFAULT (...) NOT NULL backfills
-- every existing row with the default in the same statement — SQL
-- Server computes the default for existing rows as part of adding the
-- column. Every other new field is genuinely optional (only overseas
-- suppliers carry currency/incoterm/lead time; only domestic suppliers
-- carry a Thai tax ID/VAT registration/province), so those stay
-- nullable with no default.
ALTER TABLE suppliers ADD supplier_type VARCHAR(20) NOT NULL CONSTRAINT DF_suppliers_supplier_type DEFAULT ('DOMESTIC');

-- Legal Name (TH) sits next to the existing company_name column, which
-- the redesigned form now labels "Legal Name (EN)" — no rename, since
-- company_name is the field every existing integration already reads.
ALTER TABLE suppliers ADD legal_name_th NVARCHAR(200) NULL;

-- Country is a free-text display name (matching products.country_of_origin's
-- own plain-string shape elsewhere in this schema) rather than an ISO
-- code lookup table — there is no country master table in this project,
-- and adding one for a single free-text field would be the kind of
-- unused abstraction CLAUDE.md asks to avoid. The frontend's searchable
-- Country selector is a static client-side list that writes the chosen
-- display name here.
ALTER TABLE suppliers ADD country NVARCHAR(100) NULL;

-- Overseas Trade Details.
ALTER TABLE suppliers ADD currency VARCHAR(3) NULL;
ALTER TABLE suppliers ADD default_incoterm VARCHAR(10) NULL;
ALTER TABLE suppliers ADD incoterm_named_place NVARCHAR(200) NULL;
ALTER TABLE suppliers ADD lead_time_days INT NULL;
ALTER TABLE suppliers ADD payment_terms NVARCHAR(200) NULL;
ALTER TABLE suppliers ADD shipping_method NVARCHAR(100) NULL;

-- Domestic Trade Details.
ALTER TABLE suppliers ADD thai_tax_id VARCHAR(50) NULL;
ALTER TABLE suppliers ADD vat_registration VARCHAR(50) NULL;
ALTER TABLE suppliers ADD province NVARCHAR(100) NULL;
ALTER TABLE suppliers ADD postal_code VARCHAR(20) NULL;

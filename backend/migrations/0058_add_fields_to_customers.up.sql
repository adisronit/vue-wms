-- EWTA Customer Master + Multi Ship-to: additive fields the redesigned
-- Customer Add/Edit form's Basic/Contact/Billing sections need beyond
-- what migrations/0049 originally modeled (code/company_name/tax_id/
-- contact_person/contact_phone/address/email/phone/status). No existing
-- column is renamed, dropped, or retyped, so the current API contract
-- stays byte-for-byte compatible; company_name/address keep serving as
-- "Name (EN)"/"Billing Address" under new labels only, not new columns.
--
-- All four columns are nullable with no default — every existing
-- customer row predates these fields (customer_type/customer_group/
-- branch_no/legal_name_th) and there is no sensible non-NULL value to
-- backfill, the same reasoning migrations/0050's locations.name/type/
-- capacity doc gives for its own nullable columns.
ALTER TABLE customers ADD legal_name_th NVARCHAR(200) NULL;

-- customer_type/customer_group are deliberately plain free text, not a
-- fixed enum — unlike supplier_type's real DOMESTIC/OVERSEAS dichotomy
-- (migrations/0056), this project has no specified business taxonomy
-- for "customer type" (e.g. distributor/retailer) or "customer group"
-- (e.g. key account/standard), and inventing one here would be exactly
-- the kind of fabricated classification CLAUDE.md's "no fake
-- implementations" principle warns against. Free text lets each tenant
-- use whatever vocabulary their own sales org already uses.
ALTER TABLE customers ADD customer_type NVARCHAR(50) NULL;
ALTER TABLE customers ADD customer_group NVARCHAR(50) NULL;

-- Thai tax invoices identify the issuing branch separately from the tax
-- ID itself (e.g. "00000" for head office, a 5-digit code for a branch)
-- — a distinct field from the existing tax_id column, which the
-- redesigned form now labels "Thai Tax ID" (no rename needed; tax_id
-- already holds exactly that value).
ALTER TABLE customers ADD branch_no VARCHAR(10) NULL;

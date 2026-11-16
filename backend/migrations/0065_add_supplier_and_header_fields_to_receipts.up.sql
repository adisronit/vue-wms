-- Phase 31A — Receiving: integrates the real internal/supplier master
-- (migrations/0049/0056/0057) into receipts, and adds the detailed
-- logistics/document header fields NewReceivingView.vue's redesigned
-- "Document Information" step needs (invoice/delivery-note/container/
-- transport/dock/arrival-unloading-time/currency).
--
-- supplier_id is nullable and additive: the pre-existing vendor
-- VARCHAR(150) free-text column (migrations/0011) is left completely
-- untouched — every already-confirmed receipt keeps showing exactly the
-- vendor string it always had, and a receipt may still be created
-- without picking a real supplier row (ReceivingType RETURN/TRANSFER
-- often has none). supplier_name_snapshot mirrors the Customer Ship-to
-- integration's own one-time-copy-at-write-time pattern
-- ([[wms_customer_shipto_order_integration]]) — a supplier's company
-- name is captured onto the receipt the moment it's chosen and never
-- re-synced if the supplier master record is renamed later, so a
-- historical receipt's displayed supplier name never silently changes
-- out from under an already-printed document.
--
-- Bare ADD COLUMN statements only, no FK/CHECK in this file — see
-- migrations/0053's own doc for the batch-compilation gotcha this
-- deliberately avoids; migrations/0066 adds FK_receipts_supplier
-- against supplier_id in its own, separate migration.
ALTER TABLE receipts ADD supplier_id BIGINT NULL;
ALTER TABLE receipts ADD supplier_name_snapshot VARCHAR(150) NULL;
ALTER TABLE receipts ADD invoice_no VARCHAR(50) NULL;
ALTER TABLE receipts ADD delivery_note_no VARCHAR(50) NULL;
ALTER TABLE receipts ADD container_no VARCHAR(50) NULL;
ALTER TABLE receipts ADD seal_no VARCHAR(50) NULL;
ALTER TABLE receipts ADD transport_company VARCHAR(150) NULL;
ALTER TABLE receipts ADD driver_name VARCHAR(100) NULL;
ALTER TABLE receipts ADD vehicle_reg_no VARCHAR(30) NULL;
ALTER TABLE receipts ADD dock_door VARCHAR(30) NULL;
ALTER TABLE receipts ADD arrival_time DATETIME2 NULL;
ALTER TABLE receipts ADD unloading_start_time DATETIME2 NULL;
ALTER TABLE receipts ADD unloading_end_time DATETIME2 NULL;
ALTER TABLE receipts ADD currency VARCHAR(10) NULL;

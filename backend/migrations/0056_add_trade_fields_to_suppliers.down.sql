ALTER TABLE suppliers DROP COLUMN postal_code;
ALTER TABLE suppliers DROP COLUMN province;
ALTER TABLE suppliers DROP COLUMN vat_registration;
ALTER TABLE suppliers DROP COLUMN thai_tax_id;

ALTER TABLE suppliers DROP COLUMN shipping_method;
ALTER TABLE suppliers DROP COLUMN payment_terms;
ALTER TABLE suppliers DROP COLUMN lead_time_days;
ALTER TABLE suppliers DROP COLUMN incoterm_named_place;
ALTER TABLE suppliers DROP COLUMN default_incoterm;
ALTER TABLE suppliers DROP COLUMN currency;

ALTER TABLE suppliers DROP COLUMN country;
ALTER TABLE suppliers DROP COLUMN legal_name_th;

ALTER TABLE suppliers DROP CONSTRAINT DF_suppliers_supplier_type;
ALTER TABLE suppliers DROP COLUMN supplier_type;

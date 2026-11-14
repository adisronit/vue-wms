// Static reference lists for the Supplier Master's Country/Currency/
// Incoterm selectors. There is no country/currency master table in
// this project (backend/migrations/0056's own doc explains why
// suppliers.country is plain free text, not an FK) — these lists exist
// purely to give the SearchableSelect component's client-side-filter
// mode (see its own doc: "options, a plain array: client-side
// filtering only, for lists that are always small and already fully
// loaded") something to search over, the same way ProductMasterView
// hard-codes UOM_OPTIONS for its unit-of-measure dropdown.

// Common countries for a Thailand-based WMS's domestic + overseas
// (e.g. EWTA import) supplier base — not the full ISO 3166 list, since
// an exhaustive list would mostly add noise to the typeahead for
// countries this business never actually sources from.
export const COUNTRIES = [
  'Thailand', 'Singapore', 'Malaysia', 'Indonesia', 'Vietnam', 'Philippines', 'Myanmar', 'Cambodia', 'Laos',
  'China', 'Hong Kong', 'Taiwan', 'Japan', 'South Korea', 'India',
  'United States', 'Canada', 'Mexico', 'Brazil',
  'United Kingdom', 'France', 'Germany', 'Italy', 'Spain', 'Netherlands', 'Belgium', 'Switzerland',
  'Denmark', 'Sweden', 'Norway', 'Finland', 'Poland', 'Austria', 'Ireland', 'Portugal', 'Greece',
  'Australia', 'New Zealand',
  'United Arab Emirates', 'Saudi Arabia', 'Turkey', 'Israel',
  'South Africa'
].map((name) => ({ value: name, label: name }))

// ISO 4217 currencies this business actually transacts in — overseas
// Trade Details' Currency field, not a full ISO currency table.
export const CURRENCIES = [
  { value: 'THB', label: 'THB — Thai Baht' },
  { value: 'USD', label: 'USD — US Dollar' },
  { value: 'EUR', label: 'EUR — Euro' },
  { value: 'GBP', label: 'GBP — British Pound' },
  { value: 'JPY', label: 'JPY — Japanese Yen' },
  { value: 'CNY', label: 'CNY — Chinese Yuan' },
  { value: 'SGD', label: 'SGD — Singapore Dollar' },
  { value: 'HKD', label: 'HKD — Hong Kong Dollar' },
  { value: 'AUD', label: 'AUD — Australian Dollar' },
  { value: 'NZD', label: 'NZD — New Zealand Dollar' },
  { value: 'CAD', label: 'CAD — Canadian Dollar' },
  { value: 'CHF', label: 'CHF — Swiss Franc' },
  { value: 'KRW', label: 'KRW — South Korean Won' },
  { value: 'INR', label: 'INR — Indian Rupee' },
  { value: 'VND', label: 'VND — Vietnamese Dong' },
  { value: 'IDR', label: 'IDR — Indonesian Rupiah' },
  { value: 'MYR', label: 'MYR — Malaysian Ringgit' },
  { value: 'PHP', label: 'PHP — Philippine Peso' },
  { value: 'TWD', label: 'TWD — Taiwan Dollar' },
  { value: 'AED', label: 'AED — UAE Dirham' }
]

// Incoterms 2020 — the full 11-rule set, each paired with its named
// place so the drawer's "Default Incoterm + Named Place" pair matches
// how these terms are actually quoted (e.g. "FOB Shanghai").
export const INCOTERMS = [
  { value: 'EXW', label: 'EXW — Ex Works' },
  { value: 'FCA', label: 'FCA — Free Carrier' },
  { value: 'FAS', label: 'FAS — Free Alongside Ship' },
  { value: 'FOB', label: 'FOB — Free on Board' },
  { value: 'CFR', label: 'CFR — Cost and Freight' },
  { value: 'CIF', label: 'CIF — Cost, Insurance and Freight' },
  { value: 'CPT', label: 'CPT — Carriage Paid To' },
  { value: 'CIP', label: 'CIP — Carriage and Insurance Paid To' },
  { value: 'DAP', label: 'DAP — Delivered at Place' },
  { value: 'DPU', label: 'DPU — Delivered at Place Unloaded' },
  { value: 'DDP', label: 'DDP — Delivered Duty Paid' }
]

// Shipping methods — a short, closed list (not free text) so the
// Suppliers list/filter can rely on a known vocabulary, same rationale
// as every other fixed-choice <select> in this schema.
export const SHIPPING_METHODS = ['Sea Freight', 'Air Freight', 'Land Freight', 'Courier', 'Rail Freight']

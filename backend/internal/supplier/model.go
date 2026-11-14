// Package supplier owns the Supplier master data module (Master Data
// Expansion) — tenant-scoped supplier records (company name, tax ID, a
// single contact person, address, email, phone). See
// internal/productcategory/repository.go's doc for why this module's
// Repository, unlike internal/product/internal/warehouse, threads an
// explicit, verified tenant_id through every query from day one.
package supplier

import "time"

// Status mirrors every other master-data module's ACTIVE/INACTIVE
// toggle.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive:
		return true
	default:
		return false
	}
}

// SupplierType distinguishes a domestic vendor (Thai-registered, paid in
// THB) from an overseas vendor (e.g. an EWTA import line) — it decides
// which of the two Trade Details field groups the Add/Edit drawer shows
// (migrations/0056).
type SupplierType string

const (
	SupplierTypeDomestic SupplierType = "DOMESTIC"
	SupplierTypeOverseas SupplierType = "OVERSEAS"
)

func (t SupplierType) IsValid() bool {
	switch t {
	case SupplierTypeDomestic, SupplierTypeOverseas:
		return true
	default:
		return false
	}
}

// Supplier is the GORM model for the suppliers table (migrations/0049,
// trade fields added in migrations/0056).
type Supplier struct {
	ID            int64  `gorm:"column:id;primaryKey"`
	TenantID      int64  `gorm:"column:tenant_id;not null;default:1"`
	Code          string `gorm:"column:code"`
	CompanyName   string `gorm:"column:company_name"`
	TaxID         string `gorm:"column:tax_id"`
	ContactPerson string `gorm:"column:contact_person"`
	ContactPhone  string `gorm:"column:contact_phone"`
	Address       string `gorm:"column:address"`
	Email         string `gorm:"column:email"`
	Phone         string `gorm:"column:phone"`
	Status        Status `gorm:"column:status"`

	// default:DOMESTIC mirrors TenantID's own default tag above: GORM
	// omits a zero-value field from the INSERT statement when it carries
	// a `default:` tag, letting CK_suppliers_supplier_type's DB-level
	// DEFAULT ('DOMESTIC') apply instead of inserting Go's zero value
	// ("") — which the CHECK constraint (migrations/0057) rejects. Caught
	// by TestSupplier_CodeUniqueWithinTenantOnly and the sort/pagination
	// tests' seedSupplier helper, which (like this module's other
	// existing callers) construct a bare Supplier{} without setting
	// SupplierType.
	SupplierType SupplierType `gorm:"column:supplier_type;default:DOMESTIC"`
	LegalNameTH  string       `gorm:"column:legal_name_th"`
	Country      string       `gorm:"column:country"`

	// Overseas Trade Details — meaningful only when SupplierType is
	// OVERSEAS, but never enforced NULL-on-DOMESTIC at the DB level:
	// a supplier can flip type without the service silently discarding
	// data the user may want to keep if they flip back.
	Currency           string `gorm:"column:currency"`
	DefaultIncoterm    string `gorm:"column:default_incoterm"`
	IncotermNamedPlace string `gorm:"column:incoterm_named_place"`
	LeadTimeDays       *int   `gorm:"column:lead_time_days"`
	PaymentTerms       string `gorm:"column:payment_terms"`
	ShippingMethod     string `gorm:"column:shipping_method"`

	// Domestic Trade Details — meaningful only when SupplierType is
	// DOMESTIC, same non-enforcement rationale as above.
	ThaiTaxID       string `gorm:"column:thai_tax_id"`
	VATRegistration string `gorm:"column:vat_registration"`
	Province        string `gorm:"column:province"`
	PostalCode      string `gorm:"column:postal_code"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Supplier) TableName() string { return "suppliers" }

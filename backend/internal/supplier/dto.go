package supplier

import "time"

// Response is what every supplier-returning endpoint sends back.
type Response struct {
	ID            int64        `json:"id,string" example:"1"`
	Code          string       `json:"code" example:"SUPP-0001"`
	CompanyName   string       `json:"company_name" example:"Global Packaging Ltd."`
	LegalNameTH   string       `json:"legal_name_th" example:"บริษัท โกลบอล แพ็คเกจจิ้ง จำกัด"`
	SupplierType  SupplierType `json:"supplier_type" example:"OVERSEAS"`
	Country       string       `json:"country" example:"Thailand"`
	TaxID         string       `json:"tax_id" example:"0105558000002"`
	ContactPerson string       `json:"contact_person" example:"John Lee"`
	ContactPhone  string       `json:"contact_phone" example:"+66-2-000-0001"`
	Address       string       `json:"address"`
	Email         string       `json:"email" example:"john@globalpack.example"`
	Phone         string       `json:"phone" example:"+66-2-222-2222"`
	Status        Status       `json:"status" example:"ACTIVE"`

	Currency           string `json:"currency" example:"USD"`
	DefaultIncoterm    string `json:"default_incoterm" example:"FOB"`
	IncotermNamedPlace string `json:"incoterm_named_place" example:"Rotterdam"`
	LeadTimeDays       *int   `json:"lead_time_days" example:"30"`
	PaymentTerms       string `json:"payment_terms" example:"Net 30"`
	ShippingMethod     string `json:"shipping_method" example:"Sea Freight"`

	ThaiTaxID       string `json:"thai_tax_id" example:"0105558000002"`
	VATRegistration string `json:"vat_registration" example:"0105558000002"`
	Province        string `json:"province" example:"Bangkok"`
	PostalCode      string `json:"postal_code" example:"10110"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
} //	@name	SupplierResponse

// CreateRequest is the POST /api/v1/suppliers body. SupplierType is
// required; every Trade Details field is optional regardless of type —
// the service does not reject OVERSEAS-only fields on a DOMESTIC record
// or vice versa, matching the frontend's own "switching type just
// changes which fields the drawer shows" behavior rather than erasing
// data.
type CreateRequest struct {
	Code          string       `json:"code" binding:"required" example:"SUPP-0001"`
	CompanyName   string       `json:"company_name" binding:"required" example:"Global Packaging Ltd."`
	LegalNameTH   string       `json:"legal_name_th" example:"บริษัท โกลบอล แพ็คเกจจิ้ง จำกัด"`
	SupplierType  SupplierType `json:"supplier_type" example:"OVERSEAS"`
	Country       string       `json:"country" example:"Thailand"`
	TaxID         string       `json:"tax_id" example:"0105558000002"`
	ContactPerson string       `json:"contact_person" example:"John Lee"`
	ContactPhone  string       `json:"contact_phone" example:"+66-2-000-0001"`
	Address       string       `json:"address"`
	Email         string       `json:"email" example:"john@globalpack.example"`
	Phone         string       `json:"phone" example:"+66-2-222-2222"`
	Status        Status       `json:"status" example:"ACTIVE"`

	Currency           string `json:"currency" example:"USD"`
	DefaultIncoterm    string `json:"default_incoterm" example:"FOB"`
	IncotermNamedPlace string `json:"incoterm_named_place" example:"Rotterdam"`
	LeadTimeDays       *int   `json:"lead_time_days" example:"30"`
	PaymentTerms       string `json:"payment_terms" example:"Net 30"`
	ShippingMethod     string `json:"shipping_method" example:"Sea Freight"`

	ThaiTaxID       string `json:"thai_tax_id" example:"0105558000002"`
	VATRegistration string `json:"vat_registration" example:"0105558000002"`
	Province        string `json:"province" example:"Bangkok"`
	PostalCode      string `json:"postal_code" example:"10110"`
} //	@name	CreateSupplierRequest

// UpdateRequest is the PUT /api/v1/suppliers/:id body.
type UpdateRequest struct {
	Code          string       `json:"code" binding:"required" example:"SUPP-0001"`
	CompanyName   string       `json:"company_name" binding:"required" example:"Global Packaging Ltd."`
	LegalNameTH   string       `json:"legal_name_th" example:"บริษัท โกลบอล แพ็คเกจจิ้ง จำกัด"`
	SupplierType  SupplierType `json:"supplier_type" binding:"required" example:"OVERSEAS"`
	Country       string       `json:"country" example:"Thailand"`
	TaxID         string       `json:"tax_id" example:"0105558000002"`
	ContactPerson string       `json:"contact_person" example:"John Lee"`
	ContactPhone  string       `json:"contact_phone" example:"+66-2-000-0001"`
	Address       string       `json:"address"`
	Email         string       `json:"email" example:"john@globalpack.example"`
	Phone         string       `json:"phone" example:"+66-2-222-2222"`
	Status        Status       `json:"status" binding:"required" example:"ACTIVE"`

	Currency           string `json:"currency" example:"USD"`
	DefaultIncoterm    string `json:"default_incoterm" example:"FOB"`
	IncotermNamedPlace string `json:"incoterm_named_place" example:"Rotterdam"`
	LeadTimeDays       *int   `json:"lead_time_days" example:"30"`
	PaymentTerms       string `json:"payment_terms" example:"Net 30"`
	ShippingMethod     string `json:"shipping_method" example:"Sea Freight"`

	ThaiTaxID       string `json:"thai_tax_id" example:"0105558000002"`
	VATRegistration string `json:"vat_registration" example:"0105558000002"`
	Province        string `json:"province" example:"Bangkok"`
	PostalCode      string `json:"postal_code" example:"10110"`
} //	@name	UpdateSupplierRequest

// ListParams narrows GET /api/v1/suppliers to the Suppliers screen's
// search box, status, and supplier-type filters, with pagination and
// sorting. SortBy is one of "code", "companyName", or "status";
// SortDir is "asc" or "desc" (case-insensitive) — validated by
// Service.List against allowedSortColumns, returning ErrInvalidSort
// otherwise.
type ListParams struct {
	Search       string
	Status       Status
	SupplierType SupplierType
	Page         int
	Limit        int
	SortBy       string
	SortDir      string
}

func toResponse(s Supplier) Response {
	return Response{
		ID: s.ID, Code: s.Code, CompanyName: s.CompanyName, LegalNameTH: s.LegalNameTH,
		SupplierType: s.SupplierType, Country: s.Country, TaxID: s.TaxID,
		ContactPerson: s.ContactPerson, ContactPhone: s.ContactPhone,
		Address: s.Address, Email: s.Email, Phone: s.Phone, Status: s.Status,
		Currency: s.Currency, DefaultIncoterm: s.DefaultIncoterm, IncotermNamedPlace: s.IncotermNamedPlace,
		LeadTimeDays: s.LeadTimeDays, PaymentTerms: s.PaymentTerms, ShippingMethod: s.ShippingMethod,
		ThaiTaxID: s.ThaiTaxID, VATRegistration: s.VATRegistration, Province: s.Province, PostalCode: s.PostalCode,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

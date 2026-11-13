package supplier

import "time"

// Response is what every supplier-returning endpoint sends back.
type Response struct {
	ID            int64     `json:"id,string" example:"1"`
	Code          string    `json:"code" example:"SUPP-0001"`
	CompanyName   string    `json:"company_name" example:"Global Packaging Ltd."`
	TaxID         string    `json:"tax_id" example:"0105558000002"`
	ContactPerson string    `json:"contact_person" example:"John Lee"`
	ContactPhone  string    `json:"contact_phone" example:"+66-2-000-0001"`
	Address       string    `json:"address"`
	Email         string    `json:"email" example:"john@globalpack.example"`
	Phone         string    `json:"phone" example:"+66-2-222-2222"`
	Status        Status    `json:"status" example:"ACTIVE"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
} //	@name	SupplierResponse

// CreateRequest is the POST /api/v1/suppliers body.
type CreateRequest struct {
	Code          string `json:"code" binding:"required" example:"SUPP-0001"`
	CompanyName   string `json:"company_name" binding:"required" example:"Global Packaging Ltd."`
	TaxID         string `json:"tax_id" example:"0105558000002"`
	ContactPerson string `json:"contact_person" example:"John Lee"`
	ContactPhone  string `json:"contact_phone" example:"+66-2-000-0001"`
	Address       string `json:"address"`
	Email         string `json:"email" example:"john@globalpack.example"`
	Phone         string `json:"phone" example:"+66-2-222-2222"`
	Status        Status `json:"status" example:"ACTIVE"`
} //	@name	CreateSupplierRequest

// UpdateRequest is the PUT /api/v1/suppliers/:id body.
type UpdateRequest struct {
	Code          string `json:"code" binding:"required" example:"SUPP-0001"`
	CompanyName   string `json:"company_name" binding:"required" example:"Global Packaging Ltd."`
	TaxID         string `json:"tax_id" example:"0105558000002"`
	ContactPerson string `json:"contact_person" example:"John Lee"`
	ContactPhone  string `json:"contact_phone" example:"+66-2-000-0001"`
	Address       string `json:"address"`
	Email         string `json:"email" example:"john@globalpack.example"`
	Phone         string `json:"phone" example:"+66-2-222-2222"`
	Status        Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateSupplierRequest

// ListParams narrows GET /api/v1/suppliers to the Suppliers screen's
// search box and status filter, with pagination and sorting. SortBy is
// one of "code", "companyName", or "status"; SortDir is "asc" or
// "desc" (case-insensitive) — validated by Service.List against
// allowedSortColumns, returning ErrInvalidSort otherwise.
type ListParams struct {
	Search  string
	Status  Status
	Page    int
	Limit   int
	SortBy  string
	SortDir string
}

func toResponse(s Supplier) Response {
	return Response{
		ID: s.ID, Code: s.Code, CompanyName: s.CompanyName, TaxID: s.TaxID,
		ContactPerson: s.ContactPerson, ContactPhone: s.ContactPhone,
		Address: s.Address, Email: s.Email, Phone: s.Phone, Status: s.Status,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

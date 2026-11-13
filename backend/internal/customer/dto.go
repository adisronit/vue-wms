package customer

import "time"

// Response is what every customer-returning endpoint sends back.
type Response struct {
	ID            int64     `json:"id,string" example:"1"`
	Code          string    `json:"code" example:"CUST-0001"`
	CompanyName   string    `json:"company_name" example:"Acme Retail Co."`
	TaxID         string    `json:"tax_id" example:"0105558000001"`
	ContactPerson string    `json:"contact_person" example:"Jane Somchai"`
	ContactPhone  string    `json:"contact_phone" example:"+66-2-000-0000"`
	Address       string    `json:"address"`
	Email         string    `json:"email" example:"jane@acme.example"`
	Phone         string    `json:"phone" example:"+66-2-111-1111"`
	Status        Status    `json:"status" example:"ACTIVE"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
} //	@name	CustomerResponse

// CreateRequest is the POST /api/v1/customers body.
type CreateRequest struct {
	Code          string `json:"code" binding:"required" example:"CUST-0001"`
	CompanyName   string `json:"company_name" binding:"required" example:"Acme Retail Co."`
	TaxID         string `json:"tax_id" example:"0105558000001"`
	ContactPerson string `json:"contact_person" example:"Jane Somchai"`
	ContactPhone  string `json:"contact_phone" example:"+66-2-000-0000"`
	Address       string `json:"address"`
	Email         string `json:"email" example:"jane@acme.example"`
	Phone         string `json:"phone" example:"+66-2-111-1111"`
	Status        Status `json:"status" example:"ACTIVE"`
} //	@name	CreateCustomerRequest

// UpdateRequest is the PUT /api/v1/customers/:id body.
type UpdateRequest struct {
	Code          string `json:"code" binding:"required" example:"CUST-0001"`
	CompanyName   string `json:"company_name" binding:"required" example:"Acme Retail Co."`
	TaxID         string `json:"tax_id" example:"0105558000001"`
	ContactPerson string `json:"contact_person" example:"Jane Somchai"`
	ContactPhone  string `json:"contact_phone" example:"+66-2-000-0000"`
	Address       string `json:"address"`
	Email         string `json:"email" example:"jane@acme.example"`
	Phone         string `json:"phone" example:"+66-2-111-1111"`
	Status        Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateCustomerRequest

// ListParams narrows GET /api/v1/customers to the Customers screen's
// search box and status filter, with pagination and sorting. SortBy/
// SortDir are the raw query values — Service.List is what validates
// them against allowedSortColumns and defaults them (code/ASC) before
// Repository ever sees them.
type ListParams struct {
	Search  string
	Status  Status
	Page    int
	Limit   int
	SortBy  string
	SortDir string
}

func toResponse(c Customer) Response {
	return Response{
		ID: c.ID, Code: c.Code, CompanyName: c.CompanyName, TaxID: c.TaxID,
		ContactPerson: c.ContactPerson, ContactPhone: c.ContactPhone,
		Address: c.Address, Email: c.Email, Phone: c.Phone, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

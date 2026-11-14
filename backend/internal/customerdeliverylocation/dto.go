package customerdeliverylocation

import "time"

// Response is what every delivery-location-returning endpoint sends
// back.
type Response struct {
	ID              int64     `json:"id,string" example:"1"`
	CustomerID      int64     `json:"customer_id,string" example:"1"`
	Code            string    `json:"code" example:"BKK-DC-01"`
	Name            string    `json:"name" example:"Bangkok Distribution Center"`
	Type            Type      `json:"type" example:"DISTRIBUTION_CENTER"`
	Address         string    `json:"address"`
	Province        string    `json:"province" example:"Bangkok"`
	District        string    `json:"district" example:"Bang Na"`
	PostalCode      string    `json:"postal_code" example:"10260"`
	ContactPerson   string    `json:"contact_person" example:"Somchai Jaidee"`
	ContactPhone    string    `json:"contact_phone" example:"+66-2-000-0000"`
	ReceivingWindow string    `json:"receiving_window" example:"Mon-Fri 08:00-17:00"`
	Instructions    string    `json:"instructions"`
	IsDefault       bool      `json:"is_default" example:"true"`
	Status          Status    `json:"status" example:"ACTIVE"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
} //	@name	CustomerDeliveryLocationResponse

// CreateRequest is the POST /api/v1/customer-delivery-locations body.
// CustomerID is required — a location is always created in the context
// of one customer (the Delivery Locations tab of that customer's own
// drawer). IsDefault is advisory: Service forces the customer's first
// location to be default regardless of this value, and atomically
// un-defaults any prior default when this one is explicitly requested
// as the new default — see Service.Create's own doc.
type CreateRequest struct {
	CustomerID      int64  `json:"customer_id,string" binding:"required" example:"1"`
	Code            string `json:"code" binding:"required" example:"BKK-DC-01"`
	Name            string `json:"name" binding:"required" example:"Bangkok Distribution Center"`
	Type            Type   `json:"type" example:"DISTRIBUTION_CENTER"`
	Address         string `json:"address"`
	Province        string `json:"province" example:"Bangkok"`
	District        string `json:"district" example:"Bang Na"`
	PostalCode      string `json:"postal_code" example:"10260"`
	ContactPerson   string `json:"contact_person" example:"Somchai Jaidee"`
	ContactPhone    string `json:"contact_phone" example:"+66-2-000-0000"`
	ReceivingWindow string `json:"receiving_window" example:"Mon-Fri 08:00-17:00"`
	Instructions    string `json:"instructions"`
	IsDefault       bool   `json:"is_default" example:"false"`
	Status          Status `json:"status" example:"ACTIVE"`
} //	@name	CreateCustomerDeliveryLocationRequest

// UpdateRequest is the PUT /api/v1/customer-delivery-locations/:id
// body — CustomerID is deliberately absent: a location never moves to
// a different customer, only edited in place.
type UpdateRequest struct {
	Code            string `json:"code" binding:"required" example:"BKK-DC-01"`
	Name            string `json:"name" binding:"required" example:"Bangkok Distribution Center"`
	Type            Type   `json:"type" example:"DISTRIBUTION_CENTER"`
	Address         string `json:"address"`
	Province        string `json:"province" example:"Bangkok"`
	District        string `json:"district" example:"Bang Na"`
	PostalCode      string `json:"postal_code" example:"10260"`
	ContactPerson   string `json:"contact_person" example:"Somchai Jaidee"`
	ContactPhone    string `json:"contact_phone" example:"+66-2-000-0000"`
	ReceivingWindow string `json:"receiving_window" example:"Mon-Fri 08:00-17:00"`
	Instructions    string `json:"instructions"`
	IsDefault       bool   `json:"is_default" example:"false"`
	Status          Status `json:"status" binding:"required" example:"ACTIVE"`
} //	@name	UpdateCustomerDeliveryLocationRequest

// ListParams narrows GET /api/v1/customer-delivery-locations to one
// customer's own locations (CustomerID required — mirrors
// internal/zone.ListParams' own WarehouseID-required shape), plus the
// Delivery Locations tab's search box and status filter.
type ListParams struct {
	CustomerID int64
	Search     string
	Status     Status
}

func toResponse(l Location) Response {
	return Response{
		ID: l.ID, CustomerID: l.CustomerID, Code: l.Code, Name: l.Name, Type: l.Type,
		Address: l.Address, Province: l.Province, District: l.District, PostalCode: l.PostalCode,
		ContactPerson: l.ContactPerson, ContactPhone: l.ContactPhone,
		ReceivingWindow: l.ReceivingWindow, Instructions: l.Instructions,
		IsDefault: l.IsDefault, Status: l.Status,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

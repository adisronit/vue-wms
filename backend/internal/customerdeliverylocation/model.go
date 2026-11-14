// Package customerdeliverylocation owns Customer Delivery Locations —
// the branches, distribution centers, and delivery addresses a Thai
// domestic B2B customer (internal/customer) orders against, surfaced
// as the "Delivery Locations" tab of the Customer drawer. Every
// location belongs to exactly one customer; every customer may have
// zero or more. Mirrors internal/zone's relationship to
// internal/warehouse — a separate package, a narrow interface back
// into the parent for existence/ownership checks — but, like
// internal/customer/internal/supplier (Master Data Expansion), threads
// an explicit, verified tenantID through every query from day one
// rather than relying solely on the parent's own tenant scoping.
package customerdeliverylocation

import "time"

// Status mirrors every other master-data module's ACTIVE/INACTIVE
// toggle — "deactivate" (not delete) is this module's primary retire
// action, matching the task's own wording for the Delivery Locations
// tab.
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

// Type is the location's functional role — exactly the three kinds of
// ship-to location this phase's own goal statement names ("branches,
// distribution centers and delivery addresses"), not an invented
// taxonomy. Optional: a location may predate this field or simply not
// fit one of the three (hence IsValid also accepts "").
type Type string

const (
	TypeBranch             Type = "BRANCH"
	TypeDistributionCenter Type = "DISTRIBUTION_CENTER"
	TypeDeliveryAddress    Type = "DELIVERY_ADDRESS"
)

func (t Type) IsValid() bool {
	switch t {
	case "", TypeBranch, TypeDistributionCenter, TypeDeliveryAddress:
		return true
	default:
		return false
	}
}

// Location is the GORM model for the customer_delivery_locations table
// (migrations/0059).
type Location struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	TenantID        int64     `gorm:"column:tenant_id;not null;default:1"`
	CustomerID      int64     `gorm:"column:customer_id;not null"`
	Code            string    `gorm:"column:code"`
	Name            string    `gorm:"column:name"`
	Type            Type      `gorm:"column:type"`
	Address         string    `gorm:"column:address"`
	Province        string    `gorm:"column:province"`
	District        string    `gorm:"column:district"`
	PostalCode      string    `gorm:"column:postal_code"`
	ContactPerson   string    `gorm:"column:contact_person"`
	ContactPhone    string    `gorm:"column:contact_phone"`
	ReceivingWindow string    `gorm:"column:receiving_window"`
	Instructions    string    `gorm:"column:instructions"`
	IsDefault       bool      `gorm:"column:is_default"`
	Status          Status    `gorm:"column:status"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (Location) TableName() string { return "customer_delivery_locations" }

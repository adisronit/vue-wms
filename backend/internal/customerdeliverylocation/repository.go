package customerdeliverylocation

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

var ErrNotFound = errors.New("customerdeliverylocation: not found")
var ErrDuplicateCode = errors.New("customerdeliverylocation: code already exists for this customer")

// ListFilter narrows GET /api/v1/customer-delivery-locations to one
// customer's own locations (CustomerID required — mirrors
// internal/zone.ListFilter's own WarehouseID-required shape) plus the
// Delivery Locations tab's search box and status filter. No
// page/limit: a customer's own ship-to list is expected to stay small
// enough to show in full, the same assumption internal/zone.Repository.List
// makes for one warehouse's zones.
type ListFilter struct {
	CustomerID int64
	Search     string // matched against code, name (LIKE)
	Status     Status
}

// Repository is a thin GORM wrapper — business rules (the "exactly one
// default" invariant, uniqueness messaging, ownership checks) live in
// Service. Every method takes an explicit tenantID and applies it to
// every query, the same convention internal/customer/internal/supplier
// (Master Data Expansion) established.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction runs fn inside a database transaction, handing it a
// Repository bound to that transaction — mirrors internal/receiving.Repository.Transaction's
// own doc: Service.Create/Update use this so "count existing locations,
// maybe clear the prior default, then insert/update" commits or rolls
// back as one atomic unit.
func (r *Repository) Transaction(ctx context.Context, fn func(txRepo *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

func (r *Repository) Create(ctx context.Context, tenantID int64, l *Location) error {
	l.TenantID = tenantID
	if err := r.db.WithContext(ctx).Create(l).Error; err != nil {
		return translateUniqueViolation(err)
	}
	return nil
}

// FindByID returns id's row scoped to tenantID, or ErrNotFound — never
// "found but belongs to another tenant" vs. "doesn't exist at all"
// (tenant-isolation-security.md §6).
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*Location, error) {
	var l Location
	err := r.db.WithContext(ctx).First(&l, "id = ? AND tenant_id = ?", id, tenantID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *Repository) Update(ctx context.Context, tenantID int64, l *Location) error {
	result := r.db.WithContext(ctx).Model(&Location{}).
		Where("id = ? AND tenant_id = ?", l.ID, tenantID).
		Updates(map[string]any{
			"code":             l.Code,
			"name":             l.Name,
			"type":             l.Type,
			"address":          l.Address,
			"province":         l.Province,
			"district":         l.District,
			"postal_code":      l.PostalCode,
			"contact_person":   l.ContactPerson,
			"contact_phone":    l.ContactPhone,
			"receiving_window": l.ReceivingWindow,
			"instructions":     l.Instructions,
			"is_default":       l.IsDefault,
			"status":           l.Status,
		})
	if result.Error != nil {
		return translateUniqueViolation(result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, tenantID, id int64) error {
	result := r.db.WithContext(ctx).Delete(&Location{}, "id = ? AND tenant_id = ?", id, tenantID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, tenantID int64, filter ListFilter) ([]Location, int64, error) {
	query := r.db.WithContext(ctx).Model(&Location{}).
		Where("tenant_id = ? AND customer_id = ?", tenantID, filter.CustomerID)

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("code LIKE ? OR name LIKE ?", like, like)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var locations []Location
	// Default location surfaces first, then code ascending — the
	// Delivery Locations tab's own natural reading order.
	if err := query.Order("is_default DESC, code ASC").Find(&locations).Error; err != nil {
		return nil, 0, err
	}

	return locations, total, nil
}

// CountByCustomer satisfies customer.deliveryLocationCounter — used to
// refuse deleting a customer that still has delivery locations (any
// status), and by Service.Create to detect "this is the customer's
// first location" (force is_default=true).
func (r *Repository) CountByCustomer(ctx context.Context, tenantID, customerID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Location{}).
		Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
		Count(&count).Error
	return count, err
}

// CountActiveExcluding counts the customer's own ACTIVE locations other
// than excludeID — Service's "may I un-default/deactivate/delete this
// one" check: refuse only when doing so would leave the customer with
// zero defaults while other ACTIVE locations still exist to need one.
func (r *Repository) CountActiveExcluding(ctx context.Context, tenantID, customerID, excludeID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Location{}).
		Where("tenant_id = ? AND customer_id = ? AND status = ? AND id <> ?", tenantID, customerID, StatusActive, excludeID).
		Count(&count).Error
	return count, err
}

// ClearDefault unsets is_default on every one of the customer's own
// locations except excludeID (pass 0 — no row ever has that id — to
// clear all of them) — Service calls this inside Repository.Transaction
// immediately before setting a new row as the default, so the DB-level
// filtered unique index (migrations/0059's
// UQ_customer_delivery_locations_one_default) is never asked to hold
// two defaults for the same customer even momentarily within the same
// transaction.
func (r *Repository) ClearDefault(ctx context.Context, tenantID, customerID, excludeID int64) error {
	return r.db.WithContext(ctx).Model(&Location{}).
		Where("tenant_id = ? AND customer_id = ? AND is_default = ? AND id <> ?", tenantID, customerID, true, excludeID).
		Update("is_default", false).Error
}

func translateUniqueViolation(err error) error {
	if !sqlerr.IsUniqueViolation(err) {
		return err
	}
	return ErrDuplicateCode
}

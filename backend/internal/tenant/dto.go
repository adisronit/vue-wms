package tenant

import "time"

// Response is what every tenant-returning endpoint on the Platform
// Admin API sends back. Deliberately excludes nothing sensitive —
// unlike internal/user's Response (which must never leak
// password_hash), a tenant row has no secret fields.
type Response struct {
	ID          int64      `json:"id,string" example:"1"`
	Slug        string     `json:"slug" example:"acme-logistics"`
	Name        string     `json:"name" example:"Acme Logistics Co."`
	Status      string     `json:"status" example:"ACTIVE"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	SuspendedAt *time.Time `json:"suspended_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
} //	@name	TenantResponse

// CreateRequest is the POST /api/v1/platform/tenants body. Status is
// optional — omitted or empty defaults to TRIAL, matching
// tenants.status's own DF_tenants_status default (migrations/0041), so
// a platform admin onboarding a new customer doesn't have to know or
// state the default explicitly.
type CreateRequest struct {
	Slug   string `json:"slug" binding:"required" example:"acme-logistics"`
	Name   string `json:"name" binding:"required" example:"Acme Logistics Co."`
	Status string `json:"status" example:"TRIAL"`
} //	@name	CreateTenantRequest

// UpdateRequest is the PUT /api/v1/platform/tenants/:id body. Status is
// intentionally not included here — status transitions have their own
// side effects (suspended_at/cancelled_at stamping) and audit action
// codes, handled only by PATCH .../status.
type UpdateRequest struct {
	Slug string `json:"slug" binding:"required" example:"acme-logistics"`
	Name string `json:"name" binding:"required" example:"Acme Logistics Co."`
} //	@name	UpdateTenantRequest

// UpdateStatusRequest is the PATCH /api/v1/platform/tenants/:id/status
// body. Reason is optional free text recorded only in the audit log's
// metadata — matching saas-module-roadmap.md's "Suspend Tenant" row
// ("include metadata.reason").
type UpdateStatusRequest struct {
	Status string  `json:"status" binding:"required" example:"SUSPENDED"`
	Reason *string `json:"reason" example:"Non-payment — 30 days overdue"`
} //	@name	UpdateTenantStatusRequest

// ListParams narrows GET /api/v1/platform/tenants to the Tenant
// Management screen's search box and status filter, with pagination.
type ListParams struct {
	Search  string
	Status  string
	Page    int
	Limit   int
	SortBy  string
	SortDir string
}

func toResponse(t Tenant) Response {
	return Response{
		ID:          t.ID,
		Slug:        t.Slug,
		Name:        t.Name,
		Status:      t.Status,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		SuspendedAt: t.SuspendedAt,
		CancelledAt: t.CancelledAt,
	}
}

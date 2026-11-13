package product

import (
	"context"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// categoryChecker is the narrow capability Create/Update need from
// internal/productcategory to validate an optional category_id —
// satisfied by *productcategory.Repository's Exists. Kept as a local
// interface, the same depend-on-behavior-not-packages pattern
// internal/warehouse's own zones dependency already uses, so this
// package never needs to import internal/productcategory's full
// surface (nil-safe: a Service built without one, e.g. existing tests'
// &Service{} literals, simply skips category_id validation — see
// validateCategoryID).
type categoryChecker interface {
	Exists(ctx context.Context, tenantID, id int64) (bool, error)
}

// Service holds Product Master's business rules: validation and
// uniqueness-error mapping. Handlers never call Repository directly.
type Service struct {
	repo       *Repository
	categories categoryChecker
}

func NewService(repo *Repository, categories categoryChecker) *Service {
	return &Service{repo: repo, categories: categories}
}

func (s *Service) List(ctx context.Context, params ListParams) ([]Response, int64, error) {
	page, limit := params.Page, params.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	products, total, err := s.repo.List(ctx, ListFilter{
		Search: params.Search, Category: params.Category, Status: params.Status,
		LotRequired: params.LotRequired, Page: page, Limit: limit,
	})
	if err != nil {
		return nil, 0, err
	}

	responses := make([]Response, 0, len(products))
	for _, p := range products {
		responses = append(responses, toResponse(p))
	}
	return responses, total, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Response, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Response{}, err
	}
	return toResponse(*p), nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Response, error) {
	if err := validate(req.Category, req.BaseUOM, req.Status, req.PackSize, req.MinStock, req.MaxStock); err != nil {
		return Response{}, err
	}
	if err := s.validateCategoryID(ctx, req.CategoryID); err != nil {
		return Response{}, err
	}

	status := req.Status
	if status == "" {
		status = StatusActive
	}
	packSize := req.PackSize
	if packSize == 0 {
		packSize = 1
	}

	p := &Product{
		SKU: req.SKU, Name: req.Name, Description: req.Description,
		Category: req.Category, CategoryID: req.CategoryID, BaseUOM: req.BaseUOM, PackSize: packSize, Barcode: normalizeBarcode(req.Barcode),
		LotRequired: req.LotRequired, SerialRequired: req.SerialRequired, ExpiryRequired: req.ExpiryRequired,
		DefaultZone: req.DefaultZone, MinStock: req.MinStock, MaxStock: req.MaxStock, Status: status,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, p.ID)
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (Response, error) {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return Response{}, err
	}

	if err := validate(req.Category, req.BaseUOM, req.Status, req.PackSize, req.MinStock, req.MaxStock); err != nil {
		return Response{}, err
	}
	if err := s.validateCategoryID(ctx, req.CategoryID); err != nil {
		return Response{}, err
	}

	p := &Product{
		ID: id, SKU: req.SKU, Name: req.Name, Description: req.Description,
		Category: req.Category, CategoryID: req.CategoryID, BaseUOM: req.BaseUOM, PackSize: req.PackSize, Barcode: normalizeBarcode(req.Barcode),
		LotRequired: req.LotRequired, SerialRequired: req.SerialRequired, ExpiryRequired: req.ExpiryRequired,
		DefaultZone: req.DefaultZone, MinStock: req.MinStock, MaxStock: req.MaxStock, Status: req.Status,
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return Response{}, err
	}

	return s.Get(ctx, id)
}

// Delete hard-deletes the product. There is no UI button for this today
// (Product Master only ever toggles status) and no Inventory table
// exists yet to protect against — once Inventory lands, this must gain
// the same referential-safety check warehouse/zone delete already have.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// categoryTenantHintKey is where Handler.Create/Update attach the
// caller's best-known tenant id onto ctx before calling
// Service.Create/Update, so validateCategoryID can check a provided
// category_id belongs to the same tenant without changing either
// method's existing (ctx, req) signature — preserving every existing
// caller and test (including service_test.go's &Service{} literals,
// which call Create/Update with a plain context.Background() and never
// set CategoryID) exactly as-is.
//
// This is deliberately the raw, not-database-re-verified
// active_tenant_id JWT claim (middleware.CurrentTenantID), not an
// EnforcedTenantID — Product Master's own route group has no
// RequireTenantContext today (a pre-existing, documented gap; see
// internal/productcategory/repository.go's doc), so there is no
// re-verified tenant context available here to use instead. Using the
// claim is still strictly an improvement over no check at all: it is
// signed at login time and cannot be forged, only possibly stale (a
// membership revoked after the token was issued but before it
// expires) — the same residual risk already accepted everywhere else
// CurrentTenantID is used (e.g. /auth/me). Wiring full tenant
// enforcement into Product Master itself is tracked separately
// (saas-module-roadmap.md Phase 30+) and out of scope here.
type categoryTenantHintKeyType struct{}

var categoryTenantHintKey categoryTenantHintKeyType

// WithCategoryTenantHint attaches tenantID onto ctx for
// validateCategoryID to read back. Call sites: Handler.Create/Update.
func WithCategoryTenantHint(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(ctx, categoryTenantHintKey, tenantID)
}

func categoryTenantHintFromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(categoryTenantHintKey).(int64)
	return v, ok
}

// validateCategoryID is a no-op when categoryID is nil (the field is
// optional) or when this Service was built without a categoryChecker
// (existing tests' &Service{} literals) — otherwise it requires a
// tenant hint to be present (fail closed rather than silently skip the
// check if a future caller forgets to attach one) and confirms
// categoryID exists within that tenant.
func (s *Service) validateCategoryID(ctx context.Context, categoryID *int64) error {
	if categoryID == nil || s.categories == nil {
		return nil
	}

	tenantHint, ok := categoryTenantHintFromContext(ctx)
	if !ok {
		return ErrValidation{Message: "category_id requires tenant context"}
	}

	exists, err := s.categories.Exists(ctx, tenantHint, *categoryID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrValidation{Message: "category_id does not reference an existing product category"}
	}
	return nil
}

func validate(category Category, baseUOM UOM, status Status, packSize, minStock, maxStock int) error {
	if !category.IsValid() {
		return ErrValidation{Message: "category must be one of PACKAGING, LABEL, RFID, RAW_MATERIAL"}
	}
	if !baseUOM.IsValid() {
		return ErrValidation{Message: "base_uom must be one of PCS, BOX, ROLL, KG"}
	}
	if status != "" && !status.IsValid() {
		return ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}
	if packSize < 0 {
		return ErrValidation{Message: "pack_size must not be negative"}
	}
	if minStock < 0 || maxStock < 0 {
		return ErrValidation{Message: "min_stock and max_stock must not be negative"}
	}
	if maxStock > 0 && minStock > maxStock {
		return ErrValidation{Message: "min_stock must not be greater than max_stock"}
	}
	return nil
}

// normalizeBarcode treats an empty string the same as "no barcode" —
// the frontend's barcode field is a plain text input with no
// distinction between "not filled in" and "".
func normalizeBarcode(barcode *string) *string {
	if barcode == nil || *barcode == "" {
		return nil
	}
	return barcode
}

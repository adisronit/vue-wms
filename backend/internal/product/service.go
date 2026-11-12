package product

import (
	"context"
)

// ErrValidation carries a client-facing message for a 422 response.
type ErrValidation struct {
	Message string
}

func (e ErrValidation) Error() string { return e.Message }

// Service holds Product Master's business rules: validation and
// uniqueness-error mapping. Handlers never call Repository directly.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
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
		Category: req.Category, BaseUOM: req.BaseUOM, PackSize: packSize, Barcode: normalizeBarcode(req.Barcode),
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

	p := &Product{
		ID: id, SKU: req.SKU, Name: req.Name, Description: req.Description,
		Category: req.Category, BaseUOM: req.BaseUOM, PackSize: req.PackSize, Barcode: normalizeBarcode(req.Barcode),
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

package product

import "context"

// ListBarcodes returns productID's barcode mappings, scoped to
// tenantID via Repository's own JOIN (see repository.go's doc) — never
// "found but belongs to another tenant" vs. "doesn't exist", so Get
// itself is what surfaces a cross-tenant productID as ErrNotFound; this
// method assumes the caller already resolved productID through Get.
func (s *Service) ListBarcodes(ctx context.Context, tenantID, productID int64) ([]BarcodeResponse, error) {
	barcodes, err := s.repo.ListBarcodes(ctx, tenantID, productID)
	if err != nil {
		return nil, err
	}
	responses := make([]BarcodeResponse, 0, len(barcodes))
	for _, b := range barcodes {
		responses = append(responses, toBarcodeResponse(b))
	}
	return responses, nil
}

func (s *Service) CreateBarcode(ctx context.Context, tenantID, productID int64, req CreateBarcodeRequest) (BarcodeResponse, error) {
	if err := validateBarcodeFields(req.BarcodeType, req.Status, req.QtyPerBarcode); err != nil {
		return BarcodeResponse{}, err
	}

	barcodeType := req.BarcodeType
	if barcodeType == "" {
		barcodeType = BarcodeTypeSupplier
	}
	status := req.Status
	if status == "" {
		status = StatusActive
	}
	qty := req.QtyPerBarcode
	if qty == 0 {
		qty = 1
	}

	b := &ProductBarcode{
		BarcodeValue: req.BarcodeValue, BarcodeType: barcodeType, UOM: req.UOM,
		QtyPerBarcode: qty, IsPrimary: req.IsPrimary, Status: status,
	}
	if err := s.repo.CreateBarcode(ctx, tenantID, productID, b); err != nil {
		return BarcodeResponse{}, err
	}
	return toBarcodeResponse(*b), nil
}

func (s *Service) UpdateBarcode(ctx context.Context, tenantID, productID, barcodeID int64, req UpdateBarcodeRequest) (BarcodeResponse, error) {
	if err := validateBarcodeFields(req.BarcodeType, req.Status, req.QtyPerBarcode); err != nil {
		return BarcodeResponse{}, err
	}

	barcodeType := req.BarcodeType
	if barcodeType == "" {
		barcodeType = BarcodeTypeSupplier
	}
	qty := req.QtyPerBarcode
	if qty == 0 {
		qty = 1
	}

	b := &ProductBarcode{
		ID: barcodeID, ProductID: productID, BarcodeValue: req.BarcodeValue, BarcodeType: barcodeType,
		UOM: req.UOM, QtyPerBarcode: qty, IsPrimary: req.IsPrimary, Status: req.Status,
	}
	if err := s.repo.UpdateBarcode(ctx, tenantID, productID, barcodeID, b); err != nil {
		return BarcodeResponse{}, err
	}
	return toBarcodeResponse(*b), nil
}

func (s *Service) DeleteBarcode(ctx context.Context, tenantID, productID, barcodeID int64) error {
	return s.repo.DeleteBarcode(ctx, tenantID, productID, barcodeID)
}

func validateBarcodeFields(barcodeType BarcodeType, status Status, qty int) error {
	if barcodeType != "" && !barcodeType.IsValid() {
		return ErrValidation{Message: "barcode_type must be one of SUPPLIER_BARCODE, INTERNAL_BARCODE, BOX_CODE, QR_CODE"}
	}
	if status != "" && !status.IsValid() {
		return ErrValidation{Message: "status must be ACTIVE or INACTIVE"}
	}
	if qty < 0 {
		return ErrValidation{Message: "qty_per_barcode must not be negative"}
	}
	return nil
}

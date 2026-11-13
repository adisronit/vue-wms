package product

import "time"

// BarcodeType mirrors ProductMasterView.vue's "Barcode Mapping" tab's
// fixed Type <select> — same four values the frontend has shown since
// Phase 2, now actually persisted (migrations/0055).
type BarcodeType string

const (
	BarcodeTypeSupplier BarcodeType = "SUPPLIER_BARCODE"
	BarcodeTypeInternal BarcodeType = "INTERNAL_BARCODE"
	BarcodeTypeBoxCode  BarcodeType = "BOX_CODE"
	BarcodeTypeQR       BarcodeType = "QR_CODE"
)

func (t BarcodeType) IsValid() bool {
	switch t {
	case BarcodeTypeSupplier, BarcodeTypeInternal, BarcodeTypeBoxCode, BarcodeTypeQR:
		return true
	default:
		return false
	}
}

// ProductBarcode is the GORM model for the product_barcodes table
// (migrations/0055) — a plain child of products with no own tenant_id
// column (the same "scoped via the parent" shape
// internal/role.RoleWarehouseAccess uses): every Repository method
// re-verifies product_id belongs to the caller's tenant via a JOIN,
// rather than trusting a bare barcode row ID.
type ProductBarcode struct {
	ID            int64       `gorm:"column:id;primaryKey"`
	ProductID     int64       `gorm:"column:product_id"`
	BarcodeValue  string      `gorm:"column:barcode_value"`
	BarcodeType   BarcodeType `gorm:"column:barcode_type"`
	UOM           string      `gorm:"column:uom"`
	QtyPerBarcode int         `gorm:"column:qty_per_barcode"`
	IsPrimary     bool        `gorm:"column:is_primary"`
	Status        Status      `gorm:"column:status"`
	CreatedAt     time.Time   `gorm:"column:created_at"`
	UpdatedAt     time.Time   `gorm:"column:updated_at"`
}

func (ProductBarcode) TableName() string { return "product_barcodes" }

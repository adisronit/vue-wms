// Package docrender is Phase 33C.3's shared document-history/reprint
// infrastructure — the immutable snapshot store every PDF-rendering
// module in this codebase (internal/order, internal/picking,
// internal/pickingbatch, internal/packing) records into right after
// generating a document, so a later "reprint" can return EXACTLY the
// bytes originally produced rather than a fresh re-render against
// whatever the underlying data looks like now.
//
// Deliberately its own small package rather than folded into any one
// document-owning module: a document_renders row can belong to ANY of
// the six document types this phase covers, and no single owning
// module should import another's package just to record a history
// row. Every document-owning module instead depends on this package's
// own narrow Service (satisfied structurally — see each package's own
// docRecorder interface), the same "small shared package, narrow
// interface at each call site" shape internal/pdfexport itself already
// established for the rendering engine.
//
// This package never decides WHETHER a caller may view a document's
// history or reprint one of its snapshots — that RBAC decision rides
// entirely on the SAME permission code the owning document type's own
// live PDF endpoint already requires (order.view for ORDER,
// picking.view for PICK_LIST/PICKING_CONFIRMATION, picking_batch.view
// for PICKING_BATCH_LIST, packing.view for PACKING_LIST/CARTON_LABEL —
// see handler.go's own permissionByDocumentType map). No new blanket
// "documents.view" permission exists; viewing history is exactly as
// privileged as viewing the live document it is a history of.
package docrender

import "time"

// Document type constants — the closed set document_renders.document_type
// (migrations/0104) enforces via CHECK constraint.
const (
	TypeOrder               = "ORDER"
	TypePickList            = "PICK_LIST"
	TypePickingBatchList    = "PICKING_BATCH_LIST"
	TypePickingConfirmation = "PICKING_CONFIRMATION"
	TypePackingList         = "PACKING_LIST"
	TypeCartonLabel         = "CARTON_LABEL"
)

// DocumentRender is the GORM model for the document_renders table
// (migrations/0104). PDFData holds the exact bytes returned to the
// caller at render time — see this package's own doc for why this is
// the one and only table in this codebase persisting rendered PDF
// bytes rather than only ever generating them on demand.
type DocumentRender struct {
	ID           int64     `gorm:"column:id;primaryKey"`
	TenantID     int64     `gorm:"column:tenant_id;not null"`
	DocumentType string    `gorm:"column:document_type"`
	DocumentNo   string    `gorm:"column:document_no"`
	SourceTable  string    `gorm:"column:source_table"`
	SourceID     int64     `gorm:"column:source_id"`
	RenderedAt   time.Time `gorm:"column:rendered_at"`
	RenderedBy   *int64    `gorm:"column:rendered_by"`
	IsDraft      bool      `gorm:"column:is_draft"`
	PDFData      []byte    `gorm:"column:pdf_data"`
}

func (DocumentRender) TableName() string { return "document_renders" }

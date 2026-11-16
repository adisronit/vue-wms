package docrender

import "time"

// RenderSummary is one row of GET /documents/{type}/{sourceId}/history —
// deliberately omits pdf_data (see Repository.ListBySource's own doc).
type RenderSummary struct {
	ID             int64     `json:"id,string" example:"1"`
	DocumentType   string    `json:"document_type" example:"PACKING_LIST"`
	DocumentNo     string    `json:"document_no" example:"PACK-000001"`
	RenderedAt     time.Time `json:"rendered_at"`
	RenderedBy     *int64    `json:"rendered_by,string"`
	RenderedByName *string   `json:"rendered_by_name"`
	IsDraft        bool      `json:"is_draft" example:"false"`
} //	@name	DocumentRenderSummary

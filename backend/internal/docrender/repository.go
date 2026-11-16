package docrender

import (
	"context"

	"gorm.io/gorm"
)

// Repository is a thin GORM wrapper — no business logic of its own, the
// same shape every other package's Repository in this codebase uses.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Insert writes one immutable render row. Never updated or deleted by
// anything else in this package — append-only, matching picking_scan_log's
// own precedent for an audit-trail table.
func (r *Repository) Insert(ctx context.Context, rec *DocumentRender) error {
	return r.db.WithContext(ctx).Create(rec).Error
}

// ListBySource returns every render recorded for one (tenant, document
// type, source row), newest first — the history list GET
// /documents/{type}/{sourceId}/history serves. Deliberately excludes
// pdf_data via Select (a history list may legitimately have many rows;
// loading every one's full PDF bytes just to list timestamps would be
// wasteful) — the full bytes are only ever loaded one row at a time, via
// FindByID, when a caller actually wants to re-download that specific
// snapshot.
func (r *Repository) ListBySource(ctx context.Context, tenantID int64, documentType string, sourceID int64) ([]DocumentRender, error) {
	var rows []DocumentRender
	err := r.db.WithContext(ctx).
		Select("id", "tenant_id", "document_type", "document_no", "source_table", "source_id", "rendered_at", "rendered_by", "is_draft").
		Where("tenant_id = ? AND document_type = ? AND source_id = ?", tenantID, documentType, sourceID).
		Order("rendered_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}

// FindByID loads one render row, including its full pdf_data, scoped to
// tenantID — the one and only read path that returns the actual bytes
// back out, for GET /documents/renders/{id}.
func (r *Repository) FindByID(ctx context.Context, tenantID, id int64) (*DocumentRender, error) {
	var row DocumentRender
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

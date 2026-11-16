package docrender

import (
	"context"
	"errors"
	"time"

	"wms-backend/internal/user"
)

// ErrNotFound covers both "no such render id" and "render id belongs to
// a different tenant" — the same single, indistinguishable 404 every
// other tenant-scoped Get in this codebase returns for a cross-tenant
// lookup, so a probing caller learns nothing about whether the id
// exists at all.
var ErrNotFound = errors.New("docrender: render not found")

// userNamer is the narrow, read-only slice of internal/user's own
// Repository this package needs to resolve a render's own rendered_by
// id into a display name for History — the same narrow-interface
// pattern internal/order/internal/picking/internal/packing's own
// PDFService already uses for the same lookup.
type userNamer interface {
	FindByID(ctx context.Context, id int64) (*user.User, error)
}

// Service is the one entry point every PDF-rendering module in this
// codebase depends on (structurally — see each caller's own docRecorder
// interface) to persist and later retrieve an immutable rendered-PDF
// snapshot.
type Service struct {
	repo  *Repository
	users userNamer
}

func NewService(repo *Repository, users userNamer) *Service {
	return &Service{repo: repo, users: users}
}

// Record persists one immutable snapshot of an already-rendered PDF.
// Called by every PDF-generating method across order/picking/
// pickingbatch/packing right after Bytes() succeeds and before the
// handler responds — see each package's own pdf.go for the exact call
// site. userID of 0 (no authenticated caller resolved, which should
// never actually happen behind RequireJWT but is handled defensively
// the same way CreatedBy/CancelledBy's own "*int64, nil when unknown"
// convention is used elsewhere) is recorded as a NULL rendered_by rather
// than a bogus foreign key value.
func (s *Service) Record(ctx context.Context, tenantID int64, documentType, documentNo, sourceTable string, sourceID, userID int64, isDraft bool, pdfData []byte) error {
	rec := &DocumentRender{
		TenantID:     tenantID,
		DocumentType: documentType,
		DocumentNo:   documentNo,
		SourceTable:  sourceTable,
		SourceID:     sourceID,
		RenderedAt:   time.Now().UTC(),
		IsDraft:      isDraft,
		PDFData:      pdfData,
	}
	if userID > 0 {
		rec.RenderedBy = &userID
	}
	return s.repo.Insert(ctx, rec)
}

// History lists every past render recorded for one (tenant, document
// type, source row) — GET /documents/{type}/{sourceId}/history — newest
// first, with each row's rendered_by id resolved to a display name
// where possible (a deleted/unresolvable user id degrades to a nil
// name, never an error — history listing must not fail just because a
// display-name lookup did).
func (s *Service) History(ctx context.Context, tenantID int64, documentType string, sourceID int64) ([]RenderSummary, error) {
	rows, err := s.repo.ListBySource(ctx, tenantID, documentType, sourceID)
	if err != nil {
		return nil, err
	}

	out := make([]RenderSummary, 0, len(rows))
	for _, row := range rows {
		var renderedByName *string
		if row.RenderedBy != nil {
			if u, err := s.users.FindByID(ctx, *row.RenderedBy); err == nil && u != nil {
				name := u.FullName
				renderedByName = &name
			}
		}
		out = append(out, RenderSummary{
			ID: row.ID, DocumentType: row.DocumentType, DocumentNo: row.DocumentNo,
			RenderedAt: row.RenderedAt, RenderedBy: row.RenderedBy, RenderedByName: renderedByName,
			IsDraft: row.IsDraft,
		})
	}
	return out, nil
}

// Fetch loads one specific historical render, tenant-scoped, for GET
// /documents/renders/{id} — the reprint path. Returns the full
// DocumentRender (not just []byte) deliberately: the handler needs
// DocumentType to resolve the RBAC permission to check and DocumentNo to
// build a sensible download filename, and re-querying just for those
// would be wasteful.
func (s *Service) Fetch(ctx context.Context, tenantID, renderID int64) (*DocumentRender, error) {
	rec, err := s.repo.FindByID(ctx, tenantID, renderID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, ErrNotFound
	}
	return rec, nil
}

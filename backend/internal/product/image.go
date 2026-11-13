package product

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
)

// maxImageUploadBytes is the hard 5MB cap applied even if Service was
// constructed with imageMaxBytes <= 0.
const maxImageUploadBytes = 5 * 1024 * 1024

// allowedImageExtensions maps a real, content-sniffed MIME type (never
// the client-supplied filename extension or Content-Type header alone —
// both are trivially spoofable) to the file extension this package
// writes to disk. JPG/PNG/WebP only, per the EWTA image requirements.
var allowedImageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// UploadImage validates data as a real JPG/PNG/WebP image (content-
// sniffed, not trusted from the client), confirms productID belongs to
// tenantID, writes it to disk under a server-generated UUID filename
// (never derived from originalFilename — that string is stored purely
// for display, so a malicious filename like "../../etc/passwd" can
// never become a filesystem path), and only then commits the new
// metadata to the database. If the database write fails after the file
// was written, the just-written file is removed — the database row is
// always the source of truth for "does this product have an image",
// never the filesystem alone, so the two can never disagree in a way
// that leaves an orphan.
func (s *Service) UploadImage(ctx context.Context, tenantID, productID int64, data []byte, originalFilename string) (Response, error) {
	if s.imageStorageDir == "" {
		return Response{}, ErrValidation{Message: "image storage is not configured on this server"}
	}

	maxBytes := s.imageMaxBytes
	if maxBytes <= 0 || maxBytes > maxImageUploadBytes {
		maxBytes = maxImageUploadBytes
	}
	if int64(len(data)) > maxBytes {
		return Response{}, ErrValidation{Message: "image exceeds the 5MB size limit"}
	}
	if len(data) == 0 {
		return Response{}, ErrValidation{Message: "image file is empty"}
	}

	// Confirm tenant ownership before touching disk at all — an upload
	// targeting another tenant's product ID fails here as ErrNotFound,
	// identical to every other cross-tenant lookup in this codebase.
	if _, err := s.repo.FindByIDForTenant(ctx, tenantID, productID); err != nil {
		return Response{}, err
	}

	detected := mimetype.Detect(data)
	ext, ok := allowedImageExtensions[detected.String()]
	if !ok {
		return Response{}, ErrValidation{Message: "image must be a JPG, PNG, or WebP file"}
	}

	filename := uuid.NewString() + ext
	relPath := filepath.ToSlash(filepath.Join(strconv.FormatInt(tenantID, 10), filename))
	absPath := filepath.Join(s.imageStorageDir, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return Response{}, err
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return Response{}, err
	}

	previousPath, err := s.repo.SetImage(ctx, tenantID, productID, relPath, originalFilename, detected.String(), len(data), time.Now().UTC())
	if err != nil {
		_ = os.Remove(absPath)
		return Response{}, err
	}

	// Best-effort cleanup of the file this upload just replaced — never
	// fatal: a stray old file is a disk-space nit, not a correctness or
	// security issue (it is no longer referenced by any row, and lives
	// under a tenant-prefixed, UUID-named path a client can never guess
	// or reach — see Serve's own doc for why the file is never
	// accessible except through this package's own tenant-checked
	// lookup).
	if previousPath != nil && *previousPath != "" && *previousPath != relPath {
		_ = os.Remove(filepath.Join(s.imageStorageDir, *previousPath))
	}

	return s.Get(ctx, tenantID, productID)
}

// RemoveImage clears productID's image metadata and best-effort
// deletes the now-orphaned file — see UploadImage's own doc for why a
// failed file delete here is logged-and-ignored rather than fatal
// (the database row, already cleared, remains the source of truth).
func (s *Service) RemoveImage(ctx context.Context, tenantID, productID int64) (Response, error) {
	previousPath, err := s.repo.ClearImage(ctx, tenantID, productID)
	if err != nil {
		return Response{}, err
	}
	if previousPath != nil && *previousPath != "" {
		_ = os.Remove(filepath.Join(s.imageStorageDir, *previousPath))
	}
	return s.Get(ctx, tenantID, productID)
}

// ImageFile resolves tenantID/productID's current image to an absolute
// disk path and MIME type for Handler.ServeImage's c.File call — the
// raw image_path column is never returned to any client directly (see
// dto.go's toResponse doc); every caller reaches the file only through
// this tenant-checked lookup, never a static mount.
func (s *Service) ImageFile(ctx context.Context, tenantID, productID int64) (absPath, mimeType string, err error) {
	p, err := s.repo.FindByIDForTenant(ctx, tenantID, productID)
	if err != nil {
		return "", "", err
	}
	if p.ImagePath == nil || *p.ImagePath == "" {
		return "", "", ErrNotFound
	}
	mt := "application/octet-stream"
	if p.ImageMimeType != nil && *p.ImageMimeType != "" {
		mt = *p.ImageMimeType
	}
	return filepath.Join(s.imageStorageDir, *p.ImagePath), mt, nil
}

// imageURL is the stable, handler-mediated path every Response's
// ImageURL points at — see dto.go's toResponse.
func imageURL(productID int64) string {
	return fmt.Sprintf("/api/v1/products/%d/image", productID)
}

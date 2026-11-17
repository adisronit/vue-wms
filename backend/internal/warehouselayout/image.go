package warehouselayout

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
)

// maxImageUploadBytes is the hard 5MB cap applied even if Service was
// constructed with imageMaxBytes <= 0 — mirrors internal/product's own
// image.go exactly.
const maxImageUploadBytes = 5 * 1024 * 1024

// allowedImageExtensions is JPG/PNG only — see model.go's own Image
// doc for why WebP is deliberately excluded here even though
// internal/product's own images accept it.
var allowedImageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

// UploadBackgroundImage validates data as a real JPG/PNG image
// (content-sniffed, never trusted from the client), decodes its real
// pixel dimensions (needed for on-canvas calibration), writes it to
// disk under a server-generated UUID filename (never derived from
// originalFilename), and only then commits the metadata row — mirrors
// internal/product's own UploadImage exactly, including best-effort
// cleanup of the just-written file if the database write fails.
func (s *Service) UploadBackgroundImage(ctx context.Context, tenantID, warehouseID int64, data []byte, originalFilename string, userID int64) (*ImageResponse, error) {
	if s.imageStorageDir == "" {
		return nil, ErrValidation{Message: "image storage is not configured on this server"}
	}
	if _, err := s.repo.WarehouseName(ctx, tenantID, warehouseID); err != nil {
		return nil, err
	}

	maxBytes := s.imageMaxBytes
	if maxBytes <= 0 || maxBytes > maxImageUploadBytes {
		maxBytes = maxImageUploadBytes
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrValidation{Message: "image exceeds the 5MB size limit"}
	}
	if len(data) == 0 {
		return nil, ErrValidation{Message: "image file is empty"}
	}

	detected := mimetype.Detect(data)
	ext, ok := allowedImageExtensions[detected.String()]
	if !ok {
		return nil, ErrValidation{Message: "background image must be a JPG or PNG file"}
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrValidation{Message: "could not read image dimensions"}
	}

	filename := uuid.NewString() + ext
	relPath := filepath.ToSlash(filepath.Join("warehouse-layout", strconv.FormatInt(tenantID, 10), filename))
	absPath := filepath.Join(s.imageStorageDir, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return nil, err
	}

	var originalFilenamePtr *string
	if originalFilename != "" {
		originalFilenamePtr = &originalFilename
	}
	img := &Image{
		TenantID: tenantID, WarehouseID: warehouseID, FileName: relPath,
		OriginalFilename: originalFilenamePtr, ContentType: detected.String(), ByteSize: len(data),
		WidthPx: cfg.Width, HeightPx: cfg.Height, CreatedBy: &userID, CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.CreateImage(ctx, img); err != nil {
		_ = os.Remove(absPath)
		return nil, err
	}

	url := imageURL(warehouseID, img.ID)
	return &ImageResponse{ID: img.ID, URL: url, WidthPx: img.WidthPx, HeightPx: img.HeightPx}, nil
}

// ImageFile resolves tenantID/warehouseID/imageID to an absolute disk
// path and MIME type for Handler.ServeImage's c.File call — never a
// static mount, only through this tenant-checked lookup.
func (s *Service) ImageFile(ctx context.Context, tenantID, warehouseID, imageID int64) (absPath, mimeType string, err error) {
	img, err := s.repo.ImageByID(ctx, tenantID, warehouseID, imageID)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(s.imageStorageDir, img.FileName), img.ContentType, nil
}

func imageURL(warehouseID, imageID int64) string {
	return "/api/v1/warehouses/" + strconv.FormatInt(warehouseID, 10) + "/layout/images/" + strconv.FormatInt(imageID, 10)
}

package warehouselayout

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"wms-backend/internal/pdfexport"
)

// defaultFillColor/defaultStrokeColor render a shape that was saved
// with no explicit color — never left unfilled-and-unlabeled, which
// would be indistinguishable from a rendering bug.
const (
	defaultFillColor   = "#e2e8f0"
	defaultStrokeColor = "#64748b"
	pdfDrawHeightMM    = 180.0 // usable vertical drawing budget on one A4 page
)

// RenderPDF renders the latest saved layout version as a to-scale
// drawing on one A4 page — every shape's own on-canvas x/y/width/
// height is uniformly scaled+translated to fit the page's own content
// box, preserving aspect ratio (never independently stretching width
// vs height, which would distort the real spatial layout). If no
// layout has ever been saved, renders a page saying so rather than an
// empty/broken PDF.
func (s *Service) RenderPDF(ctx context.Context, tenantID, warehouseID int64) ([]byte, error) {
	warehouseName, err := s.repo.WarehouseName(ctx, tenantID, warehouseID)
	if err != nil {
		return nil, err
	}

	v, err := s.repo.LatestVersion(ctx, tenantID, warehouseID)
	noLayoutYet := err == ErrNotFound
	if err != nil && !noLayoutYet {
		return nil, err
	}

	doc := pdfexport.NewDocument(pdfexport.Meta{
		CompanyName: warehouseName,
		DocTitle:    "Warehouse Layout",
		DocNo:       fmt.Sprintf("GENERATED-%s", time.Now().UTC().Format("20060102-150405")),
		GeneratedAt: time.Now().UTC(),
	})

	if noLayoutYet {
		doc.SectionTitle("No layout has been saved for this warehouse yet.")
		return doc.Bytes()
	}

	resp, err := s.buildVersionResponse(ctx, tenantID, v)
	if err != nil {
		return nil, err
	}

	doc.SectionTitle(fmt.Sprintf("Version %d", resp.VersionNo))
	scale := ""
	if resp.ScalePxPerMeter != nil {
		scale = fmt.Sprintf("%.1f px/meter", *resp.ScalePxPerMeter)
	} else {
		scale = "not calibrated"
	}
	doc.KeyValueGrid([][2]string{
		{"Generated", time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"},
		{"Scale", scale},
		{"Object Count", fmt.Sprintf("%d", len(resp.Objects))},
	}, 1)

	drawX := doc.MarginX()
	drawY := doc.CursorY() + 4
	drawW := doc.PageContentWidth()
	drawH := pdfDrawHeightMM

	minX, minY, maxX, maxY := boundingBox(resp)
	contentW, contentH := maxX-minX, maxY-minY
	if contentW <= 0 {
		contentW = 1
	}
	if contentH <= 0 {
		contentH = 1
	}
	scaleFactor := math.Min(drawW/contentW, drawH/contentH)

	if resp.BackgroundImageID != nil && resp.BackgroundImageWidthPx != nil && resp.BackgroundImageHeightPx != nil {
		img, err := s.repo.ImageByID(ctx, tenantID, warehouseID, *resp.BackgroundImageID)
		if err == nil {
			if data, readErr := os.ReadFile(filepath.Join(s.imageStorageDir, img.FileName)); readErr == nil {
				imgType := "JPG"
				if img.ContentType == "image/png" {
					imgType = "PNG"
				}
				w := float64(img.WidthPx) * scaleFactor
				h := float64(img.HeightPx) * scaleFactor
				doc.Image("layout-bg", data, imgType, drawX, drawY, w, h)
			}
		}
	}

	for _, o := range resp.Objects {
		x := drawX + (o.X-minX)*scaleFactor
		y := drawY + (o.Y-minY)*scaleFactor
		w := o.Width * scaleFactor
		h := o.Height * scaleFactor

		fill := defaultFillColor
		if o.FillColor != nil && *o.FillColor != "" {
			fill = *o.FillColor
		}
		stroke := defaultStrokeColor
		if o.StrokeColor != nil && *o.StrokeColor != "" {
			stroke = *o.StrokeColor
		}
		label := o.ObjectType
		if o.ReferenceLabel != nil && *o.ReferenceLabel != "" {
			label = *o.ReferenceLabel
		} else if o.Label != nil && *o.Label != "" {
			label = *o.Label
		}
		doc.Rect(x, y, w, h, fill, stroke, label, 6)
	}

	return doc.Bytes()
}

// boundingBox returns the smallest box containing every object, used
// to scale the drawing to fit one page — defaults to a 1x1 box at the
// origin when there are zero objects, so RenderPDF still produces a
// valid (empty) page rather than dividing by zero.
func boundingBox(resp *VersionResponse) (minX, minY, maxX, maxY float64) {
	if len(resp.Objects) == 0 {
		return 0, 0, 1, 1
	}
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, o := range resp.Objects {
		minX = math.Min(minX, o.X)
		minY = math.Min(minY, o.Y)
		maxX = math.Max(maxX, o.X+o.Width)
		maxY = math.Max(maxY, o.Y+o.Height)
	}
	return minX, minY, maxX, maxY
}

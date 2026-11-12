// Package csvexport is Phase 22's tiny shared CSV-writing helper — every
// domain package's own Export handler builds its own headers/rows
// (explicit field mapping, no reflection magic, matching this codebase's
// style everywhere else) and calls WriteTo/WriteAttachment here rather
// than each reimplementing encoding/csv boilerplate. Stdlib only — no
// new dependency; XLSX was not added since CSV alone already satisfies
// every report this phase requires (CLAUDE.md's own "CSV first"
// preference, and "do not introduce unnecessary dependencies").
package csvexport

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// MaxExportRows is the shared defensive ceiling every synchronous CSV
// export uses in place of the owning module's own paginated List limit
// (normally capped at 100 for the screen's own table) — high enough for
// every realistic report this phase's dataset produces, low enough that
// a single export request can't run an effectively unbounded query. The
// one dataset genuinely expected to outgrow this (inventory
// transactions, an append-only ledger) uses the async job pipeline
// instead of this constant.
const MaxExportRows = 10000

// WriteTo writes headers then rows as CSV to w.
func WriteTo(w io.Writer, headers []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// SafeFilename strips anything outside a small allow-list of characters
// — every filename this package ever produces is server-generated from
// a fixed prefix plus a timestamp/ID, never raw user input, but this
// exists as a defensive floor regardless (requirement: "sanitize
// filenames"), so a future caller can't accidentally introduce a path-
// traversal or header-injection vector by passing something less
// controlled through the same helper.
func SafeFilename(name string) string {
	cleaned := unsafeFilenameChars.ReplaceAllString(name, "_")
	if cleaned == "" {
		return "export.csv"
	}
	return cleaned
}

// ServeAttachment writes headers/rows as a downloadable CSV response
// with a sanitized, timestamped filename — the common tail end of every
// synchronous export handler in this phase.
func ServeAttachment(w http.ResponseWriter, reportName string, headers []string, rows [][]string) error {
	filename := SafeFilename(fmt.Sprintf("%s_%s.csv", reportName, time.Now().UTC().Format("20060102_150405")))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	return WriteTo(w, headers, rows)
}

// FormatFloat renders a quantity the same way across every report —
// shortest exact representation, no fixed decimal count imposed.
func FormatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// FormatTime renders a *time.Time (or the zero value) as an empty
// string when nil, RFC3339 otherwise — consistent across every report.
func FormatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// FormatTimeValue is FormatTime's non-pointer counterpart for a
// guaranteed-present timestamp column (e.g. created_at).
func FormatTimeValue(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// FormatString renders a *string as an empty string when nil.
func FormatString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// FormatID renders a *int64 as an empty string when nil, the plain
// integer otherwise — for optional FK-shaped columns.
func FormatID(id *int64) string {
	if id == nil {
		return ""
	}
	return fmt.Sprintf("%d", *id)
}

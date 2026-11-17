// Package xlsxexport is Phase 34's shared XLSX-writing helper — the
// Excel counterpart to Phase 22's internal/platform/csvexport, added
// because the Phase 34 Reports spec explicitly requires an Excel
// export alongside CSV/PDF for every report (csvexport's own package
// doc explains why XLSX didn't exist before this phase: no caller
// needed it yet). Mirrors csvexport's shape exactly — explicit
// headers/rows built by the caller, no reflection/struct-tag magic —
// so a report handler that already builds CSV rows can build the
// same [][]string for this package too.
package xlsxexport

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/xuri/excelize/v2"

	"wms-backend/internal/platform/csvexport"
)

// WriteTo writes headers then rows as a single-sheet XLSX workbook to
// w. The header row is bold so it visually matches every other
// report's table header.
func WriteTo(w io.Writer, sheetName string, headers []string, rows [][]string) error {
	f := excelize.NewFile()
	defer f.Close()

	const defaultSheet = "Sheet1"
	if sheetName == "" {
		sheetName = defaultSheet
	}
	if sheetName != defaultSheet {
		if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
			return err
		}
	}

	boldStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return err
	}

	for i, h := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheetName, cell, h); err != nil {
			return err
		}
	}
	if len(headers) > 0 {
		endCell, err := excelize.CoordinatesToCellName(len(headers), 1)
		if err != nil {
			return err
		}
		if err := f.SetCellStyle(sheetName, "A1", endCell, boldStyle); err != nil {
			return err
		}
	}

	for r, row := range rows {
		for c, value := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			if err != nil {
				return err
			}
			if err := f.SetCellValue(sheetName, cell, value); err != nil {
				return err
			}
		}
	}

	if idx, err := f.GetSheetIndex(sheetName); err == nil {
		f.SetActiveSheet(idx)
	}
	return f.Write(w)
}

// ServeAttachment writes headers/rows as a downloadable XLSX response
// with a sanitized, timestamped filename — the Excel counterpart to
// csvexport.ServeAttachment, reusing its SafeFilename so both exports
// of the same report share one filename-sanitization rule.
func ServeAttachment(w http.ResponseWriter, reportName, sheetName string, headers []string, rows [][]string) error {
	filename := csvexport.SafeFilename(fmt.Sprintf("%s_%s.xlsx", reportName, time.Now().UTC().Format("20060102_150405")))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	return WriteTo(w, sheetName, headers, rows)
}

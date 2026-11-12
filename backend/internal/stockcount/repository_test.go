package stockcount

import (
	"testing"

	mssql "github.com/microsoft/go-mssqldb"
)

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr, which has its own
// exhaustive test coverage) — this test only confirms the delegation
// itself is wired correctly. Built correct against real mssql.Error
// values from the start, the same as every phase since Phase 8.
func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(mssql.Error{Number: 2627, Message: "Violation of PRIMARY KEY constraint 'PK_stock_count_confirmations'"}) {
		t.Error("expected a PRIMARY KEY violation (2627) to be classified as a unique violation")
	}
	if !isUniqueViolation(mssql.Error{Number: 2627, Message: "Violation of UNIQUE KEY constraint 'UQ_stock_counts_count_no'"}) {
		t.Error("expected a UNIQUE KEY violation (2627) to be classified as a unique violation")
	}
	if isUniqueViolation(nil) {
		t.Error("isUniqueViolation(nil) = true, want false")
	}
}

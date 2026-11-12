package receiving

import (
	"testing"

	mssql "github.com/microsoft/go-mssqldb"
)

// isUniqueViolation now delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr, which has its own
// exhaustive test coverage) — this test only confirms the delegation
// itself is wired correctly for the real case this package's own
// idempotency guard produces: receipt_confirmations.idempotency_key is
// a PRIMARY KEY, not a plain UNIQUE constraint (see this function's own
// doc for why that distinction mattered live).
func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(mssql.Error{Number: 2627, Message: "Violation of PRIMARY KEY constraint 'PK_receipt_confirmations'"}) {
		t.Error("expected a PRIMARY KEY violation (2627) to be classified as a unique violation")
	}
	if !isUniqueViolation(mssql.Error{Number: 2627, Message: "Violation of UNIQUE KEY constraint 'UQ_products_sku'"}) {
		t.Error("expected a UNIQUE KEY violation (2627) to be classified as a unique violation")
	}
	if isUniqueViolation(nil) {
		t.Error("isUniqueViolation(nil) = true, want false")
	}
}

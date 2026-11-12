package shipping

import (
	"testing"

	mssql "github.com/microsoft/go-mssqldb"
)

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr, which has its own
// exhaustive test coverage) — this test only confirms the delegation
// itself is wired correctly for the two real cases this package's own
// idempotency guard (a PRIMARY KEY) and shipment_no/picking_task_id
// (plain UNIQUE) constraints actually produce. Built correct from the
// start here, unlike Receiving/Putaway which each needed a live fix.
func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(mssql.Error{Number: 2627, Message: "Violation of PRIMARY KEY constraint 'PK_shipment_confirmations'"}) {
		t.Error("expected a PRIMARY KEY violation (2627) to be classified as a unique violation")
	}
	if !isUniqueViolation(mssql.Error{Number: 2627, Message: "Violation of UNIQUE KEY constraint 'UQ_shipments_picking_task_id'"}) {
		t.Error("expected a UNIQUE KEY violation (2627) to be classified as a unique violation")
	}
	if isUniqueViolation(nil) {
		t.Error("isUniqueViolation(nil) = true, want false")
	}
}

package notification

import (
	"testing"

	mssql "github.com/microsoft/go-mssqldb"
)

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr) — this test only
// confirms the delegation itself is wired correctly for the real case
// this package's own idempotency guard (CreateIdempotent, on the
// (message_id, user_id) unique constraint from migrations/0033) produces,
// the same pattern every other package's own repository_test.go uses.
func TestIsUniqueViolation(t *testing.T) {
	if !isUniqueViolation(mssql.Error{Number: 2627, Message: "Violation of UNIQUE KEY constraint 'UQ_notifications_message_user'"}) {
		t.Error("expected a UNIQUE KEY violation (2627) to be classified as a unique violation")
	}
	if isUniqueViolation(nil) {
		t.Error("isUniqueViolation(nil) = true, want false")
	}
}

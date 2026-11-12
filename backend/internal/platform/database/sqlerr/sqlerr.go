// Package sqlerr classifies SQL Server errors by their real error
// number (via github.com/microsoft/go-mssqldb's mssql.Error, which
// every query through gorm.io/driver/sqlserver ultimately returns/
// wraps) instead of substring-matching the error message.
//
// Phase 6 shipped an idempotency guard whose duplicate-key detection
// checked for the substrings "2627" and "unique" in the error text.
// SQL Server's real message for a PRIMARY KEY violation is "Violation
// of PRIMARY KEY constraint ... Cannot insert duplicate key ..." — it
// contains neither substring (only a genuine UNIQUE-constraint
// violation's message contains "UNIQUE"), so the duplicate-idempotency-
// key case fell through to a generic 500 instead of the intended
// idempotent no-op, and was only caught by live testing. Matching the
// actual SQL Server error number sidesteps this whole class of bug:
// message text can differ across driver versions, server locales, or
// constraint types, but the numbers are stable and documented.
package sqlerr

import (
	"context"
	"errors"

	mssql "github.com/microsoft/go-mssqldb"
)

// SQL Server error numbers this package classifies. See
// https://learn.microsoft.com/sql/relational-databases/errors-events/database-engine-events-and-errors
const (
	numDuplicateKeyConstraint = 2627 // PRIMARY KEY or UNIQUE constraint violation
	numDuplicateKeyIndex      = 2601 // duplicate key row on a unique index with no named constraint
	numForeignKeyOrCheck      = 547  // FK or CHECK constraint violation
	numDeadlockVictim         = 1205
)

// AsSQLError extracts the underlying mssql.Error, if err is (or wraps)
// one. Returns false for connection errors, context errors, and
// anything else that never reached the server as a classified SQL
// error.
func AsSQLError(err error) (mssql.Error, bool) {
	var sqlErr mssql.Error
	if errors.As(err, &sqlErr) {
		return sqlErr, true
	}
	return mssql.Error{}, false
}

// IsUniqueViolation reports whether err is a PRIMARY KEY or UNIQUE
// constraint violation (or a duplicate key on an unnamed unique
// index) — the two cases every idempotency-guard/duplicate-code check
// in this codebase cares about, regardless of which specific
// constraint fired.
func IsUniqueViolation(err error) bool {
	sqlErr, ok := AsSQLError(err)
	if !ok {
		return false
	}
	return sqlErr.Number == numDuplicateKeyConstraint || sqlErr.Number == numDuplicateKeyIndex
}

// IsForeignKeyViolation reports whether err is a FOREIGN KEY (or CHECK)
// constraint violation — callers that need to know *which* FK still
// need to inspect the message for the constraint name (SQL Server does
// not give a separate number per named constraint), but should gate on
// this first rather than searching the raw message for a constraint
// name substring that could appear in an unrelated error.
func IsForeignKeyViolation(err error) bool {
	sqlErr, ok := AsSQLError(err)
	if !ok {
		return false
	}
	return sqlErr.Number == numForeignKeyOrCheck
}

// IsDeadlock reports whether err is SQL Server choosing this session as
// the deadlock victim (error 1205). Nothing in this codebase currently
// retries on deadlock — the guarded-UPDATE concurrency pattern
// (internal/inventory's AdjustBalance) was proven live under 20
// concurrent writers with zero deadlocks — this exists so a caller that
// does need to distinguish "genuine data conflict" from "orphaned lock/
// cancellation" can, without re-deriving the error number.
func IsDeadlock(err error) bool {
	sqlErr, ok := AsSQLError(err)
	if !ok {
		return false
	}
	return sqlErr.Number == numDeadlockVictim
}

// IsCancelled reports whether err represents the calling context being
// cancelled or exceeding its deadline — a client disconnect, or
// dbctx.Detach's own bounded timeout firing — rather than a genuine
// database error. Distinguishing this from a real SQL error matters for
// logging/alerting: a cancellation is an operational event, not a data
// or schema problem.
func IsCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

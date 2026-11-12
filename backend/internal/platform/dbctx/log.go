package dbctx

import (
	"context"
	"log/slog"
	"time"

	"wms-backend/internal/platform/database/sqlerr"
)

// LogTransaction emits one structured log line for a durable business
// mutation's transaction — request_id, operation, duration, outcome,
// and (on failure) a cancellation/timeout/deadlock/constraint
// classification via internal/platform/database/sqlerr, instead of the
// raw error text. Intentionally narrow: one line per transaction, not
// per SQL statement — this is not the GORM slow-query logger, and
// nothing here logs bind parameters or SQL text.
func LogTransaction(ctx context.Context, operation string, start time.Time, err error) {
	duration := time.Since(start)
	requestID := RequestIDFromContext(ctx)

	if err == nil {
		slog.Info("transaction committed",
			slog.String("request_id", requestID),
			slog.String("operation", operation),
			slog.Duration("duration", duration),
		)
		return
	}

	attrs := []any{
		slog.String("request_id", requestID),
		slog.String("operation", operation),
		slog.Duration("duration", duration),
		slog.String("classification", classify(err)),
	}

	slog.Error("transaction failed", attrs...)
}

func classify(err error) string {
	switch {
	case sqlerr.IsCancelled(err):
		return "cancelled_or_timeout"
	case sqlerr.IsDeadlock(err):
		return "deadlock"
	case sqlerr.IsUniqueViolation(err):
		return "unique_violation"
	case sqlerr.IsForeignKeyViolation(err):
		return "fk_violation"
	default:
		return "other"
	}
}

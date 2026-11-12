// Package dbctx is the project-wide policy for how a durable business
// mutation's SQL transaction relates to the HTTP request that triggered
// it.
//
// Gin cancels c.Request.Context() the instant the client disconnects —
// a closed tab, a hard navigation away from an in-flight submit, a
// timeout, flaky warehouse-floor wifi. Phase 7 discovered live (see
// PROJECT_STATUS.md) that when a cancellable request context flows
// straight into Repository.Transaction, a disconnect mid-transaction
// can abandon it before its Commit/Rollback ever runs: the connection
// goes back to the pool "sleeping" with an open transaction still
// holding locks, and every later request touching the same rows blocks
// indefinitely until a DBA manually kills the session.
//
// Detach breaks that link for the narrow window where it matters —
// after validation has passed and a transaction is genuinely about to
// open — without detaching every request from cancellation
// indiscriminately (reads, list/search, and pre-transaction validation
// should still abort promptly if the client is already gone).
package dbctx

import (
	"context"
	"sync/atomic"
	"time"
)

// fallbackTimeout is used only if SetDefaultTimeout was never called
// (e.g. a unit test constructing a Service directly) — main.go always
// calls it during startup with the configured value.
const fallbackTimeout = 30 * time.Second

var defaultTimeoutNanos atomic.Int64

// SetDefaultTimeout configures the bound every Detach call uses.
// Call once at process startup — the same way slog.SetDefault
// configures the process-wide logger, which this codebase already
// does in cmd/server/main.go and cmd/worker/main.go.
func SetDefaultTimeout(d time.Duration) {
	defaultTimeoutNanos.Store(int64(d))
}

func currentDefault() time.Duration {
	d := time.Duration(defaultTimeoutNanos.Load())
	if d <= 0 {
		return fallbackTimeout
	}
	return d
}

// Detach returns a context for a durable business mutation that, once
// begun, must run to a real commit or rollback regardless of whether
// the inbound HTTP client is still there to receive the response. It
// keeps every value already on parent (request_id via WithRequestID
// below, trace/correlation ids, etc.) but detaches from parent's own
// cancellation and deadline, then applies its own bounded timeout so
// the operation can never hang forever either — an orphaned lock is
// bad, but so is a transaction with no bound at all.
//
// Call this once, right before the SQL transaction actually opens —
// not at the top of every service method. Validation and lookups that
// happen before any durable mutation has started should stay on the
// original, still-cancellable context (see PROJECT_STATUS.md's
// cancellation-semantics policy: reads and pre-mutation validation are
// safe to abort with the client; once mutation begins, it isn't).
func Detach(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), currentDefault())
}

type requestIDKeyType struct{}

var requestIDKey requestIDKeyType

// WithRequestID embeds the request id onto a context.Context. Request
// ids normally live only on *gin.Context (middleware.GetRequestID),
// which never crosses the Handler -> Service boundary in this
// codebase's layering — without this, the id would simply be lost the
// moment a handler calls into its service, and Detach would have
// nothing request-scoped left to preserve. Call this in the handler,
// before invoking the service, so LogTransaction (below) can recover it
// from deep inside the service/repository layer after Detach has
// already stripped gin.Context out of the picture entirely.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext reads back what WithRequestID stored, or "" if
// none was set (e.g. a background/CLI context, or a test).
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

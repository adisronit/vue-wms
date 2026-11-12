// Package logging provides a single structured logger for the whole
// application (API server and worker), built on the standard library's
// log/slog so no extra logging dependency is required. CLAUDE.md asks for
// structured logs carrying request_id/correlation_id/message_id-style
// fields; slog's key/value attributes give us that without pulling in
// zerolog/zap.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New builds the process-wide logger. In "development" it favors
// human-readable text output; any other APP_ENV value switches to JSON,
// which is what a container log collector expects in production.
func New(appEnv, component string) *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(appEnv, "development") {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.EqualFold(appEnv, "development") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler).With(slog.String("component", component))
}

package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
)

// Recovery converts a panic into a structured 500 error response instead
// of letting Gin's default recovery dump a raw stack trace to the client.
// The stack trace is still logged server-side for debugging; it is never
// part of the HTTP response, per CLAUDE.md's "do not expose internal
// stack traces" rule.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		logger.Error("panic recovered",
			slog.String("request_id", GetRequestID(c)),
			slog.Any("panic", recovered),
		)
		apihttp.Fail(c, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		c.Abort()
	})
}

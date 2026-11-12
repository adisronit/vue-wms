package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is the header used both to accept an inbound request id
// (useful when a gateway/load balancer already assigns one) and to echo
// it back on the response.
const RequestIDHeader = "X-Request-Id"

// requestIDContextKey is the gin.Context key logging middleware reads to
// attach the id to every log line for a request.
const requestIDContextKey = "request_id"

// RequestID assigns a request id (reusing an inbound one if present) and
// makes it available to later middleware/handlers and to the client via
// the response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}

		c.Set(requestIDContextKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}

// GetRequestID reads the id set by RequestID, returning "" if the
// middleware was not installed.
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(requestIDContextKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

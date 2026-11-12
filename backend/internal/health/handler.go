package health

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apihttp "wms-backend/internal/platform/http"
)

// Handler exposes the health checker over HTTP.
type Handler struct {
	checker *Checker
}

// NewHandler builds a Handler bound to a Checker.
func NewHandler(checker *Checker) *Handler {
	return &Handler{checker: checker}
}

// Register mounts the health route on the given router group.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.Get)
}

// Get godoc
//
//	@Summary		Report application health
//	@Description	Checks live connectivity to SQL Server and RabbitMQ. Returns HTTP 200 when every dependency is connected, HTTP 503 when any dependency is unavailable. Never returns credentials, connection strings, or stack traces.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	apihttp.SuccessResponse{data=Result}
//	@Failure		503	{object}	apihttp.SuccessResponse{data=Result}	"one or more dependencies unavailable"
//	@Router			/health [get]
func (h *Handler) Get(c *gin.Context) {
	result := h.checker.Check(c.Request.Context())

	code := http.StatusOK
	if !result.Healthy() {
		code = http.StatusServiceUnavailable
	}

	apihttp.Status(c, code, result, "")
}

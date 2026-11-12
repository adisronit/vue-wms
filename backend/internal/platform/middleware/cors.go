package middleware

import (
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS builds the CORS middleware. In production the frontend is served
// same-origin through Nginx (which proxies /api to the backend), so CORS
// is mainly a development convenience for running `vite dev` against a
// backend on a different port. allowedOrigins is a comma-separated list
// from configuration; "*" (the development default) allows any origin.
func CORS(allowedOrigins string) gin.HandlerFunc {
	cfg := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-Id"},
		ExposeHeaders:    []string{"X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}

	if allowedOrigins == "" || allowedOrigins == "*" {
		cfg.AllowAllOrigins = true
	} else {
		cfg.AllowOrigins = strings.Split(allowedOrigins, ",")
	}

	return cors.New(cfg)
}

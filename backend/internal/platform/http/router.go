package http

import "github.com/gin-gonic/gin"

// NewEngine builds a bare Gin engine and applies middlewares to it
// before creating the /api/v1 route group every handler mounts onto.
// Middleware construction intentionally does not live here:
// middleware.Recovery/CORS use this package's response helpers, so this
// package importing middleware back would create an import cycle — the
// caller (cmd/server/main.go) builds the middleware.HandlerFunc values
// and passes them in.
//
// The ordering here matters and was a real, previously-undetected bug:
// gin.RouterGroup.Group() copies the parent's *current* middleware
// chain at the moment it's called — middleware added to the engine via
// Use() *afterward* does not retroactively apply to a group already
// created. This function used to return the /api/v1 group before the
// caller ever called engine.Use(...), so none of Recovery/RequestID/
// RequestLogging/CORS ever actually ran for any /api/v1/* route
// (per-route auth still worked, since RequireJWT is wired directly by
// each package's own Register(rg, ...) call, a separate mechanism).
// Discovered in Phase 8 while wiring request_id into transaction
// logging: every "transaction committed/failed" log line showed
// request_id="" because RequestID() had never run. Most seriously,
// Recovery() never running meant an unrecovered panic in any handler
// would have crashed the entire process for every in-flight request,
// not just the one that panicked.
func NewEngine(middlewares ...gin.HandlerFunc) (*gin.Engine, *gin.RouterGroup) {
	engine := gin.New()
	engine.Use(middlewares...)
	v1 := engine.Group("/api/v1")
	return engine, v1
}

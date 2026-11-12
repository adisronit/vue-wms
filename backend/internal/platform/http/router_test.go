package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// A real, previously-undetected bug: gin.RouterGroup.Group() copies the
// parent's middleware chain at the moment it's called. NewEngine used
// to return the /api/v1 group before its caller (cmd/server/main.go)
// had called engine.Use(...), so global middleware silently never ran
// for any /api/v1/* route — discovered in Phase 8 when request_id
// turned up empty in every transaction log line. This test asserts the
// fix directly: a middleware passed into NewEngine must run for a
// route registered on the returned group.
func TestNewEngine_MiddlewareAppliesToV1Group(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var ran bool
	marker := func(c *gin.Context) {
		ran = true
		c.Next()
	}

	engine, v1 := NewEngine(marker)
	v1.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if !ran {
		t.Fatal("middleware passed to NewEngine did not run for a route on the returned /api/v1 group — the group must be created after middleware is applied, not before")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestNewEngine_RecoveryProtectsV1Routes is the highest-stakes
// consequence of the ordering bug above: middleware.Recovery is what
// stops a single panicking handler from crashing the entire process
// (taking down every other in-flight request with it), and it never
// ran for any /api/v1/* route before this fix. This proves a panicking
// handler on the v1 group is caught and turned into a 500, not a
// process crash — using a minimal inline recoverer standing in for
// middleware.Recovery (this package cannot import middleware without
// creating an import cycle, per NewEngine's own doc), which exercises
// the exact same gin.HandlerFunc + Group() ordering this bug lived in.
func TestNewEngine_RecoveryProtectsV1Routes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recoverMiddleware := func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}

	engine, v1 := NewEngine(recoverMiddleware)
	v1.GET("/boom", func(c *gin.Context) {
		panic("simulated handler panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/boom", nil)
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req) // must not panic out of this call

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d — a panicking /api/v1 handler must be caught by middleware passed to NewEngine, not escape it", rec.Code, http.StatusInternalServerError)
	}
}

func TestNewEngine_NoMiddlewareStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine, v1 := NewEngine()
	v1.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(ctx context.Context) error { return f.err }

type fakeMQ struct{ connected bool }

func (f fakeMQ) IsConnected() bool { return f.connected }

func TestChecker_AllHealthy(t *testing.T) {
	c := NewChecker(fakeDB{err: nil}, fakeMQ{connected: true})
	result := c.Check(context.Background())

	if !result.Healthy() {
		t.Fatalf("expected Healthy() = true, got result = %+v", result)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want ok", result.Status)
	}
	if result.Database != Connected || result.RabbitMQ != Connected {
		t.Errorf("expected both dependencies connected, got %+v", result)
	}
}

func TestChecker_DatabaseDown(t *testing.T) {
	c := NewChecker(fakeDB{err: errors.New("connection refused")}, fakeMQ{connected: true})
	result := c.Check(context.Background())

	if result.Healthy() {
		t.Fatal("expected Healthy() = false when database is down")
	}
	if result.Status != "degraded" {
		t.Errorf("Status = %q, want degraded", result.Status)
	}
	if result.Database != Disconnected {
		t.Errorf("Database = %q, want disconnected", result.Database)
	}
	if result.RabbitMQ != Connected {
		t.Errorf("RabbitMQ = %q, want connected", result.RabbitMQ)
	}
}

func TestChecker_RabbitMQDown(t *testing.T) {
	c := NewChecker(fakeDB{err: nil}, fakeMQ{connected: false})
	result := c.Check(context.Background())

	if result.Healthy() {
		t.Fatal("expected Healthy() = false when rabbitmq is down")
	}
	if result.RabbitMQ != Disconnected {
		t.Errorf("RabbitMQ = %q, want disconnected", result.RabbitMQ)
	}
}

func init() {
	gin.SetMode(gin.TestMode)
}

func TestHandler_Get_ReturnsServiceUnavailableWhenDegraded(t *testing.T) {
	checker := NewChecker(fakeDB{err: errors.New("down")}, fakeMQ{connected: true})
	handler := NewHandler(checker)

	router := gin.New()
	group := router.Group("/api/v1")
	handler.Register(group)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestHandler_Get_ReturnsOKWhenHealthy(t *testing.T) {
	checker := NewChecker(fakeDB{err: nil}, fakeMQ{connected: true})
	handler := NewHandler(checker)

	router := gin.New()
	group := router.Group("/api/v1")
	handler.Register(group)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// Package health checks live connectivity to the application's two
// external dependencies (SQL Server, RabbitMQ) and exposes the result via
// GET /api/v1/health, per CLAUDE.md.
package health

import (
	"context"
)

// DependencyStatus is "connected" or "disconnected" — deliberately not a
// richer type, since that is exactly what the client needs to render and
// nothing in the response may leak connection strings or credentials.
type DependencyStatus string

const (
	Connected    DependencyStatus = "connected"
	Disconnected DependencyStatus = "disconnected"
)

// Result is the health check outcome.
type Result struct {
	Status   string           `json:"status" example:"ok"`
	Database DependencyStatus `json:"database" example:"connected"`
	RabbitMQ DependencyStatus `json:"rabbitmq" example:"connected"`
}

// Healthy reports whether every dependency is connected — the API layer
// uses this to pick the HTTP status code.
func (r Result) Healthy() bool {
	return r.Database == Connected && r.RabbitMQ == Connected
}

// dbPinger is satisfied by *database.Manager. Depending on this narrow
// interface instead of the concrete type keeps Checker unit-testable with
// a fake, without needing a real SQL Server connection.
type dbPinger interface {
	Ping(ctx context.Context) error
}

// mqConnectionState is satisfied by *rabbitmq.Manager, for the same
// reason as dbPinger above.
type mqConnectionState interface {
	IsConnected() bool
}

// Checker evaluates dependency connectivity on demand.
type Checker struct {
	db dbPinger
	mq mqConnectionState
}

// NewChecker builds a Checker bound to the shared connection managers.
func NewChecker(db dbPinger, mq mqConnectionState) *Checker {
	return &Checker{db: db, mq: mq}
}

// Check pings SQL Server and reads RabbitMQ's cached connection state.
// RabbitMQ does not need a network round trip here: the connection
// Manager already tracks live state via NotifyClose plus its background
// reconnect loop, so reusing that cache keeps the health check itself
// cheap and side-effect free.
func (c *Checker) Check(ctx context.Context) Result {
	result := Result{Status: "ok", Database: Connected, RabbitMQ: Connected}

	if err := c.db.Ping(ctx); err != nil {
		result.Database = Disconnected
	}

	if !c.mq.IsConnected() {
		result.RabbitMQ = Disconnected
	}

	if !result.Healthy() {
		result.Status = "degraded"
	}

	return result
}
